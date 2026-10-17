package model

import (
	"errors"

	"gorm.io/gorm"
)

var ErrActionApprovalNotPending = errors.New("approval is not pending")

const (
	ActionApprovalPending  = "pending"
	ActionApprovalApproved = "approved"
	ActionApprovalDenied   = "denied"
	ActionApprovalConsumed = "consumed"
	ActionApprovalExpired  = "expired"
)

// CreateActionApproval 创建一条待审批记录。
func CreateActionApproval(approval *ActionApproval) error {
	if approval == nil {
		return errors.New("nil approval")
	}
	return DB.Create(approval).Error
}

// GetActionApproval 按 id 读取审批记录。
func GetActionApproval(id int64) (*ActionApproval, error) {
	var approval ActionApproval
	if err := DB.Where("id = ?", id).First(&approval).Error; err != nil {
		return nil, err
	}
	return &approval, nil
}

// ListActionApprovals 分页列出审批记录（status 为空 = 全部）。
func ListActionApprovals(status string, offset, limit int) ([]ActionApproval, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := DB.Model(&ActionApproval{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ActionApproval
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ApproveActionApproval 把 pending 记录置为 approved（CAS，重复审批无副作用）。
func ApproveActionApproval(id int64, adminId int, expiresAt int64, reason string) error {
	result := DB.Model(&ActionApproval{}).
		Where("id = ? AND status = ?", id, ActionApprovalPending).
		Updates(map[string]any{
			"status":     ActionApprovalApproved,
			"admin_id":   adminId,
			"expires_at": expiresAt,
			"reason":     reason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrActionApprovalNotPending
	}
	return nil
}

// DenyActionApproval 把 pending 记录置为 denied（CAS）。
func DenyActionApproval(id int64, adminId int, reason string) error {
	result := DB.Model(&ActionApproval{}).
		Where("id = ? AND status = ?", id, ActionApprovalPending).
		Updates(map[string]any{
			"status":   ActionApprovalDenied,
			"admin_id": adminId,
			"reason":   reason,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrActionApprovalNotPending
	}
	return nil
}

// ConsumeApprovedActionApproval 原子消费一条匹配的已批准记录。
//
// 单次使用：同一审批不能授权第二次付费动作；CAS 更新保证并发下只有一次成功。
// 返回 (true, nil) 表示本次动作已获授权；找不到可用审批返回 (false, nil)。
func ConsumeApprovedActionApproval(userId int, action, resource string, now int64) (bool, error) {
	// 并发下 CAS 可能失败；继续尝试下一条已批准记录，避免「明明批了却说需要审批」。
	lastID := int64(0)
	for attempt := 0; attempt < 5; attempt++ {
		var approval ActionApproval
		err := DB.Where("user_id = ? AND action = ? AND resource = ? AND status = ? AND expires_at > ? AND id > ?",
			userId, action, resource, ActionApprovalApproved, now, lastID).
			Order("id asc").First(&approval).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		result := DB.Model(&ActionApproval{}).
			Where("id = ? AND status = ?", approval.Id, ActionApprovalApproved).
			Updates(map[string]any{
				"status":      ActionApprovalConsumed,
				"consumed_at": now,
			})
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected == 1 {
			return true, nil
		}
		lastID = approval.Id
	}
	return false, nil
}

// HasPendingActionApproval 报告是否已有同动作/资源的待审批记录（避免重复刷单）。
func HasPendingActionApproval(userId int, action, resource string) (*ActionApproval, error) {
	var approval ActionApproval
	err := DB.Where("user_id = ? AND action = ? AND resource = ? AND status = ?",
		userId, action, resource, ActionApprovalPending).
		Order("id desc").First(&approval).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &approval, nil
}

// CleanupExpiredActionApprovals 把过期未处理记录标记为 expired（有界批量）。
func CleanupExpiredActionApprovals(now int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	result := DB.Model(&ActionApproval{}).
		Where("status IN ? AND expires_at > 0 AND expires_at <= ?",
			[]string{ActionApprovalPending, ActionApprovalApproved}, now).
		Limit(limit).
		Update("status", ActionApprovalExpired)
	return result.RowsAffected, result.Error
}
