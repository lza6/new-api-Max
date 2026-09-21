package common

import (
	"fmt"
)

// formatBytesUnits 1024 进制流量单位档位（与前端 formatTraffic 契约一致）。
var formatBytesUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

// FormatBytes 将字节数格式化为人类可读的流量字符串（B/KB/MB/GB/TB/PB，1024 进制，
// 保留 2 位小数；负数/0 安全返回）。站点统计、带宽排行与前端展示共用此口径。
func FormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	value := float64(bytes)
	unit := formatBytesUnits[0]
	for _, next := range formatBytesUnits[1:] {
		if value < 1024 {
			break
		}
		value /= 1024
		unit = next
	}
	if unit == "B" {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.2f %s", value, unit)
}
