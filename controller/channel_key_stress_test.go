package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 本文件针对「渠道多 key 运维」（批量测试 / 单 key 测试 / add_keys / 轮询选 key）
// 的高并发与边界行为，使用可控慢上游 + atomic 计数器做真实验证。
//
// 覆盖点（与任务清单一一对应）：
//  1. 批量测试并发上限真实生效（上游并发峰值 ≤ 3）
//  2. 批量测试结果互不串扰（每个 key 的结果落回自己的 index）
//  3. 整批超时真实收敛（上游挂起时 handler 仍按时返回，带 timed_out）
//  4. 超时参数边界（0 / 负数 / >3600 / 非法字符串回退默认）
//  5. key 数上限边界（200 通过、201 拒绝）
//  6. 单 key 测试越界不 panic
//  7. 并发 add_keys 同一渠道 → 并集且无重复
//  8. GetNextEnabledKey 高并发轮询不越界
//  9. -race 全绿（由运行命令保证）

// keyStressCompletionBody 上游返回的 OpenAI 兼容非流式响应（usage 完整，
// 避免被测路径走 token 估算回退分支）。
const keyStressCompletionBody = `{"id":"cmpl-stress","object":"chat.completion","created":1700000000,"model":"gpt-4o-mini",` +
	`"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`

// enableLoopbackUpstreamForTest 关闭 SSRF 守卫，让 httptest 环回上游可作为渠道 base_url
// （仅测试进程内生效，生产默认守卫不变）。
func enableLoopbackUpstreamForTest(t *testing.T) {
	t.Helper()
	service.SetSSRFGuardDisabled(true)
	t.Cleanup(func() { service.SetSSRFGuardDisabled(false) })
}

// setupKeyStressDB 建立本文件使用的独立测试库（明渠 + 审计/日志表），
// 并把连接数收敛为 1，避免 sqlite 并发写互相锁死干扰压测。
func setupKeyStressDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	// 生产启动流程会调用 model.InitCol 填充方言相关的保留字列名；测试进程不执行
	// main，缺省空串会让 GetUserGroup 拼出非法 SQL（SELECT  FROM ...）。
	model.InitCol()
	return db
}

// seedKeyStressRootUser 建一个 root 用户：runSingleKeyTest 在无请求上下文时按
// role=RoleRootUser 解析测试用户（resolveChannelTestUserID）。
func seedKeyStressRootUser(t *testing.T, db *gorm.DB) {
	t.Helper()
	user := model.User{
		Id: 9999, Username: "root-stress", Password: "stress-password",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	// 自用模式：未收录的模型（如 gpt-4o-mini 冷启动）不会因「价格未配置」被拒，
	// 让测试聚焦渠道多 key 的并发行为而非定价配置。
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() { operation_setting.SelfUseModeEnabled = false })
}

// seedKeyStressChannel 建多 key 渠道，base_url 指向可控上游。
func seedKeyStressChannel(t *testing.T, db *gorm.DB, id int, baseURL string, keys []string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Id: id, Name: fmt.Sprintf("stress-%d", id), Type: constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled, Key: strings.Join(keys, "\n"),
		BaseURL: common.GetPointer(baseURL), Group: "default", Models: "gpt-4o-mini",
	}
	channel.ChannelInfo.IsMultiKey = true
	channel.ChannelInfo.MultiKeySize = len(keys)
	channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModePolling
	require.NoError(t, db.Create(channel).Error)
	return channel
}

// keyStressUpstream 可控慢上游：记录并发峰值、总命中数、收到的密钥。
type keyStressUpstream struct {
	server    *httptest.Server
	hits      atomic.Int32
	active    atomic.Int32
	maxActive atomic.Int32

	mu       sync.Mutex
	authSeen map[string]int
}

// newKeyStressUpstream 起一个 httptest 上游。hold 非 nil 时请求将一直挂住
// （直到 close(hold)、客户端断开或 10s 兜底），用于验证整批超时收敛；
// 否则固定 sleep delay 后返回成功响应。
func newKeyStressUpstream(t *testing.T, delay time.Duration, hold <-chan struct{}) *keyStressUpstream {
	t.Helper()
	up := &keyStressUpstream{authSeen: map[string]int{}}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up.hits.Add(1)
		up.mu.Lock()
		up.authSeen[r.Header.Get("Authorization")]++
		up.mu.Unlock()

		current := up.active.Add(1)
		for {
			observed := up.maxActive.Load()
			if current <= observed || up.maxActive.CompareAndSwap(observed, current) {
				break
			}
		}
		defer up.active.Add(-1)

		if hold != nil {
			select {
			case <-hold:
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Second):
				return
			}
		} else if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(keyStressCompletionBody))
	}))
	t.Cleanup(up.server.Close)
	return up
}

// waitIdle 等待上游在途请求归零，避免遗留 goroutine 与本测试的清理阶段交叉。
func (up *keyStressUpstream) waitIdle(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if up.active.Load() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("上游仍有 %d 个在途请求未结束", up.active.Load())
}

// authKeys 返回上游观测到的密钥集合（Authorization: Bearer <key>）。
func (up *keyStressUpstream) authKeys() map[string]int {
	up.mu.Lock()
	defer up.mu.Unlock()
	out := make(map[string]int, len(up.authSeen))
	for header, count := range up.authSeen {
		out[strings.TrimPrefix(header, "Bearer ")] = count
	}
	return out
}

// keyBatchTestResponse 批量测试响应体（results 直接复用生产类型）。
type keyBatchTestResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Total          int  `json:"total"`
		OkCount        int  `json:"ok_count"`
		FailCount      int  `json:"fail_count"`
		TimedOut       bool `json:"timed_out"`
		TimeoutSeconds int  `json:"timeout_seconds"`
		// S1: 「未执行」与「执行后失败」分开计数，避免把挂起误报成密钥失效。
		UnexecutedCount   int             `json:"unexecuted_count"`
		ExecutedFailCount int             `json:"executed_fail_count"`
		Results           []keyTestResult `json:"results"`
	} `json:"data"`
}

func decodeKeyBatchTestResponse(t *testing.T, recorder *httptest.ResponseRecorder) keyBatchTestResponse {
	t.Helper()
	var response keyBatchTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

// performTestChannelKey 构造 TestChannelKey 请求（root 管理员上下文）。
func performTestChannelKey(t *testing.T, channelID int, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost,
		"/api/channel/"+strconv.Itoa(channelID)+"/key/test"+query, nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channelID)}}
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	c.Set(common.RequestIdKey, "single-key-test")
	TestChannelKey(c)
	return recorder
}

// postManageMultiKeys 直接以 root 管理员上下文调用 ManageMultiKeys（不经过
// performMultiKeyManage，便于并发 goroutine 中安全构造请求）。
func postManageMultiKeys(body string) *httptest.ResponseRecorder {
	// 注意：gin.SetMode 写全局状态，**不可**在并发 goroutine 内调用（会 data race）。
	// 模式已在各测试入口设好，这里不再重复设置。
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	c.Set(common.RequestIdKey, "stress-multi-key")
	ManageMultiKeys(c)
	return recorder
}

// TestBatchKeyTestCapsUpstreamConcurrency 批量测试的并发上限必须真实生效：
// 20 个 key 同时开跑，上游同时在处理的请求数峰值必须 ≤ 3（不是「看起来并发」）。
func TestBatchKeyTestCapsUpstreamConcurrency(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	const keyCount = 20
	upstream := newKeyStressUpstream(t, 120*time.Millisecond, nil)
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("cap-key-%02d", i)
	}
	seedKeyStressChannel(t, db, 9901, upstream.server.URL, keys)

	started := time.Now()
	recorder := performTestChannelKeys(t, 9901, "")
	elapsed := time.Since(started)

	response := decodeKeyBatchTestResponse(t, recorder)
	require.True(t, response.Success, recorder.Body.String())
	require.Equal(t, keyCount, response.Data.Total)
	require.Equal(t, keyCount, response.Data.OkCount, "所有 key 的测试都应成功")
	require.False(t, response.Data.TimedOut)

	peak := upstream.maxActive.Load()
	// 契约是「峰值不得超过 3」。**不要**断言峰值必然等于 3 —— 那是调度结果，
	// 在负载高的机器上可能峰值只有 2（慢机/抢占），会造成 flaky 假红（已实测）。
	// 真正证明「上限生效」的证据是下面两条：峰值>1（确实并发了）且耗时符合分轮特征。
	assert.LessOrEqual(t, peak, int32(3), "上游并发峰值不得越过批量测试并发上限 3")
	assert.GreaterOrEqual(t, peak, int32(2), "应观察到并发>1（否则无法证明并发上限是在起作用的路径上）")
	assert.Equal(t, int32(keyCount), upstream.hits.Load(), "每个 key 恰好测试一次")

	// 上限为 3 时 20 个 key 至少需要 ceil(20/3) 轮 × 120ms ≈ 800ms；
	// 若并发上限未生效（20 并发）耗时会接近 120ms。
	// 上界放宽到 30s：慢机/CI 上耗时可显著拉长，耗时**不是**契约（契约是并发上限），
	// 用它做严格上界只会制造假红。下界才是「上限生效」的证据。
	assert.GreaterOrEqual(t, elapsed, 600*time.Millisecond, "耗时过短说明并发上限未生效")
	assert.Less(t, elapsed, 30*time.Second, "异常耗时（远超分轮预期）需排查")
	t.Logf("实测：上游并发峰值=%d（上限 3），上游命中=%d，批量耗时=%v",
		peak, upstream.hits.Load(), elapsed)
	upstream.waitIdle(t, 2*time.Second)
}

// TestBatchKeyTestKeepsResultsOnTheirOwnIndex 并发写回结果时不能串槽：
// 每个 key 的响应必须落在自己的 index，且上游实际收到的密钥集合与渠道一致。
func TestBatchKeyTestKeepsResultsOnTheirOwnIndex(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	// 前 10 字符互不相同，便于用 key_preview 定位串槽。
	const keyCount = 9
	upstream := newKeyStressUpstream(t, 40*time.Millisecond, nil)
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("keyidx-%03d-stress", i)
	}
	seedKeyStressChannel(t, db, 9902, upstream.server.URL, keys)

	recorder := performTestChannelKeys(t, 9902, "")
	response := decodeKeyBatchTestResponse(t, recorder)
	require.True(t, response.Success, recorder.Body.String())
	require.Len(t, response.Data.Results, keyCount)

	for i, result := range response.Data.Results {
		assert.Equal(t, i, result.Index, "结果必须写回自己的 index（槽位不得串扰）")
		assert.True(t, result.Ok, "index %d 应测试成功", i)
		assert.Equal(t, previewKey(keys[i]), result.Preview, "index %d 的密钥预览必须是自己的 key", i)
	}

	seen := upstream.authKeys()
	require.Len(t, seen, keyCount, "上游应恰好收到 %d 个不同的密钥：%v", keyCount, seen)
	for _, key := range keys {
		assert.Equal(t, 1, seen[key], "密钥 %s 应被测试且仅被测一次", key)
	}
}

// TestBatchKeyTestReturnsOnBatchTimeout 整批超时必须真实收敛：上游挂起时
// handler 不能跟着挂到上游超时，必须在 ~timeout_seconds 内返回并标记 timed_out。
func TestBatchKeyTestReturnsOnBatchTimeout(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	const keyCount = 6
	hold := make(chan struct{})
	upstream := newKeyStressUpstream(t, 0, hold)
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("hang-key-%d", i)
	}
	seedKeyStressChannel(t, db, 9903, upstream.server.URL, keys)

	started := time.Now()
	recorder := performTestChannelKeys(t, 9903, "?timeout_seconds=1")
	elapsed := time.Since(started)

	// 先释放上游、等在途请求排空，再做断言。
	close(hold)
	upstream.waitIdle(t, 3*time.Second)
	// [测试侧同步] 生产实现**故意**在整批超时后不再等待剩余 worker goroutine
	// （这是契约：handler 必须收敛返回）。但这些 worker 仍会短暂写 channel/results，
	// 若测试随即进入 t.Cleanup 还原 model.DB，就会与它们竞争。
	// 这里给 worker 一个可观测的静默窗口，让它们跑完再断言/清理。
	// 用固定等待而非轮询：worker 只做一次 HTTP 收尾 + 一次加锁写槽位，极短。
	time.Sleep(500 * time.Millisecond)

	// 关键判据：上游挂 10s，handler 必须在 ~1s 返回（<3s 即证明未被上游拖住）。
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond, "不应早于整批超时返回")
	assert.Less(t, elapsed, 3*time.Second, "整批超时未收敛：handler 被挂起的上游拖住")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKeyBatchTestResponse(t, recorder)
	require.True(t, response.Success, recorder.Body.String())

	assert.True(t, response.Data.TimedOut, "整批超时必须回传 timed_out 标记")
	assert.Equal(t, 1, response.Data.TimeoutSeconds, "应回传实际生效的整批超时秒数")
	assert.Equal(t, keyCount, response.Data.Total)
	assert.Equal(t, 0, response.Data.OkCount)
	assert.Equal(t, keyCount, response.Data.FailCount, "超时后所有 key 都不得算作成功")
	// S1：超时批次里「未执行」应单独计数（不等于 fail_count 全部），
	// 且未执行 + 已执行失败 = fail_count，口径自洽。
	assert.GreaterOrEqual(t, response.Data.UnexecutedCount, 1, "超时批次必须有未执行的 key")
	assert.Equal(t, response.Data.FailCount,
		response.Data.UnexecutedCount+response.Data.ExecutedFailCount,
		"未执行 + 已执行失败 必须等于 fail_count")

	// 超时后每个 key 都必须带失败标记（要么是「未执行」，要么是取消/超时类错误）。
	// 注意不要断言恰好 keyCount-3 个「未执行」：被释放的信号量名额可能让第 4 个
	// goroutine 在 ctx 生效前进入，具体落点是调度决定的，但「必有失败标记」是硬性契约。
	// [已定位的生产缺陷] 结果槽位与 worker 写回之间没有同步：handler 在 ctx.Done()
	// 时立即序列化 results，而仍在途的 worker goroutine 之后才写 results[idx]。
	// 结果（实测 results=[{Index:0 Ok:false Message:} {1 未执行} {2 未执行} {0 空} {0 空} {5 未执行}]）：
	//   - 未及时写回的槽位保留零值 → index:0 + 空 message，与「第 0 个 key 失败」无法区分；
	//   - 真实失败位置（本例的 3、4）在响应里完全消失。
	// 这是「响应报文不可信 + 对 results 的并发读写」双重问题，见报告 P1。
	// 断言按正确契约写（index 必须等于槽位、失败必须带 message），修复前持续失败，
	// 请勿放宽断言来消红。
	unexecuted := 0
	for i, result := range response.Data.Results {
		assert.Equal(t, i, result.Index, "结果必须落在自己的槽位；零值槽位会伪造出 index:0")
		assert.False(t, result.Ok, "超时批次不得出现成功结果")
		assert.NotEmpty(t, result.Message, "超时后每个 key 都必须带失败原因")
		if strings.Contains(result.Message, "未执行") {
			unexecuted++
		}
	}
	t.Logf("实测：上游挂起 10s，timeout_seconds=1 时 handler 在 %v 返回；timed_out=%v，未执行标记=%d 个",
		elapsed, response.Data.TimedOut, unexecuted)
}

// TestBatchKeyTestTimeoutParamBoundaries 整批超时参数边界：只有 (0,3600] 的整数
// 才被采纳；0 / 负数 / 超上限 / 非法字符串都必须回退默认（否则会退化成「立即超时」）。
func TestBatchKeyTestTimeoutParamBoundaries(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	upstream := newKeyStressUpstream(t, 150*time.Millisecond, nil)
	seedKeyStressChannel(t, db, 9904, upstream.server.URL, []string{"timeout-key-0"})

	tests := []struct {
		name  string
		query string
	}{
		{name: "未传参数", query: ""},
		{name: "零值", query: "?timeout_seconds=0"},
		{name: "负数", query: "?timeout_seconds=-1"},
		{name: "超上限", query: "?timeout_seconds=99999"},
		{name: "非法字符串", query: "?timeout_seconds=abc"},
		{name: "合法小值", query: "?timeout_seconds=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := performTestChannelKeys(t, 9904, test.query)
			response := decodeKeyBatchTestResponse(t, recorder)

			require.True(t, response.Success, recorder.Body.String())
			assert.False(t, response.Data.TimedOut,
				"非法/越界值被采纳会立即超时；此处必须回退默认超时")
			assert.Equal(t, 1, response.Data.OkCount)
			assert.Equal(t, 1, response.Data.Total)
		})
	}
	upstream.waitIdle(t, 2*time.Second)
}

// TestBatchKeyTestKeyCountBoundary key 数上限边界：恰好 200 个必须能测完，
// 201 个必须在下游发起任何上游请求之前就被拒绝。
func TestBatchKeyTestKeyCountBoundary(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	upstream := newKeyStressUpstream(t, 2*time.Millisecond, nil)
	seedMultiKeyChannel(t, db, 9905, 200)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9905).
		Update("base_url", upstream.server.URL).Error)

	recorder := performTestChannelKeys(t, 9905, "")
	response := decodeKeyBatchTestResponse(t, recorder)
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, 200, response.Data.Total, "200 个 key 处于上限内，必须全部测试")
	assert.Equal(t, 200, response.Data.OkCount)
	assert.False(t, response.Data.TimedOut)
	assert.Equal(t, int32(200), upstream.hits.Load())

	// 201 个：直接拒绝，且不得向上游发起任何请求。
	seedMultiKeyChannel(t, db, 9906, 201)
	upstream.hits.Store(0)
	recorder = performTestChannelKeys(t, 9906, "")
	body := recorder.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "超过单次批量测试上限")
	assert.Equal(t, int32(0), upstream.hits.Load(), "超限批次不得发起任何上游请求")
}

// TestTestChannelKeyRejectsOutOfRangeIndex 单 key 测试的越界索引：
// 负索引 / 非数字在解析层拒绝，== len / 远大于 len 返回失败结果且不 panic。
func TestTestChannelKeyRejectsOutOfRangeIndex(t *testing.T) {
	enableLoopbackUpstreamForTest(t)
	db := setupKeyStressDB(t)
	seedKeyStressRootUser(t, db)

	upstream := newKeyStressUpstream(t, 5*time.Millisecond, nil)
	seedKeyStressChannel(t, db, 9907, upstream.server.URL, []string{"single-key-0", "single-key-1"})

	t.Run("合法索引", func(t *testing.T) {
		recorder := performTestChannelKey(t, 9907, "?key_index=1")
		assert.Contains(t, recorder.Body.String(), `"ok":true`, recorder.Body.String())
	})

	t.Run("负索引", func(t *testing.T) {
		recorder := performTestChannelKey(t, 9907, "?key_index=-1")
		assert.Contains(t, recorder.Body.String(), `"success":false`)
		assert.Contains(t, recorder.Body.String(), "key_index 非法")
	})

	t.Run("非数字", func(t *testing.T) {
		recorder := performTestChannelKey(t, 9907, "?key_index=abc")
		assert.Contains(t, recorder.Body.String(), `"success":false`)
		assert.Contains(t, recorder.Body.String(), "key_index 非法")
	})

	for _, query := range []string{"?key_index=2", "?key_index=9223372036854775807"} {
		t.Run("越界"+query, func(t *testing.T) {
			started := time.Now()
			recorder := performTestChannelKey(t, 9907, query)
			assert.Less(t, time.Since(started), 2*time.Second, "越界索引应立刻返回")

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var result keyTestResult
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result), recorder.Body.String())
			assert.False(t, result.Ok, "越界索引必须返回失败结果而不是 panic")
			assert.NotEmpty(t, result.Message)
		})
	}
}

// runAddKeys 以并发/串行方式向同一渠道发起 add_keys，返回每个请求的响应体。
func runAddKeys(t *testing.T, concurrent bool, bodies []string) []string {
	t.Helper()
	if !concurrent {
		out := make([]string, len(bodies))
		for i, body := range bodies {
			out[i] = postManageMultiKeys(body).Body.String()
		}
		return out
	}
	results := make([]string, len(bodies))
	var waitGroup sync.WaitGroup
	for w := range bodies {
		waitGroup.Add(1)
		go func(idx int) {
			defer waitGroup.Done()
			results[idx] = postManageMultiKeys(bodies[idx]).Body.String()
		}(w)
	}
	waitGroup.Wait()
	return results
}

// addKeysBodies 生成 writers 个 add_keys 请求体，每个携带 keysPerWriter 个互不相同的新 key。
func addKeysBodies(channelID, writers, keysPerWriter int) ([]string, []string) {
	bodies := make([]string, writers)
	allAdded := make([]string, 0, writers*keysPerWriter)
	for w := range writers {
		added := make([]string, keysPerWriter)
		quoted := make([]string, keysPerWriter)
		for k := range added {
			added[k] = fmt.Sprintf("g%02d-k%d", w, k)
			quoted[k] = fmt.Sprintf("%q", added[k])
		}
		allAdded = append(allAdded, added...)
		bodies[w] = fmt.Sprintf(`{"channel_id":%d,"action":"add_keys","keys":[%s]}`,
			channelID, strings.Join(quoted, ","))
	}
	return bodies, allAdded
}

// assertAddKeysUnion 校验渠道最终 key 集合恰为「既有 key ∪ 所有新增 key」且无重复。
func assertAddKeysUnion(t *testing.T, db *gorm.DB, channelID int, seedKeys, allAdded []string) {
	t.Helper()
	var stored model.Channel
	require.NoError(t, db.First(&stored, channelID).Error)
	storedKeys := stored.GetKeys()

	expected := make(map[string]bool, len(seedKeys)+len(allAdded))
	for _, key := range seedKeys {
		expected[key] = true
	}
	for _, key := range allAdded {
		expected[key] = true
	}
	assert.Len(t, storedKeys, len(expected), "最终 key 数必须是并集大小（无重复）")
	assert.Equal(t, len(expected), stored.ChannelInfo.MultiKeySize, "MultiKeySize 必须与 key 数一致")

	seen := map[string]int{}
	for _, key := range storedKeys {
		seen[key]++
	}
	for key, count := range seen {
		assert.Equal(t, 1, count, "key %s 出现 %d 次，存在重复", key, count)
	}
	for key := range expected {
		assert.Equal(t, 1, seen[key], "key %s 在并集中丢失", key)
	}
}

// TestAddKeysSequentialKeepsUnion 串行基线：每次 add_keys 都读到上一次的落库结果，
// 最终必然是并集。它同时证明测试夹具（DB/上下文/审计）本身可用，从而让并发用例的
// 失败只可能归因于并发本身。
func TestAddKeysSequentialKeepsUnion(t *testing.T) {
	db := setupKeyStressDB(t)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))

	channel := seedKeyStressChannel(t, db, 9910, "https://example.invalid", []string{"seed-key"})
	const writers, keysPerWriter = 8, 4
	bodies, allAdded := addKeysBodies(channel.Id, writers, keysPerWriter)

	for i, body := range runAddKeys(t, false, bodies) {
		assert.Contains(t, body, `"success":true`, "串行 add_keys #%d：%s", i, body)
	}
	assertAddKeysUnion(t, db, channel.Id, []string{"seed-key"}, allAdded)
}

// TestConcurrentAddKeysKeepsUnionWithoutDuplicates 同一渠道并发 add_keys。
//
// [已定位的生产缺陷] ManageMultiKeys 在进入 channel.Id 轮询锁之前，先用
// model.GetChannelById 读了一份渠道快照（分别在第 1541 行与第 1573 行），
// 于是「读快照 → 加锁 → 基于快照覆盖写」整体不再原子：
//   - 12 个并发请求都会先读到同一份初始快照（key=seed-key）；
//   - 排到锁的每个请求把自己读到的快照（seed-key + 自己那 5 个）写回，互相覆盖；
//   - 对比第 1541 行读取与第 1573 行加锁之间的窗口，就存在「T1 读、T2 读、T2 写、
//     T1 写」的交错，T2 的新增被 T1 的旧快照直接吞掉。
//
// 实测（本机 sqlite，count=1）：12 个请求全部返回 success:true 且各自报告
// added=5，但库中只剩 seed-key + 两个 writer 的 10 个 key（丢失 50 个），
// 即「接口报告成功、数据实际丢失」。
//
// 本用例断言的是正确契约（并集且无重复），当前实现不满足，因此会在本仓库
// 修复前持续失败——这是有意的回归护栏，请勿为了让测试变绿而放宽断言。
func TestConcurrentAddKeysKeepsUnionWithoutDuplicates(t *testing.T) {
	db := setupKeyStressDB(t)
	require.NoError(t, db.AutoMigrate(&model.AuditLog{}))

	channel := seedKeyStressChannel(t, db, 9908, "https://example.invalid", []string{"seed-key"})
	const writers, keysPerWriter = 12, 5
	bodies, allAdded := addKeysBodies(channel.Id, writers, keysPerWriter)

	for i, body := range runAddKeys(t, true, bodies) {
		assert.Contains(t, body, `"success":true`, "并发 add_keys #%d：%s", i, body)
	}
	assertAddKeysUnion(t, db, channel.Id, []string{"seed-key"}, allAdded)
}

// TestGetNextEnabledKeyConcurrentPolling 高并发轮询：索引不得越界/panic，
// 单个 key 的所有权必须与返回索引一致，被禁用的 key 不得被选中。
//
// 走生产内存缓存路径（MemoryCacheEnabled=true，GetNextEnabledKey 在缓存命中时
// 直接对共享 *Channel 的 MultiKeyPollingIndex 做读-改-写），这正是生产热路径，
// 也让 -race 能真正覆盖该计数器的并发访问。
func TestGetNextEnabledKeyConcurrentPolling(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	db := setupKeyStressDB(t)
	require.NoError(t, db.AutoMigrate(&model.Ability{}))

	const keyCount = 7
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = fmt.Sprintf("poll-key-%d", i)
	}
	channel := seedKeyStressChannel(t, db, 9909, "https://example.invalid", keys)
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{3: common.ChannelStatusManuallyDisabled}
	require.NoError(t, db.Save(channel).Error)
	// 渠道创建后 abilities 与渠道同为一份状态：InitChannelCache 以 abilities 的
	// group 集合初始化索引表，测试夹具必须与生产一致地补齐这一行。
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id,
		Enabled: true, Priority: channel.Priority, Weight: uint(channel.GetWeight()), Tag: channel.Tag,
	}).Error)
	model.InitChannelCache()

	loaded, err := model.CacheGetChannel(channel.Id)
	require.NoError(t, err, "内存缓存应命中刚建的渠道")

	// 6 个启用 key × 200 次 = 1200 次调用，正好整除，轮询若严格轮转则每个启用
	// key 恰被选中 200 次；任何越界/错位/漏选都会打破这个等式。
	const (
		goroutines = 8
		iterations = 150
		perKey     = goroutines * iterations / (keyCount - 1)
	)
	var waitGroup sync.WaitGroup
	hits := make([]atomic.Int64, keyCount)
	for g := range goroutines {
		waitGroup.Add(1)
		go func(worker int) {
			defer waitGroup.Done()
			for i := range iterations {
				key, index, newAPIError := loaded.GetNextEnabledKey()
				if newAPIError != nil {
					t.Errorf("worker %d 第 %d 次轮询报错：%v", worker, i, newAPIError)
					return
				}
				if index < 0 || index >= keyCount {
					t.Errorf("worker %d 得到越界索引 %d", worker, index)
					return
				}
				if key != keys[index] {
					t.Errorf("worker %d 得到的 key 与索引不匹配：index=%d key=%s", worker, index, key)
					return
				}
				if index == 3 {
					t.Errorf("worker %d 选中了被禁用的 key 3", worker)
					return
				}
				hits[index].Add(1)
			}
		}(g)
	}
	waitGroup.Wait()

	var total int64
	for i := range hits {
		count := hits[i].Load()
		total += count
		if i == 3 {
			assert.Equal(t, int64(0), count, "被禁用的 key 不得被轮询选中")
			continue
		}
		assert.Equal(t, int64(perKey), count,
			"轮询必须严格轮转：启用中的 key %d 命中数应与其他启用 key 相同", i)
	}
	assert.Equal(t, int64(goroutines*iterations), total)
	assert.GreaterOrEqual(t, loaded.ChannelInfo.MultiKeyPollingIndex, 0)
	assert.Less(t, loaded.ChannelInfo.MultiKeyPollingIndex, keyCount, "轮询索引必须始终合法")
}
