package relay_setting

import (
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/setting/config"
)

// TestRelaySettingConcurrentReadWriteSnapshot 4.2.2 复现：热路径读档位
// （GetUserRateLimitTier 遍历覆盖 map）与配置热更新写入（updateConfigFromMap
// 经 json.Unmarshal 就地改写 map）并发时，不得触发 data race / 并发 map 读写。
//
// 用 -race 运行；旧实现下应报 WARNING: DATA RACE（reflect.Set / mapaccess2 vs
// mapassign）。修复后（快照替换）应 0 race。
func TestRelaySettingConcurrentReadWriteSnapshot(t *testing.T) {
	// 已注册的 relay 配置对象（&relaySetting）。
	cfg := config.GlobalConfig.Get("relay")
	if cfg == nil {
		t.Fatal("relay 配置未注册")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 读侧：持续解析档位（命中 map 遍历路径）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = GetUserRateLimitTier(7, "vip")
			_ = GetGlobalConcurrencyGate()
			_ = GetNonStreamFirstByteTimeout()
		}
	}()

	// 写侧：持续热更新 relay 配置（含覆盖 map 的整体替换）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 300 {
			payload := map[string]string{
				"group_rate_limit_overrides":    `{"vip":{"concurrency":20,"rpm":400}}`,
				"user_rate_limit_overrides":     `{"7":{"concurrency":1,"rpm":30}}`,
				"global_concurrency_limit":      "10",
				"non_stream_first_byte_timeout": "300",
			}
			_ = config.UpdateConfigFromMap(cfg, payload)
			_ = i
		}
		close(stop)
	}()

	wg.Wait()
}

// TestRelaySettingExportRaceWithWrite 4.2.2（补）回归：ExportAllConfigs 反射读取
// relay 主副本（含 map/slice）与热更新写入并发时不得 race。此前 ExportAllConfigs
// 仅持 ConfigManager.mutex，与写入的 settingMu 不是同一把锁 → 仍可 race。
func TestRelaySettingExportRaceWithWrite(t *testing.T) {
	cfg := config.GlobalConfig.Get("relay")
	if cfg == nil {
		t.Fatal("relay 配置未注册")
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 读侧：持续反射导出（读主副本 map/slice）。
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

	// 写侧：持续热更新 relay 配置。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 400 {
			_ = config.UpdateConfigFromMap(cfg, map[string]string{
				"group_rate_limit_overrides":       `{"vip":{"concurrency":20,"rpm":400}}`,
				"user_rate_limit_exempt_models":    `["google-translate"]`,
				"request_compression_threshold_kb": "128",
			})
		}
		close(stop)
	}()

	wg.Wait()
}
