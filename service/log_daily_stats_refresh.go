package service

import (
	"strconv"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

const (
	// 刷新间隔。汇总表是「展示口径」，不参与计费，10 分钟延迟可接受；
	// 间隔内新增的日志由下一次刷新补上（每次都重算整个窗口，天然幂等）。
	logDailyStatsRefreshInterval = 10 * time.Minute
	// 稳态下每次重算的窗口：2 天足够覆盖跨零点的边界与上一次刷新以来的增量。
	logDailyStatsRefreshDays = 2
	// 首次部署（表为空）时的历史回填上限。按 400 天封顶，避免误配把整表扫穿。
	logDailyStatsBackfillDays = 400
)

var (
	logDailyStatsStartOnce sync.Once
	logDailyStatsStopOnce  sync.Once
	logDailyStatsStopCh    chan struct{}
)

// StartLogDailyStatsRefresh 启动日志日汇总的定时刷新。
//
// 只应由 master 调用：多副本会重复写同一批 (day_key, dim_type, dim_value) 行，
// 虽然删+插是幂等的，但会互相覆盖并放大数据库写压力。
func StartLogDailyStatsRefresh() {
	logDailyStatsStartOnce.Do(func() {
		logDailyStatsStopCh = make(chan struct{})
		go func() {
			// 先跑一次：否则部署后到第一个 tick 之前，统计接口仍在扫原表。
			refreshLogDailyStats()
			ticker := time.NewTicker(logDailyStatsRefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-logDailyStatsStopCh:
					return
				case <-ticker.C:
					refreshLogDailyStats()
				}
			}
		}()
	})
}

// StopLogDailyStatsRefresh 停机时结束刷新循环。
func StopLogDailyStatsRefresh() {
	logDailyStatsStopOnce.Do(func() {
		if logDailyStatsStopCh != nil {
			close(logDailyStatsStopCh)
		}
	})
}

func refreshLogDailyStats() {
	days := logDailyStatsRefreshDays
	// 表为空说明是首次部署（或换了日志库），需要回填历史，否则统计页会显示 0。
	if model.DailyStatsEmpty() {
		days = logDailyStatsBackfillDays
		common.SysLog("log daily stats: summary table not ready, backfilling history")
	}
	started := time.Now()
	if err := model.RefreshLogDailyStats(days); err != nil {
		common.SysError("failed to refresh log daily stats: " + err.Error())
		return
	}
	common.SysLog("log daily stats refreshed (window days=" + strconv.Itoa(days) + ", took=" + time.Since(started).String() + ")")
}
