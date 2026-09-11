package service

import (
	"github.com/lza6/new-api-Max/setting/operation_setting"
	"github.com/lza6/new-api-Max/setting/system_setting"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
