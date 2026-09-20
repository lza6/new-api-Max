package model

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenValidateDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB, prevLogDB := DB, LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	initCol()
	t.Cleanup(func() { DB, LOG_DB = prevDB, prevLogDB })
	return db
}

// 修复：0 额度且非无限的 token 应区分“额度用尽”（ErrTokenQuotaExhausted），
// 而不是与“无效令牌”混为一谈（否则前端只会看到误导性的 401 Invalid token）。
func TestValidateUserTokenDistinguishesQuotaExhausted(t *testing.T) {
	setupTokenValidateDB(t)
	dead := &Token{
		UserId: 1, Name: "dead-quota",
		Key:    "sk-deadtoken0123456789012345678901234567890123456789",
		Status: common.TokenStatusEnabled, ExpiredTime: -1,
		RemainQuota: 0, UnlimitedQuota: false,
	}
	require.NoError(t, DB.Create(dead).Error)

	_, err := ValidateUserToken("sk-deadtoken0123456789012345678901234567890123456789")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTokenQuotaExhausted), "0-quota limited token should be ErrTokenQuotaExhausted, got %v", err)

	_, err = ValidateUserToken("sk-nonexistent01234567890123456789012345678901234567")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTokenInvalid), "unknown key should be ErrTokenInvalid, got %v", err)
}
