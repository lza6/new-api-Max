package service

import (
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// §客户端使用统计：从消费日志的 other.client 聚合「各客户端调用量」与「各模型
// 的客户端占比」。数据源是日志本身（client/cache_ratio 已在日志 other 内），
// 免新增持久表；查询按窗口限量 + 进程内短缓存，避免频繁全扫。
//
// 说明：客户端与缓存率不新增独立累加表——日志已是权威持久源（重启不丢），
// 聚合结果加 60s 缓存即可。

const clientStatsCacheTTL = 60 * time.Second

type ClientUsage struct {
	Client string  `json:"client"`
	Count  int64   `json:"count"`
	Share  float64 `json:"share"` // 占比 0-1
}

type ModelClientBreakdown struct {
	Model   string        `json:"model"`
	Total   int64         `json:"total"`
	Clients []ClientUsage `json:"clients"` // 按 count 降序
}

type ClientStatsResult struct {
	WindowDays   int                    `json:"window_days"`
	Total        int64                  `json:"total"`
	Overall      []ClientUsage          `json:"overall"`        // 全体客户端使用占比
	ByModel      []ModelClientBreakdown `json:"by_model"`       // 各模型的客户端占比
	AvgCacheRate float64                `json:"avg_cache_rate"` // 平均缓存命中率 0-1
}

var (
	clientStatsCacheMu sync.Mutex
	clientStatsCache   = map[int]struct {
		expiresAt time.Time
		data      *ClientStatsResult
	}{}
)

// GetClientStats 聚合近 windowDays 天的客户端使用与缓存率（带 60s 缓存）。
func GetClientStats(windowDays int, modelLimit int) (*ClientStatsResult, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	if modelLimit <= 0 {
		modelLimit = 20
	}
	clientStatsCacheMu.Lock()
	if e, ok := clientStatsCache[windowDays]; ok && time.Now().Before(e.expiresAt) {
		data := e.data
		clientStatsCacheMu.Unlock()
		return data, nil
	}
	clientStatsCacheMu.Unlock()

	data, err := aggregateClientStats(windowDays, modelLimit)
	if err != nil {
		return nil, err
	}
	clientStatsCacheMu.Lock()
	clientStatsCache[windowDays] = struct {
		expiresAt time.Time
		data      *ClientStatsResult
	}{expiresAt: time.Now().Add(clientStatsCacheTTL), data: data}
	clientStatsCacheMu.Unlock()
	return data, nil
}

// aggregateClientStats 扫描窗口内 consume 日志的 other，统计客户端与缓存率。
// 只 select (model_name, other) 两列，按窗口限量由 SQL 时间过滤保证。
func aggregateClientStats(windowDays int, modelLimit int) (*ClientStatsResult, error) {
	start := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour).Unix()
	type row struct {
		ModelName string
		Other     string
	}
	var rows []row
	if err := model.LOG_DB.Model(&model.Log{}).
		Select("model_name", "other").
		Where("type = ? AND created_at >= ?", model.LogTypeConsume, start).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	overall := map[string]int64{}
	perModel := map[string]map[string]int64{}
	modelTotal := map[string]int64{}
	var total int64
	var cacheHits, cacheEligible int64

	for _, r := range rows {
		client := "unknown"
		var cacheRatio float64 = -1
		if r.Other != "" {
			var o struct {
				Client     string  `json:"client"`
				CacheRatio float64 `json:"cache_ratio"`
				// cache_ratio 可能以 -1 表示未采集；用指针语义难以判断，这里仅当 >=0 计入
			}
			if common.UnmarshalJsonStr(r.Other, &o) == nil {
				if o.Client != "" {
					client = o.Client
				}
				cacheRatio = o.CacheRatio
			}
		}
		overall[client]++
		total++
		name := r.ModelName
		if name == "" {
			name = "(unknown)"
		}
		if perModel[name] == nil {
			perModel[name] = map[string]int64{}
		}
		perModel[name][client]++
		modelTotal[name]++
		if cacheRatio >= 0 {
			cacheEligible++
			cacheHits += int64(cacheRatio * 100)
		}
	}

	res := &ClientStatsResult{
		WindowDays: windowDays,
		Total:      total,
		Overall:    toClientUsage(overall, total),
	}
	if cacheEligible > 0 {
		res.AvgCacheRate = float64(cacheHits) / float64(cacheEligible) / 100.0
	}

	// 按模型总量排序取 topN，各模型内客户端按 count 降序。
	type mt struct {
		name string
		n    int64
	}
	mts := make([]mt, 0, len(modelTotal))
	for k, v := range modelTotal {
		mts = append(mts, mt{k, v})
	}
	// 简单选择排序（模型数量有限）。
	for i := 0; i < len(mts); i++ {
		for j := i + 1; j < len(mts); j++ {
			if mts[j].n > mts[i].n {
				mts[i], mts[j] = mts[j], mts[i]
			}
		}
	}
	if len(mts) > modelLimit {
		mts = mts[:modelLimit]
	}
	for _, m := range mts {
		res.ByModel = append(res.ByModel, ModelClientBreakdown{
			Model:   m.name,
			Total:   m.n,
			Clients: toClientUsage(perModel[m.name], m.n),
		})
	}
	return res, nil
}

func toClientUsage(m map[string]int64, total int64) []ClientUsage {
	out := make([]ClientUsage, 0, len(m))
	for k, v := range m {
		share := 0.0
		if total > 0 {
			share = float64(v) / float64(total)
		}
		out = append(out, ClientUsage{Client: k, Count: v, Share: share})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Count > out[i].Count {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ResetClientStatsCacheForTest 清空缓存（测试用）。
func ResetClientStatsCacheForTest() {
	clientStatsCacheMu.Lock()
	clientStatsCache = map[int]struct {
		expiresAt time.Time
		data      *ClientStatsResult
	}{}
	clientStatsCacheMu.Unlock()
}
