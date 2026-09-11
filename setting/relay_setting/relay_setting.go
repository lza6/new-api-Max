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
}

// DefaultStreamFirstTokenTimeout 首包超时默认 15 秒。
const DefaultStreamFirstTokenTimeout = 15

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
