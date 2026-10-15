package router

import (
	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/controller"
	"github.com/lza6/new-api-Max/middleware"
)

// SetTrainingDataRouter 注册训练数据采集与运营面板的**管理端** API。
//
// ## 为什么单独成文件（而不是放在 api-router.go 的 adminRoute 里）
//
// `api-router.go` 里的 `adminRoute` 是 `apiRouter.Group("/user")` 的子组，
// 所以实际前缀是 **`/api/user/...`**，而不是 `/api/...`。
// 把管理接口放进去会让路径变成 `/api/user/training-data/admin/*` ——
// 与前端调用（`/api/admin/...`）以及本仓其它管理端（web-protection）
// 的约定都不一致，表现为**全部 404**。
//
// 本文件沿用 `web-protection-router.go` 的既有范式：
// 独立 router 文件 + `/api` → `/admin` 分组 + 统一 `AdminAuth()`。
//
// ## 前缀一览
//
//	/api/admin/training-data/{overview,by-date,enabled}   采集总览与总开关
//	/api/admin/training-data/{files,file}                 云盘浏览/预览/删除
//	/api/admin/training-data/analyze                      对话深度分析
//	/api/admin/ops/{overview,consumption-ranking,grant-quota}  运营面板
func SetTrainingDataRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	admin := apiRouter.Group("/admin")
	admin.Use(middleware.AdminAuth())

	// ---- 训练数据采集管理 ----
	admin.GET("/training-data/overview", middleware.DisableCache(), controller.GetTrainingDataAdminOverview)
	admin.GET("/training-data/by-date", middleware.DisableCache(), controller.GetTrainingDataByDate)
	admin.PUT("/training-data/enabled", middleware.DisableCache(), controller.SetTrainingDataEnabled)

	// 云盘文件浏览与操作（B4/B7）。
	admin.GET("/training-data/files", middleware.DisableCache(), controller.ListTrainingDataFiles)
	admin.GET("/training-data/file", middleware.DisableCache(), controller.PreviewTrainingDataFile)
	admin.DELETE("/training-data/file", middleware.DisableCache(), controller.DeleteTrainingDataFile)

	// 对话深度分析 + 继续追问（B9/B10）。
	admin.POST("/training-data/analyze", middleware.DisableCache(), controller.AnalyzeTrainingDataConversation)

	// ---- 运营面板 ----
	admin.GET("/ops/overview", middleware.DisableCache(), controller.GetOpsOverview)
	admin.GET("/ops/consumption-ranking", middleware.DisableCache(), controller.GetOpsConsumptionRanking)
	admin.POST("/ops/grant-quota", middleware.DisableCache(), controller.GrantUserQuota)
}

// trainingDataAdminRoutes 供**路由契约测试**断言用（前缀必须与前端一致）。
//
// 单独列出而不是从上面重复注册：测试直接读这份声明去比对注册结果，
// 一旦有人改了路径却没改前端（或反之），测试会失败 ——
// 这正是「库全绿但生产 404」那类问题的防线。
var trainingDataAdminRoutes = []struct {
	Method string
	Path   string
}{
	{"GET", "/api/admin/training-data/overview"},
	{"GET", "/api/admin/training-data/by-date"},
	{"PUT", "/api/admin/training-data/enabled"},
	{"GET", "/api/admin/training-data/files"},
	{"GET", "/api/admin/training-data/file"},
	{"DELETE", "/api/admin/training-data/file"},
	{"POST", "/api/admin/training-data/analyze"},
	{"GET", "/api/admin/ops/overview"},
	{"GET", "/api/admin/ops/consumption-ranking"},
	{"POST", "/api/admin/ops/grant-quota"},
}
