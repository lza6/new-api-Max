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
// Package user_profile 提供规则版（零 LLM 成本）用户画像：
// 基于 consume/error 日志聚合模型偏好、成本习惯、活跃时段、错误率与
// 渠道亲和，带 Weibull 远期衰减与 Redis/进程内双层缓存。
//
// 读路径只查日志库与缓存，不写任何业务数据。
package user_profile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// 衰减与窗口参数：30 天特征寿命、形状 k=1.5 的 Weibull 分布，
// 使近期行为权重显著高于远期（OpenLore 风格：证据随时间衰减）。
const (
	decayLambdaSeconds = 30 * 24 * 3600
	decayK             = 1.5
	window7dSeconds    = 7 * 24 * 3600
	window30dSeconds   = 30 * 24 * 3600
	cacheTTL           = 60 * time.Second
	profileRedisTTL    = 60 * time.Second
	maxModelUsage      = 8
)

// TimeWindow 一个时间窗口的原始事实汇总（不衰减）。
type TimeWindow struct {
	Requests  int64   `json:"requests"`
	Tokens    int64   `json:"tokens"`
	Quota     int64   `json:"quota"`
	Errors    int64   `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
	Channels  int64   `json:"channels"`
}

// ModelUsage 模型用量（30 天，Weibull 衰减后的偏好）。
type ModelUsage struct {
	Model     string  `json:"model"`
	Requests  float64 `json:"requests"`
	Tokens    float64 `json:"tokens"`
	Quota     float64 `json:"quota"`
	Errors    float64 `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
	Share     float64 `json:"share"`
}

// HourBucket 24 小时活跃分布（衰减后请求权重）。
type HourBucket struct {
	Hour     int     `json:"hour"`
	Requests float64 `json:"requests"`
}

// CostPoint 近 30 天每日成本（原始 quota）。
type CostPoint struct {
	Date  string `json:"date"`
	Quota int64  `json:"quota"`
}

// ChannelUsage 渠道用量与错误率（原始事实）。
type ChannelUsage struct {
	ChannelID   int     `json:"channel_id"`
	ChannelName string  `json:"channel_name"`
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	ErrorRate   float64 `json:"error_rate"`
	Quota       int64   `json:"quota"`
}

// Suggestion 规则型建议：前端按 Code 映射 i18n 文案，Args 用于插值。
type Suggestion struct {
	Code string         `json:"code"`
	Args map[string]any `json:"args,omitempty"`
}

// Evidence 画像数据来源证据（OpenLore 风格：来源+窗口+样本量）。
type Evidence struct {
	Source  string `json:"source"`
	Window  string `json:"window"`
	Samples int64  `json:"samples"`
}

// Profile 用户画像响应。
type Profile struct {
	UserID      int            `json:"user_id"`
	GeneratedAt int64          `json:"generated_at"`
	Overview7d  TimeWindow     `json:"overview_7d"`
	Overview30d TimeWindow     `json:"overview_30d"`
	Trend       []CostPoint    `json:"trend"`
	ModelUsage  []ModelUsage   `json:"model_usage"`
	TimeHeatmap []HourBucket   `json:"time_heatmap"`
	Channels    []ChannelUsage `json:"channels"`
	Suggestions []Suggestion   `json:"suggestions"`
	Evidence    Evidence       `json:"evidence"`
}

// weibullWeight 返回 ageSeconds 龄记录的权重（0 龄=1，越远越低）。
func weibullWeight(ageSeconds float64) float64 {
	if ageSeconds <= 0 {
		return 1
	}
	x := ageSeconds / decayLambdaSeconds
	return math.Exp(-math.Pow(x, decayK))
}

// ---------------------------------------------------------------------------
// 缓存：进程内 sync.Map + Redis（key user_profile:{userID}，TTL 60s）。
// 读路径不回写业务库。
// ---------------------------------------------------------------------------

type cachedProfile struct {
	expiresAt time.Time
	raw       []byte
}

var profileCache sync.Map // userID(int) -> *cachedProfile

func redisProfileKey(userID int) string {
	return fmt.Sprintf("user_profile:%d", userID)
}

func loadFromProcessCache(userID int) ([]byte, bool) {
	value, ok := profileCache.Load(userID)
	if !ok {
		return nil, false
	}
	entry, ok := value.(*cachedProfile)
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.raw, true
}

func storeProcessCache(userID int, raw []byte) {
	profileCache.Store(userID, &cachedProfile{expiresAt: time.Now().Add(cacheTTL), raw: raw})
}

func loadFromRedis(ctx context.Context, userID int) ([]byte, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, false
	}
	raw, err := common.RDB.Get(ctx, redisProfileKey(userID)).Bytes()
	if err != nil {
		return nil, false
	}
	return raw, true
}

func storeRedis(ctx context.Context, userID int, raw []byte) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	_ = common.RDB.Set(ctx, redisProfileKey(userID), raw, profileRedisTTL).Err()
}

// InvalidateProfileCache 使单个用户的画像缓存失效（测试/运维用）。
func InvalidateProfileCache(userID int) {
	profileCache.Delete(userID)
	if common.RedisEnabled && common.RDB != nil {
		_ = common.RDB.Del(context.Background(), redisProfileKey(userID)).Err()
	}
}

// ---------------------------------------------------------------------------
// 聚合
// ---------------------------------------------------------------------------

type consumeRow struct {
	ModelName        string
	ChannelID        int
	Quota            int
	PromptTokens     int
	CompletionTokens int
	CreatedAt        int64
}

type errorRow struct {
	ModelName string
	ChannelID int
	CreatedAt int64
}

func cstNow() time.Time {
	return time.Now().In(time.FixedZone("CST", 8*3600))
}

// queryConsumeRows 拉取近 30 天该用户的 consume 日志明细（日志库）。
func queryConsumeRows(userID int, now int64) ([]consumeRow, error) {
	if model.LOG_DB == nil {
		return nil, errors.New("log database is not initialized")
	}
	var rows []consumeRow
	err := model.LOG_DB.Table("logs").
		Select("model_name, channel_id, quota, prompt_tokens, completion_tokens, created_at").
		Where("type = ? AND user_id = ? AND created_at >= ? AND created_at <= ?",
			model.LogTypeConsume, userID, now-window30dSeconds, now).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func queryErrorRows(userID int, now int64) ([]errorRow, error) {
	if model.LOG_DB == nil {
		return nil, errors.New("log database is not initialized")
	}
	var rows []errorRow
	err := model.LOG_DB.Table("logs").
		Select("model_name, channel_id, created_at").
		Where("type = ? AND user_id = ? AND created_at >= ? AND created_at <= ?",
			model.LogTypeError, userID, now-window30dSeconds, now).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func summarizeWindow(rows []consumeRow, errRows []errorRow, cutoff int64) TimeWindow {
	var win TimeWindow
	for _, r := range rows {
		if r.CreatedAt < cutoff {
			continue
		}
		win.Requests++
		win.Tokens += int64(r.PromptTokens + r.CompletionTokens)
		win.Quota += int64(r.Quota)
	}
	seenChannels := make(map[int]struct{})
	for _, r := range rows {
		if r.CreatedAt < cutoff {
			continue
		}
		seenChannels[r.ChannelID] = struct{}{}
	}
	for _, r := range errRows {
		if r.CreatedAt < cutoff {
			continue
		}
		win.Errors++
		seenChannels[r.ChannelID] = struct{}{}
	}
	win.Channels = int64(len(seenChannels))
	total := win.Requests + win.Errors
	if total > 0 {
		win.ErrorRate = float64(win.Errors) / float64(total)
	}
	return win
}

func resolveChannelName(channelID int) string {
	if channelID == 0 {
		return "-"
	}
	ch, err := model.CacheGetChannel(channelID)
	if err != nil || ch == nil {
		return fmt.Sprintf("#%d", channelID)
	}
	return ch.Name
}

// buildProfile 从日志聚合用户画像。
func buildProfile(userID int) *Profile {
	now := time.Now().Unix()
	consumeRows, consumeErr := queryConsumeRows(userID, now)
	if consumeErr != nil {
		common.SysError("user_profile: failed to query consume logs: " + consumeErr.Error())
		consumeRows = nil
	}
	errRows, errQueryErr := queryErrorRows(userID, now)
	if errQueryErr != nil {
		common.SysError("user_profile: failed to query error logs: " + errQueryErr.Error())
		errRows = nil
	}

	p := &Profile{
		UserID:      userID,
		GeneratedAt: now,
		Evidence: Evidence{
			Source:  "consume_logs",
			Window:  "30d",
			Samples: int64(len(consumeRows) + len(errRows)),
		},
	}
	p.Overview7d = summarizeWindow(consumeRows, errRows, now-window7dSeconds)
	p.Overview30d = summarizeWindow(consumeRows, errRows, now-window30dSeconds)

	// 成本趋势：近 30 天每日原始 quota，缺日补 0。
	{
		byDate := make(map[string]int64)
		for _, r := range consumeRows {
			day := time.Unix(r.CreatedAt, 0).In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
			byDate[day] += int64(r.Quota)
		}
		start := time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, -29)
		for i := range 30 {
			day := start.AddDate(0, 0, i).Format("2006-01-02")
			p.Trend = append(p.Trend, CostPoint{Date: day, Quota: byDate[day]})
		}
	}

	// 模型用量（Weibull 衰减）
	{
		type acc struct {
			req, tokens, quota, errs float64
		}
		modelAcc := make(map[string]*acc)
		var totalQuotaW float64
		for _, r := range consumeRows {
			w := weibullWeight(float64(now - r.CreatedAt))
			a := modelAcc[r.ModelName]
			if a == nil {
				a = &acc{}
				modelAcc[r.ModelName] = a
			}
			a.req += w
			a.tokens += w * float64(r.PromptTokens+r.CompletionTokens)
			a.quota += w * float64(r.Quota)
			totalQuotaW += w * float64(r.Quota)
		}
		errAcc := make(map[string]float64)
		for _, r := range errRows {
			errAcc[r.ModelName] += weibullWeight(float64(now - r.CreatedAt))
		}
		type kv struct {
			name string
			acc  *acc
		}
		list := make([]kv, 0, len(modelAcc))
		for name, a := range modelAcc {
			list = append(list, kv{name, a})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].acc.quota > list[j].acc.quota })
		if len(list) > maxModelUsage {
			list = list[:maxModelUsage]
		}
		for _, item := range list {
			errs := errAcc[item.name]
			total := item.acc.req + errs
			share := 0.0
			if totalQuotaW > 0 {
				share = item.acc.quota / totalQuotaW
			}
			p.ModelUsage = append(p.ModelUsage, ModelUsage{
				Model:     item.name,
				Requests:  item.acc.req,
				Tokens:    item.acc.tokens,
				Quota:     item.acc.quota,
				Errors:    errs,
				ErrorRate: rate(total, errs),
				Share:     share,
			})
		}
	}

	// 24 小时活跃分布（衰减后请求权重）
	{
		hourAcc := make([]float64, 24)
		for _, r := range consumeRows {
			hour := time.Unix(r.CreatedAt, 0).In(time.FixedZone("CST", 8*3600)).Hour()
			hourAcc[hour] += weibullWeight(float64(now - r.CreatedAt))
		}
		for hour := range 24 {
			p.TimeHeatmap = append(p.TimeHeatmap, HourBucket{Hour: hour, Requests: hourAcc[hour]})
		}
	}

	// 渠道用量与错误率
	{
		type chanAcc struct {
			requests int64
			quota    int64
			errs     int64
		}
		acc := make(map[int]*chanAcc)
		for _, r := range consumeRows {
			a := acc[r.ChannelID]
			if a == nil {
				a = &chanAcc{}
				acc[r.ChannelID] = a
			}
			a.requests++
			a.quota += int64(r.Quota)
		}
		for _, r := range errRows {
			a := acc[r.ChannelID]
			if a == nil {
				a = &chanAcc{}
				acc[r.ChannelID] = a
			}
			a.errs++
		}
		ids := make([]int, 0, len(acc))
		for id := range acc {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return acc[ids[i]].quota > acc[ids[j]].quota })
		for _, id := range ids {
			a := acc[id]
			total := a.requests + a.errs
			p.Channels = append(p.Channels, ChannelUsage{
				ChannelID:   id,
				ChannelName: resolveChannelName(id),
				Requests:    a.requests,
				Errors:      a.errs,
				ErrorRate:   rate(float64(total), float64(a.errs)),
				Quota:       a.quota,
			})
		}
	}

	p.Suggestions = buildSuggestions(p)
	return p
}

func rate(total, part float64) float64 {
	if total <= 0 {
		return 0
	}
	return part / total
}

func buildSuggestions(p *Profile) []Suggestion {
	var out []Suggestion
	if p.Evidence.Samples == 0 {
		out = append(out, Suggestion{Code: "no_recent_activity"})
		return out
	}
	// 深夜活跃
	peakHour, peakReqs := 0, 0.0
	for _, h := range p.TimeHeatmap {
		if h.Requests > peakReqs {
			peakHour, peakReqs = h.Hour, h.Requests
		}
	}
	if peakReqs > 0 && (peakHour >= 22 || peakHour <= 5) {
		out = append(out, Suggestion{Code: "night_usage", Args: map[string]any{"hour": peakHour}})
	}
	// 模型集中
	if len(p.ModelUsage) > 0 && p.ModelUsage[0].Share > 0.8 {
		out = append(out, Suggestion{Code: "concentrated_model", Args: map[string]any{"model": p.ModelUsage[0].Model, "share": int(p.ModelUsage[0].Share * 100)}})
	}
	// 渠道错误率偏高
	for _, ch := range p.Channels {
		if ch.Errors >= 3 && ch.ErrorRate > 0.1 {
			out = append(out, Suggestion{Code: "high_error_channel", Args: map[string]any{"channel": ch.ChannelName, "rate": int(ch.ErrorRate * 100)}})
		}
	}
	// 近 7 天占 30 天大头
	if p.Overview30d.Quota > 0 && p.Overview7d.Quota > p.Overview30d.Quota*7/10 {
		out = append(out, Suggestion{Code: "recent_spike", Args: map[string]any{"percent": int(p.Overview7d.Quota * 100 / p.Overview30d.Quota)}})
	}
	return out
}

// GetUserProfile 返回用户画像；优先进程内缓存，其次 Redis，最后重建。
func GetUserProfile(userID int) (*Profile, error) {
	if raw, ok := loadFromProcessCache(userID); ok {
		var p Profile
		if err := common.Unmarshal(raw, &p); err == nil {
			return &p, nil
		}
	}
	ctx := context.Background()
	if raw, ok := loadFromRedis(ctx, userID); ok {
		var p Profile
		if err := common.Unmarshal(raw, &p); err == nil {
			storeProcessCache(userID, raw)
			return &p, nil
		}
	}
	p := buildProfile(userID)
	raw, err := common.Marshal(p)
	if err != nil {
		return nil, err
	}
	storeProcessCache(userID, raw)
	storeRedis(ctx, userID, raw)
	return p, nil
}
