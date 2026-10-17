package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OpenAIOpsObject 记录「上游对象 id → 网关用户」的归属。
//
// 为什么需要：/v1/files、/v1/fine-tunes、/v1/batches 共用同一条渠道 key，
// 上游无法区分调用者。若不登记归属，拿到别人 file-/ft-/batch- id 的用户
// 就能下载/删除/取消别人的对象（IDOR）。
//
// 归属策略是**保守的**：只有通过本网关创建的对象会被登记；列表只返回
// 已登记到当前用户的对象。功能上线前已存在的上游对象不会自动认领。
type OpenAIOpsObject struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	UserId     int    `json:"user_id" gorm:"index"`
	Kind       string `json:"kind" gorm:"type:varchar(16);not null;index:idx_ops_object_kind_id,priority:1"`
	UpstreamId string `json:"upstream_id" gorm:"type:varchar(191);not null;index:idx_ops_object_kind_id,priority:2,unique"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
}

// RecordOpenAIOpsObject 登记对象归属；已存在（同一 kind+upstream_id）时不覆盖。
// 返回 true 表示归当前用户所有；false 表示该对象已属于其他用户。
func RecordOpenAIOpsObject(userId int, kind, upstreamId string) (bool, error) {
	if userId <= 0 || kind == "" || upstreamId == "" {
		return false, errors.New("invalid ops object")
	}
	row := &OpenAIOpsObject{UserId: userId, Kind: kind, UpstreamId: upstreamId, CreatedAt: GetDBTimestamp()}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	// 冲突：读取已有归属，判断是否同一用户。
	var existing OpenAIOpsObject
	if err := DB.Where("kind = ? AND upstream_id = ?", kind, upstreamId).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return existing.UserId == userId, nil
}

// OpenAIOpsObjectOwned 报告对象是否归属该用户。
func OpenAIOpsObjectOwned(userId int, kind, upstreamId string) (bool, error) {
	if userId <= 0 || kind == "" || upstreamId == "" {
		return false, nil
	}
	var count int64
	err := DB.Model(&OpenAIOpsObject{}).
		Where("user_id = ? AND kind = ? AND upstream_id = ?", userId, kind, upstreamId).
		Count(&count).Error
	return count > 0, err
}

// OpenAIOpsObjectIDs 返回某用户某类对象的全部 id（用于列表过滤）。
func OpenAIOpsObjectIDs(userId int, kind string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if userId <= 0 || kind == "" {
		return out, nil
	}
	var ids []string
	if err := DB.Model(&OpenAIOpsObject{}).
		Where("user_id = ? AND kind = ?", userId, kind).
		Pluck("upstream_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// DeleteOpenAIOpsObject 在对象被删除/取消后清理归属（幂等）。
func DeleteOpenAIOpsObject(userId int, kind, upstreamId string) error {
	return DB.Where("user_id = ? AND kind = ? AND upstream_id = ?", userId, kind, upstreamId).
		Delete(&OpenAIOpsObject{}).Error
}
