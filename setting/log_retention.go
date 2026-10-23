package setting

// LogRetentionDays 是数据库 logs 表的保留天数。
//
// 0 = 永不清理（默认，保持既有行为，避免升级即删数据）。
//
// 为什么需要它：磁盘日志文件（/app/logs）有手动清理入口，但 **logs 表从来没有
// 自动清理**。2026-10-23 生产实测该表已累积 184 万行 / 3.5GB / 42 天，正是那次
// PostgreSQL OOM 事故的燃料。该值由 service.StartLogRetentionTask 定期消费，
// 触发一次分批删除（复用既有的 log_cleanup 系统任务，不是一次性全表 DELETE）。
var LogRetentionDays = 0

// LogRetentionEnabled 报告是否启用数据库日志自动清理。
func LogRetentionEnabled() bool { return LogRetentionDays > 0 }
