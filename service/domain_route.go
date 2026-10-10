package service

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
)

// §4.8.1 任务/领域感知路由（默认关闭，零行为变化）。
//
// 定位：在既有「健康分 + 权重 + 优先级 + 亲和 + 组合路由」之上，叠加一层**确定性
// 领域标签策略**——请求携带领域标签（HTTP 头 `X-Route-Tag`）时，把路由分组覆盖为
// 该标签映射的目标分组，从而把 medical/legal 等领域流量导向指定渠道组。
//
// 设计要点（安全优先）：
//   - **默认关闭**（env `DOMAIN_ROUTE_ENABLED` 未设/false 时完全不介入，`ResolveDomainRoute`
//     恒返回空，distributor 行为零变化）。
//   - **白名单映射**：只认配置里显式列出的 `tag:group` 对，未命中不改分组。
//   - **不越权**：目标分组必须在该用户可用分组内（`GroupInUserUsableGroups`），否则忽略，
//     退回原分组——领域标签**不能**把用户导向其无权访问的分组。
//   - **不可绕过分组计费**：覆盖发生在选择渠道之前，后续计费/倍率仍按最终分组的既有逻辑走。
//
// 配置（env，逗号分隔 `tag:group`）：`DOMAIN_ROUTE_MAP=medical:medical-group,legal:legal-group`
//
// 开关值走能力开关注册表：值来源为「管理端『实验功能』页持久化配置 > env
// DOMAIN_ROUTE_ENABLED（默认 false）」。标签映射表仍是 env（属配置数据而非能力灰度）。
var (
	domainRouteOnce       sync.Once
	domainRouteMap        map[string]string
	domainRouteEnvDefault = common.GetEnvOrDefaultBool(common.FlagDomainRouteEnabled, false)
)

// domainRouteEnabledValue 报告领域路由开关当前值（管理端可热更新）。
func domainRouteEnabledValue() bool {
	return common.FeatureFlagValue(common.FlagDomainRouteEnabled, domainRouteEnvDefault)
}

func loadDomainRouteConfig() {
	raw := strings.TrimSpace(common.GetEnvOrDefaultString("DOMAIN_ROUTE_MAP", ""))
	domainRouteMap = map[string]string{}
	if raw == "" {
		return
	}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		tag, group, ok := strings.Cut(pair, ":")
		tag = strings.TrimSpace(tag)
		group = strings.TrimSpace(group)
		if ok && tag != "" && group != "" {
			domainRouteMap[strings.ToLower(tag)] = group
		}
	}
}

// ResolveDomainRoute 根据请求头 `X-Route-Tag` 与用户可用分组解析目标分组覆盖。
// 返回 (targetGroup, true) 表示应覆盖分组；否则 (_, false) 表示不介入。
// 默认关闭、标签未配置、或目标分组不在用户可用分组内 → 不介入。
func ResolveDomainRoute(c *gin.Context, userGroup string) (string, bool) {
	if c == nil {
		return "", false
	}
	domainRouteOnce.Do(loadDomainRouteConfig)
	if !domainRouteEnabledValue() || len(domainRouteMap) == 0 {
		return "", false
	}
	tag := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("X-Route-Tag")))
	if tag == "" {
		return "", false
	}
	target, ok := domainRouteMap[tag]
	if !ok || target == "" {
		return "", false
	}
	// 不越权：目标分组必须在该用户可用分组内。
	if userGroup != "" && target != userGroup && !GroupInUserUsableGroups(userGroup, target) {
		return "", false
	}
	return target, true
}

// SetDomainRouteForTest 测试覆盖（enabled + map）；同时标记 Once 已完成，
// 避免随后 ResolveDomainRoute 触发 loadDomainRouteConfig 把测试值覆盖回 env 默认。
func SetDomainRouteForTest(enabled bool, mapping map[string]string) {
	domainRouteOnce.Do(func() {})
	common.SetFeatureFlagOverride(common.FlagDomainRouteEnabled, common.BoolFeatureFlagOverride(&enabled))
	if mapping == nil {
		domainRouteMap = map[string]string{}
		return
	}
	domainRouteMap = mapping
}

// resetDomainRouteForTest 恢复为「已加载、禁用」状态。
func resetDomainRouteForTest() {
	domainRouteOnce.Do(func() {})
	common.SetFeatureFlagOverride(common.FlagDomainRouteEnabled, nil)
	domainRouteMap = map[string]string{}
}
