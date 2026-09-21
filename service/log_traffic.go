package service

import (
	"sort"
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

// TrafficBytesRecord 带宽统计输入行（consume log 持久化的 request/response 字节列）。
type TrafficBytesRecord struct {
	CreatedAt     int64
	RequestBytes  int64
	ResponseBytes int64
}

// BandwidthDay 带宽排行单日行（按日期分组后按字节降序）。
type BandwidthDay struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
	Bytes    int64  `json:"bytes"`
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

// AggregateBandwidthByDay 将持久化字节列按日聚合、按带宽降序并限量（limit<=0 不限）。
// 站点「每日网络带宽消耗排行」与前端共用；时区按 loc 归属日期。
func AggregateBandwidthByDay(records []TrafficBytesRecord, loc *time.Location, limit int) []BandwidthDay {
	if len(records) == 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	index := make(map[string]*BandwidthDay)
	for _, r := range records {
		date := time.Unix(r.CreatedAt, 0).In(loc).Format("2006-01-02")
		d, ok := index[date]
		if !ok {
			d = &BandwidthDay{Date: date}
			index[date] = d
		}
		d.Requests++
		d.Bytes += r.RequestBytes + r.ResponseBytes
	}
	out := make([]BandwidthDay, 0, len(index))
	for _, d := range index {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Date < out[j].Date
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
