package service

import (
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/relay_setting"
)

// ResolveUserRateLimit 返回用户「当前生效」的限速档位（并发/秒 + RPM）与来源。
// 语义与 relay 热路径限流中间件一致：
//   - 存在 active 订阅且套餐/覆盖档位 > 0 → 订阅档位（并发/RPM 由套餐与单订阅覆盖决定）
//   - 否则：用户覆盖 > 分组覆盖 > 系统默认（可关闭=不限）。
//
// 用于个人资料页展示真实生效数字，不改变任何限流行为。
func ResolveUserRateLimit(userId int, group string) (concurrency, rpm int, source string) {
	if userId > 0 && model.DB != nil {
		if subs, err := model.GetAllActiveUserSubscriptions(userId); err == nil && len(subs) > 0 {
			sub := subs[0].Subscription
			if sub != nil {
				if plan, err := model.GetSubscriptionPlanById(sub.PlanId); err == nil && plan != nil {
					c, r := sub.EffectiveTier(plan.ConcurrencyLimit, plan.RpmLimit)
					if c > 0 || r > 0 {
						return c, r, "subscription"
					}
				}
			}
		}
	}

	s := relay_setting.GetRelaySetting()
	if s == nil {
		return relay_setting.DefaultUserBaseConcurrencyLimit, relay_setting.DefaultUserBaseRpmLimit, "base"
	}
	if tier, ok := s.UserRateLimitOverrides[userId]; ok {
		return tier.Concurrency, tier.Rpm, "user"
	}
	if tier, ok := s.GroupRateLimitOverrides[group]; ok {
		return tier.Concurrency, tier.Rpm, "group"
	}
	enabled := true
	if s.UserBaseRateLimitEnabled != nil {
		enabled = *s.UserBaseRateLimitEnabled
	}
	if !enabled {
		return 0, 0, "off"
	}
	c := s.UserBaseConcurrencyLimit
	if c <= 0 {
		c = relay_setting.DefaultUserBaseConcurrencyLimit
	}
	r := s.UserBaseRpmLimit
	if r <= 0 {
		r = relay_setting.DefaultUserBaseRpmLimit
	}
	return c, r, "base"
}
