package service

import (
	"testing"
	"time"

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
