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
// cost_detail.go 提供 B5-2 费用明细 API：GET /api/log/usage/:id/cost-detail。
// 返回某条消费日志的计费分段（model_ratio/group_ratio/completion_ratio/cache_ratio、
// B5-1 explain.tier_matched）与 B5-2 微美元影子价（api_equivalent_usd）。
// 纯读路径：不改计费数值，仅从 Log.Other 解析口径字段。
package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// logCostDetail 费用明细响应（字段均来自 Log.Other 既有口径，无新增计算）。
type logCostDetail struct {
	LogID            int64   `json:"log_id"`
	ModelName        string  `json:"model_name"`
	Quota            int     `json:"quota"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	ModelRatio       float64 `json:"model_ratio"`
	GroupRatio       float64 `json:"group_ratio"`
	CompletionRatio  float64 `json:"completion_ratio"`
	CacheRatio       float64 `json:"cache_ratio"`
	TierMatched      any     `json:"tier_matched,omitempty"`
	ApiEquivalentUsd int64   `json:"api_equivalent_usd,omitempty"`
	ShadowKnown      bool    `json:"shadow_known"`
}

// GetLogCostDetail 返回消费日志的费用分段与影子价明细。
func GetLogCostDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid log id"})
		return
	}
	role := c.GetInt("role")
	isAdmin := role >= common.RoleAdminUser
	log, exists, err := model.GetConsumeLogByID(id, c.GetInt("id"), isAdmin)
	if err != nil {
		common.SysError("query cost detail log error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query log"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "log not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": parseLogCostDetail(log)})
}

func parseLogCostDetail(log *model.Log) *logCostDetail {
	detail := &logCostDetail{
		LogID:            int64(log.Id),
		ModelName:        log.ModelName,
		Quota:            log.Quota,
		PromptTokens:     log.PromptTokens,
		CompletionTokens: log.CompletionTokens,
	}
	var other map[string]any
	if err := common.UnmarshalJsonStr(log.Other, &other); err != nil || other == nil {
		return detail
	}
	detail.ModelRatio = detailFloat(other, "model_ratio")
	detail.GroupRatio = detailFloat(other, "group_ratio")
	detail.CompletionRatio = detailFloat(other, "completion_ratio")
	detail.CacheRatio = detailFloat(other, "cache_ratio")
	if usd, ok := other["api_equivalent_usd"]; ok {
		if v, ok2 := toInt64Any(usd); ok2 {
			detail.ApiEquivalentUsd = v
			detail.ShadowKnown = true
		}
	}
	if explain, ok := other["explain"].(map[string]any); ok {
		if facts, ok2 := explain["facts"].([]any); ok2 {
			for _, item := range facts {
				fm, ok3 := item.(map[string]any)
				if !ok3 {
					continue
				}
				if label, _ := fm["label"].(string); label == "tier_matched" {
					detail.TierMatched = fm["value"]
				}
			}
		}
	}
	return detail
}

// T2 黑匣子日志分级透明化：把同一条日志的诊断数据渲染成两档视图。
//   - plain：白话版（任何能看该日志的用户可读）——「这次请求花了多久、慢在哪、
//     省了多少流量、是否命中缓存」，不含渠道 id/内部分组等敏感信息。
//   - technical：技术版（日志所有者或管理员）——延迟拆解、压缩、客户端、
//     路由候选数、quota_saturation 等结构化诊断。
//
// 关键：technical 视图只回显**该用户本就有权看到**的字段——owner 看自己日志的
// public+自身诊断，admin 看含 admin_info 的完整诊断；不越过既有可见性边界。
type logTransparencyView struct {
	Plain     map[string]any `json:"plain"`
	Technical map[string]any `json:"technical,omitempty"`
}

// GetLogTransparency 返回分级透明视图。GET /api/log/usage/:id/transparency
func GetLogTransparency(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid log id"})
		return
	}
	role := c.GetInt("role")
	isAdmin := role >= common.RoleAdminUser
	log, exists, err := model.GetConsumeLogByID(id, c.GetInt("id"), isAdmin)
	if err != nil {
		common.SysError("query transparency log error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to query log"})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "log not found"})
		return
	}
	var other map[string]any
	_ = common.UnmarshalJsonStr(log.Other, &other)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": buildTransparencyView(log, other, isAdmin)})
}

// buildTransparencyView 组装分级透明视图（纯函数）。
func buildTransparencyView(log *model.Log, other map[string]any, isAdmin bool) logTransparencyView {
	view := logTransparencyView{Plain: map[string]any{}}

	// —— 白话版：无敏感字段，负责「这段请求发生了什么」。
	frt := detailFloat(other, "frt")
	view.Plain["model"] = log.ModelName
	view.Plain["tokens"] = log.PromptTokens + log.CompletionTokens
	view.Plain["prompt_tokens"] = log.PromptTokens
	view.Plain["completion_tokens"] = log.CompletionTokens
	if frt > 0 {
		view.Plain["first_token_ms"] = frt
	}
	if v := detailFloat(other, "upstream_ttfb_ms"); v > 0 {
		view.Plain["upstream_wait_ms"] = v
	}
	if rb := detailFloat(other, "request_bytes"); rb > 0 {
		view.Plain["request_bytes"] = int64(rb)
	}
	if cb := detailFloat(other, "response_bytes"); cb > 0 {
		view.Plain["response_bytes"] = int64(cb)
	}
	if cacheTokens := detailFloat(other, "cache_tokens"); cacheTokens > 0 && log.PromptTokens > 0 {
		view.Plain["cache_hit_rate"] = cacheTokens / float64(log.PromptTokens)
	}
	if log.Quota > 0 {
		view.Plain["quota"] = log.Quota
	}
	view.Plain["summary"] = transparencySummary(log, other)

	// —— 技术版：结构化诊断。owner 见 public 范围，admin 见全部（含 admin_info）。
	tech := map[string]any{
		"timeline_stages":          other["timeline_stages"],
		"upstream_connect_ms":      other["upstream_connect_ms"],
		"upstream_upload_ms":       other["upstream_upload_ms"],
		"upstream_ttfb_ms":         other["upstream_ttfb_ms"],
		"request_compression_ms":   other["request_compression_ms"],
		"request_original_bytes":   other["request_original_bytes"],
		"request_compressed_bytes": other["request_compressed_bytes"],
		"cache_tokens":             other["cache_tokens"],
		"client":                   other["client"],
		"request_path":             other["request_path"],
		"stream_status":            other["stream_status"],
	}
	// 清理 nil，避免前端出现全 null 字段。
	for k, v := range tech {
		if v == nil {
			delete(tech, k)
		}
	}
	if adminInfo, ok := other["admin_info"].(map[string]any); ok && isAdmin {
		tech["channels_considered"] = adminInfo["channels_considered"]
		tech["quota_saturation"] = adminInfo["quota_saturation"]
		tech["use_channel"] = adminInfo["use_channel"]
		tech["conversion_diagnostics"] = adminInfo["conversion_diagnostics"]
	}
	view.Technical = tech
	return view
}

// transparencySummary 生成一句白话摘要（无敏感信息）。
func transparencySummary(log *model.Log, other map[string]any) string {
	if log.Quota == 0 && log.PromptTokens == 0 && log.CompletionTokens == 0 {
		if ss, ok := other["stream_status"].(map[string]any); ok {
			if s, _ := ss["status"].(string); s == "error" {
				return "Request did not complete normally."
			}
		}
	}
	return "Request completed."
}

func detailFloat(other map[string]any, key string) float64 {
	v, ok := other[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func toInt64Any(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}
