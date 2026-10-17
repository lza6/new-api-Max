package controller

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/lza6/new-api-Max/service/training_capture"
)

// errSinkCannotReportTruncation 当前云盘后端不支持「如实报告列举是否被截断」。
//
// 与导出链路同一个判据：**无法证明完整就不当作完整**。
// 这里不把「不支持」当成「没截断」，否则聚合出的每日数字可能只是前 N 条。
var errSinkCannotReportTruncation = errors.New(
	"the configured sink cannot report listing truncation, so daily totals may be incomplete")

// errCacheTypeMismatch 缓存的 singleflight 返回值类型与期望不符。
//
// 正常情况下不可能发生（T 是编译期确定的类型）。定义成显式错误而不是
// 返回零值，是为了**不把异常变成「看起来正常的空数据」**。
var errCacheTypeMismatch = errors.New(
	"training data cache: unexpected value type from cache loader")

// 管理端云盘查询的**进程内缓存 + 请求合并**。
//
// ## 为什么必须缓存（这不是优化，是保护）
//
// 管理端页面会轮询：总览 5s 一次、每日历史 30s 一次。而这些查询背后是
// **云盘 API 调用**（`CountItems` 要走几十~几百次目录列举；`ListTree`
// 在大部署上可涉及上万条）。两个后果：
//
//  1. 管理员只要开着页面，就会持续消耗云盘的**速率配额**；
//  2. 采集 worker 用的是**同一份配额** —— 被管理端打满后采集上传会收到
//     429/限流，表现为「采集莫名开始失败」，而根因在另一个页面。
//
// 同时并发请求会放大问题：多开几个标签页 = 同时发起多份全量列举。
// 因此这里不仅缓存，还用 singleflight 把**并发请求合并成一次**。
//
// ## 为什么缓存「聚合结果」而不是原始目录树
//
// 原始树在大部署上可达十万条（每条约 100 字节 → 10MB 级）。把它驻留内存
// 与宪法 V 的「内存有硬上限」相冲突，而管理端真正需要的只是聚合后的
// 每天几个数字（≤ 数百条、几十 KB）。缓存小对象、丢弃大对象。

const (
	// trainingDailyStatsTTL 每日采集聚合的缓存时长。
	//
	// 取 5 分钟：这张表的用途是「哪几天采了多少」，不需要秒级新鲜；
	// 而每次刷新的代价是一次全量目录列举。5 分钟能把典型浏览
	// （轮询 30s）的云盘调用量压到约 1/10。
	trainingDailyStatsTTL = 5 * time.Minute

	// trainingSinkCountsTTL 目录/文件计数的缓存时长。
	//
	// 取 1 分钟：总览每 5s 轮询，但「有多少个文件夹」这种数字
	// 分钟级足够；队列/在传字节等实时指标走的是内存计数，不受此影响。
	trainingSinkCountsTTL = time.Minute

	// trainingCloudListTimeout 一次云盘列举/计数的**总时限**。
	//
	// 缓存加载不挂在任何请求的 context 上（它是共享资源），因此必须自带
	// 超时；否则云盘挂起时 singleflight 会让**所有**等待者永久阻塞 ——
	// 后续每个轮询请求都排在同一次永不返回的加载后面，管理页会一直转圈。
	//
	// 取 60s：正常列举在秒级到十几秒；给足余量但仍保证「挂死会自愈」。
	trainingCloudListTimeout = 60 * time.Second
)

// timedCache 一个极小的 TTL 缓存，并用 singleflight 合并并发未命中。
//
// 为什么不用现成的缓存库：这里只有两个标量/小切片缓存点，引入依赖与
// 生命周期管理不值得；且必须**在 Redis 未启用时也生效**（管理端打满
// 云盘配额的风险不该取决于是否配了 Redis）。
type timedCache[T any] struct {
	mu       sync.RWMutex
	value    T
	ok       bool
	loadedAt time.Time
	ttl      time.Duration
	sf       singleflight.Group
}

func newTimedCache[T any](ttl time.Duration) *timedCache[T] {
	return &timedCache[T]{ttl: ttl}
}

// get 返回缓存值；未命中时调用 load 并缓存。
//
// load 返回 error 时**不缓存**（错误不该被记住 5 分钟 —— 云盘恢复后
// 应该立刻能看到数据，而不是继续展示旧错误）。
func (c *timedCache[T]) get(load func() (T, error)) (T, error) {
	c.mu.RLock()
	if c.ok && time.Since(c.loadedAt) < c.ttl {
		v := c.value
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	// 合并并发未命中：多个标签页/多个管理员同时刷新只打一次云盘。
	res, err, _ := c.sf.Do("load", func() (any, error) {
		// 进入临界区后再确认一次：可能已有别的请求刚填好。
		c.mu.RLock()
		if c.ok && time.Since(c.loadedAt) < c.ttl {
			v := c.value
			c.mu.RUnlock()
			return v, nil
		}
		c.mu.RUnlock()

		v, lerr := load()
		if lerr != nil {
			return v, lerr
		}
		c.mu.Lock()
		c.value = v
		c.ok = true
		c.loadedAt = time.Now()
		c.mu.Unlock()
		return v, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	// singleflight 的返回值是 any，这里做一次受控断言。
	//
	// **断言失败必须报错，不能返回零值 + nil error**：那会让调用方把
	// 「0 个文件 / 空日期列表」当成真实数据展示给管理员 ——
	// 这正是本项目反复在修的「自信但错误的数字」。
	// （正常情况下 T 是具体结构体，断言不会失败；这里只为不静默。）
	v, asserted := res.(T)
	if !asserted {
		var zero T
		return zero, errCacheTypeMismatch
	}
	return v, nil
}

// 两个缓存实例。`ctx` 不参与缓存键 —— 云盘是全局资源，与调用者无关。
var (
	trainingDailyStatsCache = newTimedCache[trainingDailyStatsResult](trainingDailyStatsTTL)
	trainingSinkCountsCache = newTimedCache[sinkCounts](trainingSinkCountsTTL)
)

// trainingDailyStatsResult 每日聚合的缓存载荷。
type trainingDailyStatsResult struct {
	Days      []training_capture.DailyCaptureStat
	Truncated bool
}

// sinkCounts 目录/文件计数的缓存载荷。
type sinkCounts struct {
	Folders   int
	Files     int
	Truncated bool
}
