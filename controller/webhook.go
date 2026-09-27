package controller

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	"github.com/lza6/new-api-Max/setting/config"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// GetWebhookSettings 返回全局 Webhook 事件通知配置（管理员）。
func GetWebhookSettings(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetWebhookSetting())
}

// UpdateWebhookSettings 更新全局 Webhook 配置。
// 两阶段提交：先在副本上组装+校验，全部通过后再写入运行时配置并持久化；
// 任一失败（校验/UURL/SSRF/持久化）均回滚，运行时行为不受被拒配置影响。
func UpdateWebhookSettings(c *gin.Context) {
	var input map[string]any
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	configMap := make(map[string]string)
	for key, value := range input {
		switch v := value.(type) {
		case bool:
			configMap[key] = strconv.FormatBool(v)
		case float64:
			configMap[key] = strconv.FormatInt(int64(v), 10)
		case string:
			configMap[key] = v
		case []any:
			encoded, err := common.Marshal(v)
			if err != nil {
				common.ApiErrorI18n(c, i18n.MsgInvalidParams)
				return
			}
			configMap[key] = string(encoded)
		default:
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}

	// ① 在深拷贝上组装。
	prev := operation_setting.SnapshotWebhookSetting()
	next := prev
	if err := config.UpdateConfigFromMap(&next, configMap); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(&next); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidInput)
		return
	}

	// ② 启用时必须配置有效 http(s) URL + secret；并对 URL 做 SSRF fail-fast。
	if next.Enabled {
		if next.URL == "" || next.Secret == "" {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if !strings.HasPrefix(next.URL, "https://") && !strings.HasPrefix(next.URL, "http://") {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if err := service.ValidateSSRFProtectedFetchURL(next.URL); err != nil {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}

	// ③ 提交运行时配置。
	operation_setting.ReplaceWebhookSetting(next)

	// ④ 持久化；失败回滚运行时配置。
	if err := saveWebhookConfig(); err != nil {
		operation_setting.ReplaceWebhookSetting(prev)
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, operation_setting.GetWebhookSetting())
}

func saveWebhookConfig() error {
	exported := config.GlobalConfig.ExportAllConfigs()
	for key, value := range exported {
		if !strings.HasPrefix(key, "webhook.") {
			continue
		}
		if err := model.UpdateOption(key, value); err != nil {
			return err
		}
	}
	return nil
}