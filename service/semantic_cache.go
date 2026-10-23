package service

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// 语义响应缓存（T-B6）。
//
// 定位：**精确缓存的补充层**。精确缓存只在请求体逐字节同类时命中；本层在精确
// 未命中后，把本次 prompt 与同用户/同模型/同格式的历史 prompt 做相似度比较，
// 足够接近就复用当时的响应。
//
// 三条边界必须写在代码里，因为它们决定这个功能是省钱还是事故：
//  1. **默认关**：`SEMANTIC_RESPONSE_CACHE` 默认 false，且必须配白名单模型
//     （`semantic_allow_models`）——对一个没被显式列入的模型永不命中。
//  2. **错命中的代价是给出另一个问题的答案**：因此阈值默认 0.97（保守），
//     命中会在响应头与指标里标出来，便于发现异常。
//  3. **内置 embedding 是词袋哈希，不是语义模型**：它对大小写、空白、标点、
//     轻微措辞差异鲁棒，但**不做同义改写匹配**。真正的语义匹配需要外部
//     embedding 服务（OpenAI 兼容 `/v1/embeddings`），那属于外部付费依赖，
//     必须由管理员显式配置与批准 —— 本层不擅自发起任何外部请求。
//
// 状态只在本进程内（不去写 Redis/磁盘）：语义索引是「尽量省一次上游调用」的
// 加速层，丢了只是多打一次上游，绝不能因此产生新的持久化形态。

const (
	semanticEmbeddingDim     = 256
	semanticMaxEntriesPerKey = 64
	semanticMaxEntriesTotal  = 2048
)

var (
	semanticCacheHits   atomic.Int64
	semanticCacheMisses atomic.Int64
)

// SemanticEmbedder 把文本转成向量。内置实现见 hashingEmbedder；
// 后续接入真实 embedding 服务只需实现该接口并替换 activeEmbedder。
type SemanticEmbedder interface {
	ID() string
	Embed(text string) ([]float32, error)
}

// hashingEmbedder 词袋哈希 embedder：零依赖、确定性、无外部调用。
//
// 分词规则：ASCII 字母数字按词切分，CJK 按字符 bigram（中文没有空格，
// bigram 是可用的最小语义单位）；每个 token 哈希到固定维度后累加，最后 L2 归一化。
type hashingEmbedder struct{}

func (hashingEmbedder) ID() string { return "hashing-v1" }

func (hashingEmbedder) Embed(text string) ([]float32, error) {
	vector := make([]float32, semanticEmbeddingDim)
	for _, token := range semanticTokens(text) {
		// FNV-1a，避免引入额外依赖。
		var hash uint32 = 2166136261
		for i := 0; i < len(token); i++ {
			hash ^= uint32(token[i])
			hash *= 16777619
		}
		vector[hash%semanticEmbeddingDim] += 1
	}
	var norm float64
	for _, v := range vector {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return vector, nil
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] = float32(float64(vector[i]) / norm)
	}
	return vector, nil
}

// semanticTokens 生成小写 token 序列（ASCII 词 + CJK bigram）。
func semanticTokens(text string) []string {
	lower := strings.ToLower(text)
	tokens := make([]string, 0, len(lower)/4+1)
	var word strings.Builder
	cjk := make([]rune, 0, 8)
	flushWord := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	flushCJK := func() {
		if len(cjk) == 1 {
			tokens = append(tokens, string(cjk[0]))
		}
		for i := 0; i+1 < len(cjk); i++ {
			tokens = append(tokens, string(cjk[i:i+2]))
		}
		cjk = cjk[:0]
	}
	for _, r := range lower {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			flushCJK()
			word.WriteRune(r)
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			flushWord()
			cjk = append(cjk, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return tokens
}

// cosineSimilarity 两个已归一化向量的余弦相似度（内积）。
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

// extractSemanticPromptText 从请求体里取出用于比较的 prompt 文本。
// 覆盖 chat/completions（messages[].content 支持字符串与分段数组）、
// responses（input）与 completions（prompt）。解析失败返回空串（上层不命中）。
func extractSemanticPromptText(rawBody []byte) string {
	if len(rawBody) == 0 {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(rawBody, &payload); err != nil {
		return ""
	}
	var builder strings.Builder
	appendContent := func(value any) {
		switch typed := value.(type) {
		case string:
			builder.WriteString(typed)
			builder.WriteString("\n")
		case []any:
			for _, item := range typed {
				part, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := part["text"].(string); ok {
					builder.WriteString(text)
					builder.WriteString("\n")
				}
			}
		}
	}
	if messages, ok := payload["messages"].([]any); ok {
		for _, message := range messages {
			entry, ok := message.(map[string]any)
			if !ok {
				continue
			}
			if role, _ := entry["role"].(string); role != "" {
				builder.WriteString(role)
				builder.WriteString(": ")
			}
			appendContent(entry["content"])
		}
	}
	if input, ok := payload["input"]; ok {
		appendContent(input)
	}
	if prompt, ok := payload["prompt"]; ok {
		appendContent(prompt)
	}
	return builder.String()
}

// semanticCacheEntry 一条可复用响应。
type semanticCacheEntry struct {
	body        []byte
	totalTokens int64
	vector      []float32
	storedAt    time.Time
}

var semanticCacheStore = struct {
	mu      sync.RWMutex
	buckets map[string][]*semanticCacheEntry
	total   int
}{
	buckets: map[string][]*semanticCacheEntry{},
}

func semanticBucketKey(userID int, model, relayFormat string) string {
	return fmt.Sprintf("%d|%s|%s", userID, model, relayFormat)
}

func semanticModelAllowed(model string) bool {
	for _, allowed := range response_cache_setting.SemanticModels() {
		if allowed == model {
			return true
		}
	}
	return false
}

// SemanticCacheLookup 在精确缓存未命中后尝试相似度匹配。
// 命中返回上游原始字节（与精确缓存同形），调用方按精确命中处理。
func SemanticCacheLookup(userID int, model, relayFormat string, rawBody []byte) (*CachedResponse, bool) {
	if !response_cache_setting.SemanticCacheActive() || !semanticModelAllowed(model) {
		return nil, false
	}
	text := extractSemanticPromptText(rawBody)
	if strings.TrimSpace(text) == "" {
		return nil, false
	}
	embedder := SemanticEmbedder(hashingEmbedder{})
	vector, err := embedder.Embed(text)
	if err != nil {
		return nil, false
	}

	bucket := semanticBucketKey(userID, model, relayFormat)
	threshold := response_cache_setting.SemanticThreshold()
	ttl := time.Duration(response_cache_setting.TTLSeconds()) * time.Second
	now := time.Now()

	semanticCacheStore.mu.RLock()
	entries := semanticCacheStore.buckets[bucket]
	best := -1.0
	var bestEntry *semanticCacheEntry
	for _, entry := range entries {
		if ttl > 0 && now.Sub(entry.storedAt) > ttl {
			continue
		}
		score := cosineSimilarity(vector, entry.vector)
		if score >= threshold && score > best {
			best = score
			bestEntry = entry
		}
	}
	semanticCacheStore.mu.RUnlock()

	if bestEntry == nil {
		// 白名单内但没找到足够接近的历史 → 计一次 miss，便于计算命中率。
		semanticCacheMisses.Add(1)
		return nil, false
	}
	semanticCacheHits.Add(1)
	return &CachedResponse{
		Body:        append([]byte(nil), bestEntry.body...),
		TotalTokens: bestEntry.totalTokens,
		StoredAt:    bestEntry.storedAt,
	}, true
}

// SemanticCacheStore 为一次成功响应建立语义索引。失败不报错（缓存写失败不能
// 影响请求结果），容量超限时淘汰该桶最旧条目。
func SemanticCacheStore(userID int, model, relayFormat string, rawBody, responseBody []byte, totalTokens int64) {
	if !response_cache_setting.SemanticCacheActive() || !semanticModelAllowed(model) {
		return
	}
	if len(responseBody) == 0 {
		return
	}
	text := extractSemanticPromptText(rawBody)
	if strings.TrimSpace(text) == "" {
		return
	}
	embedder := SemanticEmbedder(hashingEmbedder{})
	vector, err := embedder.Embed(text)
	if err != nil {
		return
	}

	bucket := semanticBucketKey(userID, model, relayFormat)
	entry := &semanticCacheEntry{
		body:        append([]byte(nil), responseBody...),
		totalTokens: totalTokens,
		vector:      vector,
		storedAt:    time.Now(),
	}

	semanticCacheStore.mu.Lock()
	defer semanticCacheStore.mu.Unlock()
	entries := append(semanticCacheStore.buckets[bucket], entry)
	if len(entries) > semanticMaxEntriesPerKey {
		drop := len(entries) - semanticMaxEntriesPerKey
		entries = entries[drop:]
		semanticCacheStore.total -= drop
	}
	semanticCacheStore.buckets[bucket] = entries
	semanticCacheStore.total++
	if semanticCacheStore.total > semanticMaxEntriesTotal {
		semanticCacheEvictOldestLocked(semanticCacheStore.total - semanticMaxEntriesTotal)
	}
}

// semanticCacheEvictOldestLocked 全局淘汰最旧条目（调用方持写锁）。
func semanticCacheEvictOldestLocked(count int) {
	for i := 0; i < count; i++ {
		oldestBucket := ""
		oldestIndex := -1
		var oldestAt time.Time
		for bucket, entries := range semanticCacheStore.buckets {
			for index, entry := range entries {
				if oldestIndex == -1 || entry.storedAt.Before(oldestAt) {
					oldestBucket, oldestIndex, oldestAt = bucket, index, entry.storedAt
				}
			}
		}
		if oldestIndex == -1 {
			return
		}
		entries := semanticCacheStore.buckets[oldestBucket]
		entries = append(entries[:oldestIndex], entries[oldestIndex+1:]...)
		if len(entries) == 0 {
			delete(semanticCacheStore.buckets, oldestBucket)
		} else {
			semanticCacheStore.buckets[oldestBucket] = entries
		}
		semanticCacheStore.total--
	}
}

// SemanticCacheLiveEntries 当前索引条目数（供 /metrics）。
func SemanticCacheLiveEntries() int {
	semanticCacheStore.mu.RLock()
	defer semanticCacheStore.mu.RUnlock()
	return semanticCacheStore.total
}

// SemanticCacheMetrics 返回语义命中/未命中计数（供 /metrics）。
func SemanticCacheMetrics() (hits, misses int64) {
	return semanticCacheHits.Load(), semanticCacheMisses.Load()
}

// SemanticCacheClear 清空索引并返回清空条数（管理端/测试用）。
func SemanticCacheClear() int {
	semanticCacheStore.mu.Lock()
	defer semanticCacheStore.mu.Unlock()
	cleared := semanticCacheStore.total
	semanticCacheStore.buckets = map[string][]*semanticCacheEntry{}
	semanticCacheStore.total = 0
	return cleared
}

// SemanticCacheResetMetricsForTest 归零计数器（仅测试）。
func SemanticCacheResetMetricsForTest() {
	semanticCacheHits.Store(0)
	semanticCacheMisses.Store(0)
}
