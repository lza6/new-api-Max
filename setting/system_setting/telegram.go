package system_setting

import (
	"strings"

	"github.com/lza6/new-api-Max/setting/config"
)

type TelegramSettings struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

var telegramSettings TelegramSettings

func init() {
	config.GlobalConfig.Register("telegram", &telegramSettings)
}

func GetTelegramSettings() *TelegramSettings {
	return &telegramSettings
}

func (s *TelegramSettings) IsConfigured() bool {
	return strings.TrimSpace(s.ClientID) != "" && strings.TrimSpace(s.ClientSecret) != ""
}
