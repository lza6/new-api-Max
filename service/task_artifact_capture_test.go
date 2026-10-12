package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/setting/system_setting"
	"github.com/lza6/new-api-Max/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetchCountingStore 记录 Persist 收到的产物 key，用于断言「挂点真的调了 Persist」。
type fetchCountingStore struct {
	keys []string
}

func (s *fetchCountingStore) Enabled() bool { return true }

func (s *fetchCountingStore) Resolve(*model.Task, string) (*StoredArtifactRef, error) {
	return nil, nil
}

func (s *fetchCountingStore) Persist(_ context.Context, _ *model.Task, artifact types.TaskArtifact, content io.Reader) (*StoredArtifactRef, error) {
	s.keys = append(s.keys, artifact.Key)
	if _, err := io.Copy(io.Discard, content); err != nil {
		return nil, err
	}
	return &StoredArtifactRef{Backend: TaskArtifactStoreModeLocal, ObjectKey: artifact.Key, MimeType: artifact.MimeType}, nil
}

func (s *fetchCountingStore) Serve(*gin.Context, *model.Task, *StoredArtifactRef) error { return nil }

// taskPollingSuccessAdaptor 让一次轮询直接返回 SUCCESS 与指定产物 URL。
type taskPollingSuccessAdaptor struct {
	resultURL string
}

func (a *taskPollingSuccessAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *taskPollingSuccessAdaptor) FetchTask(_ string, _ string, task *model.Task, _ string) (*http.Response, error) {
	// 返回**非** New-API 信封的响应体，使 updateVideoSingleTask 走
	// adaptor.ParseTaskResult 分支——这正是 legacy 直连上游的真实路径。
	body, err := common.Marshal(map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"status":  string(model.TaskStatusSuccess),
	})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (a *taskPollingSuccessAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: a.resultURL}, nil
}

func (a *taskPollingSuccessAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func mustGetPollingChannel(t *testing.T, id int) *model.Channel {
	t.Helper()
	channel, err := model.CacheGetChannel(id)
	require.NoError(t, err)
	require.NotNil(t, channel)
	return channel
}

func captureTestTask(t *testing.T, id string, resultURL string) *model.Task {
	t.Helper()
	task := &model.Task{TaskID: id, UserId: 1, ChannelId: 1}
	task.PrivateData.ResultURL = resultURL
	return task
}

func testPlan(key, fetchURL string) TaskArtifactFetchPlan {
	return TaskArtifactFetchPlan{
		Artifact: types.TaskArtifact{Key: key, Type: "video"},
		URL:      fetchURL,
	}
}

// useArtifactCaptureFetchSetting 打开 SSRF 防护并放行测试服务器的临时端口。
//
// 默认 `AllowedPorts` 只有 80/443/8080/8443，而 `httptest.NewServer` 用临时端口。
// 为让「开关启用 → 真实抓到字节」这条正向路径可测，这里放行该端口；
// 私网拦截本身由 `TestPersistTaskArtifactPlanSSRFBlocksPrivateTarget` 单独证明。
func useArtifactCaptureFetchSetting(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	require.NoError(t, err)

	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })

	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	fetchSetting.AllowedPorts = []string{parsed.Port()}
	fetchSetting.ApplyIPFilterForDomain = false
}

// TestCaptureTaskArtifactsDisabledStoreIsNoop 「upstream 模式零行为变化」：
// store.Enabled()==false 时不得发起任何出站请求，也不得解析计划。
func TestCaptureTaskArtifactsDisabledStoreIsNoop(t *testing.T) {
	var upstreamHits atomic.Int64
	var plannerCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHits.Add(1)
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer upstream.Close()

	t.Cleanup(SetTaskArtifactStoreForTest(&disabledArtifactStore{}))
	previousPlanner := TaskArtifactFetchPlanner
	TaskArtifactFetchPlanner = func(*model.Task) ([]TaskArtifactFetchPlan, error) {
		plannerCalls.Add(1)
		return []TaskArtifactFetchPlan{testPlan("a", upstream.URL+"/a.mp4")}, nil
	}
	t.Cleanup(func() { TaskArtifactFetchPlanner = previousPlanner })

	captureTaskArtifacts(context.Background(), captureTestTask(t, "task_noop", upstream.URL+"/a.mp4"))

	assert.Zero(t, upstreamHits.Load(), "disabled store 必须零出站请求")
	assert.Zero(t, plannerCalls.Load(), "disabled store 时连计划都不该解析")
}

// TestPersistTaskArtifactPlanFetchesBytes store 启用时真实抓取产物字节并交给 Persist。
func TestPersistTaskArtifactPlanFetchesBytes(t *testing.T) {
	payload := []byte("real-artifact-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}

	task := captureTestTask(t, "task_persist", "")
	require.NoError(t, persistTaskArtifactPlan(t.Context(), store, task, testPlan("a", upstream.URL+"/a.mp4")))

	assert.FileExists(t, filepath.Join(root, "task_persist", "a"))
}

// TestPersistTaskArtifactPlanFailureDoesNotAffectTask 抓取失败不得改动任务状态与计费字段。
func TestPersistTaskArtifactPlanFailureDoesNotAffectTask(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}

	task := captureTestTask(t, "task_fail", "")
	task.Status = model.TaskStatusSuccess
	task.Quota = 4242

	err := persistTaskArtifactPlan(t.Context(), store, task, testPlan("a", upstream.URL+"/missing.mp4"))
	require.Error(t, err, "上游 404 必须报错（由调用方记日志），而不是静默成功")
	assert.Equal(t, string(model.TaskStatusSuccess), string(task.Status))
	assert.Equal(t, 4242, task.Quota)
	assert.NoDirExists(t, filepath.Join(root, "task_fail"))
}

// TestPersistTaskArtifactPlanSkipsSelfProxyURL 站内代理 URL 不得被抓取（防回环）。
func TestPersistTaskArtifactPlanSkipsSelfProxyURL(t *testing.T) {
	store := &localArtifactStore{root: t.TempDir(), maxBytes: 1 << 20, enabled: true}
	task := captureTestTask(t, "task_self", "")

	err := persistTaskArtifactPlan(t.Context(), store, task, testPlan("a", "/v1/tasks/task_self/artifacts/a/content"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "points back at this gateway")
}

// TestPersistTaskArtifactPlanRejectsUnsupportedMethod HEAD 没有字节可落盘。
func TestPersistTaskArtifactPlanRejectsUnsupportedMethod(t *testing.T) {
	store := &localArtifactStore{root: t.TempDir(), maxBytes: 1 << 20, enabled: true}
	plan := testPlan("a", "https://example.invalid/a.mp4")
	plan.Method = http.MethodHead

	err := persistTaskArtifactPlan(t.Context(), store, captureTestTask(t, "task_head", ""), plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not supported")
}

// TestPersistTaskArtifactPlanDecodesDataURL 纯函数型插件的 data: URL 产物同样可落盘。
func TestPersistTaskArtifactPlanDecodesDataURL(t *testing.T) {
	payload := []byte("inline-plugin-bytes")
	dataURL := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(payload)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}
	require.NoError(t, persistTaskArtifactPlan(t.Context(), store, captureTestTask(t, "task_data", ""), testPlan("a", dataURL)))

	got, err := os.ReadFile(filepath.Join(root, "task_data", "a"))
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestPersistTaskArtifactPlanRejectsDangerousHeaders 插件声明的请求头不可覆盖主机类头。
func TestPersistTaskArtifactPlanRejectsDangerousHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	store := &localArtifactStore{root: t.TempDir(), maxBytes: 1 << 20, enabled: true}
	plan := testPlan("a", upstream.URL+"/a.mp4")
	plan.Headers = map[string]string{"Host": "evil.example"}

	err := persistTaskArtifactPlan(t.Context(), store, captureTestTask(t, "task_hdr", ""), plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// TestPersistTaskArtifactPlanLocalStoreWritesRealFile 端到端：产物真实落盘并可被 Resolve 读回。
func TestPersistTaskArtifactPlanLocalStoreWritesRealFile(t *testing.T) {
	payload := []byte("hello-station-artifact")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}

	task := captureTestTask(t, "task_e2e", "")
	require.NoError(t, persistTaskArtifactPlan(t.Context(), store, task, testPlan("a", upstream.URL+"/a.png")))

	got, err := os.ReadFile(filepath.Join(root, "task_e2e", "a"))
	require.NoError(t, err, "产物必须真实落盘到站内图床目录")
	assert.Equal(t, payload, got)

	ref, err := store.Resolve(task, "a")
	require.NoError(t, err)
	require.NotNil(t, ref, "落盘后必须能被 Resolve 读回")
	assert.EqualValues(t, len(payload), ref.Size)
}

// TestPersistTaskArtifactPlanRespectsMaxBytes 单产物上限必须生效，且不留半截文件。
func TestPersistTaskArtifactPlanRespectsMaxBytes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 512, enabled: true}

	err := persistTaskArtifactPlan(t.Context(), store, captureTestTask(t, "task_too_big", ""), testPlan("a", upstream.URL+"/big.mp4"))
	require.Error(t, err)
	entries, readErr := os.ReadDir(filepath.Join(root, "task_too_big"))
	if readErr == nil {
		assert.Empty(t, entries, "超限产物不得落盘，也不得留半截文件")
	}
}

// TestPersistTaskArtifactPlanSSRFBlocksPrivateTarget 产物 URL 属外部输入，必须走 SSRF 防护。
func TestPersistTaskArtifactPlanSSRFBlocksPrivateTarget(t *testing.T) {
	var hits atomic.Int64
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("should-not-be-fetched"))
	}))
	defer private.Close()

	// 产品默认策略：AllowPrivateIp=false + 端口白名单不含临时端口。
	fetchSetting := system_setting.GetFetchSetting()
	original := *fetchSetting
	t.Cleanup(func() { *fetchSetting = original })
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false
	fetchSetting.AllowedPorts = []string{"80", "443", "8080", "8443"}
	fetchSetting.DomainFilterMode = false
	fetchSetting.IpFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpList = nil
	fetchSetting.ApplyIPFilterForDomain = true

	store := &localArtifactStore{root: t.TempDir(), maxBytes: 1 << 20, enabled: true}
	err := persistTaskArtifactPlan(t.Context(), store, captureTestTask(t, "task_ssrf", ""), testPlan("a", private.URL+"/internal.mp4"))

	require.Error(t, err)
	assert.Zero(t, hits.Load(), "SSRF 防护必须拦下目标，不得发起请求")
}

// TestUpdateVideoSingleTaskCapturesArtifact 接线用例：证明 `updateVideoSingleTask`
// 的成功终态路径**真的调用了**捕获（而不是只有单元函数可测）。
//
// 把调用点注释掉，本用例必须 FAIL。
func TestUpdateVideoSingleTaskCapturesArtifact(t *testing.T) {
	truncate(t)

	const channelID = 901
	seedTaskPollingChannel(t, channelID, true)

	payload := []byte("video-artifact-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	t.Cleanup(SetTaskArtifactStoreForTest(&localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}))

	previousPlanner := TaskArtifactFetchPlanner
	TaskArtifactFetchPlanner = func(*model.Task) ([]TaskArtifactFetchPlan, error) {
		return []TaskArtifactFetchPlan{testPlan("clip", upstream.URL+"/clip.mp4")}, nil
	}
	t.Cleanup(func() { TaskArtifactFetchPlanner = previousPlanner })

	task := seedPollingTask(t, channelID, "task_public_capture", "upstream_capture")
	adaptor := &taskPollingSuccessAdaptor{resultURL: upstream.URL + "/out.mp4"}

	require.NoError(t, updateVideoSingleTask(
		context.Background(), adaptor, mustGetPollingChannel(t, channelID), task.GetUpstreamTaskID(),
		map[string]*model.Task{task.GetUpstreamTaskID(): task},
	))

	// 捕获经 gopool 异步派发（避免几十 MB 下载拖住轮询循环），故此处等待落盘。
	var onDisk []byte
	var readErr error
	require.Eventually(t, func() bool {
		onDisk, readErr = os.ReadFile(filepath.Join(root, "task_public_capture", "clip"))
		return readErr == nil
	}, 5*time.Second, 20*time.Millisecond, "轮询成功终态必须触发产物落盘（Persist 的真实生产调用方）")
	assert.Equal(t, payload, onDisk)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, string(model.TaskStatusSuccess), string(reloaded.Status))
}

// TestCapturedArtifactsAreReachableByCleanupLoop 清理对侧：`Persist` 写入的目录必须落在
// `runTaskArtifactCleanupOnce` 的扫描范围内（否则磁盘只涨不消）。
//
// 本用例真实走捕获 + 真实走清理 loop 入口，不 mock。
func TestCapturedArtifactsAreReachableByCleanupLoop(t *testing.T) {
	payload := []byte("cleanup-me")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	// LocalArtifactStore() 读进程级 store，清理 loop 只认它。
	t.Cleanup(SetTaskArtifactStoreForTest(&localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}))

	// 保留期置 0 秒：目录 mtime 立即算「过期」。
	t.Setenv("TASK_ARTIFACT_RETENTION_SECONDS", "0")

	require.NoError(t, persistTaskArtifactPlan(t.Context(),
		&localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true},
		captureTestTask(t, "task_cleanup", ""), testPlan("a", upstream.URL+"/a.mp4")))
	require.FileExists(t, filepath.Join(root, "task_cleanup", "a"), "前置条件：产物已落盘")

	runTaskArtifactCleanupOnce()

	// 注意：不能用 assert.NoFileExists —— testify 对**目录**也返回 true，
	// 断言会永远通过。必须显式判 IsNotExist。
	_, statErr := os.Stat(filepath.Join(root, "task_cleanup"))
	assert.True(t, os.IsNotExist(statErr),
		"清理 loop 必须认识 Persist 写入的目录（当前 err=%v）", statErr)
}

// TestTaskArtifactFetchPlannerNotWiredIsNoop 未注入 planner（或任务无插件产物）时捕获为 no-op。
func TestTaskArtifactFetchPlannerNotWiredIsNoop(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(SetTaskArtifactStoreForTest(&localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}))

	previousPlanner := TaskArtifactFetchPlanner
	TaskArtifactFetchPlanner = nil
	t.Cleanup(func() { TaskArtifactFetchPlanner = previousPlanner })

	captureTaskArtifacts(context.Background(), captureTestTask(t, "task_no_planner", "https://example.invalid/a.mp4"))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestPersistTaskArtifactPlansContinuesAfterFailure 单个产物失败不影响其余产物。
func TestPersistTaskArtifactPlansContinuesAfterFailure(t *testing.T) {
	payload := []byte("second-one-ok")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bad.mp4") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	useArtifactCaptureFetchSetting(t, upstream.URL)

	root := t.TempDir()
	store := &localArtifactStore{root: root, maxBytes: 1 << 20, enabled: true}
	task := captureTestTask(t, "task_partial", "")

	persistTaskArtifactPlans(t.Context(), store, task, []TaskArtifactFetchPlan{
		testPlan("bad", upstream.URL+"/bad.mp4"),
		testPlan("good", upstream.URL+"/good.mp4"),
	})

	assert.NoFileExists(t, filepath.Join(root, "task_partial", "bad"))
	got, err := os.ReadFile(filepath.Join(root, "task_partial", "good"))
	require.NoError(t, err, "前一个产物失败不得阻断后续产物")
	assert.Equal(t, payload, got)
}
