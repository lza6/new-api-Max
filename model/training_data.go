package model

import (
	"time"

	"gorm.io/gorm"
)

// 训练数据采集的两张表。
//
// ## 为什么是两张新表而不是复用现有的
//
// 现有表都不能承载这两件事：
//   - 授权是**用户级**的显式同意（带声明版本、同意/撤回时间），
//     `users` 表加列会让「授权历史」无法留痕（撤回会覆盖同意时间）；
//   - 去重水位是**会话级**的 O(1) 状态，`logs` 表是每请求一行且不含会话维度。
//
// 两张表都是**纯新增**，不修改任何既有表——回滚只需 DROP TABLE，
// 对既有数据零影响（见 spec 的风险表）。

// TrainingDataConsent 用户对「训练数据采集」的授权状态。
//
// 一个用户一行（`user_id` 唯一）。撤回**不删行**而是置 `granted=false`
// 并记 `revoked_at`——保留「曾经同意过」的事实，这对合规自证是必要的。
type TrainingDataConsent struct {
	ID uint `json:"id" gorm:"primaryKey"`

	// UserId 用户 ID。唯一索引：一个用户只能有一条授权记录。
	UserId int `json:"user_id" gorm:"uniqueIndex:idx_training_consent_user"`

	// Granted 当前是否授权。撤回后置 false（**不删行**，保留留痕）。
	Granted bool `json:"granted"`

	// GrantedAt 最近一次**同意**的时间（unix 秒）。
	// 撤回后再同意会更新它——旧同意时间通过审计日志追溯（本表只保留最近一次）。
	GrantedAt int64 `json:"granted_at"`

	// RevokedAt 最近一次**撤回**的时间（unix 秒）。从未撤回为 0。
	RevokedAt int64 `json:"revoked_at"`

	// PolicyVersion 用户同意的是**哪一版**免责声明。
	//
	// 声明内容实质性变更时必须递增版本号，使旧同意**失效**并重新征求同意。
	// 没有这一列就无法区分「用户同意的是旧条款」还是「当前条款」——
	// 那是合规上的实质缺口，不是可选字段。
	PolicyVersion string `json:"policy_version" gorm:"type:varchar(32)"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名（避免 GORM 复数化规则在不同版本间的差异）。
func (TrainingDataConsent) TableName() string {
	return "training_data_consents"
}

// TrainingDataWatermark 会话级采集水位（增量去重状态）。
//
// 见 `service/training_capture/dedup.go` 的设计说明：
// 记录「该会话已上传多少条消息」+「末次消息指纹」，
// 使长期会话只上传**新增轮次**，而不是整段重传。
//
// `(user_id, conversation_id)` 唯一：同一用户的同一会话只有一条水位。
// 不同用户即使 conversationId 相同（客户端可能用同一个字符串）也互不影响。
type TrainingDataWatermark struct {
	ID uint `json:"id" gorm:"primaryKey"`

	UserId         int    `json:"user_id" gorm:"uniqueIndex:idx_training_watermark_user_conv,priority:1"`
	ConversationId string `json:"conversation_id" gorm:"type:varchar(128);uniqueIndex:idx_training_watermark_user_conv,priority:2"`

	// UploadedCount 已上传的消息条数（下次从该下标继续）。
	UploadedCount int `json:"uploaded_count"`

	// LastFingerprint 已上传的最后一条消息指纹，用于校验水位自洽。
	// 历史被改写时指纹不匹配 → 采集侧保守全量重传（见 SelectIncrement）。
	LastFingerprint string `json:"last_fingerprint" gorm:"type:varchar(64)"`

	// LastUploadedAt 最近一次成功上传的时间（unix 秒）。
	LastUploadedAt int64 `json:"last_uploaded_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (TrainingDataWatermark) TableName() string {
	return "training_data_watermarks"
}

// ---------- 访问方法 ----------

// GetTrainingDataConsent 读取用户的授权记录。无记录返回 (nil, nil)。
func GetTrainingDataConsent(userId int) (*TrainingDataConsent, error) {
	var consent TrainingDataConsent
	err := DB.Where("user_id = ?", userId).First(&consent).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &consent, nil
}

// UpsertTrainingDataConsent 写入/更新授权状态（同意或撤回）。
//
// 用「先查后写」而不是 ON CONFLICT：`ON CONFLICT ... AS new` 需要
// PostgreSQL 15+ / MySQL 8.0.19+，而本项目要兼容 MySQL 5.7.8+（宪法 IV）。
// 本方法只在用户点开关时调用（**非热路径**），先查后写的代价可忽略。
func UpsertTrainingDataConsent(userId int, granted bool, policyVersion string, now int64) error {
	consent, err := GetTrainingDataConsent(userId)
	if err != nil {
		return err
	}
	if consent == nil {
		record := TrainingDataConsent{
			UserId:        userId,
			Granted:       granted,
			PolicyVersion: policyVersion,
		}
		if granted {
			record.GrantedAt = now
		} else {
			record.RevokedAt = now
		}
		return DB.Create(&record).Error
	}

	updates := map[string]any{
		"granted":        granted,
		"policy_version": policyVersion,
	}
	if granted {
		updates["granted_at"] = now
	} else {
		updates["revoked_at"] = now
	}
	return DB.Model(&TrainingDataConsent{}).
		Where("user_id = ?", userId).
		Updates(updates).Error
}

// ListGrantedTrainingDataUserIds 返回所有**当前授权且声明版本匹配**的用户 ID。
//
// 声明版本校验是必须的：声明实质变更后，同意旧版本的用户**不应**继续被采集
// （他们同意的是旧条款）。版本不匹配时视为未授权，直到重新同意。
func ListGrantedTrainingDataUserIds(policyVersion string) ([]int, error) {
	var ids []int
	err := DB.Model(&TrainingDataConsent{}).
		Where("granted = ? AND policy_version = ?", true, policyVersion).
		Pluck("user_id", &ids).Error
	return ids, err
}

// GetTrainingDataWatermark 读取某用户某会话的水位。无记录返回 (nil, nil)。
func GetTrainingDataWatermark(userId int, conversationId string) (*TrainingDataWatermark, error) {
	var wm TrainingDataWatermark
	err := DB.Where("user_id = ? AND conversation_id = ?", userId, conversationId).
		First(&wm).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &wm, nil
}

// SaveTrainingDataWatermark 写入/更新水位（**只在采集 worker 里调用**，非热路径）。
func SaveTrainingDataWatermark(userId int, conversationId string, uploadedCount int, lastFingerprint string, now int64) error {
	wm, err := GetTrainingDataWatermark(userId, conversationId)
	if err != nil {
		return err
	}
	if wm == nil {
		return DB.Create(&TrainingDataWatermark{
			UserId:          userId,
			ConversationId:  conversationId,
			UploadedCount:   uploadedCount,
			LastFingerprint: lastFingerprint,
			LastUploadedAt:  now,
		}).Error
	}
	return DB.Model(&TrainingDataWatermark{}).
		Where("user_id = ? AND conversation_id = ?", userId, conversationId).
		Updates(map[string]any{
			"uploaded_count":   uploadedCount,
			"last_fingerprint": lastFingerprint,
			"last_uploaded_at": now,
		}).Error
}

// DeleteStaleTrainingDataWatermarks 清理长期未更新的水位记录，防止表无界增长。
//
// 水位是「会话级」状态：会话结束/长期不再活跃后它的价值归零，但行会一直留着。
// 按 `last_uploaded_at` 早于 cutoff 删除即可（删了只会导致该会话下次重新全量采集，
// 不会丢数据）。保留窗口给得较宽（调用方决定），避免误删活跃会话。
func DeleteStaleTrainingDataWatermarks(cutoffUnix int64) (int64, error) {
	result := DB.Where("last_uploaded_at > 0 AND last_uploaded_at < ?", cutoffUnix).
		Delete(&TrainingDataWatermark{})
	return result.RowsAffected, result.Error
}
