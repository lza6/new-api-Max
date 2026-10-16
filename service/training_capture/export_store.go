package training_capture

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 导出产物的本地存储 + 断点续传（Range）服务。
//
// ## 为什么产物落在本地磁盘而不是回写到云盘
//
// 导出是**一次性、短命**的产物（管理员下载完即可清理）。回写云盘要额外
// 走上传链路（配额、限速、可能的失败重试），且下载时又要从云盘读回，
// 多两次往返。落本地磁盘：打包即写、下载即读、清理即删 —— 简单直接。
//
// 目录由 `TRAINING_EXPORT_DIR` 指定（默认 `<工作目录>/data/training-exports`）。

// ExportStore 导出产物的本地磁盘存储。
type ExportStore struct {
	root    string
	enabled bool
}

// defaultExportDir 默认导出目录。
const defaultExportDir = "data/training-exports"

// ErrExportStoreDisabled 导出存储不可用（目录无法创建等）。
var ErrExportStoreDisabled = errors.New("training export: store not available")

var (
	activeExportStore *ExportStore
	exportStoreOnce   sync.Once
)

// InitExportStore 初始化导出存储（幂等）。返回错误以便调用方决定是否禁用导出。
func InitExportStore() error {
	root := strings.TrimSpace(common.GetEnvOrDefaultString("TRAINING_EXPORT_DIR", ""))
	if root == "" {
		root = defaultExportDir
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		activeExportStore = &ExportStore{enabled: false}
		return fmt.Errorf("invalid TRAINING_EXPORT_DIR: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		activeExportStore = &ExportStore{enabled: false}
		return fmt.Errorf("cannot create training export dir: %w", err)
	}
	activeExportStore = &ExportStore{root: abs, enabled: true}
	common.SysLog("training export store: local disk at " + abs)
	return nil
}

// ExportStoreInstance 返回当前导出存储（永不为 nil）。
//
// **惰性初始化**：首次调用时按 `TRAINING_EXPORT_DIR` 建目录。这样导出功能
// 不需要在启动路径里额外挂一个初始化调用（启动装配在 controller/router 之外，
// 本功能不应侵入）。已由测试注入过的 store 不会被覆盖。
func ExportStoreInstance() *ExportStore {
	if activeExportStore != nil {
		return activeExportStore
	}
	exportStoreOnce.Do(func() {
		if err := InitExportStore(); err != nil {
			common.SysError("training export store init failed: " + err.Error())
		}
	})
	if activeExportStore == nil {
		activeExportStore = &ExportStore{enabled: false}
	}
	return activeExportStore
}

// Enabled 报告存储是否可用。
func (s *ExportStore) Enabled() bool { return s != nil && s.enabled }

// Dir 返回导出根目录（测试用）。
func (s *ExportStore) Dir() string { return s.root }

// PathFor 返回某导出的产物绝对路径（`export-<id>.tar`）。
//
// exportID 是数据库自增主键（不可预测性不作为安全边界 —— 直链 token 才是；
// 这里只用于命名）。用固定前缀 + 数字，无任何用户可控输入参与路径拼接，
// 结构上排除目录穿越。
func (s *ExportStore) PathFor(exportID uint) string {
	return filepath.Join(s.root, fmt.Sprintf("export-%d.tar", exportID))
}

// Open 打开某导出的产物（不存在返回 ErrExportNotFound）。
func (s *ExportStore) Open(exportID uint) (*os.File, os.FileInfo, error) {
	if !s.Enabled() {
		return nil, nil, ErrExportStoreDisabled
	}
	full := s.PathFor(exportID)
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, ErrExportNotFound
		}
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, ErrExportNotFound
	}
	return f, info, nil
}

// Remove 删除某导出的产物（失败忽略 —— 清理是尽力而为）。
func (s *ExportStore) Remove(exportID uint) {
	if !s.Enabled() {
		return
	}
	_ = os.Remove(s.PathFor(exportID))
}

// ---------- Range 解析（纯逻辑，单独测试） ----------

// ByteRange 一个已解析的字节区间（闭区间 [Start, End]，单位为字节）。
type ByteRange struct {
	Start int64
	End   int64
}

// ErrRangeNotSatisfiable 请求的 Range 无法满足（越界）。
// 调用方据此返回 416 + `Content-Range: bytes */<size>`。
var ErrRangeNotSatisfiable = errors.New("training export: range not satisfiable")

// ErrRangeMalformed Range 头格式非法。调用方应忽略 Range 按全量响应
// （RFC 7233：非法 Range 头应被忽略，而不是报错）。
var ErrRangeMalformed = errors.New("training export: malformed range header")

// ParseSingleRange 解析单区间 Range 头。
//
// ## 支持的形态与边界（逐条对应 RFC 7233 §2.1/§3.1）
//
//   - `bytes=0-499`          → [0,499]
//   - `bytes=500-`           → [500,size-1]（开区间到末尾）
//   - `bytes=-500`           → 末 500 字节（后缀范围）
//   - 越界（start >= size）  → ErrRangeNotSatisfiable → 416
//   - end 超出 size 时**夹取**到 size-1（RFC 允许）
//   - 多区间（含逗号）       → 明确拒绝：返回 ErrRangeMalformed。
//     RDE 多区间需要 multipart/byteranges 响应，实现复杂度高而收益低；
//     本实现只支持单区间，对多区间**显式**拒绝而不是静默只取第一个
//     （静默会让客户端以为拿到了全部，实际只拿到一段）。
//   - 非法格式（非 bytes 单位、空、非数字） → ErrRangeMalformed（按忽略处理）
//   - start > end 且都合法位置 → 视为不可满足（416）
//
// 返回的区间保证 0 <= Start <= End <= size-1。
func ParseSingleRange(rangeHeader string, size int64) (ByteRange, error) {
	header := strings.TrimSpace(rangeHeader)
	if header == "" {
		return ByteRange{}, ErrRangeMalformed
	}
	// 单位必须是 bytes（大小写不敏感）。
	unit, spec, ok := strings.Cut(header, "=")
	if !ok || !strings.EqualFold(strings.TrimSpace(unit), "bytes") {
		return ByteRange{}, ErrRangeMalformed
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ByteRange{}, ErrRangeMalformed
	}
	// 多区间：显式拒绝（见上）。
	if strings.Contains(spec, ",") {
		return ByteRange{}, ErrRangeMalformed
	}
	startStr, endStr, ok := strings.Cut(spec, "-")
	if !ok {
		return ByteRange{}, ErrRangeMalformed
	}
	startStr = strings.TrimSpace(startStr)
	endStr = strings.TrimSpace(endStr)

	// 后缀范围：`-N` 表示末 N 字节。
	if startStr == "" {
		if endStr == "" {
			return ByteRange{}, ErrRangeMalformed
		}
		suffixLen, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || suffixLen < 0 {
			return ByteRange{}, ErrRangeMalformed
		}
		if suffixLen == 0 {
			// `-0` 请求末 0 字节 → 不可满足。
			return ByteRange{}, ErrRangeNotSatisfiable
		}
		if size == 0 {
			return ByteRange{}, ErrRangeNotSatisfiable
		}
		if suffixLen > size {
			suffixLen = size
		}
		return ByteRange{Start: size - suffixLen, End: size - 1}, nil
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return ByteRange{}, ErrRangeMalformed
	}
	if size == 0 || start >= size {
		return ByteRange{}, ErrRangeNotSatisfiable
	}
	// 开区间到末尾。
	if endStr == "" {
		return ByteRange{Start: start, End: size - 1}, nil
	}
	end, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < 0 {
		return ByteRange{}, ErrRangeMalformed
	}
	if end < start {
		return ByteRange{}, ErrRangeNotSatisfiable
	}
	if end >= size {
		end = size - 1 // 夹取（RFC 允许）
	}
	return ByteRange{Start: start, End: end}, nil
}

// IfRangeMatches 判断是否应尊重 Range 头（RFC 7233 §3.2）。
//
// 语义：客户端带 `If-Range` 时，只有当**资源自条件以来未变**才应用 Range，
// 否则返回整个资源（200）。这样续传在文件被替换后不会拼出损坏的文件。
//
// 支持两种 If-Range 值：
//   - strong ETag（形如 `"abc"`）：必须与当前 ETag **完全相等**（含引号）。
//   - HTTP-date：资源修改时间**不晚于**该时间则匹配。
//
// 空 If-Range → 视为匹配（没有条件，直接应用 Range）。
// 无法解析的值 → 视为不匹配（保守：返回全量而非冒险续传）。
func IfRangeMatches(ifRange string, modTime time.Time, etag string) bool {
	cond := strings.TrimSpace(ifRange)
	if cond == "" {
		return true
	}
	// ETag 形态：以 `"` 开头（含弱 ETag `W/"..."`）。
	if strings.HasPrefix(cond, "\"") || strings.HasPrefix(cond, "W/") {
		return cond == etag
	}
	// HTTP-date：解析为时间后比较（modTime 不晚于条件时间则未变）。
	t, err := http.ParseTime(cond)
	if err != nil {
		return false
	}
	// 允许 1 秒粒度误差：文件系统的 mtime 精度与 HTTP-date 的秒级精度不同。
	return !modTime.After(t.Add(time.Second))
}

// ---------- 产物清理（盲点 B7） ----------
//
// 导出产物是**几十 GB 级**的本地文件，且只增不减：
// `training_data_exports` 每次导出落一个 tar，`training_data_export_downloads`
// 每次下载落一行。这正是本项目反复踩过的「磁盘黑洞」
// （记忆里生产磁盘真凶就是无界增长）。
//
// 策略：按**文件 mtime** 清理超过 TTL 的产物，并对目录设**总量硬上限**
// （超限时从最旧的开始删）。两条同时生效 —— 只靠 TTL 挡不住
// 「一天导出 100 次」；只靠总量挡不住「长期不导出但留着一堆老文件」。

const (
	// exportArtifactTTL 产物默认保留时长。
	//
	// 默认 7 天：够管理员下载/重下，又不至于长期占盘。
	// 注意**只清本地产物**，云盘上的源分片不受影响（那是采集数据本身）。
	exportArtifactTTL = 7 * 24 * time.Hour

	// exportArtifactMaxBytes 产物目录总量上限（默认 20GB）。
	//
	// 超过就从**最旧的**开始删，直到降到 target ratio。
	// 取 20GB：单次导出通常几十 MB～几 GB，够留若干次；
	// 而生产磁盘余量有限，必须有硬顶。
	exportArtifactMaxBytes = int64(20) << 30

	// exportArtifactTargetRatio 超限后回落到上限的百分比。
	// 一次多删一点，避免刚清完又立刻超限、每次都要清。
	exportArtifactTargetRatio = 0.8

	// exportFilePrefix 本模块产物的文件名前缀。
	// **只清自己产生的文件**：目录可由运维指向共享路径，
	// 删别人的文件是不可接受的风险。
	exportFilePrefix = "export-"

	// exportArtifactSuffix 已完成产物的后缀。
	// 构建期的临时文件以 `.tar.tmp` 结尾，**不匹配**这个后缀 ——
	// 否则清理会在 Linux 上毁掉进行中的导出（见清理逻辑的说明）。
	exportArtifactSuffix = ".tar"
)

// exportArtifactInfo 一个产物的元信息。
type exportArtifactInfo struct {
	name    string
	path    string
	size    int64
	modTime time.Time
}

// selectExportArtifactsToDelete 是**纯函数**：给定产物列表与限制，算出该删哪些。
//
// 抽成纯函数是为了能不碰文件系统就把边界测穷举 ——
// 这个函数算错就是删错文件，或该删的没删（磁盘照样涨）。
//
// 返回按「先该删」排序的列表（最旧优先）。
func selectExportArtifactsToDelete(
	files []exportArtifactInfo,
	now time.Time,
	ttl time.Duration,
	maxBytes int64,
	targetRatio float64,
) []exportArtifactInfo {
	if len(files) == 0 {
		return nil
	}
	// 先按 mtime 升序（最旧在前）。
	sorted := make([]exportArtifactInfo, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].modTime.Before(sorted[j].modTime)
	})

	doomed := make([]exportArtifactInfo, 0, len(sorted))
	kept := make([]exportArtifactInfo, 0, len(sorted))

	// 第一轮：TTL 过期的一律删。
	var total int64
	for _, f := range sorted {
		if ttl > 0 && now.Sub(f.modTime) > ttl {
			doomed = append(doomed, f)
			continue
		}
		kept = append(kept, f)
		total += f.size
	}

	// 第二轮：总量仍超限 → 从最旧的开始继续删，直到回落到 target。
	if maxBytes > 0 && total > maxBytes {
		target := int64(float64(maxBytes) * targetRatio)
		for len(kept) > 0 && total > target {
			oldest := kept[0]
			kept = kept[1:]
			total -= oldest.size
			doomed = append(doomed, oldest)
		}
	}
	return doomed
}

// CleanupExportArtifacts 清理本地导出产物。返回 (删除数, 释放字节, error)。
//
// **best-effort**：单个文件删失败只记录，不中断整体
// （部分清理也比不清理好）。
func CleanupExportArtifacts(now time.Time) (int, int64, error) {
	store := ExportStoreInstance()
	if store == nil || !store.enabled {
		return 0, 0, nil // 未启用导出存储：无事可做
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return 0, 0, fmt.Errorf("read export dir: %w", err)
	}

	files := make([]exportArtifactInfo, 0, len(entries))
	for _, e := range entries {
		// 只认**已完成的**产物（`export-<id>.tar`）。
		//
		// [修复防御] 构建期会先写 `export-<id>.tar.tmp` 再 rename ——
		// 那个 `.tmp` 也匹配 `export-` 前缀。若把它算进清理范围，
		// 一次长时间导出（目录又恰好超了总量上限）可能写到一半被清理任务删掉：
		// Linux 上文件被 unlink 后 rename 失败 → **数小时的导出白做**；
		// Windows 上删除失败，两平台行为不一致、极难排查。
		//
		// 用**白名单后缀**而不是「排除 .tmp」：将来新增别的临时后缀时，
		// 白名单继续挡得住，黑名单挡不住。
		if e.IsDir() || !strings.HasPrefix(e.Name(), exportFilePrefix) ||
			!strings.HasSuffix(e.Name(), exportArtifactSuffix) {
			// 只碰自己的、已完成产物：目录可能被运维指向共享路径。
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, exportArtifactInfo{
			name:    e.Name(),
			path:    filepath.Join(store.root, e.Name()),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	doomed := selectExportArtifactsToDelete(
		files, now, exportArtifactTTL, exportArtifactMaxBytes, exportArtifactTargetRatio)

	var deleted int
	var freed int64
	for _, f := range doomed {
		if err := os.Remove(f.path); err != nil {
			common.SysError(fmt.Sprintf(
				"training export cleanup: failed to remove %s: %s", f.name, err.Error()))
			continue
		}
		deleted++
		freed += f.size
	}
	return deleted, freed, nil
}

// ---------- 测试接缝 ----------

// setExportStoreForTest 把导出存储指向临时目录（仅测试用）。
// 返回恢复函数。
func setExportStoreForTest(root string) func() {
	prev := activeExportStore
	activeExportStore = &ExportStore{root: root, enabled: true}
	return func() { activeExportStore = prev }
}
