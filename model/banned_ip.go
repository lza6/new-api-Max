package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// BannedIP 记录被封禁的客户端 IP（Web 防刷）。
// ExpiresAt 为 0 表示永久封禁。
type BannedIP struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	IP        string `json:"ip" gorm:"type:varchar(64);uniqueIndex;not null"`
	Reason    string `json:"reason" gorm:"type:varchar(256)"`
	BannedAt  int64  `json:"banned_at"`
	ExpiresAt int64  `json:"expires_at"`
	BannedBy  string `json:"banned_by" gorm:"type:varchar(64)"`
}

func (BannedIP) TableName() string {
	return "banned_ips"
}

// IsIPBanned 返回该 IP 是否在封禁期（含永久），并给出封禁到期时间。
func IsIPBanned(ip string) (int64, bool) {
	if ip == "" {
		return 0, false
	}
	var banned BannedIP
	if err := DB.Where("ip = ?", ip).First(&banned).Error; err != nil {
		return 0, false
	}
	if banned.ExpiresAt == 0 || banned.ExpiresAt > time.Now().Unix() {
		return banned.ExpiresAt, true
	}
	return 0, false
}

// BanIP 封禁一个 IP。minutes <= 0 表示永久封禁；已存在则覆盖更新。
func BanIP(ip, reason, bannedBy string, minutes int64) error {
	if ip == "" {
		return errors.New("ban ip is empty")
	}
	var expiresAt int64
	if minutes > 0 {
		expiresAt = time.Now().Add(time.Duration(minutes) * time.Minute).Unix()
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing BannedIP
		err := tx.Where("ip = ?", ip).First(&existing).Error
		if err == nil {
			return tx.Model(&existing).Updates(map[string]any{
				"reason":     reason,
				"banned_at":  time.Now().Unix(),
				"expires_at": expiresAt,
				"banned_by":  bannedBy,
			}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&BannedIP{
			IP:        ip,
			Reason:    reason,
			BannedAt:  time.Now().Unix(),
			ExpiresAt: expiresAt,
			BannedBy:  bannedBy,
		}).Error
	})
}

// UnbanIP 解封一个 IP。
func UnbanIP(ip string) error {
	if ip == "" {
		return errors.New("unban ip is empty")
	}
	return DB.Where("ip = ?", ip).Delete(&BannedIP{}).Error
}

// UnbanIPs 批量解封多个 IP（B1-3）。空列表直接返回 nil；忽略不存在/已解封的 IP。
func UnbanIPs(ips []string) error {
	cleaned := make([]string, 0, len(ips))
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			cleaned = append(cleaned, ip)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return DB.Where("ip IN ?", cleaned).Delete(&BannedIP{}).Error
}

// CountActiveBannedIPs 返回当前生效封禁数（含永久；过期未清理的按 is-active 语义排除）。
func CountActiveBannedIPs(now int64) (int64, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	var count int64
	err := DB.Model(&BannedIP{}).
		Where("expires_at = 0 OR expires_at > ?", now).
		Count(&count).Error
	return count, err
}

// DeleteBannedIP 按 ID 删除封禁记录。
func DeleteBannedIP(id uint) error {
	return DB.Delete(&BannedIP{}, id).Error
}

// ListBannedIPs 分页返回封禁列表（新的在前）。
// DeleteExpiredBannedIPs 删除已过期的封禁记录（幂等）。返回删除条数。
func DeleteExpiredBannedIPs(now int64) (int64, error) {
	if now <= 0 {
		return 0, nil
	}
	result := DB.Where("expires_at > 0 AND expires_at <= ?", now).Delete(&BannedIP{})
	return result.RowsAffected, result.Error
}

func ListBannedIPs(page, size int) ([]BannedIP, int64, error) {
	var items []BannedIP
	var total int64
	if err := DB.Model(&BannedIP{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	if err := DB.Order("banned_at desc").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
