# ADR-005：管理端路由放 `/api/admin/...`，而不是 `adminRoute`

> 目录：`计划书/adr/`（编号在本目录内独立）。本次决策批次：训练数据采集管理端。
> 最后核对：2026-10-16（`VERSION` = `v1.3.147`）。

## 背景

新增训练数据/运营面板的管理端接口时，最初把它们注册进 `router/api-router.go` 里既有的
`adminRoute` 组。结果**整套管理界面 404**。

根因：`adminRoute` 是 `userRoute`（`apiRouter.Group("/user")`）的子组
——`router/api-router.go:89`（`userRoute := apiRouter.Group("/user")`）、
`:167-168`（`adminRoute := userRoute.Group("/")` + `AdminAuth()`）。
因此它的实际前缀是 **`/api/user/...`**，而不是 `/api/...`（`:174` 的注释已写明这点）。
接口路径于是变成 `/api/user/training-data/admin/*`，与前端调用的 `/api/admin/...`
以及本仓其它管理端（web-protection）的约定都不一致，表现为全部 404。

## 决策

管理端路由**独立成文件**，沿用 `web-protection-router.go` 的既有范式：
`/api` → `/admin` 分组 + 统一 `AdminAuth()`。

- 范式：`router/web-protection-router.go:10-26`
- 本批：`router/training-data-router.go:29-51`
- 在 `router/main.go` 的 `Set*Router` 序列登记（`:32-33`）。
- 读接口用 `middleware.DisableCache()`。

配套：为**防回归**加「路由契约测试」——扫描前端源码里的每个 `/api/...` 路径，
断言它在后端路由表里存在（`router/training_data_router_test.go:107-133`），
并断言管理路由都在 `/api/admin` 前缀下（`:62-73`）。

## 被拒绝的替代方案

1. **放进 `adminRoute`**：前缀 `/api/user/...`，与前端和其它管理端不一致 → 全 404（见背景）。
2. **改前端去适配 `/api/user/...`**：与 web-protection 等既有管理端约定割裂，且违反
   「管理端统一在 /api/admin」的仓库现状；治标不治本。
3. **只写「直接调 handler」的集成测试**：这正是 404 漏网的原因——测试不走真实路由注册表，
   路径写错也全绿。故改为契约测试走 `buildRouterForTest()` 的真实路由表。

## 后果（含代价）

- ✅ 前缀与前端、与其它管理端一致；`AdminAuth()` 统一挂载。
- ✅ 契约测试成为「库全绿但生产 404」这类问题的防线（RED：改错一个前端路径 → 测试失败并指出会 404）。
- ⚠️ 新增前端 API 文件后，要记得把该文件路径加进契约测试的扫描列表
  （`router/training_data_router_test.go:115-118`），否则该文件不受保护。

## 证据

- `router/api-router.go:89,167-168,174`（`adminRoute` 前缀真相）
- `router/web-protection-router.go:10-26`（范式）
- `router/training-data-router.go:10-72`（决策与路由清单）
- `router/main.go:32-33`（登记）
- 回归用例：`router/training_data_router_test.go:50`（`TestTrainingDataAdminRoutesAreRegistered`）、
  `:62`（`TestTrainingDataAdminRoutesAreUnderAdminPrefix`）、
  `:107`（`TestFrontendTrainingDataPathsExistOnBackend`）、`:138`（`TestFrontendProfileTrainingDataPathExistsOnBackend`）
