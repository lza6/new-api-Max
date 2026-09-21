package model

import (
	"github.com/lza6/new-api-Max/common"
	"gorm.io/gorm"
)

// GetDBTimestamp returns a UNIX timestamp from database time.
// Falls back to application time on error.
func GetDBTimestamp() int64 {
	return getDBTimestampTx(nil)
}

// getDBTimestampTx 与 GetDBTimestamp 同语义，但使用给定 tx 执行：
// 供 *Tx 后缀函数在事务内取 DB 时间，避免用全局 DB 在连接池=1 时与当前
// 事务争用唯一连接而死锁（并保证事务内时间与事务快照一致）。
func getDBTimestampTx(tx *gorm.DB) int64 {
	var ts int64
	var err error
	query := DB
	if tx != nil {
		query = tx
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		err = query.Raw("SELECT EXTRACT(EPOCH FROM NOW())::bigint").Scan(&ts).Error
	case common.UsingMainDatabase(common.DatabaseTypeSQLite):
		err = query.Raw("SELECT strftime('%s','now')").Scan(&ts).Error
	default:
		err = query.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error
	}
	if err != nil || ts <= 0 {
		return common.GetTimestamp()
	}
	return ts
}
