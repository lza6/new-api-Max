package model

import "gorm.io/gorm"

// WebRequestLog 记录 Web/管理请求的聚合流量（Web 防刷可见性）。
// 一条记录 = 一个 IP 在一个 60s 窗口内的聚合。
type WebRequestLog struct {
	ID            uint64  `json:"id" gorm:"primaryKey"`
	IP            string  `json:"ip" gorm:"type:varchar(64);index"`
	Path          string  `json:"path" gorm:"type:varchar(512)"`
	Method        string  `json:"method" gorm:"type:varchar(16)"`
	Status        int     `json:"status"`
	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	UserAgent     string  `json:"user_agent" gorm:"type:varchar(512)"`
	RequestCount  int     `json:"request_count"`
	RatePerSecond float64 `json:"rate_per_second"`
	WindowStart   int64   `json:"window_start" gorm:"index"`
}

func (WebRequestLog) TableName() string {
	return "web_request_logs"
}

// RecordWebRequestLogs 批量写入 Web 请求聚合日志。
// DeleteWebRequestLogsBefore 删除 window_start 早于 cutoff 的聚合日志（幂等，保留策略）。
func DeleteWebRequestLogsBefore(cutoff int64) (int64, error) {
	if cutoff <= 0 {
		return 0, nil
	}
	result := DB.Where("window_start < ?", cutoff).Delete(&WebRequestLog{})
	return result.RowsAffected, result.Error
}

func RecordWebRequestLogs(rows []WebRequestLog) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(rows, 200).Error
	})
}

// WebRequestLogAggregateRow 按 IP 聚合的结果行。
type WebRequestLogAggregateRow struct {
	IP            string  `json:"ip"`
	RequestCount  int64   `json:"request_count"`
	RatePerSecond float64 `json:"rate_per_second"`
	BytesTotal    int64   `json:"bytes_total"`
	LastRequestAt int64   `json:"last_request_at"`
}

// AggregateWebRequestLogs 按 IP 聚合 Web 请求日志。
// ip 非空时仅统计该 IP；sort=rate|count|bytes；order=asc|desc。
func AggregateWebRequestLogs(ip, sort, order string, page, size int) ([]WebRequestLogAggregateRow, int64, error) {
	where := ""
	args := []any{}
	if ip != "" {
		where = "WHERE ip = ?"
		args = append(args, ip)
	}
	var total int64
	if err := DB.Raw("SELECT COUNT(DISTINCT ip) FROM web_request_logs "+where, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	sortCol := "MAX(rate_per_second)"
	switch sort {
	case "count":
		sortCol = "SUM(request_count)"
	case "bytes":
		sortCol = "SUM(bytes_sent)"
	}
	orderSQL := "DESC"
	if order != "desc" {
		orderSQL = "ASC"
	}
	selectSQL := "ip, SUM(request_count) AS request_count, MAX(rate_per_second) AS rate_per_second, SUM(bytes_sent) AS bytes_total, MAX(window_start) AS last_request_at"
	query := "SELECT " + selectSQL + " FROM web_request_logs " + where + " GROUP BY ip ORDER BY " + sortCol + " " + orderSQL + " LIMIT ? OFFSET ?"
	bind := append(append([]any{}, args...), size, (page-1)*size)
	var rows []WebRequestLogAggregateRow
	if err := DB.Raw(query, bind...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// WebRequestLogDetailByIP 返回指定 IP 的路径明细。
func WebRequestLogDetailByIP(ip string, page, size int) ([]WebRequestLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	var rows []WebRequestLog
	var total int64
	base := DB.Model(&WebRequestLog{}).Where("ip = ?", ip)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := base.Order("window_start desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
