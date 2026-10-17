package model

import "gorm.io/gorm"

// ActionApproval 是「动作级 deny-by-default」的审批记录（G5-3 / MCP tools/call）。
//
// 语义：
//   - 未登记的动作一律拒绝；只读动作由 service 层的静态白名单放行。
//   - 付费/有副作用的动作必须先有一条 approved 且未过期的记录，**一次消费一条**。
//   - pending 由用户发起、管理员审批；过期由查询条件懒清理，无需后台扫描。
type ActionApproval struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	UserId     int    `json:"user_id" gorm:"index"`
	Action     string `json:"action" gorm:"type:varchar(64);not null;index"`
	Resource   string `json:"resource" gorm:"type:varchar(191);not null"`
	Status     string `json:"status" gorm:"type:varchar(20);not null;index"` // pending|approved|denied|consumed|expired
	Reason     string `json:"reason" gorm:"type:text"`
	AdminId    int    `json:"admin_id"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
	ExpiresAt  int64  `json:"expires_at" gorm:"bigint;index"`
	ConsumedAt int64  `json:"consumed_at" gorm:"bigint"`
}

func (a *ActionApproval) BeforeCreate(tx *gorm.DB) error {
	now := getDBTimestampTx(tx)
	a.CreatedAt = now
	a.UpdatedAt = now
	return nil
}

func (a *ActionApproval) BeforeUpdate(tx *gorm.DB) error {
	a.UpdatedAt = getDBTimestampTx(tx)
	return nil
}
