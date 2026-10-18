package training_capture

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"sync"

	"github.com/lza6/new-api-Max/common"
)

// 多账号云盘后端：把 N 个**同构**后端聚合成一个 sink，按**目录**做粘性分片。
//
// ## 为什么是「目录级粘性」而不是「每次写入轮询」
//
// 分片序号（`part-0001`、`part-0002`…）是按**目录**递增的：worker 上传前会
// `List(dir)` 取现有分片、算出下一个序号。如果同一目录的写入被轮询到不同账号，
// 两个账号各自从 `part-0001` 开始 → **序号撞车**，云盘上出现两份同名不同内容的分片，
// 导出/分析时表现为重复或互相覆盖（不可逆）。
//
// 因此：**一个目录（`日期/用户-模型/模型`）自始至终只落在一个账号上**。
//
// ## 加账号时如何避免老数据「漂移」
//
// 纯哈希（`hash(dir) % N`）在 N 变化时会把老目录判到新账号上 —— 于是新分片
// 写到 B、老分片留在 A，同一目录被劈成两半（序号再次撞车）。
//
// 所以解析顺序是：
//  1. **哪个账号已经有这个目录的数据，就继续用它**（老目录永不漂移）；
//  2. 全新目录才按路径哈希分片（把新数据铺到各账号上）。
//
// 解析结果在进程内缓存（按目录）。重启后缓存为空，但第 1 步会从云盘
// 重新得出同样的答案 —— 因此**跨重启、跨增删账号都稳定**。
//
// ## 代价（诚实说明）
//
// 全新目录首次上传会多出 N-1 次 `List`（用来确认「没人有这个目录」）。
// 这些都是元数据调用、发生在**异步 worker**，不在用户请求路径上；
// 且结果被缓存，同一目录只付一次。

// multiSinkCacheLimit 目录→账号解析结果的缓存上限。
//
// 目录数会随「天数 × 用户 × 模型」增长，必须设上限：超限就整体清空重建
// （重新解析只是一次 List，比引入 LRU 的复杂度划算）。
const multiSinkCacheLimit = 20000

// MultiAccountSink 聚合多个同构后端。
type MultiAccountSink struct {
	accounts []TrainingDataSink

	mu     sync.RWMutex
	dirIdx map[string]int
}

// NewMultiAccountSink 用给定后端构造多账号 sink。
//
// 每个后端**必须同时实现 SinkReader**：管理端的浏览/预览/删除/分析/导出
// 都走 reader 接口，只写不读的后端在这里没有意义（会让管理端功能半残）。
func NewMultiAccountSink(sinks []TrainingDataSink) (*MultiAccountSink, error) {
	if len(sinks) == 0 {
		return nil, errors.New("multi sink: no backends")
	}
	for i, s := range sinks {
		if s == nil {
			return nil, fmt.Errorf("multi sink: backend %d is nil", i+1)
		}
		if _, ok := s.(SinkReader); !ok {
			return nil, fmt.Errorf("multi sink: backend %d (%s) does not implement SinkReader", i+1, s.Name())
		}
	}
	return &MultiAccountSink{accounts: sinks, dirIdx: make(map[string]int)}, nil
}

// Name 返回聚合名（展示用；会带上账号数量，便于管理员确认「B 真的接上了」）。
func (m *MultiAccountSink) Name() string {
	return fmt.Sprintf("%s(x%d)", m.accounts[0].Name(), len(m.accounts))
}

// AccountCount 账号数量（供启动日志/自检）。
func (m *MultiAccountSink) AccountCount() int { return len(m.accounts) }

// dirOf 取路径的目录部分（分片路径形如 `日期/用户-模型/模型/part-0001.jsonl.gz`）。
func dirOf(path string) string {
	p := strings.Trim(path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// accountFor 返回某个**目录**应使用的账号下标（带缓存）。
func (m *MultiAccountSink) accountFor(ctx context.Context, dir string) int {
	m.mu.RLock()
	if i, ok := m.dirIdx[dir]; ok {
		m.mu.RUnlock()
		return i
	}
	m.mu.RUnlock()

	i := m.resolve(ctx, dir)

	m.mu.Lock()
	if len(m.dirIdx) >= multiSinkCacheLimit {
		// 超限整体清空：重新解析的代价是一次 List，远小于无界增长的内存风险。
		m.dirIdx = make(map[string]int, multiSinkCacheLimit)
	}
	m.dirIdx[dir] = i
	m.mu.Unlock()
	return i
}

// resolve 决定目录归属：已有数据者优先，其次按路径哈希。
func (m *MultiAccountSink) resolve(ctx context.Context, dir string) int {
	// 1) 任何账号已存在该目录的分片 → 沿用（老目录永不漂移）。
	for i, a := range m.accounts {
		names, err := a.List(ctx, dir)
		if err != nil {
			// 单个账号查询失败不改变归属判断（可能只是瞬时故障）；
			// 继续看下一个，避免把有数据的目录误判成"全新"而分到别的账号。
			continue
		}
		if len(names) > 0 {
			return i
		}
	}
	// 2) 全新目录 → 按路径哈希分片（稳定、可复现、无需持久化）。
	return int(crc32.ChecksumIEEE([]byte(dir)) % uint32(len(m.accounts)))
}

// EnsureDir 在归属账号上创建目录（幂等）。
func (m *MultiAccountSink) EnsureDir(ctx context.Context, path string) error {
	return m.accounts[m.accountFor(ctx, path)].EnsureDir(ctx, path)
}

// WritePart 写入归属账号。
func (m *MultiAccountSink) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	return m.accounts[m.accountFor(ctx, dirOf(path))].WritePart(ctx, path, data)
}

// List 列举归属账号的分片。
//
// 只问归属账号：同一目录的所有分片必然在同一个账号上（构造所保证），
// 逐个账号都问一遍只会白白放大 API 调用。
func (m *MultiAccountSink) List(ctx context.Context, path string) ([]string, error) {
	return m.accounts[m.accountFor(ctx, path)].List(ctx, path)
}

// Stat 汇总各账号的容量与统计。
//
// 任一账号报错时**不整体失败**：把能拿到的累加，并把错误如实带上
// （否则一个账号故障会让整个管理端显示"未知"，掩盖另一个账号其实正常）。
func (m *MultiAccountSink) Stat(ctx context.Context) (SinkStat, error) {
	out := SinkStat{Provider: m.Name()}
	var errs []string
	for _, a := range m.accounts {
		st, err := a.Stat(ctx)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out.TotalBytes += st.TotalBytes
		out.UsedBytes += st.UsedBytes
		out.RemainingBytes += st.RemainingBytes
		out.FolderCount += st.FolderCount
		out.FileCount += st.FileCount
	}
	if len(errs) > 0 && out.TotalBytes == 0 {
		return out, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

// ---------- 读侧（管理端浏览 / 预览 / 删除 / 分析 / 导出） ----------

// readerAt 取第 i 个账号的 reader（构造时已校验实现）。
func (m *MultiAccountSink) readerAt(i int) SinkReader {
	return m.accounts[i].(SinkReader)
}

// ReadFile 先问归属账号，找不到再兜底问其它账号。
//
// 兜底是必要的：归属解析依赖「目录已有数据」，而单文件路径（分析单个分片）
// 的目录可能就是分片所在目录，正常能命中；但若历史数据因**账号增删或手工
// 迁移**落在非归属账号上，兜底能让它仍然可读，而不是报"文件不存在"。
func (m *MultiAccountSink) ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	first := m.accountFor(ctx, dirOf(path))
	b, err := m.readerAt(first).ReadFile(ctx, path, maxBytes)
	if err == nil {
		return b, nil
	}
	firstErr := err
	for i := range m.accounts {
		if i == first {
			continue
		}
		if b2, err2 := m.readerAt(i).ReadFile(ctx, path, maxBytes); err2 == nil {
			// 记住真实归属，后续不再重复试探。
			m.remember(dirOf(path), i)
			return b2, nil
		}
	}
	return nil, firstErr
}

// DeleteFile 同 ReadFile：先归属，再兜底。
func (m *MultiAccountSink) DeleteFile(ctx context.Context, path string) error {
	first := m.accountFor(ctx, dirOf(path))
	err := m.readerAt(first).DeleteFile(ctx, path)
	if err == nil {
		return nil
	}
	firstErr := err
	for i := range m.accounts {
		if i == first {
			continue
		}
		if err2 := m.readerAt(i).DeleteFile(ctx, path); err2 == nil {
			m.remember(dirOf(path), i)
			return nil
		}
	}
	return firstErr
}

// remember 记录目录的真实归属（读侧发现"数据其实在别的账号"时纠正缓存）。
func (m *MultiAccountSink) remember(dir string, idx int) {
	m.mu.Lock()
	m.dirIdx[dir] = idx
	m.mu.Unlock()
}

// ListTreeWithTruncation 合并各账号的目录树。
//
// 各账号的路径都相对**各自**的 RootPath，布局相同，因此可以直接合并。
// 同一路径不应在两个账号同时存在（目录级粘性所保证）；若因手工迁移出现重复，
// 这里**按路径去重**（保留先出现的），避免管理端出现两行同名文件。
func (m *MultiAccountSink) ListTreeWithTruncation(
	ctx context.Context, root string, maxEntries int,
) ([]TreeEntry, bool, error) {
	seen := make(map[string]struct{})
	out := make([]TreeEntry, 0, 64)
	truncated := false

	for _, a := range m.accounts {
		reader := a.(SinkReader)
		// `ListTreeWithTruncation` 是**可选**能力（不在 SinkReader 主接口里）。
		// 不支持的后端无法证明列举完整 → 保守地把结果标记为截断，
		// 让导出等依赖完整性的调用方 **fail-closed**，而不是产出半包。
		var entries []TreeEntry
		var trunc bool
		var err error
		if t, ok := reader.(interface {
			ListTreeWithTruncation(ctx context.Context, root string, maxEntries int) ([]TreeEntry, bool, error)
		}); ok {
			entries, trunc, err = t.ListTreeWithTruncation(ctx, root, maxEntries)
		} else {
			entries, err = reader.ListTree(ctx, root, maxEntries)
			trunc = true
		}
		if err != nil {
			// 单个账号失败不整体失败：把其它账号的数据给出去，并如实标记不完整。
			truncated = true
			continue
		}
		truncated = truncated || trunc
		for _, e := range entries {
			if _, dup := seen[e.Path]; dup {
				continue
			}
			seen[e.Path] = struct{}{}
			out = append(out, e)
		}
	}
	// 排序让输出稳定（各账号返回顺序不同，合并后必须自己定序）。
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if maxEntries > 0 && len(out) > maxEntries {
		out = out[:maxEntries]
		truncated = true
	}
	return out, truncated, nil
}

// ListTree 丢弃截断标志的便捷包装（与既有接口保持一致）。
func (m *MultiAccountSink) ListTree(ctx context.Context, root string, maxEntries int) ([]TreeEntry, error) {
	entries, _, err := m.ListTreeWithTruncation(ctx, root, maxEntries)
	return entries, err
}

// CountItems 汇总各账号的目录/文件计数（供管理端总览）。
func (m *MultiAccountSink) CountItems(ctx context.Context, maxDepth int) (folders, files int, truncated bool, err error) {
	for _, a := range m.accounts {
		c, ok := a.(interface {
			CountItems(ctx context.Context, maxDepth int) (int, int, bool, error)
		})
		if !ok {
			// 有后端不支持计数 → 总数不可能准确，如实标记不完整。
			truncated = true
			continue
		}
		f, fl, tr, cErr := c.CountItems(ctx, maxDepth)
		if cErr != nil {
			truncated = true
			continue
		}
		folders += f
		files += fl
		truncated = truncated || tr
	}
	return folders, files, truncated, nil
}

// MaxWritePartBytes 取各账号声明上限的**最小值**。
//
// 取最小值而不是最大值：worker 用这个值切分，若按最大上限切，
// 上限较小的账号会收到超限分片并报错。
func (m *MultiAccountSink) MaxWritePartBytes() int {
	limit := 0
	for _, a := range m.accounts {
		if l, ok := a.(PartUploadLimiter); ok {
			if v := l.MaxWritePartBytes(); v > 0 && (limit == 0 || v < limit) {
				limit = v
			}
		}
	}
	if limit == 0 {
		return defaultMaxWritePartBytes
	}
	return limit
}

// logMultiSinkStartup 启动时如实告诉运维「用了几个账号」。
//
// 用户要求「账号 B 也要用上」—— 没有这行日志，运维无法从服务器上
// 确认 B 到底接没接上（只能靠翻管理端 UI）。
func logMultiSinkStartup(m *MultiAccountSink) {
	common.SysLog(fmt.Sprintf(
		"training data capture: multi-account sink active with %d accounts; "+
			"each directory is pinned to one account (part numbering would collide otherwise)",
		m.AccountCount()))
}
