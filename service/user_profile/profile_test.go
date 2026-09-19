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
package user_profile

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

func setupLOGDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Channel{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
	})
}

func seedLogRow(t *testing.T, userID int, logType int, modelName string, channelID int, quota int, tokens int, createdAt int64) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:           userID,
		CreatedAt:        createdAt,
		Type:             logType,
		ModelName:        modelName,
		ChannelId:        channelID,
		Quota:            quota,
		PromptTokens:     tokens,
		CompletionTokens: tokens / 2,
		Content:          "seed",
	}).Error)
}

func TestEmptyUserProfile(t *testing.T) {
	setupLOGDB(t)
	p, err := GetUserProfile(999)
	require.NoError(t, err)
	assert.Equal(t, 999, p.UserID)
	assert.Zero(t, p.Overview30d.Requests)
	assert.Zero(t, p.Overview30d.Quota)
	assert.Empty(t, p.ModelUsage)
	assert.Len(t, p.Trend, 30)
	assert.Len(t, p.TimeHeatmap, 24)
	require.Len(t, p.Suggestions, 1)
	assert.Equal(t, "no_recent_activity", p.Suggestions[0].Code)
}

func TestUserProfileAggregates(t *testing.T) {
	setupLOGDB(t)
	now := time.Now().Unix()
	userID := 7
	// young usage (today) and a couple of error rows on channel 20
	seedLogRow(t, userID, model.LogTypeConsume, "deepseek-v4-flash", 20, 1000, 100, now-3600)
	seedLogRow(t, userID, model.LogTypeConsume, "gpt-4o-mini", 4, 200, 40, now-2*3600)
	seedLogRow(t, userID, model.LogTypeError, "deepseek-v4-flash", 20, 0, 0, now-1800)
	seedLogRow(t, userID, model.LogTypeError, "deepseek-v4-flash", 20, 0, 0, now-900)
	seedLogRow(t, userID, model.LogTypeError, "deepseek-v4-flash", 20, 0, 0, now-300)

	p, err := GetUserProfile(userID)
	require.NoError(t, err)
	// overview
	assert.Equal(t, int64(2), p.Overview30d.Requests)
	assert.Equal(t, int64(1200), p.Overview30d.Quota)
	assert.Equal(t, int64(3), p.Overview30d.Errors)
	assert.InDelta(t, 0.6, p.Overview30d.ErrorRate, 1e-9)
	assert.GreaterOrEqual(t, p.Overview7d.Requests, int64(2))
	// model usage: deepseek first, share > ? (decayed but both recent -> share = quota share)
	require.Len(t, p.ModelUsage, 2)
	assert.Equal(t, "deepseek-v4-flash", p.ModelUsage[0].Model)
	assert.InDelta(t, 1000.0/1200.0, p.ModelUsage[0].Share, 0.05)
	assert.InDelta(t, 3.0, p.ModelUsage[0].Errors, 0.01)
	// heatmap: 24 buckets, today hours sum ~ decayed requests > 0
	totalHeat := 0.0
	for _, h := range p.TimeHeatmap {
		totalHeat += h.Requests
	}
	assert.InDelta(t, 2.0, totalHeat, 0.01)
	// channels
	require.Len(t, p.Channels, 2)
	assert.Equal(t, 20, p.Channels[0].ChannelID)
	assert.Equal(t, "#20", p.Channels[0].ChannelName) // no channel row in main DB -> fallback name
	assert.Equal(t, int64(3), p.Channels[0].Errors)
	// trend 30 entries
	require.Len(t, p.Trend, 30)
	lastDay := p.Trend[29].Date
	assert.NotEmpty(t, lastDay)
	// suggestions: error rate on #20 is 0.5 with 2 errors -> high_error_channel
	codes := make([]string, 0, len(p.Suggestions))
	for _, s := range p.Suggestions {
		codes = append(codes, s.Code)
	}
	assert.Contains(t, codes, "high_error_channel")
}

func TestWeibullDecaysOldUsage(t *testing.T) {
	setupLOGDB(t)
	now := time.Now().Unix()
	userID := 8
	seedLogRow(t, userID, model.LogTypeConsume, "model-a", 20, 1000, 100, now-86400)    // 1 day ago
	seedLogRow(t, userID, model.LogTypeConsume, "model-a", 20, 1000, 100, now-29*86400) // 29 days ago

	oldW := weibullWeight(float64(now - (now - 29*86400)))
	recentW := weibullWeight(float64(now - (now - 86400)))
	assert.Greater(t, recentW, oldW, "recent row must weigh more than the old row")
	assert.Less(t, oldW, 0.5, "29-day-old row should have decayed noticeably")

	p, err := GetUserProfile(userID)
	require.NoError(t, err)
	require.Len(t, p.ModelUsage, 1)
	// decayed quota must be less than the raw 2000 and reflect the weighted mix
	assert.InDelta(t, 1000*recentW+1000*oldW, p.ModelUsage[0].Quota, 1e-6)
	// raw overview stays factual
	assert.Equal(t, int64(2000), p.Overview30d.Quota)
}

func TestProfileCacheHitAndInvalidate(t *testing.T) {
	setupLOGDB(t)
	now := time.Now().Unix()
	userID := 9
	seedLogRow(t, userID, model.LogTypeConsume, "model-c", 20, 500, 50, now-600)

	p1, err := GetUserProfile(userID)
	require.NoError(t, err)
	generated1 := p1.GeneratedAt
	// second call within TTL returns the cached copy (no rebuild -> same timestamp)
	p2, err := GetUserProfile(userID)
	require.NoError(t, err)
	assert.Equal(t, generated1, p2.GeneratedAt)

	// add a log row, invalidate, rebuild -> timestamp changes and quota updates
	seedLogRow(t, userID, model.LogTypeConsume, "model-c", 20, 300, 30, now-300)
	InvalidateProfileCache(userID)
	p3, err := GetUserProfile(userID)
	require.NoError(t, err)
	assert.Equal(t, int64(800), p3.Overview30d.Quota, "rebuild must observe the newly added row")
}
