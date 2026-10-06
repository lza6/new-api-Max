package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/i18n"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
)

// errInvalidWebhookInput 端点输入非法（映射到统一 400 文案，不泄露具体校验细节）。
var errInvalidWebhookInput = errors.New("invalid webhook endpoint input")

// B2-3 通用 Webhook 多端点管理端接口（管理员）。
// 端点配置（URL/Secret/事件订阅）经 SSRF 校验；Secret 以明文存储于
// webhook_endpoints 表（与渠道密钥同策略：同机 .env 主密钥防不了拖库，故不加密）。

type webhookEndpointInput struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Enabled bool     `json:"enabled"`
	Events  []string `json:"events"`
}

// GetWebhookEndpoints 返回全部端点（Secret 脱敏：仅回显是否存在）。
func GetWebhookEndpoints(c *gin.Context) {
	endpoints, err := model.ListWebhookEndpoints()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	out := make([]gin.H, 0, len(endpoints))
	for _, e := range endpoints {
		out = append(out, gin.H{
			"id":         e.Id,
			"name":       e.Name,
			"url":        e.URL,
			"enabled":    e.Enabled,
			"events":     service.ParseWebhookEndpointEvents(e.Events),
			"has_secret": e.Secret != "",
			"created_at": e.CreatedAt,
			"updated_at": e.UpdatedAt,
		})
	}
	common.ApiSuccess(c, out)
}

// CreateWebhookEndpoint 新建端点（校验 URL 与 SSRF）。
func CreateWebhookEndpoint(c *gin.Context) {
	var input webhookEndpointInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := validateWebhookEndpointInput(&input, true); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	endpoint := &model.WebhookEndpoint{
		Name:    strings.TrimSpace(input.Name),
		URL:     strings.TrimSpace(input.URL),
		Secret:  input.Secret,
		Enabled: input.Enabled,
		Events:  encodeWebhookEvents(input.Events),
	}
	if err := model.CreateWebhookEndpoint(endpoint); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "webhook.endpoint.create", map[string]any{"id": endpoint.Id, "name": endpoint.Name})
	common.ApiSuccess(c, gin.H{"id": endpoint.Id})
}

// UpdateWebhookEndpoint 更新端点（不含 secret 时保留原密钥）。
func UpdateWebhookEndpoint(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	existing, err := model.GetWebhookEndpoint(id)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var input webhookEndpointInput
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := validateWebhookEndpointInput(&input, false); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	existing.Name = strings.TrimSpace(input.Name)
	existing.URL = strings.TrimSpace(input.URL)
	existing.Enabled = input.Enabled
	existing.Events = encodeWebhookEvents(input.Events)
	if strings.TrimSpace(input.Secret) != "" {
		existing.Secret = input.Secret
	}
	if err := model.UpdateWebhookEndpoint(existing); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "webhook.endpoint.update", map[string]any{"id": id, "name": existing.Name})
	common.ApiSuccess(c, gin.H{"id": id})
}

// DeleteWebhookEndpoint 删除端点。
func DeleteWebhookEndpoint(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteWebhookEndpoint(id); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "webhook.endpoint.delete", map[string]any{"id": id})
	common.ApiSuccess(c, nil)
}

func validateWebhookEndpointInput(input *webhookEndpointInput, requireSecret bool) error {
	if strings.TrimSpace(input.Name) == "" {
		return errInvalidWebhookInput
	}
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return errInvalidWebhookInput
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return errInvalidWebhookInput
	}
	if err := service.ValidateSSRFProtectedFetchURL(url); err != nil {
		return err
	}
	if requireSecret && strings.TrimSpace(input.Secret) == "" {
		return errInvalidWebhookInput
	}
	return nil
}

func encodeWebhookEvents(events []string) string {
	normalized := make([]string, 0, len(events))
	for _, e := range events {
		if s := strings.TrimSpace(e); s != "" {
			normalized = append(normalized, s)
		}
	}
	encoded, err := common.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
