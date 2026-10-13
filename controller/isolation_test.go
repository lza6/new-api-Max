package controller

import (
	"testing"

	"github.com/lza6/new-api-Max/model"
	"github.com/stretchr/testify/require"
)

// 测试隔离：把「全量顺序依赖失败、单独跑通过」这一类既有噪声从根上消除。
//
// ## 问题
//
// `TestMain` 为整个 `controller` 包只建**一个** `:memory:` 库，所有用例共享。
// 任何用例只要写入 `users`/`tokens`/`channels`/`options`，就会污染后续用例；
// 于是出现「单独跑 PASS、全量跑 FAIL」的假回归（台账记录了 9 项这类噪声，
// 每次判回归都要先用 stash 对照才能排除，成本极高）。
//
// ## 做法
//
// 与 `web/src/test-setup.ts` 的 `clearMocks/restoreMocks` 同一思路：让隔离成为
// **默认行为**，而不是要求每个用例作者记得清理。`cleanControllerTables(t)` 在用例
// 结束时把共享库恢复到干净状态。
//
// ## 为什么不用事务回滚
//
// 控制器用例大量走真实 HTTP handler，其中不少路径会**显式提交**或依赖
// `sqlDB.SetMaxOpenConns(1)` 下的连接复用；把整个用例包进一个会被回滚的事务，
// 会在这些路径上产生与生产不一致的行为（提交被吞掉、连接被事务占满）。
// 逐表 DELETE 虽然朴素，但语义与生产完全一致，且不改变任何被测代码的控制流。

// controllerTestTables 是需要在用例之间清空的表，顺序 = 外键/引用关系的逆序。
//
// 刻意用**显式清单**而不是「遍历 sqlite_master 全删」：迁移表（如 schema 版本类）
// 与只读种子数据不应被误删；显式清单也让新增表时必须显式登记，避免悄悄漏清。
var controllerTestTables = []string{
	"site_daily_visitors",
	"audit_logs",
	"event_deliveries",
	"web_request_logs",
	"banned_ips",
	"authz_roles",
	"casbin_rule",
	"system_task_locks",
	"system_tasks",
	"system_instances",
	"perf_metrics",
	"custom_oauth_providers",
	"subscription_plan_grants",
	"subscription_pre_consume_records",
	"user_subscriptions",
	"subscription_orders",
	"subscription_plans",
	"checkins",
	"two_fa_backup_codes",
	"two_fas",
	"setups",
	"prefill_groups",
	"vendors",
	"models",
	"task_plugins",
	"task_events",
	"tasks",
	"quota_data",
	"top_ups",
	"midjourneys",
	"logs",
	"abilities",
	"channel_combos",
	"redemption_usages",
	"redemptions",
	"login_encryption_keys",
	"user_oauth_bindings",
	"passkey_credentials",
	"external_identity_claims",
	"auth_flows",
	"user_sessions",
	"tokens",
	"channels",
	"users",
}

// cleanControllerTables 在用例结束时清空共享测试库。
//
// 用法（需要隔离的用例首行调用）：
//
//	func TestSomething(t *testing.T) {
//	    cleanControllerTables(t)
//	    ...
//	}
func cleanControllerTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		for _, table := range controllerTestTables {
			// 表可能因迁移差异不存在（如可选表）：忽略错误而不是让 cleanup 失败，
			// 否则一个无关的表缺失会把所有用例标记为失败。
			model.DB.Exec("DELETE FROM " + table)
		}
	})
}

// resetControllerOptions 清空 options 表并返回恢复函数。
//
// `options` 单独处理：它承载配置快照，部分用例会改全局开关（如压缩阈值、
// 复杂度路由），而 `model.OptionMap` 是进程内的第二份状态。只清表不够，
// 还要让调用方显式决定是否重置内存快照 —— 语义清楚优于隐式魔法。
func resetControllerOptions(t *testing.T) {
	t.Helper()
	previous := map[string]string{}
	var rows []model.Option
	if err := model.DB.Find(&rows).Error; err == nil {
		for _, r := range rows {
			previous[r.Key] = r.Value
		}
	}
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM options").Error)
		for k, v := range previous {
			model.DB.Create(&model.Option{Key: k, Value: v})
		}
	})
}
