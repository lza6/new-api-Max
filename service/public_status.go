package service

import (
	"context"
	"sort"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"

	"gorm.io/gorm"
)

// 公开状态页聚合（G7）。
//
// ## 只报告**可证明的事实**
//
// 这个端点是公开的，因此它的每一条输出都必须能被本进程真实观测到，且口径要写在
// 字段名/文档里让前端如实转述：
//   - 组件状态 = 本次聚合时刻对主库/日志库/Redis 的**真实探测结果**；
//   - 请求量 / 错误数 = 近 24 小时 `logs` 表的**真实计数**（消费日志 type=2、错误
//     日志 type=5），并明确带上「窗口」与「该口径所需的日志开关是否开启」；
//   - 后台任务 = `common.LoopHeartbeatSnapshot` 里各 loop 的**真实心跳时间戳**。
//
// **刻意不输出**的东西：uptime 百分比 / SLA / 可用性历史 —— 本进程没有采样过可用性，
// 任何此类数字都是编的。同理不返回探测的原始错误文本（可能含 DSN 主机名、内网地址）。
//
// ## 为什么必须走 SWR 缓存
//
// 公开端点会被爬虫以任意频率打：本聚合虽然只做 3 次 ping + 1 次 GROUP BY，
// 但仍属于「请求路径上的 DB 访问」。因此复用 `siteVisitorCache`（stale-while-revalidate
// + 单飞）——只要手里有值就立刻返回旧值并后台刷新，请求路径上不会阻塞在聚合上，
// 且并发多少请求都只有一个刷新在跑。代价是数字可能比 TTL 旧一轮，对状态展示可接受。
const (
	publicStatusCacheTTL = 30 * time.Second
	// publicStatusWindowSeconds 是流量指标的统计窗口（24 小时）。**必须随响应返回**，
	// 否则读者不知道那两个数字是哪段时间的。
	publicStatusWindowSeconds = int64(24 * 3600)
	// publicStatusProbeTimeout 是依赖探测（ping）的单次超时。公开请求不该被一个
	// hang 住的依赖拖住；超时即视为不可用。
	publicStatusProbeTimeout = 2 * time.Second
	// publicStatusLoopActivityWindow 是「后台任务最近是否在跑」的判定窗口。
	// 它**就是这条结论的定义**（"最近 N 秒内有没有任何 loop 心跳"），
	// 因此必须随响应返回并原样展示，而不是让读者以为它是每个任务自己的周期。
	publicStatusLoopActivityWindow = 5 * time.Minute
)

// 组件状态取值（前端按此映射文案，不要在两端各写一套不同的词）。
const (
	PublicStatusStateOperational = "operational"
	PublicStatusStateDegraded    = "degraded"
	PublicStatusStateDown        = "down"
	PublicStatusStateDisabled    = "disabled"
	PublicStatusStateUnknown     = "unknown"
)

// 组件 key（前端按 key 映射展示名）。
const (
	PublicStatusComponentDatabase      = "database"
	PublicStatusComponentLogDatabase   = "log_database"
	PublicStatusComponentRedis         = "redis"
	PublicStatusComponentBackgroundJob = "background_jobs"
)

// PublicStatusComponent 是一个被检查项的状态。**不含任何细节文本**：
// 探测失败的原因可能包含主机名/端口/内部地址，公开页面不返回它。
type PublicStatusComponent struct {
	Key   string `json:"key"`
	State string `json:"state"`
	// Shared 仅对 log_database 有意义：与主库同一个实例时为 true
	// （此时不再重复探测，状态由主库那次探测决定）。
	Shared bool `json:"shared,omitempty"`
}

// PublicStatusTraffic 是近一个窗口内的请求量口径。
//
// 三个指针字段为 nil 表示**该数字不可用**（对应日志开关未开启，或本次统计失败）——
// 用 null 表达「不可用」，而不是用 0 冒充「没有请求」。成功率只在请求量与错误数
// **都**可用且分母非 0 时给出。
type PublicStatusTraffic struct {
	WindowSeconds         int64    `json:"window_seconds"`
	Requests              *int64   `json:"requests"`
	Errors                *int64   `json:"errors"`
	SuccessRate           *float64 `json:"success_rate"`
	ConsumeLoggingEnabled bool     `json:"consume_logging_enabled"`
	ErrorLoggingEnabled   bool     `json:"error_logging_enabled"`
}

// PublicStatusLoop 是一个后台 loop 的最近心跳。
type PublicStatusLoop struct {
	Name      string `json:"name"`
	LastRunAt int64  `json:"last_run_at"`
}

// PublicStatusBackground 是后台常驻任务的观测视图。
type PublicStatusBackground struct {
	// ActivityWindowSeconds 是「最近是否在跑」的判定窗口（见常量说明）。
	ActivityWindowSeconds int64 `json:"activity_window_seconds"`
	// MostRecentRunAt 是全部 loop 里最近一次心跳（0 表示还没有任何心跳）。
	MostRecentRunAt int64              `json:"most_recent_run_at"`
	Loops           []PublicStatusLoop `json:"loops"`
}

// PublicStatus 是公开状态页的完整响应。
type PublicStatus struct {
	Status        string                  `json:"status"`
	Version       string                  `json:"version"`
	StartedAt     int64                   `json:"started_at"`
	UptimeSeconds int64                   `json:"uptime_seconds"`
	GeneratedAt   int64                   `json:"generated_at"`
	Components    []PublicStatusComponent `json:"components"`
	Traffic       PublicStatusTraffic     `json:"traffic"`
	Background    PublicStatusBackground  `json:"background"`
}

// publicStatusCache 复用站点统计那套 SWR + 单飞缓存（见 siteVisitorCache 的说明）。
var publicStatusCache = newSiteVisitorCache[PublicStatus](publicStatusCacheTTL)

// GetPublicStatus 返回状态页数据（经 SWR 缓存；请求路径上不阻塞在聚合上）。
func GetPublicStatus() (*PublicStatus, error) {
	return publicStatusCache.get(computePublicStatus)
}

// ResetPublicStatusCacheForTest 清空状态页缓存。
//
// 缓存是**进程级**的，而测试在同一进程里跑：不重置会让后一个用例读到前一个用例的
// 快照（进而断言到旧数字）。生产代码没有任何调用方。
func ResetPublicStatusCacheForTest() { publicStatusCache.store(nil) }

// computePublicStatus 执行一次完整聚合（只由缓存的后台刷新/冷启动调用）。
func computePublicStatus() (*PublicStatus, error) {
	now := time.Now()
	nowUnix := now.Unix()

	components := []PublicStatusComponent{
		{Key: PublicStatusComponentDatabase, State: publicStatusDatabaseState()},
	}
	logDBComponent := PublicStatusComponent{Key: PublicStatusComponentLogDatabase}
	switch {
	case model.LOG_DB == nil:
		logDBComponent.State = PublicStatusStateDown
	case model.LOG_DB == model.DB:
		// 主库与日志库同实例：重复探测没有额外信息量，状态跟随主库。
		logDBComponent.State = PublicStatusStateOperational
		logDBComponent.Shared = true
	default:
		logDBComponent.State = publicStatusPingState(model.LOG_DB)
	}
	components = append(components, logDBComponent)
	components = append(components, PublicStatusComponent{
		Key:   PublicStatusComponentRedis,
		State: publicStatusRedisState(),
	})

	background := publicStatusBackground()
	components = append(components, PublicStatusComponent{
		Key:   PublicStatusComponentBackgroundJob,
		State: publicStatusBackgroundState(background, nowUnix),
	})

	status := PublicStatusStateOperational
	for _, c := range components {
		if c.State == PublicStatusStateDown || c.State == PublicStatusStateDegraded {
			status = PublicStatusStateDegraded
			break
		}
	}

	return &PublicStatus{
		Status:        status,
		Version:       common.Version,
		StartedAt:     common.StartTime,
		UptimeSeconds: nowUnix - common.StartTime,
		GeneratedAt:   nowUnix,
		Components:    components,
		Traffic:       publicStatusTraffic(nowUnix),
		Background:    background,
	}, nil
}

// publicStatusDatabaseState 探测主库连通性。
func publicStatusDatabaseState() string {
	return publicStatusPingState(model.DB)
}

// publicStatusPingState 在超时窗口内 ping 一次数据库，返回组件状态。
// 失败原因**不进返回值**（见 PublicStatusComponent 的说明）。
func publicStatusPingState(db *gorm.DB) string {
	if db == nil {
		return PublicStatusStateDown
	}
	sqlDB, err := db.DB()
	if err != nil {
		return PublicStatusStateDown
	}
	ctx, cancel := context.WithTimeout(context.Background(), publicStatusProbeTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return PublicStatusStateDown
	}
	return PublicStatusStateOperational
}

// publicStatusRedisState 探测 Redis。未配置（未启用）时如实报告 disabled ——
// 它没有被使用，与"不可用"是两件事，混为一谈会让状态页误报。
func publicStatusRedisState() string {
	if !common.RedisEnabled || common.RDB == nil {
		return PublicStatusStateDisabled
	}
	ctx, cancel := context.WithTimeout(context.Background(), publicStatusProbeTimeout)
	defer cancel()
	if err := common.RDB.Ping(ctx).Err(); err != nil {
		return PublicStatusStateDown
	}
	return PublicStatusStateOperational
}

// publicStatusBackground 汇总各 loop 的心跳（按名字排序，保证同一状态输出稳定）。
func publicStatusBackground() PublicStatusBackground {
	snapshot := common.LoopHeartbeatSnapshot()
	out := PublicStatusBackground{
		ActivityWindowSeconds: int64(publicStatusLoopActivityWindow / time.Second),
		Loops:                 make([]PublicStatusLoop, 0, len(snapshot)),
	}
	for name, lastRunAt := range snapshot {
		out.Loops = append(out.Loops, PublicStatusLoop{Name: name, LastRunAt: lastRunAt / 1000})
		if lastRunAt/1000 > out.MostRecentRunAt {
			out.MostRecentRunAt = lastRunAt / 1000
		}
	}
	sort.Slice(out.Loops, func(i, j int) bool { return out.Loops[i].Name < out.Loops[j].Name })
	return out
}

// publicStatusBackgroundState 把心跳汇总成一个状态：窗口内有过心跳即 operational。
// 完全没有登记过任何 loop 时是 unknown（不是 down）—— 那只能说明"没有接入观测"，
// 不能说后台任务坏了。
func publicStatusBackgroundState(background PublicStatusBackground, nowUnix int64) string {
	if len(background.Loops) == 0 {
		return PublicStatusStateUnknown
	}
	if nowUnix-background.MostRecentRunAt > background.ActivityWindowSeconds {
		return PublicStatusStateDegraded
	}
	return PublicStatusStateOperational
}

// publicStatusTraffic 统计近一个窗口的请求量与错误数。
//
// 统计**受日志开关门控**：消费日志关掉就没有请求量，错误日志关掉就没有错误数。
// 这两种情况下对应字段返回 null，成功率也一并留空 —— 绝不能把"没在记录"显示成
// "0 个错误 / 100% 成功"。这是本文件最重要的一条纪律。
func publicStatusTraffic(nowUnix int64) PublicStatusTraffic {
	out := PublicStatusTraffic{
		WindowSeconds:         publicStatusWindowSeconds,
		ConsumeLoggingEnabled: common.LogConsumeEnabled,
		ErrorLoggingEnabled:   common.ErrorLogEnabledValue(),
	}
	if model.LOG_DB == nil {
		return out
	}

	types := make([]int, 0, 2)
	if out.ConsumeLoggingEnabled {
		types = append(types, model.LogTypeConsume)
	}
	if out.ErrorLoggingEnabled {
		types = append(types, model.LogTypeError)
	}
	if len(types) == 0 {
		return out
	}

	counts, err := model.CountLogsByTypeSince(nowUnix-publicStatusWindowSeconds, types...)
	if err != nil {
		// 统计失败：保持"不可用"（null），而不是退化成 0 让人误以为没有流量。
		common.SysError("public status: count logs by type failed: " + err.Error())
		return out
	}
	if out.ConsumeLoggingEnabled {
		requests := counts[model.LogTypeConsume]
		out.Requests = &requests
	}
	if out.ErrorLoggingEnabled {
		errors := counts[model.LogTypeError]
		out.Errors = &errors
	}
	// 成功率 = 非错误日志 / 全部已记录日志。口径详见前端文案：它统计的是**落了日志的
	// 请求**，未走到日志环节就失败（例如鉴权被拒）的请求不在分母里。
	if out.Requests != nil && out.Errors != nil {
		if total := *out.Requests + *out.Errors; total > 0 {
			rate := float64(*out.Requests) / float64(total)
			out.SuccessRate = &rate
		}
	}
	return out
}
