package service

import (
	"fmt"

	"github.com/lza6/new-api-Max/model"
)

// CheckSubscriptionModelAccess 校验订阅模型矩阵：用户存在 active 订阅时，
// 请求的模型必须在其套餐 Models 列表内（Models 为空 = 不限）。
// 无订阅 / 无套餐 / 查询失败一律放行（fail-open，沿用既有模型访问控制），
// 保证订阅矩阵上线不误伤存量请求。
func CheckSubscriptionModelAccess(userId int, modelName string) (bool, string, error) {
	if userId <= 0 || modelName == "" {
		return true, "", nil
	}
	if model.DB == nil {
		return true, "", nil
	}
	subs, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil || len(subs) == 0 {
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
