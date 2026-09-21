package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/model"
)

// 无订阅负缓存：非订阅用户（relay 热路径的常见情形）短时间跳过 DB 订阅查询，
// 避免每请求 2 次主库读（限流档位解析 + 模型矩阵校验各一次）。
// TTL 15s：购买/到期最迟 15s 内生效，对限流档位语义可接受且已注明。
// 订阅用户不缓存（人数少，保持矩阵/档位实时正确）。
const noSubscriptionCacheTTL = 15 * time.Second

var noSubscriptionCache = struct {
	sync.Mutex
	m map[int]time.Time
}{m: make(map[int]time.Time)}

// HasCachedNoSubscription 返回该用户是否命中"无 active 订阅"负缓存。
func HasCachedNoSubscription(userId int) bool {
	noSubscriptionCache.Lock()
	defer noSubscriptionCache.Unlock()
	expires, ok := noSubscriptionCache.m[userId]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(noSubscriptionCache.m, userId)
		return false
	}
	return true
}

// CacheNoSubscription 记录"无 active 订阅"负缓存；容量超限时整体清空（粗粒度但安全）。
func CacheNoSubscription(userId int) {
	noSubscriptionCache.Lock()
	defer noSubscriptionCache.Unlock()
	if len(noSubscriptionCache.m) > 10000 {
		clear(noSubscriptionCache.m)
	}
	noSubscriptionCache.m[userId] = time.Now().Add(noSubscriptionCacheTTL)
}

// CheckSubscriptionModelAccess 校验订阅模型矩阵：用户存在 active 订阅时，
// 请求的模型必须在其套餐 Models 列表内（Models 为空 = 不限）。
// 无订阅 / 无套餐 / 查询失败一律放行（fail-open，沿用既有模型访问控制），
// 保证订阅矩阵上线不误伤存量请求。
func CheckSubscriptionModelAccess(userId int, modelName string) (bool, string, error) {
	if userId <= 0 || modelName == "" {
		return true, "", nil
	}
	if HasCachedNoSubscription(userId) {
		return true, "", nil
	}
	if model.DB == nil {
		return true, "", nil
	}
	subs, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil || len(subs) == 0 {
		if err == nil {
			CacheNoSubscription(userId)
		}
		return true, "", nil
	}
	sub := subs[0].Subscription
	if sub == nil {
		return true, "", nil
	}
	plan, err := model.GetSubscriptionPlanById(sub.PlanId)
	if err != nil || plan == nil {
		return true, "", nil
	}
	if plan.PlanAllowsModel(modelName) {
		return true, "", nil
	}
	return false, fmt.Sprintf("模型 %s 不在当前订阅套餐（%s）可用模型内，请升级套餐或联系微信 Tf00798 定制", modelName, plan.Title), nil
}
