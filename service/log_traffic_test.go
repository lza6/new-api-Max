package service

import (
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/require"
)

func TestAggregateTrafficByDay(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	day1 := time.Date(2026, 9, 20, 10, 0, 0, 0, loc).Unix()
	day2 := time.Date(2026, 9, 19, 23, 0, 0, 0, loc).Unix()
	records := []TrafficRecord{
		{CreatedAt: day1, Other: `{"request_bytes": 100, "response_bytes": 900}`},
		{CreatedAt: day1, Other: `{"request_bytes": 200, "response_bytes": 800}`},
		{CreatedAt: day2, Other: `{"request_bytes": 50}`},
		{CreatedAt: day1, Other: `not-json`},
	}
	got := AggregateTrafficByDay(records, loc)
	require.Len(t, got, 2)
	require.Equal(t, "2026-09-20", got[0].Date)
	require.Equal(t, 3, got[0].Requests)
	require.Equal(t, int64(2000), got[0].Bytes)
	require.InDelta(t, float64(2000)/(1024*1024), got[0].MB, 1e-9)
	require.Equal(t, "2026-09-19", got[1].Date)
	require.Equal(t, 1, got[1].Requests)
	require.Equal(t, int64(50), got[1].Bytes)
}

func TestAggregateTrafficByDayEmpty(t *testing.T) {
	require.Nil(t, AggregateTrafficByDay(nil, time.UTC))
	require.Empty(t, AggregateTrafficByDay([]TrafficRecord{}, time.UTC))
}

func TestAggregateBandwidthByDay(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	day1 := time.Date(2026, 9, 20, 10, 0, 0, 0, loc).Unix()
	day2 := time.Date(2026, 9, 19, 23, 0, 0, 0, loc).Unix()
	records := []TrafficBytesRecord{
		{CreatedAt: day1, RequestBytes: 100, ResponseBytes: 900},
		{CreatedAt: day1, RequestBytes: 200, ResponseBytes: 800},
		{CreatedAt: day2, RequestBytes: 50, ResponseBytes: 10},
	}
	got := AggregateBandwidthByDay(records, loc, 0)
	require.Len(t, got, 2)
	require.Equal(t, "2026-09-20", got[0].Date)
	require.Equal(t, int64(2000), got[0].Bytes)
	require.Equal(t, int64(2), got[0].Requests)
	require.Equal(t, "2026-09-19", got[1].Date)
	require.Equal(t, int64(60), got[1].Bytes)

	limited := AggregateBandwidthByDay(records, loc, 1)
	require.Len(t, limited, 1)
	require.Equal(t, "2026-09-20", limited[0].Date)

	require.Nil(t, AggregateBandwidthByDay(nil, time.UTC, 0))
	require.Empty(t, AggregateBandwidthByDay([]TrafficBytesRecord{}, time.UTC, 10))
}

func TestAggregateBandwidthByModel(t *testing.T) {
	records := []TrafficBytesRecord{
		{ModelName: "deepseek-v4-flash", RequestBytes: 100, ResponseBytes: 900},
		{ModelName: "deepseek-v4-flash", RequestBytes: 200, ResponseBytes: 800},
		{ModelName: "gpt-5.6-sol", RequestBytes: 50, ResponseBytes: 10},
		{ModelName: "", RequestBytes: 10, ResponseBytes: 20},
	}
	got := AggregateBandwidthByModel(records, 0)
	require.Len(t, got, 3)
	require.Equal(t, "deepseek-v4-flash", got[0].Model)
	require.Equal(t, int64(2), got[0].Requests)
	require.Equal(t, int64(2000), got[0].Bytes)
	require.Equal(t, "gpt-5.6-sol", got[1].Model)
	require.Equal(t, int64(1), got[1].Requests)
	require.Equal(t, int64(60), got[1].Bytes)
	require.Equal(t, "(unknown)", got[2].Model)
	require.Equal(t, int64(30), got[2].Bytes)

	limited := AggregateBandwidthByModel(records, 1)
	require.Len(t, limited, 1)
	require.Equal(t, "deepseek-v4-flash", limited[0].Model)

	require.Nil(t, AggregateBandwidthByModel(nil, 0))
	require.Empty(t, AggregateBandwidthByModel([]TrafficBytesRecord{}, 10))
}

func TestMergeModelStats(t *testing.T) {
	got := MergeModelStats(
		map[string]int64{"a": 10, "b": 1},
		map[string]int64{"a": 2},
		map[string]int64{"a": 100, "b": 50},
		map[string]int64{"a": 5, "c": 3},
	)
	require.Len(t, got, 3)
	require.Equal(t, "a", got[0].Model)
	require.Equal(t, int64(12), got[0].TodayTotal)
	require.Equal(t, int64(10), got[0].TodaySuccess)
	require.Equal(t, int64(105), got[0].Days30Total)
	require.Equal(t, int64(100), got[0].Days30Success)
	require.Equal(t, "b", got[1].Model)
	require.Equal(t, "c", got[2].Model)

	empty := MergeModelStats(nil, nil, nil, nil)
	require.Empty(t, empty)
}

// TestAggregateTrafficByDaySaturatesOtherBytes guards the aggregation against
// legacy other JSON rows whose request_bytes/response_bytes are absurdly large
// (production 72% legacy rows only carry bytes in other, and the value is
// user/upstream-controlled): the float->int conversion must saturate via
// common.QuotaFromFloat instead of wrapping into a negative byte total.
func TestAggregateTrafficByDaySaturatesOtherBytes(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	day1 := time.Date(2026, 9, 20, 10, 0, 0, 0, loc).Unix()
	records := []TrafficRecord{
		{CreatedAt: day1, Other: `{"request_bytes": 18446744073686646784, "response_bytes": 900}`},
		{CreatedAt: day1, Other: `{"request_bytes": -18446744073686646784, "response_bytes": 100}`},
		{CreatedAt: day1, Other: `{"request_bytes": 100, "response_bytes": 200}`},
	}
	got := AggregateTrafficByDay(records, loc)
	require.Len(t, got, 1)
	require.Equal(t, 3, got[0].Requests)
	// int64(float64) would wrap to negative values; saturation must keep the
	// total strictly positive and bounded.
	require.GreaterOrEqual(t, got[0].Bytes, int64(0))
	require.Equal(t, getSaturatedTrafficTotal(), got[0].Bytes)
}

// getSaturatedTrafficTotal returns the exact expected sum for the saturated
// fixture rows: request_bytes 1.84e19 -> common.MaxQuota, -1.84e19 ->
// common.MinQuota, then the plain 900+100+300 response bytes.
func getSaturatedTrafficTotal() int64 {
	return int64(common.MaxQuota) + int64(common.MinQuota) + 1300
}
