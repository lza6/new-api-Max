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
	// 财务：逐日营收/成本/利润 + 成本配置（只追加历史，不修改旧行）。
	admin.GET("/ops/daily", middleware.DisableCache(), controller.GetOpsDailySeries)
	admin.GET("/ops/model-costs", middleware.DisableCache(), controller.GetModelCosts)
	admin.POST("/ops/model-costs", middleware.DisableCache(), controller.CreateModelCost)

	// ---- 一键导出 + 下载记录 ----
	admin.POST("/training-data/export", middleware.DisableCache(), controller.CreateTrainingDataExport)
	admin.GET("/training-data/export", middleware.DisableCache(), controller.ListTrainingDataExports)
	admin.GET("/training-data/export/detail", middleware.DisableCache(), controller.GetTrainingDataExportDetail)
	admin.POST("/training-data/export/link", middleware.DisableCache(), controller.IssueTrainingDataExportLink)
	admin.DELETE("/training-data/export", middleware.DisableCache(), controller.DeleteTrainingDataExport)

	// ---- 直链下载（**刻意不在 AdminAuth 之下**）----
	//
	// 为什么这里不挂 AdminAuth：直链是给 curl / aria2 / wget 用的，
	// 它们不带会话 Cookie —— 挂 AdminAuth 会让「复制直链给工具下载」
	// 这个明确的需求直接 401 不可用。
	//
	// 那鉴权靠什么：**限时 HMAC 签名 token**（`service/training_capture/export_sign.go`），
	// 签名绑定 exportID + 签发人 + 过期时间，改动任一处即验签失败。
	// `TryUserAuth()` 是**可选**会话识别：浏览器下载时能把记录归到具体的人，
	// 无会话（curl）时回落到签发人 —— 它不做拒绝。
	exportGroup := apiRouter.Group("/training-data")
	exportGroup.Use(middleware.TryUserAuth())
	exportGroup.GET("/export/download", middleware.DisableCache(), controller.DownloadTrainingDataExport)
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
	{"GET", "/api/admin/ops/daily"},
	{"GET", "/api/admin/ops/model-costs"},
	{"POST", "/api/admin/ops/model-costs"},
	// 导出（管理端）
	{"POST", "/api/admin/training-data/export"},
	{"GET", "/api/admin/training-data/export"},
	{"GET", "/api/admin/training-data/export/detail"},
	{"POST", "/api/admin/training-data/export/link"},
	{"DELETE", "/api/admin/training-data/export"},
	// 直链下载（签名 token 鉴权，不在 AdminAuth 之下 —— 见上方注释）
	{"GET", "/api/training-data/export/download"},
}
