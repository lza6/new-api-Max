package service

import (
	"time"

	"github.com/lza6/new-api-Max/common"
)

// TrafficRecord 流量统计输入行（consume log 的 created_at + other）。
type TrafficRecord struct {
	CreatedAt int64
	Other     string
}

// DailyTraffic 按日聚合结果。
type DailyTraffic struct {
	Date     string  `json:"date"`
	Requests int     `json:"requests"`
	Bytes    int64   `json:"bytes"`
	MB       float64 `json:"mb"`
}

// AggregateTrafficByDay 解析 other.request_bytes/response_bytes 并按日聚合。
func AggregateTrafficByDay(records []TrafficRecord, loc *time.Location) []DailyTraffic {
	if len(records) == 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	index := make(map[string]*DailyTraffic)
	var order []string
	for _, r := range records {
		var req, resp int64
		var other map[string]any
		if r.Other != "" && common.Unmarshal([]byte(r.Other), &other) == nil {
			if v, ok := other["request_bytes"].(float64); ok {
				req = int64(v)
			}
			if v, ok := other["response_bytes"].(float64); ok {
				resp = int64(v)
			}
		}
		t := time.Unix(r.CreatedAt, 0).In(loc)
		date := t.Format("2006-01-02")
		d, ok := index[date]
		if !ok {
			d = &DailyTraffic{Date: date}
			index[date] = d
			order = append(order, date)
		}
		d.Requests++
		d.Bytes += req + resp
	}
	out := make([]DailyTraffic, 0, len(order))
	for _, date := range order {
		d := index[date]
		d.MB = float64(d.Bytes) / (1024 * 1024)
		out = append(out, *d)
	}
	return out
}
