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
package console_setting

import (
	"testing"

	"github.com/lza6/new-api-Max/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

// TestGetApiInfoFallsBackToServerAddress T3：未配置 api_info 时按部署地址
// 生成默认条目，避免前端"未配置 API 路由"空态。
func TestGetApiInfoFallsBackToServerAddress(t *testing.T) {
	prev := GetConsoleSetting().ApiInfo
	prevAddr := system_setting.ServerAddress
	t.Cleanup(func() {
		GetConsoleSetting().ApiInfo = prev
		system_setting.ServerAddress = prevAddr
	})

	GetConsoleSetting().ApiInfo = ""
	system_setting.ServerAddress = "http://103.233.252.213:3000"
	list := GetApiInfo()
	assert.Len(t, list, 1)
	assert.Equal(t, "http://103.233.252.213:3000", list[0]["url"])
	assert.Equal(t, "/v1", list[0]["route"])
}

// TestGetApiInfoUsesConfiguredWhenPresent：已配置时原样返回，不做兜底。
func TestGetApiInfoUsesConfiguredWhenPresent(t *testing.T) {
	prev := GetConsoleSetting().ApiInfo
	prevAddr := system_setting.ServerAddress
	t.Cleanup(func() {
		GetConsoleSetting().ApiInfo = prev
		system_setting.ServerAddress = prevAddr
	})

	GetConsoleSetting().ApiInfo = `[{"url":"https://custom.example.com","route":"/v1","description":"custom","color":"blue"}]`
	system_setting.ServerAddress = "http://localhost:3000"
	list := GetApiInfo()
	assert.Len(t, list, 1)
	assert.Equal(t, "https://custom.example.com", list[0]["url"])
}
