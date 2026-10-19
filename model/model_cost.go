package model

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/operation_setting"
)

// 上游成本（管理员口径，**绝不能暴露给普通用户**）。
//
// ## 为什么需要独立的表 + 历史行
//
// 成本是**按渠道 × 模型**配置的（同一模型在不同渠道的进货价可能不同），
// 而定价会调整。财务口径要求「调价不重写历史」：
//   - 每次调价**追加**一条带 `effective_from` 的记录，旧记录永不修改/删除；
//   - 每个请求结算时按当时生效的记录把成本**快照**写进 logs.cost_quota；
//   - 日报/月报/年报只 SUM 日志里的快照，因此调价前后的利润各自准确。
//
// ## 金额单位
//
// 数据库存 **微元（1e-6 人民币）** 的 int64，避免 float 累加误差。
// 运营者按「人民币元」录入（例如 0.0007 元/次 = 700 微元）。
// 换算成内部额度：`micros / 1e6 / USDExchangeRate * QuotaPerUnit`，
// 与前端展示营收使用同一套换算（站点显示货币为 CNY 时 500000 quota = 7.3 元）。

const (
	ModelCostModePerCall  = "call"
	ModelCostModePerToken = "token"

	modelCostCacheTTL = time.Minute
)

// ModelCost 一条成本配置（按 effective_from 形成历史链）。
type ModelCost struct {
	Id uint `json:"id" gorm:"primaryKey"`

	ChannelId int    `json:"channel_id" gorm:"index:idx_model_cost_lookup,priority:1"`
	Model     string `json:"model" gorm:"type:varchar(128);default:'';index:idx_model_cost_lookup,priority:2"`
	// BillingMode：call（按次）或 token（按输入/输出 token）。
	BillingMode string `json:"billing_mode" gorm:"type:varchar(16);default:'call'"`

	// 以下均为**微元/单位**：CostPerCallMicros 为每次调用成本；
	// CostInputPerMillionMicros / CostOutputPerMillionMicros 为每 100 万 token 成本。
	CostPerCallMicros          int64 `json:"cost_per_call_micros" gorm:"default:0"`
	CostInputPerMillionMicros  int64 `json:"cost_input_per_million_micros" gorm:"default:0"`
	CostOutputPerMillionMicros int64 `json:"cost_output_per_million_micros" gorm:"default:0"`

	// EffectiveFrom 生效时间（unix 秒）。查询「当时生效」= effective_from <= at 的最新一条。
	EffectiveFrom int64 `json:"effective_from" gorm:"bigint;index:idx_model_cost_lookup,priority:3"`

	CreatedBy int       `json:"created_by" gorm:"default:0"`
	Note      string    `json:"note" gorm:"type:varchar(255);default:''"`
	CreatedAt time.Time `json:"created_at"`
}

func (ModelCost) TableName() string { return "model_costs" }

// ModelCostInput 新增一条成本配置的入参（人民币元，最多 6 位小数）。
type ModelCostInput struct {
	ChannelId               int     `json:"channel_id"`
	Model                   string  `json:"model"`
	BillingMode             string  `json:"billing_mode"`
	CostPerCallCNY          float64 `json:"cost_per_call_cny"`
	CostInputPerMillionCNY  float64 `json:"cost_input_per_million_cny"`
	CostOutputPerMillionCNY float64 `json:"cost_output_per_million_cny"`
	EffectiveFrom           int64   `json:"effective_from"`
	Note                    string  `json:"note"`
}

// ValidateModelCostInput 校验并规范化入参。
func ValidateModelCostInput(in *ModelCostInput, now time.Time) error {
	if in == nil {
		return fmt.Errorf("missing cost input")
	}
	if in.ChannelId <= 0 {
		return fmt.Errorf("channel_id must be positive")
	}
	in.Model = strings.TrimSpace(in.Model)
	if len(in.Model) > 128 {
		return fmt.Errorf("model too long")
	}
	switch in.BillingMode {
	case ModelCostModePerCall, ModelCostModePerToken:
	default:
		return fmt.Errorf("billing_mode must be %q or %q", ModelCostModePerCall, ModelCostModePerToken)
	}
	if in.CostPerCallCNY < 0 || in.CostInputPerMillionCNY < 0 || in.CostOutputPerMillionCNY < 0 {
		return fmt.Errorf("cost values must not be negative")
	}
	if in.BillingMode == ModelCostModePerCall && in.CostPerCallCNY <= 0 {
		return fmt.Errorf("cost_per_call_cny must be positive for per-call billing")
	}
	if in.BillingMode == ModelCostModePerToken && in.CostInputPerMillionCNY <= 0 && in.CostOutputPerMillionCNY <= 0 {
		return fmt.Errorf("at least one token cost must be positive for token billing")
	}
	if in.EffectiveFrom <= 0 {
		in.EffectiveFrom = now.Unix()
	}
	// 防止把「元」误填成「分/微元」：单次/百万 token 成本超过 1000 元几乎必然是笔误。
	if in.CostPerCallCNY > 1000 || in.CostInputPerMillionCNY > 100000 || in.CostOutputPerMillionCNY > 100000 {
		return fmt.Errorf("cost value looks too large (expected CNY units)")
	}
	return nil
}

func toMicros(cny float64) int64 {
	if cny <= 0 {
		return 0
	}
	return int64(math.Round(cny * 1e6))
}

// CreateModelCost 追加一条成本配置（**永不修改历史行**）。
func CreateModelCost(in ModelCostInput, createdBy int) (*ModelCost, error) {
	row := &ModelCost{
		ChannelId:                  in.ChannelId,
		Model:                      in.Model,
		BillingMode:                in.BillingMode,
		CostPerCallMicros:          toMicros(in.CostPerCallCNY),
		CostInputPerMillionMicros:  toMicros(in.CostInputPerMillionCNY),
		CostOutputPerMillionMicros: toMicros(in.CostOutputPerMillionCNY),
		EffectiveFrom:              in.EffectiveFrom,
		CreatedBy:                  createdBy,
		Note:                       strings.TrimSpace(in.Note),
	}
	if err := DB.Create(row).Error; err != nil {
		return nil, err
	}
	modelCostCache.Range(func(k, _ any) bool { modelCostCache.Delete(k); return true })
	return row, nil
}

// ListModelCosts 返回成本配置历史（按渠道/模型/生效时间倒序）。
// channelID <= 0 表示不按渠道过滤；model 为空表示不按模型过滤。
func ListModelCosts(channelId int, modelName string, limit int) ([]ModelCost, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := DB.Model(&ModelCost{}).Order("effective_from DESC, id DESC").Limit(limit)
	if channelId > 0 {
		q = q.Where("channel_id = ?", channelId)
	}
	if m := strings.TrimSpace(modelName); m != "" {
		q = q.Where("model = ?", m)
	}
	var rows []ModelCost
	err := q.Find(&rows).Error
	return rows, err
}

// ModelCostTrackingSince 返回最早的生效时间（用于报告标注「成本从何时开始有数据」）。
func ModelCostTrackingSince() (int64, int64) {
	var agg struct {
		Count int64 `gorm:"column:cnt"`
		Since int64 `gorm:"column:since"`
	}
	_ = DB.Model(&ModelCost{}).Select("COUNT(*) AS cnt, COALESCE(MIN(effective_from), 0) AS since").Scan(&agg).Error
	return agg.Since, agg.Count
}

type cachedModelCost struct {
	row      ModelCost
	loadedAt time.Time
}

var modelCostCache sync.Map // key: "<channel>:<model>" -> cachedModelCost

// activeModelCost 查「at 时刻生效」的成本：先查模型级，再回退渠道级（model=""）。
func activeModelCost(channelId int, modelName string, at time.Time) (*ModelCost, bool) {
	modelName = strings.TrimSpace(modelName)
	// 只对「当前时刻」的查询走缓存：历史时刻（测试/补算）必须按 effective_from
	// 精确解析，缓存会返回第一次查询的规则（跨生效点就错了）。
	useCache := time.Since(at) < 2*time.Minute && time.Until(at) < 2*time.Minute
	key := fmt.Sprintf("%d:%s", channelId, modelName)
	if useCache {
		if v, ok := modelCostCache.Load(key); ok {
			c := v.(cachedModelCost)
			if time.Since(c.loadedAt) < modelCostCacheTTL {
				return &c.row, true
			}
		}
	}
	var row ModelCost
	query := func(model string) bool {
		err := DB.Where("channel_id = ? AND model = ? AND effective_from <= ?", channelId, model, at.Unix()).
			Order("effective_from DESC, id DESC").First(&row).Error
		return err == nil
	}
	found := query(modelName)
	if !found && modelName != "" {
		found = query("")
	}
	if !found {
		return nil, false
	}
	if useCache {
		modelCostCache.Store(key, cachedModelCost{row: row, loadedAt: time.Now()})
	}
	return &row, true
}

// RequestCostQuota 计算一次请求的上游成本（额度单位），并返回所用配置的 ID。
//
// ok=false 表示该渠道/模型没有配置成本 —— 此时成本按 0 计，运营报表必须
// 用 cost_configured 明确提示「未配置的模型成本为 0」，不能把利润当成真实值。
func RequestCostQuota(channelId int, modelName string, promptTokens, completionTokens int, at time.Time) (quota int64, ruleId uint, ok bool) {
	cfg, found := activeModelCost(channelId, modelName, at)
	if !found {
		return 0, 0, false
	}
	var micros int64
	switch cfg.BillingMode {
	case ModelCostModePerToken:
		if promptTokens > 0 {
			micros += cfg.CostInputPerMillionMicros * int64(promptTokens) / 1_000_000
		}
		if completionTokens > 0 {
			micros += cfg.CostOutputPerMillionMicros * int64(completionTokens) / 1_000_000
		}
	default: // per call
		micros = cfg.CostPerCallMicros
	}
	if micros <= 0 {
		return 0, cfg.Id, true
	}
	rate := operation_setting.USDExchangeRate
	if rate <= 0 {
		rate = 1
	}
	quotaFloat := float64(micros) / 1e6 / rate * common.QuotaPerUnit
	if quotaFloat <= 0 {
		return 0, cfg.Id, true
	}
	return int64(common.QuotaRound(quotaFloat)), cfg.Id, true
}
