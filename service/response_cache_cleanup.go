package service

import (
	"context"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// 响应缓存磁盘后端的**周期清理 loop**（Batch-11）。
//
// 为什么必须有：磁盘后端一旦启用就是「持续往盘上写」的系统，没有周期回收必然
// 涨到配额上限（甚至涨满宿主盘）。本 loop 每轮做三件事：
//
//  1. **过期清理**：删掉超过 TTL 的条目（同时把进程内的占用/条目计数校正为真实值）；
//  2. **配额兜底**：即便 TTL 被配得很长，也按最旧优先淘汰到配额以内 ——
//     这是「绝不无限增长」的硬保证；
//  3. **紧急回收**：宿主盘剩余空间触及 `min_free_disk_bytes` 时，直接砍掉一半配额
//     把空间还给系统 —— 这是「不要让磁盘爆满」的最后一道闸。
//
// 心跳接进 `background_loop_last_run_timestamp_seconds{loop="response_cache_disk_cleanup"}`，
// 卡死可见。

var responseCacheCleanupLoop backgroundLoop

// StartResponseCacheCleanup 启动清理 loop（幂等）。
// 非磁盘后端时为 no-op —— 不为不用的东西常驻 goroutine。
func StartResponseCacheCleanup() {
	if response_cache_setting.Backend() != "disk" {
		return
	}
	if err := ensureResponseCacheDiskRoot(); err != nil {
		common.SysError("response cache: " + err.Error())
		// 建目录失败不阻止启动：后续写入会自然失败并留痕。
	}
	responseCacheCleanupLoop.start(func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(response_cache_setting.DiskCleanupSeconds()) * time.Second)
		defer ticker.Stop()
		runResponseCacheCleanupOnce()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runResponseCacheCleanupOnce()
			}
		}
	})
}

// StopResponseCacheCleanup 停止清理 loop（优雅关闭/测试用）。
func StopResponseCacheCleanup() { responseCacheCleanupLoop.stop() }

// runResponseCacheCleanupOnce 执行一轮清理。可被测试直接调用。
func runResponseCacheCleanupOnce() {
	if response_cache_setting.Backend() != "disk" {
		return
	}
	now := time.Now()

	// 1) 过期清理（顺带校正占用/条目计数）
	_, _ = responseCacheDiskSweep(now)

	// 2) 配额兜底：TTL 配得再长也不会越界
	maxBytes := response_cache_setting.DiskMaxBytes()
	if responseCacheDiskUsage.Load() > maxBytes {
		if freed := responseCacheDiskEvictTo(maxBytes, now); freed > 0 {
			logger.LogInfo(context.Background(), "response cache: evicted to fit the disk budget")
		}
	}

	// 3) 剩余空间闸：宿主盘吃紧就紧急回收一半
	if free, ok := common.AvailableDiskBytes(responseCacheDiskRoot()); ok {
		minFree := response_cache_setting.MinFreeDiskBytes()
		if free < minFree {
			freed := responseCacheDiskEmergencyReclaim(now)
			common.SysError("response cache: host disk is low (" +
				itoa64(free) + " bytes free < " + itoa64(minFree) +
				" floor); emergency-reclaimed " + itoa64(freed) + " bytes")
		}
	}

	common.RecordLoopHeartbeat("response_cache_disk_cleanup")
}

// itoa64 是给日志用的十进制转换（避免为几行日志引入额外依赖）。
func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
