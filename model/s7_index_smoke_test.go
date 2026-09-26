package model_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestS7IndexAutoMigrateSmoke(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.BannedIP{}, &model.TopUp{}))
	mig := db.Migrator()
	require.True(t, mig.HasIndex(&model.BannedIP{}, "ExpiresAt"), "banned_ips should index ExpiresAt")
	require.True(t, mig.HasIndex(&model.TopUp{}, "CreateTime"), "top_ups should index CreateTime")
}
