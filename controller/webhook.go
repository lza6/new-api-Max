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

// UpdateWebhookSettings 更新全局 Webhook 配置并热生效+持久化。
// URL 必须为 https（SSRF 防护在发送时二次校验）；events 为订阅事件白名单。
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
	target := config.GlobalConfig.Get("webhook")
	if target == nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := config.UpdateConfigFromMap(target, configMap); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := common.Validate.Struct(target); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidInput)
		return
	}
	// 启用时必须配置有效 http(s) URL + secret；否则拒绝保存（fail fast）。
	setting := operation_setting.GetWebhookSetting()
	if setting.Enabled {
		if setting.URL == "" || setting.Secret == "" {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		if !strings.HasPrefix(setting.URL, "https://") && !strings.HasPrefix(setting.URL, "http://") {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		// 保存时即校验 SSRF（私网/环回/云元数据拒绝），避免「配置了但永不发送」的静默失败。
		if err := service.ValidateSSRFProtectedFetchURL(setting.URL); err != nil {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
	}
	if err := saveWebhookConfig(); err != nil {
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