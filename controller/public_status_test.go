package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// 公开状态页端点（G7）的契约测试。
//
// 这个端点是**匿名可访问**的，因此要锁住的不是渲染，而是两件事：
//  1. 响应结构完整（前端渲染任一卡片都不会拿到 undefined）；
//  2. **不泄露**：不得出现探测错误文本、主机名/端口、渠道名、用户标识 ——
//     本用例用「整段 JSON 里不得出现标记串」的方式直接验证这一点。
func TestGetPublicStatusShapeAndLeaks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetPublicStatusCacheForTest()

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/status/page", nil)
	GetPublicStatus(c)

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()

	var payload struct {
		Success bool                 `json:"success"`
		Data    service.PublicStatus `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	data := payload.Data

	// 结构完整：前端渲染所需的每一项都存在。
	assert.Equal(t, common.Version, data.Version)
	assert.GreaterOrEqual(t, data.GeneratedAt, common.StartTime)
	assert.NotEmpty(t, data.Components)
	assert.Greater(t, data.Traffic.WindowSeconds, int64(0))

	keys := map[string]bool{}
	for _, component := range data.Components {
		assert.NotEmpty(t, component.Key)
		// 状态必须是前端认识的取值之一（否则徽章会落到默认样式而看不出异常）。
		switch component.State {
		case service.PublicStatusStateOperational,
			service.PublicStatusStateDegraded,
			service.PublicStatusStateDown,
			service.PublicStatusStateDisabled,
			service.PublicStatusStateUnknown:
		default:
			t.Fatalf("component %s has unknown state %q", component.Key, component.State)
		}
		keys[component.Key] = true
	}
	assert.True(t, keys[service.PublicStatusComponentDatabase], "必须报告主库状态")
	assert.True(t, keys[service.PublicStatusComponentBackgroundJob], "必须报告后台任务状态")
	// 只列实际存在的组件：不得出现未接入观测的臆造项。
	assert.Len(t, data.Components, 4, "组件清单必须恰好是主库/日志库/缓存/后台任务")

	// 不泄露：整段响应里不得出现这些标记串。
	for _, marker := range []string{
		"down: ", "not initialized", "[::1]", "127.0.0.1", "localhost",
		"sk-", "password", "DSN", "/var/lib", "C:\\",
	} {
		assert.NotContainsf(t, body, marker, "公开端点不得包含 %q", marker)
	}
	assert.NotContains(t, strings.ToLower(body), "sqlite",
		"公开端点不得暴露数据库引擎/驱动细节")
}

// 缓存生效：同一时刻连续请求返回同一份快照（生成时刻不变）。
//
// 这不是测实现细节 —— 它保护的是「公开端点不会被每个请求都打一次 DB 聚合」这一
// 运维契约（爬虫可无限触发）。缓存过期/刷新路径由 service 层用例覆盖。
func TestGetPublicStatusServesCachedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetPublicStatusCacheForTest()

	call := func() service.PublicStatus {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/status/page", nil)
		GetPublicStatus(c)
		require.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Data service.PublicStatus `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
		return payload.Data
	}

	first := call()
	second := call()
	assert.Equal(t, first.GeneratedAt, second.GeneratedAt,
		"TTL 内必须复用同一份快照（否则公开端点会变成每请求一次 DB 聚合）")
}

// 主库/日志库均未初始化时端点仍必须 200 且结构完整：公开页面不能因为依赖缺失
// 就整页报错 —— 那正是状态页要展示的情况。
func TestGetPublicStatusWithoutDatabases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevDB, prevLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = nil, nil
	service.ResetPublicStatusCacheForTest()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = prevDB, prevLogDB
		service.ResetPublicStatusCacheForTest()
	})

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/status/page", nil)
	GetPublicStatus(c)

	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Data service.PublicStatus `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))

	assert.Equal(t, service.PublicStatusStateDegraded, payload.Data.Status)
	assert.Nil(t, payload.Data.Traffic.Requests, "没有日志库时请求量必须报告为不可用，而不是 0")
	assert.Nil(t, payload.Data.Traffic.SuccessRate)
	_ = time.Now()
}
