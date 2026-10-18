package training_capture

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
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

// errCannotResolveAccount 无法判定目录归属（有账号列举失败，且没有任何账号
// 能证明自己拥有这个目录）。
//
// 这时**绝不能**猜一个账号写下去：如果该目录其实有数据在某个账号上，
// 猜错就会把同一目录劈到两个账号，分片序号撞车（不可逆）。
// 宁可让这次写入失败、由 worker 重试（有次数上限，见 workerMaxSignalRetries）。
var errCannotResolveAccount = errors.New(
	"multi sink: cannot resolve which account owns this directory (an account failed to list it and no account has data); refusing to guess")

// accountFor 返回某个**目录**应使用的账号下标（带缓存）。
// ok=false 表示所有账号都读不到、无法安全判定。
func (m *MultiAccountSink) accountFor(ctx context.Context, dir string) (int, bool) {
	m.mu.RLock()
	if i, ok := m.dirIdx[dir]; ok {
		m.mu.RUnlock()
		return i, true
	}
	m.mu.RUnlock()

	i, decided := m.resolve(ctx, dir)
	if !decided {
		return 0, false
	}

	m.mu.Lock()
	if len(m.dirIdx) >= multiSinkCacheLimit {
		// 超限整体清空：重新解析的代价是一次 List，远小于无界增长的内存风险。
		// 注意：清空**不会改变归属**——哈希是确定性的，老目录也会被
		// 「已有数据者优先」重新判回原账号。
		m.dirIdx = make(map[string]int, multiSinkCacheLimit)
	}
	m.dirIdx[dir] = i
	m.mu.Unlock()
	return i, true
}

// resolve 决定目录归属：已有数据者优先，其次按路径哈希。
//
// 返回值 decided=false 表示**有账号的列举失败、且没有任何账号能证明自己有数据**，
// 此时不猜（见下方「为什么不能猜」）。
//
// ## 同一目录出现在多个账号上怎么办（修复 N4）
//
// 目录级粘性保证正常情况下只会有一个账号持有某目录。若运维手工迁移/复制过，
// 可能出现两个账号都有同一个目录 —— 此时**不能静默挑第一个**：
// 挑错会让新分片继续写到「旧的那一半」，序号再次撞车。
// 判据是**分片序号**：part 编号按目录递增，序号最大的账号才是真正在写入的那一个。
// 同时打 ERROR 日志，让运维知道数据已经被劈开、需要人工合并。
func (m *MultiAccountSink) resolve(ctx context.Context, dir string) (int, bool) {
	failed := 0
	owner := -1
	ownerSeq, ownerCount := -1, 0

	for i, a := range m.accounts {
		names, err := a.List(ctx, dir)
		if err != nil {
			// 单个账号查询失败不改变归属判断（可能只是瞬时故障）；
			// 继续看下一个，避免把有数据的目录误判成"全新"而分到别的账号。
			failed++
			continue
		}
		if len(names) == 0 {
			continue
		}
		seq := maxPartSeq(names)
		if owner < 0 {
			owner, ownerSeq, ownerCount = i, seq, len(names)
			continue
		}
		// 目录同时存在于多个账号：取分片序号最大的（真正在写入的那个）。
		common.SysError(fmt.Sprintf(
			"training capture: directory %q exists on multiple accounts (account #%d: %d files, max part %d; "+
				"account #%d: %d files, max part %d); keeping writes on the account with the highest part number — "+
				"the copies should be merged/cleaned manually",
			dir, owner+1, ownerCount, ownerSeq, i+1, len(names), seq))
		if seq > ownerSeq || (seq == ownerSeq && len(names) > ownerCount) {
			owner, ownerSeq, ownerCount = i, seq, len(names)
		}
	}

	// 1) 有账号已存在该目录的分片 → 沿用（老目录永不漂移）。
	if owner >= 0 {
		if failed > 0 {
			// 有账号读不到、却已经有人证明自己有数据：沿用有数据的账号。
			// 不在这里 fail-closed，否则任意一个账号的瞬时故障都会让所有目录停止写入。
			common.SysError(fmt.Sprintf(
				"training capture: %d account(s) failed to list directory %q while resolving ownership; "+
					"using account #%d which already has data", failed, dir, owner+1))
		}
		return owner, true
	}

	// 2) **只要有账号读不到，就不能断定这是全新目录。**
	//
	// [修复防御] 这是最容易写错、后果最重的一处。想当然的做法是
	// 「全部账号都失败才不猜」——但真正危险的组合是：
	//   归属账号 A 的 List 瞬时失败（429 / 超时 / 取 token 失败），
	//   而 B 成功且为空（B 本来就没这个目录）→ 若此时按哈希选账号，
	//   约一半概率选中 B → 在 B 写 part-0001，而 A 早已有 part-0001。
	// 同一目录、同名分片、不同内容，**不可逆**。
	//
	// 「B 是空的」证明不了任何事：能证明「这个目录还没人写过」的，
	// 只有**每个**账号都成功读到了空列表。
	if failed > 0 {
		return 0, false
	}
	// 3) 全部账号都读成功且都为空 → 确实是全新目录，按路径哈希分片
	//    （确定性、可复现、无需持久化）。
	return int(crc32.ChecksumIEEE([]byte(dir)) % uint32(len(m.accounts))), true
}

// maxPartSeq 返回分片名里的最大序号（解析不出时返回 -1）。
//
// 分片名形如 `part-0007.jsonl.gz`；序号是**目录级**递增的，
// 因此「序号最大的账号」就是最近仍在写入的那个账号。
func maxPartSeq(names []string) int {
	maxSeq := -1
	for _, name := range names {
		if !strings.HasPrefix(name, "part-") {
			continue
		}
		rest := strings.TrimPrefix(name, "part-")
		end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
		if end == 0 {
			continue
		}
		if end < 0 {
			end = len(rest)
		}
		if n, err := strconv.Atoi(rest[:end]); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	return maxSeq
}

// EnsureDir 在归属账号上创建目录（幂等）。
func (m *MultiAccountSink) EnsureDir(ctx context.Context, path string) error {
	i, ok := m.accountFor(ctx, path)
	if !ok {
		return errCannotResolveAccount
	}
	return m.accounts[i].EnsureDir(ctx, path)
}

// WritePart 写入归属账号。
func (m *MultiAccountSink) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	i, ok := m.accountFor(ctx, dirOf(path))
	if !ok {
		return "", errCannotResolveAccount
	}
	return m.accounts[i].WritePart(ctx, path, data)
}

// List 列举归属账号的分片。
//
// 只问归属账号：同一目录的所有分片必然在同一个账号上（构造所保证），
// 逐个账号都问一遍只会白白放大 API 调用。
func (m *MultiAccountSink) List(ctx context.Context, path string) ([]string, error) {
	i, ok := m.accountFor(ctx, path)
	if !ok {
		return nil, errCannotResolveAccount
	}
	return m.accounts[i].List(ctx, path)
}

// Stat 汇总各账号的容量与统计。
//
// 任一账号报错时**不整体失败**：把能拿到的累加，并把错误如实带上。
//
// [修复防御] 早期实现只在 `TotalBytes == 0` 时才返回错误 —— 那等于
// 「只要还有一个账号能报出容量，就假装所有账号都正常」。
// 两个账号里挂了一个时，管理端会显示一个偏小的用量且**没有任何提示**，
// 运维会以为数据都齐了。现在只要有账号失败就返回错误，
// 同时保留能拿到的部分数据（调用方据此显示「部分数据 + 原因」）。
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
	if len(errs) > 0 {
		return out, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

// ---------- 读侧（管理端浏览 / 预览 / 删除 / 分析 / 导出） ----------

// readerAt 取第 i 个账号的 reader（构造时已校验实现）。
func (m *MultiAccountSink) readerAt(i int) SinkReader {
	return m.accounts[i].(SinkReader)
}

// isNotFound 判定后端是否**明确回答**「没有这个文件」。
//
// 兼容两种哨兵：底层后端契约 `ErrFileNotFound`，以及管理端沿用已久的
// `ErrExportNotFound`（历史实现/其它后端可能仍用后者表示"找不到"）。
// 只有这两者能触发跨账号兜底；429/500/超时一律不兜底。
func isNotFound(err error) bool {
	return errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrExportNotFound)
}

// ReadFile 先问归属账号，**只有确定 not-found 时**才兜底问其它账号。
//
// [修复防御] 早期实现是「归属账号读失败（任何原因）就去别的账号找，找到就改归属」。
// 这会把**瞬时故障**误判成「数据搬家」：归属账号 429/超时 → 在 B 上读到
// 一份旧副本（手工迁移遗留）→ 把 B 记成新归属 → 后续 part 全写到 B，
// 而 A 上还有同名 part。读一次失败，换来一次不可逆的目录劈裂。
//
// 正确规则：只有后端明确回答 `ErrFileNotFound`（数据确实不在这个账号）
// 才允许去别处找。429/500/超时一律原样返回 —— 什么都没证明，就不要改结论。
func (m *MultiAccountSink) ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	dir := dirOf(path)
	if first, ok := m.accountFor(ctx, dir); ok {
		b, err := m.readerAt(first).ReadFile(ctx, path, maxBytes)
		if err == nil {
			return b, nil
		}
		if !isNotFound(err) {
			return nil, err
		}
		return m.readFromOthers(ctx, dir, path, maxBytes, first)
	}

	// 归属判定不出来（有账号的 List 失败且无人有数据）：读是幂等的，
	// 逐个账号试。读到即证明数据在这个账号上，可以纠正归属缓存。
	return m.readFromOthers(ctx, dir, path, maxBytes, -1)
}

// readFromOthers 在除 exclude 之外的账号上找文件。
//
// 全部候选都 not-found → 返回 ErrExportNotFound（保持管理端既有语义）；
// 任何候选报真实错误 → 返回该错误（不能把「读不到」伪装成「不存在」）。
func (m *MultiAccountSink) readFromOthers(
	ctx context.Context, dir, path string, maxBytes int64, exclude int,
) ([]byte, error) {
	var firstRealErr error
	for i := range m.accounts {
		if i == exclude {
			continue
		}
		b, err := m.readerAt(i).ReadFile(ctx, path, maxBytes)
		if err == nil {
			// 文件确实在这里：纠正归属，后续写入跟着数据走。
			m.remember(dir, i)
			return b, nil
		}
		if !isNotFound(err) && firstRealErr == nil {
			firstRealErr = err
		}
	}
	if firstRealErr != nil {
		return nil, firstRealErr
	}
	return nil, ErrExportNotFound
}

// DeleteFile 同 ReadFile：先归属，再兜底；兜底也只在**确定 not-found** 时进行。
//
// 为什么不能「失败就换账号删」：OneDrive 的 404 曾经被吞成 nil，
// 于是在错误账号上删除会**返回成功但文件仍在另一个账号**（见 onedrive_reader.go）。
// 现在 404 映射成 ErrFileNotFound，这里据此区分：
//   - not-found → 目标不在这个账号 → 去别的账号找；
//   - 其他错误 → 原样返回（删除是最不该"猜"的操作）。
//
// 所有账号都明确回答 not-found 时返回 nil：重复点击删除不应报错（幂等）。
func (m *MultiAccountSink) DeleteFile(ctx context.Context, path string) error {
	dir := dirOf(path)
	if first, ok := m.accountFor(ctx, dir); ok {
		err := m.readerAt(first).DeleteFile(ctx, path)
		if err == nil {
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		return m.deleteFromOthers(ctx, dir, path, first)
	}
	return m.deleteFromOthers(ctx, dir, path, -1)
}

// deleteFromOthers 在除 exclude 之外的账号上删除文件。
func (m *MultiAccountSink) deleteFromOthers(ctx context.Context, dir, path string, exclude int) error {
	var firstRealErr error
	for i := range m.accounts {
		if i == exclude {
			continue
		}
		err := m.readerAt(i).DeleteFile(ctx, path)
		if err == nil {
			m.remember(dir, i)
			return nil
		}
		if !isNotFound(err) && firstRealErr == nil {
			firstRealErr = err
		}
	}
	if firstRealErr != nil {
		// 有账号没回答清楚（可能文件就在那里）：不能报告"已删除"。
		return firstRealErr
	}
	// 每个账号都明确说没有这个文件 → 目标已经不存在，幂等成功。
	return nil
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
