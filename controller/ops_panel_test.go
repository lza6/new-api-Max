package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 运营面板的**口径一致性**测试。
//
// 这一组性质比单个数字是否正确更重要：运营者拿这些数字做扩容/充值决策，
// 两个面板互相矛盾比两个都错更危险（会让人以为其中一个"在刷新"）。

// setupOpsPanelTest 起一个内存库并把它同时当作主库与日志库。
func setupOpsPanelTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	// 关掉缓存：同一用例内要多次查询并看到最新数据。
	common.RedisEnabled = false

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
	})
	return db
}

func opsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ops/ranking", GetOpsConsumptionRanking)
	r.GET("/ops/overview", GetOpsOverview)
	return r
}

// TestOpsRankingExcludesAdmins 锁定排行榜必须排除管理员。
//
// 与 `total_remaining_quota`（只统计 role < RoleAdminUser）**同口径**。
// 两处口径不一致时，运营者会看到「榜首消耗最多，但他的额度不计入剩余额度」
// 这种自相矛盾的数字；而管理员常拿自己账号跑分析/测试，会长期霸榜。
func TestOpsRankingExcludesAdmins(t *testing.T) {
	db := setupOpsPanelTest(t)

	require.NoError(t, db.Create(&model.User{
		Username: "normal-heavy", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		AffCode: "aff-normal-heavy",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "boss-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
		AffCode: "aff-boss-admin",
	}).Error)

	normal := model.User{}
	require.NoError(t, db.Where("username = ?", "normal-heavy").First(&normal).Error)
	admin := model.User{}
	require.NoError(t, db.Where("username = ?", "boss-admin").First(&admin).Error)

	now := common.GetTimestamp()
	// 管理员消耗**更大** —— 若未排除，它会排在第一位。
	require.NoError(t, db.Create(&model.Log{
		UserId: admin.Id, Username: admin.Username, Type: model.LogTypeConsume,
		Quota: 9_000_000, CreatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: normal.Id, Username: normal.Username, Type: model.LogTypeConsume,
		Quota: 1_000_000, CreatedAt: now,
	}).Error)

	w := httptest.NewRecorder()
	opsTestRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/ranking?days=7&limit=20", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				UserID   int    `json:"user_id"`
				Username string `json:"username"`
				Quota    int64  `json:"quota"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success, "响应体：%s", w.Body.String())

	require.Len(t, body.Data.Items, 1, "排行榜只应有 1 个普通用户，实际：%+v", body.Data.Items)
	assert.Equal(t, normal.Id, body.Data.Items[0].UserID)
	assert.Equal(t, int64(1_000_000), body.Data.Items[0].Quota)
}

// TestOpsRankingSurvivesManyAdmins 管理员数量超过内联上限时，
// 必须**仍然**把他们排除掉（退化为 Go 侧过滤），而不是把他们放回榜单。
func TestOpsRankingSurvivesManyAdmins(t *testing.T) {
	db := setupOpsPanelTest(t)

	// 造出比内联上限更多的管理员，强制走回退路径。
	adminIds := make([]int, 0, opsMaxInlineAdminIds+2)
	for range opsMaxInlineAdminIds + 2 {
		n := len(adminIds)
		u := &model.User{
			Username: fmt.Sprintf("adm-%d", n), Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
			AffCode: fmt.Sprintf("aff-adm-%d", n)}
		require.NoError(t, db.Create(u).Error)
		adminIds = append(adminIds, u.Id)
	}
	require.NoError(t, db.Create(&model.User{
		Username: "real-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		AffCode: "aff-real-user",
	}).Error)
	normal := model.User{}
	require.NoError(t, db.Where("username = ?", "real-user").First(&normal).Error)

	now := common.GetTimestamp()
	// 每个管理员都有较大消耗；回退路径若实现错了，这些会漏进榜单。
	for _, id := range adminIds {
		require.NoError(t, db.Create(&model.Log{
			UserId: id, Type: model.LogTypeConsume, Quota: 5_000_000, CreatedAt: now,
		}).Error)
	}
	require.NoError(t, db.Create(&model.Log{
		UserId: normal.Id, Type: model.LogTypeConsume, Quota: 100, CreatedAt: now,
	}).Error)

	w := httptest.NewRecorder()
	opsTestRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/ranking?days=7&limit=50", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Items []struct {
				UserID int `json:"user_id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	adminSet := make(map[int]bool, len(adminIds))
	for _, id := range adminIds {
		adminSet[id] = true
	}
	for _, item := range body.Data.Items {
		assert.Falsef(t, adminSet[item.UserID],
			"回退路径下管理员 %d 仍出现在排行榜里", item.UserID)
	}
	require.Len(t, body.Data.Items, 1, "应只剩普通用户：%+v", body.Data.Items)
	assert.Equal(t, normal.Id, body.Data.Items[0].UserID)
}
