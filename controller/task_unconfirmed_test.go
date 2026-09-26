package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// buildUnconfirmedTestPriceData 构造带基础价格的 PriceData。
func buildUnconfirmedTestPriceData() types.PriceData {
	return types.PriceData{Quota: 4000, QuotaToPreConsume: 4000}
}

// newUnconfirmedTestBillingSession 走真实预扣路径创建 BillingSession
// （ForcePreConsume 强制预扣，与任务提交路径一致）。
func newUnconfirmedTestBillingSession(t *testing.T, relayInfo *relaycommon.RelayInfo, quota int) *service.BillingSession {
	t.Helper()
	relayInfo.ForcePreConsume = true
	relayInfo.UserSetting.BillingPreference = "wallet_only"
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session, apiErr := service.NewBillingSession(ctx, relayInfo, quota)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	return session
}

// TestPersistUnconfirmedTask 契约：提交结果不可确认时，落库的 UNCONFIRMED
// 任务行必须携带完整计费上下文与 submit 标记，保证轮询阶段可以结算/退款。
func TestPersistUnconfirmedTask(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.AutoMigrate(&model.Task{}, &model.TaskEvent{}, &model.User{}, &model.Token{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	require.NoError(t, database.Create(&model.User{
		Id: 88, Username: "unconfirmed_owner", Status: common.UserStatusEnabled, Quota: 100_000,
	}).Error)

	require.NoError(t, database.Create(&model.Token{
		Id: 7, UserId: 88, Key: "unconfirmed-token-88", Name: "unconfirmed", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000, ExpiredTime: -1,
	}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/tasks", nil)
	c.Set("platform", "kling")

	relayInfo := &relaycommon.RelayInfo{
		UserId:        88,
		UsingGroup:    "default",
		BillingSource: service.BillingSourceWallet,
		TokenId:       7,
		PriceData:     buildUnconfirmedTestPriceData(),
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "text_to_video"},
	}
	relayInfo.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: 3}
	relayInfo.OriginModelName = "kling-v2"
	relayInfo.Billing = newUnconfirmedTestBillingSession(t, relayInfo, 4000)

	info := service.SubmitUnconfirmedInfo{
		Unconfirmed:      true,
		RemoteTaskIDHint: "upstream-hint-1",
		FailedAt:         time.Now().Unix(),
	}

	require.True(t, persistUnconfirmedTask(c, relayInfo, info))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted).Error)
	assert.EqualValues(t, model.TaskStatusUnconfirmed, persisted.Status)
	assert.Equal(t, 4000, persisted.Quota, "预扣额度冻结在任务行上")
	assert.Equal(t, "text_to_video", persisted.Action)
	assert.Equal(t, "kling", string(persisted.Platform))
	assert.Equal(t, "upstream-hint-1", persisted.PrivateData.UpstreamTaskID, "hint 写入 UpstreamTaskID 供轮询查询")
	assert.Equal(t, 7, persisted.PrivateData.TokenId)
	assert.Equal(t, service.BillingSourceWallet, persisted.PrivateData.BillingSource)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, "kling-v2", persisted.PrivateData.BillingContext.OriginModelName)

	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "unconfirmed", data["submit_state"])
	assert.Equal(t, "upstream-hint-1", data["remote_task_id_hint"])
	assert.Positive(t, data["failed_at"])
}

// TestPersistUnconfirmedTaskWithoutHint 无 hint 时不写 UpstreamTaskID，
// Task.Data 仍带 submit_state 标记，轮询按窗口期兜底。
func TestPersistUnconfirmedTaskWithoutHint(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.AutoMigrate(&model.Task{}, &model.TaskEvent{}, &model.User{}, &model.Token{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	require.NoError(t, database.Create(&model.User{
		Id: 89, Username: "unconfirmed_owner2", Status: common.UserStatusEnabled, Quota: 100_000,
	}).Error)

	require.NoError(t, database.Create(&model.Token{
		Id: 7, UserId: 89, Key: "unconfirmed-token-89", Name: "unconfirmed", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000, ExpiredTime: -1,
	}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/tasks", nil)
	c.Set("platform", "kling")

	relayInfo := &relaycommon.RelayInfo{
		UserId:        89,
		TokenId:       7,
		TokenKey:      "unconfirmed-token-89",
		UsingGroup:    "default",
		BillingSource: service.BillingSourceWallet,
		PriceData:     buildUnconfirmedTestPriceData(),
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "text_to_video"},
	}
	relayInfo.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: 3}
	relayInfo.OriginModelName = "kling-v2"
	relayInfo.Billing = newUnconfirmedTestBillingSession(t, relayInfo, 2500)

	info := service.SubmitUnconfirmedInfo{Unconfirmed: true, FailedAt: time.Now().Unix()}
	require.True(t, persistUnconfirmedTask(c, relayInfo, info))

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted).Error)
	assert.Empty(t, persisted.PrivateData.UpstreamTaskID)
	assert.Equal(t, 2500, persisted.Quota)
	var data map[string]any
	require.NoError(t, common.Unmarshal(persisted.Data, &data))
	assert.Equal(t, "unconfirmed", data["submit_state"])
}
