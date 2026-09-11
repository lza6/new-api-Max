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
package operation_setting

import "github.com/lza6/new-api-Max/setting/config"

// LoginRateLimitSetting 登录接口专属限流（热更新，注册名 "login_rate_limit"）。
// 与全局 CriticalRateLimit（影响重置密码/OAuth 等全部敏感端点）解耦：
// 管理员可在后台单独开关登录限流，默认关闭（登录不限流）。
type LoginRateLimitSetting struct {
	// Enabled 登录限流开关，默认关闭。
	Enabled bool `json:"enabled"`
	// Duration 限流窗口（秒），默认 1200（20 分钟，对齐全局 Critical 默认）。
	Duration int64 `json:"duration"`
	// Num 窗口内允许的请求次数，默认 20。
	Num int `json:"num"`
}

var loginRateLimitSetting = LoginRateLimitSetting{
	Enabled:  false,
	Duration: 1200,
	Num:      20,
}

func init() {
	config.GlobalConfig.Register("login_rate_limit", &loginRateLimitSetting)
}

func GetLoginRateLimitSetting() *LoginRateLimitSetting {
	return &loginRateLimitSetting
}

// IsLoginRateLimitEnabled 登录限流是否开启（默认关闭）。
func IsLoginRateLimitEnabled() bool {
	return loginRateLimitSetting.Enabled
}

// GetLoginRateLimitNum 窗口内允许次数（<=0 时回退默认 20）。
func GetLoginRateLimitNum() int {
	if loginRateLimitSetting.Num > 0 {
		return loginRateLimitSetting.Num
	}
	return 20
}

// GetLoginRateLimitDuration 限流窗口秒数（<=0 时回退默认 1200）。
func GetLoginRateLimitDuration() int64 {
	if loginRateLimitSetting.Duration > 0 {
		return loginRateLimitSetting.Duration
	}
	return 1200
}