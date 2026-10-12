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

// LoginLockoutSetting 账号级登录失败锁定（热更新，注册名 "login_lockout"）。
//
// OWASP Authentication Cheat Sheet（Account Lockout）原文要求：
// 「The counter of failed logins should be associated with the account itself, rather
// than the source IP address, in order to prevent an attacker from making login attempts
// from a large number of different IP addresses.」
// 仅按 IP 限流（login_rate_limit）无法覆盖代理池/僵尸网络轮换出口 IP 的爆破：
// 每个 IP 各自未满配额即换 IP，服务端永不触发 429，单账号失败次数无上限累积。
//
// 安全默认（AGENTS.md：安全默认必须落在"安全的那一侧"）：默认 **开启**，
// 阈值与 IP 桶默认一致（20 次），退避上限 15 分钟。
//
// **锁定时间退避（而非永久锁定）是刻意的 DoS 取舍**：攻击者可以主动打满他人账号
// 的失败计数，若锁定是永久或很长的，就变成"任何人都能锁死别人的账号"。
// 因此退避上限（MaxLockSeconds）必须有限；且**任何一次成功认证都会清零计数**
// （见 service/account_lockout.go 的 RecordPasswordAuthSuccess），
// 使受害者用正确密码等一个短窗口即可自解，无需管理员介入。
type LoginLockoutSetting struct {
	// Enabled 账号级失败锁定开关，默认开启。
	Enabled bool `json:"enabled"`
	// Threshold 触发锁定所需的连续失败次数，默认 20（对齐 IP 桶默认 num。
	// 实测正常用户极难在 20 分钟内输错 20 次密码，而攻击者需要完成 20 倍工作
	// 才能换来一次退避封锁——提高攻击成本而不影响正常用户）。
	Threshold int `json:"threshold"`
	// WindowSeconds 失败计数窗口（秒），默认 1200（20 分钟，对齐 IP 桶窗口）。
	// 仅统计该窗口内的失败；更早的失败即使没有成功登录也不计入。
	WindowSeconds int64 `json:"window_seconds"`
	// BaseLockSeconds 首次锁定的时长（秒），默认 300（5 分钟，对齐 2FA 的
	// common.LockoutDuration）。
	BaseLockSeconds int64 `json:"base_lock_seconds"`
	// MaxLockSeconds 指数退避的锁定上限（秒），默认 900（15 分钟）。
	// 必须有限：这是「锁定不可被滥用为永久 DoS」的硬边界。
	MaxLockSeconds int64 `json:"max_lock_seconds"`
}

var loginLockoutSetting = LoginLockoutSetting{
	Enabled:         true,
	Threshold:       20,
	WindowSeconds:   1200,
	BaseLockSeconds: 300,
	MaxLockSeconds:  900,
}

func init() {
	config.GlobalConfig.Register("login_lockout", &loginLockoutSetting)
}

func GetLoginLockoutSetting() *LoginLockoutSetting {
	return &loginLockoutSetting
}

// IsLoginLockoutEnabled 账号级登录失败锁定是否开启（默认开启）。
func IsLoginLockoutEnabled() bool {
	return loginLockoutSetting.Enabled
}

// GetLoginLockoutThreshold 触发锁定的失败次数（<=0 回退默认 20）。
func GetLoginLockoutThreshold() int {
	if loginLockoutSetting.Threshold > 0 {
		return loginLockoutSetting.Threshold
	}
	return 20
}

// GetLoginLockoutWindowSeconds 失败计数窗口秒数（<=0 回退默认 1200）。
func GetLoginLockoutWindowSeconds() int64 {
	if loginLockoutSetting.WindowSeconds > 0 {
		return loginLockoutSetting.WindowSeconds
	}
	return 1200
}

// GetLoginLockoutBaseLockSeconds 首次锁定秒数（<=0 回退默认 300）。
func GetLoginLockoutBaseLockSeconds() int64 {
	if loginLockoutSetting.BaseLockSeconds > 0 {
		return loginLockoutSetting.BaseLockSeconds
	}
	return 300
}

// GetLoginLockoutMaxLockSeconds 锁定退避上限秒数。
// <=0 或小于基数时回退默认 900：上限小于基数会让退避曲线失去意义
// （第二次失败获得的锁定比第一次还短）。
func GetLoginLockoutMaxLockSeconds() int64 {
	if loginLockoutSetting.MaxLockSeconds <= 0 {
		return 900
	}
	if base := GetLoginLockoutBaseLockSeconds(); loginLockoutSetting.MaxLockSeconds < base {
		return base
	}
	return loginLockoutSetting.MaxLockSeconds
}
