package model

import "gorm.io/gorm"

// McpSession 是 MCP Streamable HTTP 的会话记录（G11 / MCP Gateway）。
//
// 为什么落库而不是进程内 map：蓝绿部署期间会短暂存在两个实例，进程内会话
// 会让滚动更新打断进行中的 MCP 会话。DB 会话天然跨实例；过期记录由
// CleanupExpiredMcpSessions 有界清理。
type McpSession struct {
	Id              string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserId          int    `json:"user_id" gorm:"index"`
	TokenId         int    `json:"token_id" gorm:"index"`
	ProtocolVersion string `json:"protocol_version" gorm:"type:varchar(32)"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint"`
	LastSeenAt      int64  `json:"last_seen_at" gorm:"bigint;index"`
	ExpiresAt       int64  `json:"expires_at" gorm:"bigint;index"`
	ClosedAt        int64  `json:"closed_at" gorm:"bigint"`
}

func (s *McpSession) BeforeCreate(tx *gorm.DB) error {
	if s.CreatedAt == 0 {
		s.CreatedAt = getDBTimestampTx(tx)
	}
	return nil
}
