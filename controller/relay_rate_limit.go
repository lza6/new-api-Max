package controller

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/relay_setting"
)

// RelayRateLimitOverrideRequest 限速覆盖请求（concurrency/rpm 均为 0 表示移除覆盖）。
type RelayRateLimitOverrideRequest struct {
	UserId      int    `json:"user_id"`
	Group       string `json:"group"`
	Concurrency int    `json:"concurrency"`
	Rpm         int    `json:"rpm"`
}

func persistRelayRateLimitSetting() error {
	raw, err := common.Marshal(relay_setting.GetRelaySetting())
	if err != nil {
		return err
	}
	return model.UpdateOption("relay", string(raw))
}

// GetRelayRateLimitOverrides 返回每用户基础限速配置与分组/用户覆盖（管理端）。
// GET /api/setting/relay/rate_limit/overrides
func GetRelayRateLimitOverrides(c *gin.Context) {
	s := relay_setting.GetRelaySetting()
	baseEnabled := true
	if s.UserBaseRateLimitEnabled != nil {
		baseEnabled = *s.UserBaseRateLimitEnabled
	}
	common.ApiSuccess(c, gin.H{
		"base_enabled":     baseEnabled,
		"base_concurrency": s.UserBaseConcurrencyLimit,
		"base_rpm":         s.UserBaseRpmLimit,
		"group_overrides":  s.GroupRateLimitOverrides,
		"user_overrides":   s.UserRateLimitOverrides,
	})
}

// SetRelayRateLimitGroupOverride 设置/移除分组限速覆盖。
// PUT /api/setting/relay/rate_limit/overrides/group
func SetRelayRateLimitGroupOverride(c *gin.Context) {
	var req RelayRateLimitOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Concurrency < 0 || req.Rpm < 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	req.Group = strings.TrimSpace(req.Group)
	if req.Group == "" {
		common.ApiErrorMsg(c, "分组不能为空")
		return
	}
	relay_setting.SetGroupRateLimitOverride(req.Group, relay_setting.RateLimitTier{
		Concurrency: req.Concurrency,
		Rpm:         req.Rpm,
	})
	if err := persistRelayRateLimitSetting(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "relay.rate_limit.group_override", map[string]any{
		"group": req.Group, "concurrency": req.Concurrency, "rpm": req.Rpm,
	})
	common.ApiSuccess(c, nil)
}

// SetRelayRateLimitUserOverride 设置/移除用户限速覆盖。
// PUT /api/setting/relay/rate_limit/overrides/user
func SetRelayRateLimitUserOverride(c *gin.Context) {
	var req RelayRateLimitOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UserId <= 0 || req.Concurrency < 0 || req.Rpm < 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	relay_setting.SetUserRateLimitOverride(req.UserId, relay_setting.RateLimitTier{
		Concurrency: req.Concurrency,
		Rpm:         req.Rpm,
	})
	if err := persistRelayRateLimitSetting(); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "relay.rate_limit.user_override", map[string]any{
		"user_id": strconv.Itoa(req.UserId), "concurrency": req.Concurrency, "rpm": req.Rpm,
	})
	common.ApiSuccess(c, nil)
}
