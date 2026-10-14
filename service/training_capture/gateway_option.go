package training_capture

import (
	"strconv"
	"strings"

	"github.com/lza6/new-api-Max/common"
)

// 管理员总开关的**持久化**（Spec-008 / ADR-008-7）。
//
// 为什么存 options 表而不是环境变量：管理员要在后台随时改、立即生效、
// 重启后保持。环境变量三条都不满足。
//
// 为什么读写在 service 层而不是 controller 层：这层的测试不依赖
// HTTP 栈，且「开关状态」是采集语义的一部分（`IsGranted` 直接用），
// 让 controller 去管它的持久化会把语义与传输层耦合起来。

// GatewayOptionKey 总开关在 `options` 表里的键名。
//
// 命名沿用项目既有习惯（`WaffoPayMethods` 这类自定义开关也是
// 同名风格的驼峰键）。
const GatewayOptionKey = "TrainingDataCaptureGatewayEnabled"

// LoadGatewayEnabledFromDB 从 options 表恢复开关状态。
//
// 返回是否成功读到。**读不到时保持当前值不变**，不修改状态：
// 默认值是 true（用户明确要求「默认打开」），若因读失败而写成 false，
// 会出现「升级后采集莫名停止」且没有任何错误提示。
func LoadGatewayEnabledFromDB() bool {
	common.OptionMapRWMutex.RLock()
	raw := ""
	if common.OptionMap != nil {
		raw = common.OptionMap[GatewayOptionKey]
	}
	common.OptionMapRWMutex.RUnlock()

	raw = strings.TrimSpace(raw)
	if raw == "" {
		// 从未设置过 → 用默认（开），并同步热路径标志。
		SyncHotPathFlag()
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		common.SysError("training data capture: gateway option is not a bool, keeping current value: " + raw)
		return false
	}
	SetGatewayEnabled(enabled)
	return true
}

// SetGatewayEnabledPersisted 设置总开关并写入 options 表（供管理端调用）。
//
// 先写库再改内存：反过来会出现「内存已关但库没写成功」，
// 重启后开关又自己打开了 —— 管理员会认为关闭没生效。
func SetGatewayEnabledPersisted(enabled bool) error {
	if err := saveGatewayOption(enabled); err != nil {
		return err
	}
	SetGatewayEnabled(enabled)
	return nil
}

// saveGatewayOption 写 options 表 + 同步内存 OptionMap。
//
// 两处都要写：`model.UpdateOption` 落库，`common.OptionMap` 是运行时缓存
// （其它模块读配置都走它）。只写库不写缓存会导致本进程内读到旧值。
func saveGatewayOption(enabled bool) error {
	value := strconv.FormatBool(enabled)

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[GatewayOptionKey] = value
	common.OptionMapRWMutex.Unlock()

	if DBUpdater == nil {
		// 未注入落库函数（测试环境）：内存态已更新，视为成功。
		// 不报错，否则单测必须起一个真库才能验证开关语义。
		return nil
	}
	return DBUpdater(GatewayOptionKey, value)
}

// DBUpdater 由 controller 在启动时注入（`model.UpdateOption`）。
//
// 用函数变量而不是直接 import model 的 Option 相关代码：
// 本包只依赖 model 的**自己那两张表**，不把整个 options 机制拉进来
// （那会让本包的测试必须初始化 options 表）。
var DBUpdater func(key, value string) error

// SetDBUpdater 注入落库函数（controller 启动时调用）。
func SetDBUpdater(fn func(key, value string) error) {
	DBUpdater = fn
}
