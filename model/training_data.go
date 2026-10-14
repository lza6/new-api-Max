package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	// PrefixFingerprint 已上传**整个前缀**的累积指纹（不是只有末条）。
	//
	// [修复防御] 原先只存「末条指纹」，导致**中间**消息被编辑时校验通过，
	// 被改过的内容**永久不再上传**——采集不可逆，等于静默数据丢失。
	// 改为前缀指纹后，历史中任意位置的改动都会让指纹不匹配
	// → 保守全量重传（见 SelectIncrement）。
	PrefixFingerprint string `json:"prefix_fingerprint" gorm:"type:varchar(64)"`

	// RequestTextBytes 已上传的**拼接文本字节数**。
	//
	// 网关的请求路径只能拿到 `TokenCountMeta.CombineText`（一整块拼接文本），
	// 因此每次请求只产生 1 条消息 —— 消息级水位恒为 1，**无法表达增量**。
	// 实测确认这一缺口后新增本列：改为按**文本前缀**做增量，
	// 长期会话只上传追加的那一段（用户点名的核心需求）。
	// 见 `service/training_capture/text_increment.go`。
	RequestTextBytes int `json:"request_text_bytes"`

	// RequestTextHash 已上传前缀的指纹（校验前缀未被改动）。
	//
	// 只比长度不够：同样长度的不同内容会被误判为「未改动」，
	// 从而把被编辑掉的部分永久丢弃。必须比内容指纹。
	RequestTextHash string `json:"request_text_hash" gorm:"type:varchar(64)"`

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &consent, nil
}

// UpsertTrainingDataConsent 写入/更新授权状态（同意或撤回）。
//
// 用 GORM 的 `clause.OnConflict` 做**原子 upsert**，而不是「先查后写」：
// 后者在并发下（用户双击开关 / 两个标签页同时操作）两个请求都读到 nil、
// 都尝试 Create，其中一个撞唯一索引 `idx_training_consent_user` 并把
// 「唯一约束冲突」这种内部错误直接甩给用户。
//
// 关于三库兼容：`clause.OnConflict{DoUpdates: ...}` 由 GORM 按方言生成 SQL——
// MySQL 走 `ON DUPLICATE KEY UPDATE col=VALUES(col)`（**5.7 支持**），
// PostgreSQL/SQLite 走 `ON CONFLICT (col) DO UPDATE SET`。
// 刻意**不用** GORM 的 `ON CONFLICT ... AS new` 语法（那是 MySQL 8.0.19+ /
// PG 15+ 才有的别名形式，违反宪法 IV 对 MySQL 5.7.8 的兼容要求）。
func UpsertTrainingDataConsent(userId int, granted bool, policyVersion string, now int64) error {
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

	// 冲突时更新：授权状态、声明版本、以及对应的时间戳。
	columns := []string{"granted", "policy_version", "updated_at"}
	if granted {
		columns = append(columns, "granted_at")
	} else {
		columns = append(columns, "revoked_at")
	}

	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&record).Error
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &wm, nil
}

// SaveTrainingDataWatermark 写入/更新水位（**只在采集 worker 里调用**，非热路径）。
func SaveTrainingDataWatermark(userId int, conversationId string, uploadedCount int, prefixFingerprint string, requestTextBytes int, requestTextHash string, now int64) error {
	// 同样用原子 upsert：省掉一次「先查」，并让它在（未来的）多 worker
	// 场景下也正确。当前 worker 是单 goroutine，但正确的写法不更贵。
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"}, {Name: "conversation_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"uploaded_count", "prefix_fingerprint",
			"request_text_bytes", "request_text_hash",
			"last_uploaded_at", "updated_at",
		}),
	}).Create(&TrainingDataWatermark{
		UserId:            userId,
		ConversationId:    conversationId,
		UploadedCount:     uploadedCount,
		PrefixFingerprint: prefixFingerprint,
		RequestTextBytes:  requestTextBytes,
		RequestTextHash:   requestTextHash,
		LastUploadedAt:    now,
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
