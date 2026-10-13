package training_capture

import "time"

// timeUTCDate 返回 UTC 日期字符串（YYYY-MM-DD）。单独成函数便于测试注入。
func timeUTCDate(unixSeconds int64) string {
	return time.Unix(unixSeconds, 0).UTC().Format("2006-01-02")
}
