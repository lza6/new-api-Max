package model

import (
	"github.com/lza6/new-api-Max/common"
	"gorm.io/gorm"
)

// EventDelivery 事件投递幂等记录（P2-2 持久化）。
//
// event_id + handler 联合唯一：保证同一事件对同一处理器跨重启/跨实例只处理
// 一次，是「重复 webhook 推送只生效一次」的持久化底座。状态机与
// service.EventBus 对齐：pending -> success | failed(重试) -> dead。
type EventDelivery struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	EventID   string `json:"event_id" gorm:"type:varchar(128);not null;index:idx_event_delivery_event_handler,unique"`
	Handler   string `json:"handler" gorm:"type:varchar(128);not null;index:idx_event_delivery_event_handler,unique"`
	EventType string `json:"event_type" gorm:"type:varchar(128);index"`
	State     string `json:"state" gorm:"type:varchar(32);index"`
	Attempts  int    `json:"attempts" gorm:"int"`
	LastError string `json:"last_error" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint;index"`
}

func (d *EventDelivery) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if d.CreatedAt == 0 {
		d.CreatedAt = now
	}
	if d.UpdatedAt == 0 {
		d.UpdatedAt = now
	}
	return nil
}
