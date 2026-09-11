package common

import "github.com/lza6/new-api-Max/constant"

const defaultAnonymousRequestBodyLimitKB = 512

func GetAnonymousRequestBodyLimitBytes() int64 {
	limitKB := constant.AnonymousRequestBodyLimitKB
	if limitKB < 0 {
		limitKB = defaultAnonymousRequestBodyLimitKB
	}
	return int64(limitKB) << 10
}
