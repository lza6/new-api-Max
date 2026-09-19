package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConsumeLogFlushDrainsQueue(t *testing.T) {
	origDB, origLogDB := DB, LOG_DB
	origFlush, origConsume, origExport := common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	defer func() {
		DB, LOG_DB = origDB, origLogDB
		common.LogFlushEnabled, common.LogConsumeEnabled, common.DataExportEnabled = origFlush, origConsume, origExport
	}()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB, LOG_DB = db, db
	common.LogFlushEnabled = true
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false

	for i := 0; i < 3; i++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		c.Set("username", "u")
		RecordConsumeLog(c, 1, RecordConsumeLogParams{ModelName: "m", Other: NewLogOther()})
	}
	FlushConsumeLogs()

	var n int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&n).Error)
	require.Equal(t, int64(3), n)
}
