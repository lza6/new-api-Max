package model

import (
	"time"

	"gorm.io/gorm"
)

// RedemptionUsage 兑换码使用明细（可多次码每次兑换一条）。
type RedemptionUsage struct {
	Id           int            `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	RedemptionId int            `json:"redemption_id" gorm:"index"`
	UserId       int            `json:"user_id" gorm:"index"`
	Quota        int            `json:"quota"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (RedemptionUsage) TableName() string {
	return "redemption_usages"
}

// RecordRedemptionUsage 记录一次使用明细（可多次码）。
func RecordRedemptionUsage(redemptionId, userId, quota int) error {
	usage := RedemptionUsage{
		RedemptionId: redemptionId,
		UserId:       userId,
		Quota:        quota,
		CreatedTime:  time.Now().Unix(),
	}
	return DB.Create(&usage).Error
}

// CountRedemptionUsage 返回某码已使用次数。
func CountRedemptionUsage(redemptionId int) (int64, error) {
	var count int64
	err := DB.Model(&RedemptionUsage{}).
		Where("redemption_id = ?", redemptionId).
		Count(&count).Error
	return count, err
}

// SumRedemptionUsageByUser 返回某用户通过该码已兑换的次数（防止单用户刷）。
func SumRedemptionUsageByUser(redemptionId, userId int) (int64, error) {
	var count int64
	err := DB.Model(&RedemptionUsage{}).
		Where("redemption_id = ? AND user_id = ?", redemptionId, userId).
		Count(&count).Error
	return count, err
}

// WithRedemptionUsageExpiry 在清理过期/作废码时一并软删其使用明细（级联）。
func DeleteRedemptionUsagesByRedemptionIds(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Where("redemption_id IN ?", ids).Delete(&RedemptionUsage{}).Error
}
