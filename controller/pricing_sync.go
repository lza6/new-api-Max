package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
)

// pricingSyncUpstream 单个上游价目源（由环境变量 PRICING_SYNC_UPSTREAMS 配置）。
type pricingSyncUpstream struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Path    string `json:"path,omitempty"`
}

// pricingSyncSummary 一次 pricing_sync 任务的执行摘要（幂等、失败仅告警）。
type pricingSyncSummary struct {
	Skipped     bool     `json:"skipped,omitempty"`
	Fetched     []string `json:"fetched,omitempty"`
	Applied     []string `json:"applied,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	AppliedWhen string   `json:"applied_at,omitempty"`
}

// pricingSyncResponse 上游 /api/pricing 响应（与本站公开价目结构对齐）。
type pricingSyncResponse struct {
	Success bool               `json:"success"`
	Data    []map[string]any   `json:"data"`
	Group   map[string]float64 `json:"group_ratio"`
}

// runPricingSyncTaskOnce B5-4：定时同步上游价目到本地 ratio 表（幂等 merge）。
// 无配置时空转（Skipped），失败仅告警不阻断；仅覆盖上游出现的模型，不删除
// 本地既有条目（保护管理员手工配置）。
func runPricingSyncTaskOnce(ctx context.Context) pricingSyncSummary {
	summary := pricingSyncSummary{}
	upstreams, err := pricingSyncUpstreamsFromEnv()
	if err != nil {
		logger.LogWarn(ctx, "pricing_sync invalid PRICING_SYNC_UPSTREAMS: "+err.Error())
		summary.Errors = append(summary.Errors, "invalid config: "+err.Error())
		return summary
	}
	if len(upstreams) == 0 {
		summary.Skipped = true
		return summary
	}

	client := &http.Client{Timeout: 15 * time.Second}
	mergedRatio := map[string]float64{}
	mergedPrice := map[string]float64{}
	mergedCompletion := map[string]float64{}
	for _, up := range upstreams {
		path := up.Path
		if path == "" {
			path = "/api/pricing"
		}
		url := strings.TrimRight(up.BaseURL, "/") + path
		resp, err := client.Get(url)
		if err != nil {
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: fetch failed: %v", up.Name, err))
			continue
		}
		var body pricingSyncResponse
		if err := common.DecodeJson(resp.Body, &body); err != nil {
			resp.Body.Close()
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: decode failed: %v", up.Name, err))
			continue
		}
		resp.Body.Close()
		if !body.Success {
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: upstream returned success=false", up.Name))
			continue
		}
		summary.Fetched = append(summary.Fetched, up.Name)
		for _, item := range body.Data {
			name, _ := item["model_name"].(string)
			if name == "" {
				continue
			}
			if ratio, ok := numField(item, "model_ratio"); ok {
				mergedRatio[name] = ratio
			}
			if price, ok := numField(item, "model_price"); ok {
				mergedPrice[name] = price
			}
			if cr, ok := numField(item, "completion_ratio"); ok {
				mergedCompletion[name] = cr
			}
		}
	}

	if len(mergedRatio) == 0 && len(mergedPrice) == 0 && len(mergedCompletion) == 0 {
		summary.Errors = append(summary.Errors, "no usable pricing data fetched")
		return summary
	}

	// 幂等 merge：取本地当前 map 为基底，仅覆盖上游出现的模型。
	localRatio := ratio_setting.GetModelRatioCopy()
	for name, v := range mergedRatio {
		localRatio[name] = v
	}
	localPrice := ratio_setting.GetModelPriceCopy()
	for name, v := range mergedPrice {
		localPrice[name] = v
	}
	localCompletion := ratio_setting.GetCompletionRatioCopy()
	for name, v := range mergedCompletion {
		localCompletion[name] = v
	}

	// 写回并持久化到 option 表（失败仅告警，不阻断）。
	if err := applyPricingSyncMaps(ratio_setting.ModelRatio2JSONString(), localRatio, ratio_setting.UpdateModelRatioByJSONString, "ModelRatio"); err != nil {
		summary.Errors = append(summary.Errors, err.Error())
	} else {
		summary.Applied = append(summary.Applied, "model_ratio")
	}
	if err := applyPricingSyncMaps(ratio_setting.ModelPrice2JSONString(), localPrice, ratio_setting.UpdateModelPriceByJSONString, "ModelPrice"); err != nil {
		summary.Errors = append(summary.Errors, err.Error())
	} else {
		summary.Applied = append(summary.Applied, "model_price")
	}
	if err := applyPricingSyncMaps(ratio_setting.CompletionRatio2JSONString(), localCompletion, ratio_setting.UpdateCompletionRatioByJSONString, "CompletionRatio"); err != nil {
		summary.Errors = append(summary.Errors, err.Error())
	} else {
		summary.Applied = append(summary.Applied, "completion_ratio")
	}
	summary.AppliedWhen = time.Now().Format(time.RFC3339)
	return summary
}

func pricingSyncUpstreamsFromEnv() ([]pricingSyncUpstream, error) {
	raw := common.GetEnvOrDefaultString("PRICING_SYNC_UPSTREAMS", "")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var upstreams []pricingSyncUpstream
	if err := common.UnmarshalJsonStr(raw, &upstreams); err != nil {
		return nil, err
	}
	valid := upstreams[:0]
	for _, u := range upstreams {
		if u.Name != "" && strings.HasPrefix(u.BaseURL, "http") {
			valid = append(valid, u)
		}
	}
	return valid, nil
}

func numField(item map[string]any, key string) (float64, bool) {
	switch v := item[key].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// applyPricingSyncMaps 把合并后的 map 写回本地并持久化到 option 表。
// optionKey 与 JSON 编码结果保存为一条 option（与现有 ratio 持久化一致）。
func applyPricingSyncMaps(_ string, values map[string]float64, update func(string) error, optionKey string) error {
	encoded, err := common.Marshal(values)
	if err != nil {
		return fmt.Errorf("pricing_sync marshal %s failed: %w", optionKey, err)
	}
	jsonStr := string(encoded)
	if err := update(jsonStr); err != nil {
		return fmt.Errorf("pricing_sync apply %s failed: %w", optionKey, err)
	}
	if err := model.UpdateOption(optionKey, jsonStr); err != nil {
		return fmt.Errorf("pricing_sync persist %s failed: %w", optionKey, err)
	}
	return nil
}
