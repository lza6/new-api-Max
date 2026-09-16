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
package relay_setting

import "github.com/lza6/new-api-Max/setting/config"

// RelaySetting relay 层运行参数（热更新，注册名 "relay"）。
type RelaySetting struct {
	// StreamFallover B3-2 流式首包缓冲 fallover：缓冲 SSE 直到首个有效
	// data 块才向客户端提交响应头；首包超时判定本次渠道失败并走重试链。
	// 默认 off（不缓冲，行为与现状完全一致）。
	StreamFallover bool `json:"stream_fallover"`
	// StreamFirstTokenTimeout 首包超时（秒），默认 15；<=0 表示禁用首包超时
	// （仅缓冲不判超时）。仅在 StreamFallover 开启时生效。
	StreamFirstTokenTimeout int `json:"stream_first_token_timeout"`

	// GlobalConcurrencyEnabled T6 全局真实并发桶：开启后限制整个网关同时
	// 处理中的模型请求数；超出的请求进入有界排队，等待并发释放而不是直接
	// 拒绝。默认 off（行为与现状完全一致）。
	GlobalConcurrencyEnabled bool `json:"global_concurrency_enabled"`
	// GlobalConcurrencyLimit 全局并发上限（同时处理中的请求数）。<=0 表示
	// 不限制（仅在 Enabled 时生效）。
	GlobalConcurrencyLimit int `json:"global_concurrency_limit"`
	// GlobalConcurrencyQueue 排队容量（等待中的请求数上限）。超过则直接 429。
	// <=0 表示不允许排队（满即 429）。
	GlobalConcurrencyQueue int `json:"global_concurrency_queue"`
	// GlobalConcurrencyWaitTimeout 排队最长等待秒数；到期仍未获得并发则 429。
	// <=0 默认 30 秒。
	GlobalConcurrencyWaitTimeout int `json:"global_concurrency_wait_timeout"`
}

// DefaultStreamFirstTokenTimeout 首包超时默认 15 秒。
const DefaultStreamFirstTokenTimeout = 15

// 全局并发桶默认值（T6）。
const (
	DefaultGlobalConcurrencyLimit       = 0 // 0 = 不限制
	DefaultGlobalConcurrencyQueue       = 1000
	DefaultGlobalConcurrencyWaitTimeout = 30
)

var relaySetting = RelaySetting{
	StreamFirstTokenTimeout: DefaultStreamFirstTokenTimeout,
}

func init() {
	config.GlobalConfig.Register("relay", &relaySetting)
}

func GetRelaySetting() *RelaySetting {
	return &relaySetting
}

// GetStreamFirstTokenTimeout 返回首包超时（秒）；未配置时用默认 15。
func GetStreamFirstTokenTimeout() int {
	if s := GetRelaySetting(); s != nil && s.StreamFirstTokenTimeout > 0 {
		return s.StreamFirstTokenTimeout
	}
	return DefaultStreamFirstTokenTimeout
}

// GlobalConcurrencyGate 全局并发桶运行参数快照，供中间件读取。
type GlobalConcurrencyGate struct {
	Enabled     bool
	Limit       int
	Queue       int
	WaitTimeout int
}

func GetGlobalConcurrencyGate() GlobalConcurrencyGate {
	g := GlobalConcurrencyGate{}
	if s := GetRelaySetting(); s != nil {
		g.Enabled = s.GlobalConcurrencyEnabled
		g.Limit = s.GlobalConcurrencyLimit
		g.Queue = s.GlobalConcurrencyQueue
		g.WaitTimeout = s.GlobalConcurrencyWaitTimeout
	}
	if g.Limit <= 0 {
		g.Limit = DefaultGlobalConcurrencyLimit
	}
	if g.Queue <= 0 {
		g.Queue = DefaultGlobalConcurrencyQueue
	}
	if g.WaitTimeout <= 0 {
		g.WaitTimeout = DefaultGlobalConcurrencyWaitTimeout
	}
	return g
}
