package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unconfirmedFetchAdaptor 可编程的 unconfirmed 解析适配器。
type unconfirmedFetchAdaptor struct {
	status model.TaskStatus
	reason string
	url    string
}

func (a *unconfirmedFetchAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *unconfirmedFetchAdaptor) FetchTask(_ string, _ string, _ *model.Task, _ string) (*http.Response, error) {
	body := fmt.Sprintf(`{"status":"%s","reason":"%s"}`, a.status, a.reason)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}
func (a *unconfirmedFetchAdaptor) ParseTaskResult(_ *model.Task, _ *http.Response, body []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{
		Status: string(a.status),
		Reason: a.reason,
		Url:    a.url,
	}, nil
}
func (a *unconfirmedFetchAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

// seedUnconfirmedTask 落库一条 UNCONFIRMED 任务。
func seedUnconfirmedTask(t *testing.T, userID, channelID, quota, tokenID int, taskID, upstreamID, hint string, failedAt int64) *model.Task {
	t.Helper()
	task := makeTask(userID, channelID, quota, tokenID, BillingSourceWallet, 0)
	task.TaskID = taskID
	task.Status = model.TaskStatusUnconfirmed
	task.Progress = "50%"
	task.SubmitTime = failedAt
	if upstreamID != "" {
		task.PrivateData.UpstreamTaskID = upstreamID
	}
	data := map[string]any{"submit_state": "unconfirmed", "failed_at": failedAt}
	if hint != "" {
		data["remote_task_id_hint"] = hint
	}
	task.SetData(data)
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

// seedUnconfirmedChannel 落库解析用的测试渠道（含 key/baseURL 元数据）。
func seedUnconfirmedChannel(t *testing.T, channelID int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     channelID,
		Type:   constant.ChannelTypeKling,
		Name:   "unconf-ch",
		Key:    "sk-unconf-ch",
		Status: common.ChannelStatusEnabled,
	}).Error)
}

func TestClassifySubmitFailure(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   []byte
		upErr  error
		wantUn bool
	}{
		{name: "network error", status: 0, upErr: fmt.Errorf("dial tcp: timeout"), wantUn: true},
		{name: "500 server", status: http.StatusInternalServerError, wantUn: true},
		{name: "503 unavailable", status: http.StatusServiceUnavailable, wantUn: true},
		{name: "408 timeout", status: http.StatusRequestTimeout, wantUn: true},
		{name: "400 bad request", status: http.StatusBadRequest, wantUn: false},
		{name: "401 unauthorized", status: http.StatusUnauthorized, wantUn: false},
		{name: "2xx empty body unreadable", status: http.StatusOK, wantUn: true},
		{name: "2xx with body", status: http.StatusOK, body: []byte(`{"ok":1}`), wantUn: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := ClassifySubmitFailure(tc.status, tc.body, tc.upErr)
			assert.Equal(t, tc.wantUn, info.Unconfirmed)
		})
	}
}

func TestExtractRemoteTaskIDHint(t *testing.T) {
	assert.Equal(t, "kling-task", ExtractRemoteTaskIDHint([]byte(`{"data":{"task_id":"kling-task"}}`)))
	assert.Equal(t, "alibaba-task", ExtractRemoteTaskIDHint([]byte(`{"output":{"task_id":"alibaba-task"}}`)))
	assert.Equal(t, "top-task", ExtractRemoteTaskIDHint([]byte(`{"task_id":"top-task"}`)))
	assert.Equal(t, "doubao-id", ExtractRemoteTaskIDHint([]byte(`{"id":"doubao-id"}`)))
	assert.Empty(t, ExtractRemoteTaskIDHint(nil))
	assert.Empty(t, ExtractRemoteTaskIDHint([]byte(`not json`)))
	assert.Empty(t, ExtractRemoteTaskIDHint([]byte(`{"task_id":123}`)))
}

func TestMarkUnconfirmedOnContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := ClassifySubmitFailure(http.StatusBadGateway, nil, nil)
	MarkUnconfirmedOnContext(c, info)
	got, ok := ReadUnconfirmedFromContext(c)
	require.True(t, ok)
	require.True(t, got.Unconfirmed)
	assert.Positive(t, got.FailedAt)

	// ClearUnconfirmedFromContext 之后标记等价于无标记。
	ClearUnconfirmedFromContext(c)
	cleared, ok := ReadUnconfirmedFromContext(c)
	require.False(t, ok && cleared.Unconfirmed)
}

// TestResolveUnconfirmedTasksHintSuccess 有 hint → 按 hint 查到成功 → 正常结算。
func TestResolveUnconfirmedTasksHintSuccess(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 701, 701, 701
	const initialQuota, preConsumed = 10_000, 4_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-unconfirmed-hint", 7_000)

	task := seedUnconfirmedTask(t, userID, channelID, preConsumed, tokenID,
		"task_unconfirmed_hint_success", "upstream_hint", "upstream_hint",
		time.Now().Add(-time.Minute).Unix())

	adaptor := &unconfirmedFetchAdaptor{status: model.TaskStatusSuccess, url: "https://media.example/v.mp4"}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	seedUnconfirmedChannel(t, channelID)

	require.NoError(t, ResolveUnconfirmedTasks(context.Background()))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, persisted.Status)
	assert.Equal(t, "https://media.example/v.mp4", persisted.GetResultURL())
	// 结算成功不退款；预扣保持（与常规轮询成功路径语义一致）。
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
	assert.Zero(t, countLogs(t), "成功解析不产生退款/结算日志")
	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "success", data["resolution"])
}

// TestResolveUnconfirmedTasksHintFailure 有 hint → 查到失败 → 退款并标注。
func TestResolveUnconfirmedTasksHintFailure(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 702, 702, 702
	const initialQuota, preConsumed, tokenRemain = 10_000, 4_000, 7_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-unconfirmed-hint-fail", tokenRemain)

	task := seedUnconfirmedTask(t, userID, channelID, preConsumed, tokenID,
		"task_unconfirmed_hint_fail", "upstream_hint_fail", "upstream_hint_fail",
		time.Now().Add(-time.Minute).Unix())

	adaptor := &unconfirmedFetchAdaptor{status: model.TaskStatusFailure, reason: "upstream rejected"}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	seedUnconfirmedChannel(t, channelID)

	require.NoError(t, ResolveUnconfirmedTasks(context.Background()))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, persisted.Status)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, initialQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "failure", data["resolution"])
	// B2-3 退款可见性标记同时存在于 Task.Data。
	refund, ok := data["refund"].(map[string]any)
	require.True(t, ok, "退款任务 Task.Data 必须带 refund 摘要")
	assert.Equal(t, float64(preConsumed), refund["quota"])
}

// TestResolveUnconfirmedTasksWindowExpiredNoHint 无 hint 且超窗 → 退款并标记 refunded_after_window。
func TestResolveUnconfirmedTasksWindowExpiredNoHint(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 703, 703, 703
	const initialQuota, preConsumed, tokenRemain = 10_000, 4_000, 7_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-unconfirmed-window", tokenRemain)

	// 超窗 31 分钟（默认窗口 30 分钟）。
	task := seedUnconfirmedTask(t, userID, channelID, preConsumed, tokenID,
		"task_unconfirmed_window", "", "",
		time.Now().Add(-31*time.Minute).Unix())

	adaptor := &unconfirmedFetchAdaptor{status: model.TaskStatusInProgress}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	seedUnconfirmedChannel(t, channelID)

	require.NoError(t, ResolveUnconfirmedTasks(context.Background()))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, persisted.Status)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, initialQuota+preConsumed, getUserQuota(t, userID))
	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "refunded_after_window", data["resolution"])
}

// TestResolveUnconfirmedTasksPendingWithinWindow 窗口内无终态 → 保持 UNCONFIRMED。
func TestResolveUnconfirmedTasksPendingWithinWindow(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 704, 704, 704
	const initialQuota, preConsumed = 10_000, 4_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-unconfirmed-pending", 7_000)

	task := seedUnconfirmedTask(t, userID, channelID, preConsumed, tokenID,
		"task_unconfirmed_pending", "upstream_pending", "upstream_pending",
		time.Now().Add(-time.Minute).Unix())

	adaptor := &unconfirmedFetchAdaptor{status: model.TaskStatusInProgress}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	seedUnconfirmedChannel(t, channelID)

	require.NoError(t, ResolveUnconfirmedTasks(context.Background()))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusUnconfirmed, persisted.Status)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
}

// TestRefundTaskQuotaAppendsRefundMarker B2-3 退款可见性：
// RefundTaskQuota 成功后 Task.Data 必须携带 refund 摘要（quota/reason/settled_at），
// 任务详情页据此在失败原因同一处展示退回额度。
func TestRefundTaskQuotaAppendsRefundMarker(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 705, 705, 705
	const initialQuota, preConsumed, tokenRemain = 10_000, 4_000, 7_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "sk-refund-marker", tokenRemain)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task_refund_marker"
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.SetData(map[string]any{"provider_payload": "keep-me"})
	require.NoError(t, model.DB.Create(task).Error)

	require.True(t, RefundTaskQuota(context.Background(), task, "task_failed"))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, initialQuota+preConsumed, getUserQuota(t, userID))

	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "keep-me", data["provider_payload"], "既有 Data 字段不能被 refund 标记覆盖")
	refund, ok := data["refund"].(map[string]any)
	require.True(t, ok, "Task.Data 必须包含 refund 摘要")
	assert.Equal(t, float64(preConsumed), refund["quota"])
	assert.Equal(t, "task_failed", refund["reason"])
	assert.Positive(t, refund["settled_at"])
}
