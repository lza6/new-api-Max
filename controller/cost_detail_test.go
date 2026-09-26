/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"
)

func setupCostDetailDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
	})
}

func seedConsumeLog(t *testing.T, userID int, other string) *model.Log {
	t.Helper()
	log := &model.Log{
		UserId:           userID,
		Type:             model.LogTypeConsume,
		ModelName:        "gpt-4o-mini",
		Quota:            123,
		PromptTokens:     1000,
		CompletionTokens: 100,
		Other:            other,
	}
	require.NoError(t, model.LOG_DB.Create(log).Error)
	return log
}

func TestParseLogCostDetail(t *testing.T) {
	setupCostDetailDB(t)
	log := seedConsumeLog(t, 7, `{"model_ratio":5,"group_ratio":0.1,"completion_ratio":2,"cache_ratio":0.5,"api_equivalent_usd":210,"explain":{"facts":[{"label":"tier_matched","value":"0-4k"}]}}`)
	detail := parseLogCostDetail(log)
	assert.Equal(t, int64(123), int64(detail.Quota), "quota 必须保持原值（B5-2 不改计费）")
	assert.Equal(t, 5.0, detail.ModelRatio)
	assert.Equal(t, 0.1, detail.GroupRatio)
	assert.Equal(t, 2.0, detail.CompletionRatio)
	assert.Equal(t, 0.5, detail.CacheRatio)
	assert.Equal(t, "0-4k", detail.TierMatched)
	assert.Equal(t, int64(210), detail.ApiEquivalentUsd)
	assert.True(t, detail.ShadowKnown)
}

func TestParseLogCostDetailNoOther(t *testing.T) {
	setupCostDetailDB(t)
	log := seedConsumeLog(t, 7, "")
	detail := parseLogCostDetail(log)
	assert.Equal(t, int64(123), int64(detail.Quota))
	assert.Equal(t, 0.0, detail.ModelRatio)
	assert.False(t, detail.ShadowKnown)
}

func TestGetConsumeLogByIDOwnership(t *testing.T) {
	setupCostDetailDB(t)
	log := seedConsumeLog(t, 7, `{"api_equivalent_usd":1}`)

	// 本人可见
	got, exists, err := model.GetConsumeLogByID(int64(log.Id), 7, false)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, log.Id, got.Id)

	// 越权（其他用户）不可见
	_, exists2, err2 := model.GetConsumeLogByID(int64(log.Id), 8, false)
	require.NoError(t, err2)
	assert.False(t, exists2)

	// 管理员可见任意
	got3, exists3, err3 := model.GetConsumeLogByID(int64(log.Id), 8, true)
	require.NoError(t, err3)
	require.True(t, exists3)
	assert.Equal(t, log.Id, got3.Id)
}
