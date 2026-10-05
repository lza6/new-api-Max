package billing_setting

import (
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/setting/config"
)

// TestBillingSettingConcurrentReadWriteSnapshot 复现并守护 be-audit P0：
// 计费热路径读 （GetBillingMode/GetBillingExpr 命中 map）与配置热更新写入
// （updateConfigFromMap 经 json.Unmarshal 整体替换 BillingMode/BillingExpr 裸
// map）并发时，不得触发 data race / 并发 map 读写。
//
// 触发条件在生产无需管理端操作：SyncOptions 默认每 60s 全量反射重载一次配置。
// 修复前本用例在 -race 下报 WARNING: DATA RACE；修复后（快照替换）应 0 race。
//
// 用法：go test -race ./setting/billing_setting/ -run TestBillingSettingConcurrentReadWriteSnapshot
func TestBillingSettingConcurrentReadWriteSnapshot(t *testing.T) {
	cfg := config.GlobalConfig.Get("billing_setting")
	if cfg == nil {
		t.Fatal("billing_setting 配置未注册")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 读侧：持续按模型读计费模式与表达式（命中 map 读路径）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = GetBillingMode("deepseek-v4.1-flash")
			_, _ = GetBillingExpr("deepseek-v4.1-flash")
			_ = GetBillingModeCopy()
			_ = GetBillingExprCopy()
		}
	}()

	// 写侧：持续热更新计费配置（整体替换两个 map）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 300 {
			payload := map[string]string{
				"billing_mode": `{"deepseek-v4.1-flash":"tiered_expr"}`,
				"billing_expr": `{"deepseek-v4.1-flash":"tier(\"request\", fixed(0.01))"}`,
			}
			_ = config.UpdateConfigFromMap(cfg, payload)
			_ = i
		}
		close(stop)
	}()

	wg.Wait()
}

// TestBillingSettingExportRaceWithWrite 回归：反射导出主副本（ExportAllConfigs /
// ConfigToMap 读 map）与热更新写入并发时不得 race —— 由 configReadGuard 与
// configWriteHook 共用同一把写锁保证。
func TestBillingSettingExportRaceWithWrite(t *testing.T) {
	cfg := config.GlobalConfig.Get("billing_setting")
	if cfg == nil {
		t.Fatal("billing_setting 配置未注册")
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = config.GlobalConfig.ExportAllConfigs()
			_, _ = config.ConfigToMap(cfg)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 400 {
			_ = config.UpdateConfigFromMap(cfg, map[string]string{
				"billing_mode": `{"alpha":"tiered_expr","beta":"ratio"}`,
				"billing_expr": `{"alpha":"tier(\"request\", fixed(0.02))"}`,
			})
		}
		close(stop)
	}()

	wg.Wait()
}
