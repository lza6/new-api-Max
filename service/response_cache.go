package service

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// 网关层**精确响应缓存**（Batch-9 / G3）。
//
// 定位：只做**精确匹配** —— 不做 embedding/语义缓存（那会引入向量库与误命中风险，
// 成本高于收益）。命中时把上游的字节原样回给客户端，因此 `usage` 天然如实回填，
// 客户端看到的 token 数与他"本该"看到的一致。
//
// 三个硬约束（违反任意一条都不如不做）：
//  1. **默认关**（走能力开关注册表 `RESPONSE_CACHE_ENABLED`）；
//  2. **默认按 user 隔离**（跨用户泄漏是这项最严重的风险）；
//  3. **宁可不命中，不可错命中** —— 缓存键的归一化必须保守。
//
// 只缓存：① 非流式；② HTTP 200；③ 完整读到 EOF 且 usage 存在。
// 流式、上游错误、空壳响应一律不缓存。

// responseCacheKeyVersion 是**键格式版本**。归一化规则一旦变化必须 bump ——
// 否则新旧规则产生的键会在同一次部署的滚动窗口里混用，可能错命中。
const responseCacheKeyVersion = "rc-v1"

// responseCacheExcludedTopLevelKeys 是不影响输出的顶层请求字段，构建键时剔除。
//
// 保守原则：只剔除**确定与输出无关**的字段。`temperature`/`top_p`/`max_tokens`/
// `tools`/`response_format`/`seed`/`stop`/`messages` 等一律保留在键里。
var responseCacheExcludedTopLevelKeys = map[string]struct{}{
	"stream":         {}, // 流式本身不进缓存；显式剔除以避免 stream:false/缺省 产生两个键
	"stream_options": {},
	"user":           {}, // 客户端自报的用户标识，与输出无关
}

// CachedResponse 一条缓存命中结果。
type CachedResponse struct {
	// Body 是**原样**的上游响应体（含 usage），直接回写客户端即可。
	Body []byte
	// TotalTokens 是命中响应里的总 token 数（用于计费口径，不参与上游成本）。
	TotalTokens int64
	// StoredAt 是写入时间（用于排障与 TTL 展示）。
	StoredAt time.Time
}

// responseCacheEntry 进程内条目。
type responseCacheEntry struct {
	key         string
	body        []byte
	totalTokens int64
	storedAt    time.Time
}

// responseCacheLRU 是带 TTL 的进程内 LRU。
//
// 为什么要 LRU 而不是普通 map：内存必须有**硬上界**（用户关切的无界增长问题）。
// 容量由 `response_cache_setting.MaxEntries()` 决定；每次写入淘汰最旧一条；
// 读取时顺带做过期判定。**没有后台 goroutine** —— 清扫在写入路径上分批做，
// 避免为一个小功能常驻一个 ticker。
type responseCacheLRU struct {
	mu         sync.Mutex
	capacity   int
	ll         *list.List               // front = 最近使用
	items      map[string]*list.Element // key -> element(*responseCacheEntry)
	sinceSweep int
}

var responseCacheStore = &responseCacheLRU{
	ll:    list.New(),
	items: make(map[string]*list.Element),
}

// responseCacheSweepEvery 每 N 次写入做一次全量过期清扫。
const responseCacheSweepEvery = 256

func (c *responseCacheLRU) get(key string, now time.Time, ttl time.Duration) (*responseCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*responseCacheEntry)
	if now.Sub(entry.storedAt) > ttl {
		c.ll.Remove(el)
		delete(c.items, key)
		response_cache_setting.RecordCacheEvict(1)
		return nil, false
	}
	c.ll.MoveToFront(el)
	return entry, true
}

func (c *responseCacheLRU) put(key string, entry *responseCacheEntry, capacity int, now time.Time, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacity = capacity
	if el, ok := c.items[key]; ok {
		el.Value = entry
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(entry)
	response_cache_setting.RecordCacheStore()

	// 容量淘汰（硬上界）
	for c.ll.Len() > capacity {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.items, back.Value.(*responseCacheEntry).key)
		response_cache_setting.RecordCacheEvict(1)
	}

	// 过期清扫：分批做，避免每次写入都全量遍历
	c.sinceSweep++
	if c.sinceSweep >= responseCacheSweepEvery {
		c.sinceSweep = 0
		c.sweepLocked(now, ttl)
	}
}

// sweepLocked 移除全部过期条目。调用方必须持锁。
func (c *responseCacheLRU) sweepLocked(now time.Time, ttl time.Duration) {
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		entry := el.Value.(*responseCacheEntry)
		if now.Sub(entry.storedAt) > ttl {
			c.ll.Remove(el)
			delete(c.items, entry.key)
			response_cache_setting.RecordCacheEvict(1)
		}
		el = prev
	}
}

// len 返回当前条目数。
func (c *responseCacheLRU) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// reset 清空全部条目（管理端清理与测试使用）。
func (c *responseCacheLRU) reset() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.ll.Len()
	c.ll = list.New()
	c.items = make(map[string]*list.Element)
	c.sinceSweep = 0
	return n
}

// buildResponseCacheKey 构建缓存键。
//
// 组成：`版本 + model + relayFormat + [userID] + 归一化请求体`，各段**带长度前缀**写入，
// 避免拼接歧义（"a"+"bc" 与 "ab"+"c" 必须是不同的键）。
// `userID` 仅在未开启跨用户共享时参与 —— 这保证默认配置下**不可能**发生跨用户泄漏。
func buildResponseCacheKey(userID int, model, relayFormat string, rawBody []byte) (string, error) {
	canonical, err := canonicalRequestFingerprint(rawBody)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writeCacheKeyField(h, responseCacheKeyVersion)
	writeCacheKeyField(h, strings.ToLower(strings.TrimSpace(model)))
	writeCacheKeyField(h, strings.ToLower(strings.TrimSpace(relayFormat)))
	if response_cache_setting.AllowCrossUser() {
		writeCacheKeyField(h, "u:*")
	} else {
		// 默认路径：userID 参与键 → 不同用户**不可能**互相命中。
		writeCacheKeyField(h, "u:"+strconv.Itoa(userID))
	}
	writeCacheKeyField(h, canonical)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeCacheKeyField 以「长度:内容」形式写入一段，消除拼接歧义
// （"a"+"bc" 与 "ab"+"c" 必须产生不同的键）。
func writeCacheKeyField(h hash.Hash, value string) {
	h.Write([]byte(strconv.Itoa(len(value))))
	h.Write([]byte{':'})
	h.Write([]byte(value))
}

// canonicalRequestFingerprint 生成请求体的稳定指纹。
//
// 做法：把顶层解析成 `map[string]json.RawMessage` —— **值保持原始字节**。这一点很关键：
// 若解成 `map[string]any`，数字会变成 float64，像 `seed: 12345678901234567890` 这类
// 大整数会被舍入，两个**不同的**请求就可能算出同一个键 → **错命中**。
// 这里只做两件事：
//  1. 剔除 `responseCacheExcludedTopLevelKeys` 里的字段；
//  2. 重新序列化（Go 的 map 序列化按 key 排序 → 顶层键序无关；RawMessage 会顺带压缩空白）。
//
// 刻意**不**递归排序嵌套对象的键：那要求把值解成 any，正是上面要避免的舍入风险。
// 代价是「嵌套键序不同的等价请求」不命中 —— 方向正确：宁可漏命中，不可错命中。
func canonicalRequestFingerprint(rawBody []byte) (string, error) {
	if len(rawBody) == 0 {
		return "", nil
	}
	var top map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &top); err != nil {
		return "", err
	}
	kept := make(map[string]json.RawMessage, len(top))
	for k, v := range top {
		if _, skip := responseCacheExcludedTopLevelKeys[strings.ToLower(k)]; skip {
			continue
		}
		kept[k] = v
	}
	out, err := common.Marshal(kept)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ResponseCacheEligible 判定一次请求是否**有资格**进缓存。
// 只做资格判定，不做 I/O；调用方在拿到完整非流式响应后再决定是否写入。
//
// relayFormat 传空串表示"未知格式" —— 未知格式一律不缓存（保守）。
func ResponseCacheEligible(userID int, model, relayFormat string, isStream bool) bool {
	if !response_cache_setting.Enabled() {
		return false
	}
	if isStream {
		return false // v1 不缓存流式
	}
	if userID <= 0 {
		return false // 无用户归属的请求不缓存（无法保证隔离）
	}
	if strings.TrimSpace(relayFormat) == "" {
		return false
	}
	return response_cache_setting.ModelCacheable(model)
}

// ResponseCacheLookup 命中则返回缓存条目。
func ResponseCacheLookup(userID int, model, relayFormat string, rawBody []byte) (*CachedResponse, bool) {
	if !response_cache_setting.Enabled() {
		return nil, false
	}
	key, err := buildResponseCacheKey(userID, model, relayFormat, rawBody)
	if err != nil {
		return nil, false
	}
	ttl := time.Duration(response_cache_setting.TTLSeconds()) * time.Second

	if response_cache_setting.Backend() == "redis" && common.RedisEnabled {
		if entry, ok := redisResponseCacheGet(key); ok {
			response_cache_setting.RecordCacheHit(entry.totalTokens)
			return &CachedResponse{Body: entry.body, TotalTokens: entry.totalTokens, StoredAt: entry.storedAt}, true
		}
		response_cache_setting.RecordCacheMiss()
		return nil, false
	}

	entry, ok := responseCacheStore.get(key, time.Now(), ttl)
	if !ok {
		response_cache_setting.RecordCacheMiss()
		return nil, false
	}
	response_cache_setting.RecordCacheHit(entry.totalTokens)
	return &CachedResponse{Body: entry.body, TotalTokens: entry.totalTokens, StoredAt: entry.storedAt}, true
}

// ResponseCacheStore 写入一条缓存。调用方必须先确认：
// 非流式 + HTTP 200 + body 完整 + usage 存在。
func ResponseCacheStore(userID int, model, relayFormat string, rawBody, responseBody []byte, totalTokens int64) {
	if !response_cache_setting.Enabled() || len(responseBody) == 0 {
		return
	}
	key, err := buildResponseCacheKey(userID, model, relayFormat, rawBody)
	if err != nil {
		return
	}
	entry := &responseCacheEntry{
		key:         key,
		body:        append([]byte(nil), responseBody...),
		totalTokens: totalTokens,
		storedAt:    time.Now(),
	}
	if response_cache_setting.Backend() == "redis" && common.RedisEnabled {
		if err := redisResponseCacheSet(key, entry, time.Duration(response_cache_setting.TTLSeconds())*time.Second); err != nil {
			// 写 Redis 失败不算错误路径：降级为不缓存（下一次请求走上游）。
			common.SysError("response cache redis set failed: " + err.Error())
			return
		}
		response_cache_setting.RecordCacheStore()
		return
	}
	responseCacheStore.put(key, entry, response_cache_setting.MaxEntries(), time.Now(),
		time.Duration(response_cache_setting.TTLSeconds())*time.Second)
}

// ResponseCacheLiveEntries 返回进程内当前条目数（供 /metrics 与效果度量）。
func ResponseCacheLiveEntries() int { return responseCacheStore.len() }

// ResponseCacheClear 清空进程内缓存（管理端清理与测试使用），返回被清掉的条目数。
func ResponseCacheClear() int { return responseCacheStore.reset() }

// responseCacheMaxFingerprintBytes 是构建键时读取请求体的上限。
// 我们只需要一个稳定的指纹，不需要完整大 body；超过此值直接放弃缓存（不缓存超大请求）。
const responseCacheMaxFingerprintBytes int64 = 4 << 20

// ResponseCacheRawBody 从请求上下文取**原始请求体**，用于构建缓存键。
//
// 用 `ReplayableBody.NewReader()` 拿一个**独立** reader —— 不会消费主流程正在用的那份，
// 因此放在转发之前调用是安全的。取不到（无存储 / 超限 / 读失败）时返回 nil，
// 调用方据此放弃缓存（宁可漏命中）。
func ResponseCacheRawBody(c *gin.Context) []byte {
	if c == nil || c.Request == nil {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil || storage == nil {
		return nil
	}
	replayable, ok := storage.(common.ReplayableBody)
	if !ok {
		return nil
	}
	if replayable.Size() > responseCacheMaxFingerprintBytes {
		return nil
	}
	reader, err := replayable.NewReader()
	if err != nil {
		return nil
	}
	defer func() { _ = reader.Close() }()
	data, err := common.ReadAllLimited(reader, responseCacheMaxFingerprintBytes)
	if err != nil {
		return nil
	}
	return data
}

// responseCacheUsageEnvelope 用于从缓存响应体里取回 usage。
//
// 为什么一个结构体就够：本项目写回客户端的响应体**已经**是目标格式（OpenAI / Claude /
// Gemini 各自由转换层产出），而这三种格式的 usage 都挂在顶层 `usage` 字段下，
// 且 `dto.Usage` 同时覆盖了 `prompt_tokens|completion_tokens|total_tokens` 与
// `input_tokens|output_tokens` 两套命名。
type responseCacheUsageEnvelope struct {
	Usage *dto.Usage `json:"usage"`
}

// ResponseCacheServeHit 判定并取出可用的缓存响应体（命中返回上游原始字节）。
//
// 调用点必须是「即将请求上游」的位置（`adaptor.DoRequest` 之前）：命中时调用方
// 用返回的字节**伪造一个 200 响应**喂给既有链路，让计费/日志/健康分原样运行。
func ResponseCacheServeHit(c *gin.Context, info *relaycommon.RelayInfo) ([]byte, bool) {
	if c == nil || info == nil {
		return nil, false
	}
	relayFormat := string(info.RelayFormat)
	if !ResponseCacheEligible(info.UserId, info.OriginModelName, relayFormat, info.IsStream) {
		return nil, false
	}
	rawBody := ResponseCacheRawBody(c)
	if len(rawBody) == 0 {
		return nil, false
	}
	hit, ok := ResponseCacheLookup(info.UserId, info.OriginModelName, relayFormat, rawBody)
	if !ok {
		return nil, false
	}
	return hit.Body, true
}

// MarkResponseCacheServed 标记本次请求已由缓存服务。
// 写入侧据此跳过重复写回（命中会把同样的字节再写一遍，没必要）。
func MarkResponseCacheServed(c *gin.Context) {
	if c != nil {
		c.Set(responseCacheServedContextKey, true)
	}
}

// IsResponseCacheServed 报告本次请求是否已由缓存服务。
func IsResponseCacheServed(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(responseCacheServedContextKey)
	if !ok {
		return false
	}
	served, _ := v.(bool)
	return served
}

// responseCacheServedContextKey 是内部标记键（不跨包，故不需要进 constant）。
const responseCacheServedContextKey = "newapi_response_cache_served"

// ExtractUsageFromResponseBody 从响应体里提取 usage（取不到返回 nil）。
// 仅供缓存命中时回填计费口径使用 —— 与真实响应走同一套 usage 结构，不另造口径。
func ExtractUsageFromResponseBody(body []byte) *dto.Usage {
	if len(body) == 0 {
		return nil
	}
	var env responseCacheUsageEnvelope
	if err := common.Unmarshal(body, &env); err != nil {
		return nil
	}
	return env.Usage
}
