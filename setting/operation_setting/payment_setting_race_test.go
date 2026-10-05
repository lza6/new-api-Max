package operation_setting

import (
	"sync"
	"testing"

	"github.com/lza6/new-api-Max/setting/config"
)

// TestPaymentSettingConcurrentReadWriteSnapshot 守护配置热更新的并发安全：
// 充值/合规热路径读 AmountOptions/AmountDiscount（map/slice）与配置热更新写入
// （updateConfigFromMap 经反射整体替换 map/slice）并发时，不得触发 data race /
// 并发 map 读写（Go UB，可致不可 recover 的 fatal 杀进程）。
//
// 触发条件在生产无需管理端操作：SyncOptions 默认每 60s 全量反射重载一次配置。
// 用 -race 运行；快照修复前会报 WARNING: DATA RACE，修复后 0 race。
func TestPaymentSettingConcurrentReadWriteSnapshot(t *testing.T) {
	cfg := config.GlobalConfig.Get("payment_setting")
	if cfg == nil {
		t.Fatal("payment_setting 配置未注册")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 读侧：持续读充值金额与折扣 map/slice。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s := GetPaymentSetting()
			_ = s.AmountOptions
			_, _ = s.AmountDiscount[100]
		}
	}()

	// 写侧：持续热更新充值配置（整体替换 map/slice）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 300 {
			_ = config.UpdateConfigFromMap(cfg, map[string]string{
				"amount_options":  `[10,20,50,100,200,500]`,
				"amount_discount": `{"100":0.9,"200":0.85}`,
			})
		}
		close(stop)
	}()

	wg.Wait()
}

// TestPaymentSettingExportRaceWithWrite 回归：反射导出主副本（读 map/slice）与
// 热更新写入并发时不得 race —— configReadGuard 与 configWriteHook 共用同一把锁。
func TestPaymentSettingExportRaceWithWrite(t *testing.T) {
	cfg := config.GlobalConfig.Get("payment_setting")
	if cfg == nil {
		t.Fatal("payment_setting 配置未注册")
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
				"amount_options":  `[10,100]`,
				"amount_discount": `{"100":0.9}`,
			})
		}
		close(stop)
	}()

	wg.Wait()
}
