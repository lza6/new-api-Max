package model

import (
	"errors"

	"gorm.io/gorm"
)

// CreateMcpSession 创建会话。
func CreateMcpSession(session *McpSession) error {
	if session == nil || session.Id == "" {
		return errors.New("mcp session id is required")
	}
	return DB.Create(session).Error
}

// GetMcpSession 读取未关闭且未过期的会话。
func GetMcpSession(id string, now int64) (*McpSession, error) {
	if id == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var session McpSession
	err := DB.Where("id = ? AND closed_at = 0 AND expires_at > ?", id, now).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// TouchMcpSession 刷新 last_seen（滑动过期由调用方按 TTL 更新 expires_at）。
func TouchMcpSession(id string, now, expiresAt int64) error {
	return DB.Model(&McpSession{}).
		Where("id = ? AND closed_at = 0", id).
		Updates(map[string]any{"last_seen_at": now, "expires_at": expiresAt}).Error
}

// CloseMcpSession 关闭会话（幂等）。
func CloseMcpSession(id string, now int64) error {
	return DB.Model(&McpSession{}).
		Where("id = ? AND closed_at = 0", id).
		Update("closed_at", now).Error
}

// CleanupExpiredMcpSessions 有界删除过期会话。
func CleanupExpiredMcpSessions(now int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	result := DB.Where("expires_at <= ? OR closed_at > 0 AND closed_at <= ?", now, now-3600).
		Limit(limit).Delete(&McpSession{})
	return result.RowsAffected, result.Error
}

// CountActiveMcpSessions 返回某用户当前未过期的会话数（用于限制会话膨胀）。
func CountActiveMcpSessions(userId int, now int64) (int64, error) {
	var count int64
	err := DB.Model(&McpSession{}).
		Where("user_id = ? AND closed_at = 0 AND expires_at > ?", userId, now).
		Count(&count).Error
	return count, err
}

// CloseOldestMcpSessions 关闭某用户最旧的若干活跃会话，返回关闭数量。
// 用于 initialize 时超过每用户会话上限的自动淘汰（而不是直接拒绝握手）。
func CloseOldestMcpSessions(userId int, keep int, now int64) (int64, error) {
	if userId <= 0 || keep < 0 {
		return 0, nil
	}
	var ids []string
	if err := DB.Model(&McpSession{}).
		Where("user_id = ? AND closed_at = 0 AND expires_at > ?", userId, now).
		Order("last_seen_at asc, id asc").
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) <= keep {
		return 0, nil
	}
	toClose := ids[:len(ids)-keep]
	result := DB.Model(&McpSession{}).Where("id IN ?", toClose).Update("closed_at", now)
	return result.RowsAffected, result.Error
}
