package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOpenAIOpsObjectOwnership(t *testing.T) {
	previous := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&OpenAIOpsObject{}))
	DB = db
	t.Cleanup(func() { DB = previous })

	ok, err := RecordOpenAIOpsObject(1, "file", "file-1")
	require.NoError(t, err)
	assert.True(t, ok)

	owned, err := OpenAIOpsObjectOwned(1, "file", "file-1")
	require.NoError(t, err)
	assert.True(t, owned)

	owned, err = OpenAIOpsObjectOwned(2, "file", "file-1")
	require.NoError(t, err)
	assert.False(t, owned, "其他用户不得访问")

	// 第二个用户尝试登记同一上游对象：不得抢占。
	ok, err = RecordOpenAIOpsObject(2, "file", "file-1")
	require.NoError(t, err)
	assert.False(t, ok)

	ids, err := OpenAIOpsObjectIDs(1, "file")
	require.NoError(t, err)
	assert.Contains(t, ids, "file-1")

	require.NoError(t, DeleteOpenAIOpsObject(1, "file", "file-1"))
	owned, err = OpenAIOpsObjectOwned(1, "file", "file-1")
	require.NoError(t, err)
	assert.False(t, owned)
}
