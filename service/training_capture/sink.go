package training_capture

import (
	"context"
	"errors"
	"sync"
)

// 云盘存储后端的抽象。
//
// ## 为什么要有这层抽象（用户明确要求）
//
// 「同时你也要扩展一下其他云盘的对接方式等等的哈」——采集核心逻辑不应知道
// OneDrive 的存在。接口足够小（5 个方法），OneDrive / S3 / OSS / WebDAV
// 都能实现；新增后端**不需要改采集 worker**。
//
// ## 接口设计的取舍
//
// - `EnsureDir` 而非 `CreateDir`：**幂等**是采集侧真正需要的语义
//   （目录已存在不该是错误）。并发建同名目录时也必须不报错。
// - `WritePart` 接收整个 `[]byte` 而非 `io.Reader`：分片本身是**有界**的
//   （见 `MaxPartBytes`），整块写入让重试简单且不需要处理 reader 不可重放。
// - `Stat` 允许返回 `ErrSinkNotSupported`：不是所有后端都能报配额，
//   管理员视图要能优雅降级而不是显示错误数字。
// - `List` 用于**恢复分片序号**（重启后不覆盖已上传分片）。

// SinkStat 云盘容量与统计（供管理员视图）。
type SinkStat struct {
	// TotalBytes 总容量（未知为 0）。
	TotalBytes int64
	// UsedBytes 已用容量（未知为 0）。
	UsedBytes int64
	// RemainingBytes 剩余容量（未知为 0）。
	RemainingBytes int64
	// FolderCount 本程序管理的目录数（后端不支持时为 0）。
	FolderCount int
	// FileCount 本程序管理的文件数（后端不支持时为 0）。
	FileCount int
	// Provider 后端名（如 "onedrive"），用于前端展示。
	Provider string
}

// ErrSinkNotSupported 后端不支持该操作（如查询配额）。
//
// 调用方应据此**降级展示**（显示「不支持」）而不是把 0 当成真实值——
// 把 0 当成「云盘满了」会让管理员做出错误决策。
var ErrSinkNotSupported = errors.New("training data sink: operation not supported")

// ErrSinkNotConfigured 后端未配置（缺凭据）。
//
// 与「配置了但连不上」区分：前者是部署问题（管理员需配置），
// 后者是运行时问题（可能重试）。管理端据此给出不同的提示。
var ErrSinkNotConfigured = errors.New("training data sink: not configured")

// TrainingDataSink 云盘后端接口。
type TrainingDataSink interface {
	// Name 后端名（稳定字符串，用于展示与日志）。
	Name() string

	// EnsureDir 确保目录存在（幂等）。已存在必须返回 nil，不视为错误。
	// path 为以 `/` 分隔的相对路径（如 `2026-10-14/alice-7/gpt-4o`）。
	EnsureDir(ctx context.Context, path string) error

	// WritePart 写入一个分片，返回远端引用（如远端路径或 URL）。
	// data 大小由调用方保证不超过后端单次上传上限。
	WritePart(ctx context.Context, path string, data []byte) (string, error)

	// List 列举目录下的**文件名**（不含子目录）。
	// 用于重启后恢复分片序号。目录不存在时返回空切片而非错误
	// （「还没写过」是正常状态，不是故障）。
	List(ctx context.Context, path string) ([]string, error)

	// Stat 返回容量与统计。不支持时返回 ErrSinkNotSupported。
	Stat(ctx context.Context) (SinkStat, error)
}

// ---------- disabled sink ----------

// disabledSink 未配置任何后端时的占位实现。
//
// **不静默成功**：所有写操作返回 `ErrSinkNotConfigured`，
// 让上层能明确告知管理员「云盘没配」，而不是以为采集正常但数据丢了。
type disabledSink struct{}

func (disabledSink) Name() string { return "disabled" }

func (disabledSink) EnsureDir(context.Context, string) error {
	return ErrSinkNotConfigured
}

func (disabledSink) WritePart(context.Context, string, []byte) (string, error) {
	return "", ErrSinkNotConfigured
}

func (disabledSink) List(context.Context, string) ([]string, error) {
	return nil, ErrSinkNotConfigured
}

func (disabledSink) Stat(context.Context) (SinkStat, error) {
	return SinkStat{Provider: "disabled"}, ErrSinkNotConfigured
}

// ---------- 注册表 ----------

var (
	sinkMu       sync.RWMutex
	sinkRegistry                  = map[string]func() (TrainingDataSink, error){}
	activeSink   TrainingDataSink = disabledSink{}
)

// RegisterSink 注册一个后端工厂。供各后端实现包在 init 中调用。
//
// 用注册表而非 switch：新增后端只需在自己的文件里注册，
// 采集核心与接口层一行都不用改（这正是用户要求的「可扩展」）。
func RegisterSink(name string, factory func() (TrainingDataSink, error)) {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	sinkRegistry[name] = factory
}

// ConfigureSink 按名字用已注册的工厂构建并激活后端。
// 未注册的名字返回错误（**明确报错，不静默退回 disabled**——
// 配错名字时静默降级会让管理员以为在采集）。
func ConfigureSink(name string) error {
	sinkMu.Lock()
	defer sinkMu.Unlock()
	if name == "" || name == "disabled" {
		activeSink = disabledSink{}
		return nil
	}
	factory, ok := sinkRegistry[name]
	if !ok {
		return errors.New("training data sink: unknown backend " + name)
	}
	sink, err := factory()
	if err != nil {
		return err
	}
	activeSink = sink
	return nil
}

// SetSinkForTest 注入后端（仅测试用）。
func SetSinkForTest(s TrainingDataSink) func() {
	sinkMu.Lock()
	previous := activeSink
	if s == nil {
		activeSink = disabledSink{}
	} else {
		activeSink = s
	}
	sinkMu.Unlock()
	return func() {
		sinkMu.Lock()
		activeSink = previous
		sinkMu.Unlock()
	}
}

// ActiveSink 返回当前生效的后端（未配置时是 disabledSink，永不为 nil）。
func ActiveSink() TrainingDataSink {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return activeSink
}

// IsSinkConfigured 报告后端是否已配置（供管理员视图判「能否开启总开关」）。
func IsSinkConfigured() bool {
	sink := ActiveSink()
	if sink == nil {
		return false
	}
	_, disabled := sink.(disabledSink)
	return !disabled
}

// RegisteredSinkNames 返回已注册的后端名（供配置校验与文档）。
func RegisteredSinkNames() []string {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	names := make([]string, 0, len(sinkRegistry))
	for name := range sinkRegistry {
		names = append(names, name)
	}
	return names
}
