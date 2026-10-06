package model

import (
	"github.com/lza6/new-api-Max/common"
	"gorm.io/gorm"
)

// WebhookEndpoint 通用 Webhook 多端点配置（B2-3）。
//
// 与 operation_setting.WebhookSetting（单 URL 全局配置）并存：该表支持多个接收方，
// 每个端点可独立订阅事件、独立 HMAC 密钥、独立启用开关。事件投递经事件总线
// （service.EventBus）+ event_deliveries 幂等表，跨重启/跨实例只投递一次。
type WebhookEndpoint struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
	Name    string `json:"name" gorm:"type:varchar(128);not null"`
	URL     string `json:"url" gorm:"type:varchar(512);not null"`
	Secret  string `json:"secret" gorm:"type:varchar(256);not null"`
	Enabled bool   `json:"enabled" gorm:"index"`
	// Events 订阅的事件类型（JSON 数组字符串，如 ["epay.topup.success","task.settled"]）。
	// 空数组 = 不订阅任何事件（与单 URL 配置语义一致：必须显式勾选）。
	Events    string `json:"events" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

func (e *WebhookEndpoint) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if e.CreatedAt == 0 {
		e.CreatedAt = now
	}
	if e.UpdatedAt == 0 {
		e.UpdatedAt = now
	}
	return nil
}

func (e *WebhookEndpoint) BeforeSave(_ *gorm.DB) error {
	e.UpdatedAt = common.GetTimestamp()
	return nil
}

// ListEnabledWebhookEndpoints 返回所有已启用的端点（投递路径只读这里）。
func ListEnabledWebhookEndpoints() ([]*WebhookEndpoint, error) {
	var endpoints []*WebhookEndpoint
	err := DB.Where("enabled = ?", true).Order("id").Find(&endpoints).Error
	return endpoints, err
}

// ListWebhookEndpoints 返回全部端点（管理端用）。
func ListWebhookEndpoints() ([]*WebhookEndpoint, error) {
	var endpoints []*WebhookEndpoint
	err := DB.Order("id").Find(&endpoints).Error
	return endpoints, err
}

func GetWebhookEndpoint(id int64) (*WebhookEndpoint, error) {
	var endpoint WebhookEndpoint
	if err := DB.Where("id = ?", id).First(&endpoint).Error; err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func CreateWebhookEndpoint(endpoint *WebhookEndpoint) error {
	return DB.Create(endpoint).Error
}

func UpdateWebhookEndpoint(endpoint *WebhookEndpoint) error {
	return DB.Model(&WebhookEndpoint{}).Where("id = ?", endpoint.Id).Updates(map[string]any{
		"name":       endpoint.Name,
		"url":        endpoint.URL,
		"secret":     endpoint.Secret,
		"enabled":    endpoint.Enabled,
		"events":     endpoint.Events,
		"updated_at": common.GetTimestamp(),
	}).Error
}

func DeleteWebhookEndpoint(id int64) error {
	return DB.Where("id = ?", id).Delete(&WebhookEndpoint{}).Error
}
