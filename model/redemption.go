package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	// MaxUses 兑换码可被兑换的最大次数（0=未设置，按一次性处理；>0 时
	// 同一码可被多个用户各兑换一次，剩余次数见 RemainingUses）。
	MaxUses int `json:"max_uses" gorm:"default:0"`
	// RemainingUses 剩余可兑换次数（MaxUses=0 时忽略；每次成功兑换 -1，
	// 归零后该码不再可兑换）。非持久化计算字段，随列表查询填充。
	RemainingUses int `json:"remaining_uses" gorm:"-:all"`
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, status string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Redemption{})

	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
		} else {
			query = query.Where("name LIKE ?", keyword+"%")
		}
	}

	if status != "" {
		now := common.GetTimestamp()
		switch status {
		case "expired":
			query = query.Where(
				"status = ? AND expired_time != 0 AND expired_time < ?",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusEnabled):
			query = query.Where(
				"status = ? AND (expired_time = 0 OR expired_time >= ?)",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusDisabled):
			query = query.Where("status = ?", common.RedemptionCodeStatusDisabled)
		case strconv.Itoa(common.RedemptionCodeStatusUsed):
			query = query.Where("status = ?", common.RedemptionCodeStatusUsed)
		}
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return 0, errors.New("无效的 user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}

		// 一次性码（MaxUses=0）：CAS 从 enabled 直接翻到 used（保持现状，防并发）。
		// 可多次码（MaxUses>0）：CAS 条件改为 remaining>0，成功则剩余次数 -1；
		//   归零后该码不可再兑（区别于一次性的 Status=used）。
		if redemption.MaxUses <= 0 {
			result := tx.Model(&Redemption{}).
				Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).
				Updates(map[string]any{
					"redeemed_time": common.GetTimestamp(),
					"status":        common.RedemptionCodeStatusUsed,
					"used_user_id":  userId,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errors.New("该兑换码已被使用")
			}
		} else {
			// 多用户可多次兑换：按使用记录数求剩余次数。
			used := int64(0)
			if err := tx.Model(&RedemptionUsage{}).
				Where("redemption_id = ?", redemption.Id).
				Count(&used).Error; err != nil {
				return err
			}
			remaining := int64(redemption.MaxUses) - used
			if remaining <= 0 {
				return errors.New("该兑换码已被使用")
			}
			result := tx.Model(&Redemption{}).
				Where("id = ? AND status = ? AND (max_uses - (SELECT COUNT(*) FROM redemption_usages WHERE redemption_id = ?)) > 0",
					redemption.Id, common.RedemptionCodeStatusEnabled, redemption.Id).
				Updates(map[string]any{
					"redeemed_time": common.GetTimestamp(),
					"used_user_id":  userId,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errors.New("该兑换码已被使用")
			}
			// 记录使用明细（同用户重复兑换仍计入次数）。
			usage := RedemptionUsage{
				RedemptionId: redemption.Id,
				UserId:       userId,
				Quota:        redemption.Quota,
				CreatedTime:  common.GetTimestamp(),
			}
			if err := tx.Create(&usage).Error; err != nil {
				return err
			}
			// 归零时仅标记「已用尽」（保持 status=disabled 语义，区别于一次性 used）。
			if remaining == 1 {
				if err := tx.Model(&Redemption{}).
					Where("id = ?", redemption.Id).
					Update("status", common.RedemptionCodeStatusDisabled).Error; err != nil {
					return err
				}
			}
		}
		return creditTopUpQuota(tx, userId, redemption.Quota, nil)
	})
	if err != nil {
		common.SysError("redemption failed: " + err.Error())
		return 0, ErrRedeemFailed
	}
	syncCreditUserQuotaCache(userId, redemption.Quota, "redemption")
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}

// BatchDeleteRedemptions soft-deletes the selected codes in one statement.
func BatchDeleteRedemptions(ids []int) (int64, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return 0, errors.New("select between 1 and 1000 redemption codes")
	}
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("redemption IDs must be positive")
		}
	}
	result := DB.Where("id IN ?", ids).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
