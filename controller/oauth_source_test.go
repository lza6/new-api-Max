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

	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/oauth"
	"github.com/stretchr/testify/assert"
)

// TestOAuthSourceForProvider 注册来源打标：内置 provider 映射到稳定枚举，
// 自定义 provider 用 slug。
func TestOAuthSourceForProvider(t *testing.T) {
	cases := []struct {
		name string
		prov oauth.Provider
		want string
	}{
		{"github", &oauth.GitHubProvider{}, model.UserSourceGithub},
		{"discord", &oauth.DiscordProvider{}, model.UserSourceDiscord},
		{"telegram", oauth.NewTelegramProvider(nil), model.UserSourceTelegram},
		{"linuxdo", &oauth.LinuxDOProvider{}, model.UserSourceLinuxDO},
		{"oidc", &oauth.OIDCProvider{}, model.UserSourceOIDC},
		{"custom-slug", oauth.NewGenericOAuthProvider(&model.CustomOAuthProvider{
			Slug: "gitlab-enterprise", Name: "GitLab",
		}), "gitlab-enterprise"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, oauthSourceForProvider(c.prov))
		})
	}
}
