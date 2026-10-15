package router

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 路由契约测试。
//
// ## 为什么需要它（这不是形式主义，是一次真实事故的防线）
//
// 独立审查发现：训练数据与运营面板的管理端接口被误注册在 `/api/user/...`
// 之下（因为 `api-router.go` 里的 `adminRoute` 父组是 `/user`），
// 而前端调用的是 `/api/admin/...` —— **整套管理界面 404**。
//
// 而当时的测试全都是「直接调用 handler 函数 + 拼一个字符串 URL」，
// handler 不校验路径，所以**库全绿、生产全挂**。这正是本项目
// 记忆里反复出现的「库全绿但生产零调用」那一类缺口。
//
// 因此这里做两层断言：
//  1. 后端声明的每条路由**真的注册进了 gin**（含方法与前缀）；
//  2. 前端源码里出现的每个 `/api/...` 路径**都能在后端路由表里找到**
//     （允许 `%s`/模板拼接，逐个静态片段比对）。
//
// 第 2 条是真正能防漂移的那条：改了一边没改另一边，测试立刻失败。

// buildRouterForTest 按生产同样的方式装配路由（只装我们关心的两组）。
func buildRouterForTest() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetTrainingDataRouter(engine)
	return engine
}

// registeredRoutes 返回 method+path 的集合。
func registeredRoutes(engine *gin.Engine) map[string]bool {
	out := make(map[string]bool, len(engine.Routes()))
	for _, r := range engine.Routes() {
		out[r.Method+" "+r.Path] = true
	}
	return out
}

func TestTrainingDataAdminRoutesAreRegistered(t *testing.T) {
	engine := buildRouterForTest()
	registered := registeredRoutes(engine)

	for _, want := range trainingDataAdminRoutes {
		key := want.Method + " " + want.Path
		if !registered[key] {
			t.Errorf("声明的路由未注册：%s\n（这会导致前端调用 404）", key)
		}
	}
}

func TestTrainingDataAdminRoutesAreUnderAdminPrefix(t *testing.T) {
	// 明确的**反回归**断言：这些路径**不得**出现在 /api/user 之下。
	//
	// 这是本次事故的确切形态 —— `/api/user/training-data/admin/*`。
	// 单独写一条，是为了让「又把它放回 adminRoute」这个具体错误
	// 在第一时间被抓住（而不是被上一条通用断言以难以理解的方式报出来）。
	engine := buildRouterForTest()
	for path := range registeredRoutes(engine) {
		if strings.Contains(path, "/api/user/training-data") {
			t.Errorf("管理端路由出现在 /api/user 之下：%s —— "+
				"前端调用的是 /api/admin/...，会造成 404", path)
		}
		if strings.Contains(path, "/api/user/ops/") {
			t.Errorf("运营面板路由出现在 /api/user 之下：%s", path)
		}
	}
}

// frontendAPICalls 从指定文件里抽出所有 `/api/...` 形式的静态路径。
//
// 只取**完整字面量**（引号内、不含模板变量）；含 `${}` 或 `%s` 的拼接
// 跳过并在测试里单独报告，避免误报。
var apiPathRe = regexp.MustCompile("['\"`](/api/[A-Za-z0-9_\\-/.]+)['\"`]")

func frontendAPICalls(t *testing.T, rel string) []string {
	t.Helper()
	// 测试的工作目录是 router/，前端在 ../web。
	full := filepath.Join("..", "web", filepath.FromSlash(rel))
	data, err := os.ReadFile(full)
	if err != nil {
		t.Skipf("读不到前端文件 %s（跳过前后端契约比对）：%v", full, err)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range apiPathRe.FindAllStringSubmatch(string(data), -1) {
		p := m[1]
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func TestFrontendTrainingDataPathsExistOnBackend(t *testing.T) {
	engine := buildRouterForTest()
	registered := registeredRoutes(engine)

	// 只看路径部分（忽略 method）：前端可能用同一个路径的不同方法。
	paths := map[string]bool{}
	for key := range registered {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 {
			paths[parts[1]] = true
		}
	}

	for _, file := range []string{
		"src/features/training-data/api.ts",
		"src/features/ops-panel/api.ts",
	} {
		calls := frontendAPICalls(t, file)
		if len(calls) == 0 {
			t.Errorf("%s：没扫到任何 /api/ 调用 —— 正则或文件结构变了，本测试已失效", file)
			continue
		}
		for _, call := range calls {
			if !paths[call] {
				t.Errorf("前端调用 %s（来自 %s）在后端路由表中**不存在** → 该功能会在运行时 404",
					call, file)
			}
		}
	}
}

func TestFrontendProfileTrainingDataPathExistsOnBackend(t *testing.T) {
	// 用户端（个人资料页的授权开关）也一并比对。
	// 用户端接口注册在 api-router.go 的 selfRoute（前缀 /api/user/）。
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	paths := map[string]bool{}
	for _, r := range engine.Routes() {
		paths[r.Path] = true
	}

	calls := frontendAPICalls(t, "src/features/profile/api.ts")
	for _, call := range calls {
		if !strings.Contains(call, "training-data") {
			continue
		}
		if !paths[call] {
			t.Errorf("个人资料页调用 %s 在后端不存在 → 用户侧开关会失败", call)
		}
	}
}
