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
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service/probe"
	"github.com/lza6/new-api-Max/setting/channel_setting"
)

// probeTargetFromChannel 从渠道行构造探测目标。
// v1.1 探测仅覆盖 OpenAI 兼容端点（/v1/chat/completions）；其它协议渠道
// 返回 supported=false。
func probeTargetFromChannel(ch *model.Channel) (*probe.ProbeTarget, bool) {
	if !probeSupportedChannelType(ch.Type) {
		return nil, false
	}
	baseURL := ch.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(ch.Type)
	}
	return &probe.ProbeTarget{
		ChannelID:   ch.Id,
		Name:        ch.Name,
		BaseURL:     baseURL,
		Key:         ch.Key,
		Model:       probeModelForChannel(ch),
		TimeoutSecs: 60,
	}, true
}

// probeSupportedChannelType v1.1 仅探测 OpenAI 兼容渠道。
func probeSupportedChannelType(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeCustom:
		return true
	default:
		return false
	}
}

// probeModelForChannel 取渠道模型列表第一个作为探测模型。
func probeModelForChannel(ch *model.Channel) string {
	models := strings.Split(ch.Models, ",")
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m != "" {
			return m
		}
	}
	return ""
}

// ProbeChannel 手动触发一次渠道验真探测（管理员）。
func ProbeChannel(c *gin.Context) {
	channelId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	ch, err := model.GetChannelById(int(channelId), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	target, supported := probeTargetFromChannel(ch)
	if !supported {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "probe only supports OpenAI-compatible channels in v1.1"})
		return
	}
	report := probe.RunProbe(c.Request.Context(), target)
	reportJSON, err := common.Marshal(report)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.SaveChannelProbeResult(int(channelId), string(reportJSON)); err != nil {
		common.SysError("save probe result failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "probe ran but saving result failed: " + err.Error(), "data": report})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

// GetChannelProbeResult 返回渠道探测历史（最近 5 次）。
func GetChannelProbeResult(c *gin.Context) {
	channelId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	history, err := model.GetChannelProbeHistory(int(channelId))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}

// probeScheduledChannelsHandler 每日定时探测（默认 off，channel.probe_schedule_enabled）。
type probeScheduledChannelsHandler struct{}

func (probeScheduledChannelsHandler) Type() string { return model.SystemTaskTypeChannelProbe }

func (probeScheduledChannelsHandler) Enabled() bool {
	return channel_setting.GetChannelSetting().ProbeScheduleEnabled
}

func (probeScheduledChannelsHandler) Interval() time.Duration { return 24 * time.Hour }

func (probeScheduledChannelsHandler) NewPayload() any { return nil }

func (probeScheduledChannelsHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runScheduledProbeOnce(ctx)
	// B4-2 失败重试 1 次：首轮有渠道探测失败（被跳过/得分 F）时立即补跑一轮，
	// 第二轮无论结果如何都记为成功（防无限重试）。失败原因已留在渠道 probe_result。
	if n := retryableProbeFailures(summary); n > 0 {
		common.SysLog(fmt.Sprintf("scheduled probe retry: %d channel(s) failed in first round", n))
		retry := runScheduledProbeOnce(ctx)
		summary["first_round_failed"] = n
		summary["retried"] = true
		for k, v := range retry {
			if _, exists := summary[k]; !exists {
				summary[k] = v
			}
		}
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// retryableProbeFailures 统计首轮需重试的渠道数：探测被跳过（配置缺失）
// 或整轮 Grade F（渠道疑似不可用）视为可重试失败。
func retryableProbeFailures(summary map[string]any) int {
	failed, _ := summary["failed"].(int)
	return failed
}

// runScheduledProbeOnce 对所有「到期」的 OpenAI 兼容渠道各跑一轮探测。
func runScheduledProbeOnce(ctx context.Context) map[string]any {
	channels, err := model.GetAllChannels(0, 0, false, true)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	probed, skipped, unsupported, failed := 0, 0, 0, 0
	for _, ch := range channels {
		if ctx.Err() != nil {
			break
		}
		if !model.ChannelProbeDue(ch.Id) {
			skipped++
			continue
		}
		target, supported := probeTargetFromChannel(ch)
		if !supported {
			unsupported++
			continue
		}
		report := probe.RunProbe(ctx, target)
		if reportJSON, err := common.Marshal(report); err == nil {
			_ = model.SaveChannelProbeResult(ch.Id, string(reportJSON))
		}
		probed++
		// B4-2 失败判定：整轮被跳过（无探测模型）或 Grade F（渠道疑似不可用）
		// 计入 failed，触发调度重试一轮。
		if report.Skipped != "" || report.Grade == probe.GradeF {
			failed++
		}
	}
	summary := map[string]any{
		"probed":      probed,
		"not_due":     skipped,
		"unsupported": unsupported,
		"failed":      failed,
	}
	common.SysLog(fmt.Sprintf("scheduled channel probe done: %+v", summary))
	return summary
}
