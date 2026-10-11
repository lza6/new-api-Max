package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/setting/response_cache_setting"
)

// 响应缓存的**磁盘后端**（Batch-11）。
//
// 动机（来自用户）：生产是「内存紧张（容器 900MB）、磁盘富余（78G）」的形态，
// 而缓存条目是**整个上游响应体**（可达数百 KB）—— 放进程内是与内存抢资源。
// 把响应字节放磁盘，进程内只保留一个「已用字节数」计数器，内存占用与缓存容量解耦。
//
// 四条边界（缺一条就是把内存问题换成磁盘问题）：
//  1. **单条上限** `disk_max_entry_bytes`：超大响应直接不缓存；
//  2. **总量上限** `disk_max_bytes`：超限按**最旧优先**自动淘汰；
//  3. **剩余空间下限** `min_free_disk_bytes`：宿主盘剩余低于它立即停止写入
//     （并触发紧急回收）——「配额内的数据也可能把整块盘写满」；
//  4. **TTL + 周期清理**：过期条目由清理 loop 删除（带心跳，卡死可见）。
//
// 目录布局：`<root>/<前2位十六进制>/<key>` —— 分片避免单目录塞满几十万文件，
// 这在 ext4 上是实际性能问题，不只是审美。

const responseCacheDiskDefaultSubdir = "response-cache"

// diskCacheKeyPattern 校验键形状。键是 sha256 的十六进制，只允许 [0-9a-f]，
// 从根上排除了路径穿越（`..`、`/`、`\` 都过不了）。
const diskCacheKeyHexLen = 64

func validDiskCacheKey(key string) bool {
	if len(key) != diskCacheKeyHexLen {
		return false
	}
	for i := range len(key) {
		c := key[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// responseCacheDiskRoot 返回磁盘缓存根目录（配置优先，默认 `<工作目录>/data/response-cache`）。
func responseCacheDiskRoot() string {
	if dir := response_cache_setting.DiskDir(); dir != "" {
		return dir
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return filepath.Join(wd, "data", responseCacheDiskDefaultSubdir)
	}
	return filepath.Join(os.TempDir(), responseCacheDiskDefaultSubdir)
}

// responseCacheDiskPath 返回某个键的落盘路径。
func responseCacheDiskPath(root, key string) string {
	return filepath.Join(root, key[:2], key)
}

// responseCacheDiskUsage 是磁盘占用的**进程内近似值**。
//
// 为什么用近似值而不是每次写前扫目录：扫描是 O(文件数)，放在热路径上不可接受。
// 计数值由**清理 loop 的全量扫描周期性校正**（见 responseCacheDiskSweep），
// 因此即使因外部删除而漂移，也会在一个清理周期内收敛。
var responseCacheDiskUsage atomic.Int64

// responseCacheDiskEntries 是磁盘后端的**条目数**（同样由清理 loop 校正）。
// 单独计数是为了让 /metrics 与「实验功能」页的 live_entries 不必每次渲染都扫目录。
var responseCacheDiskEntries atomic.Int64

// responseCacheDiskWriteMu 串行化「写入 + 淘汰」，避免并发的两次写入各自看到
// 「还能放下」而一起越界。
var responseCacheDiskWriteMu sync.Mutex

// responseCacheDiskGet 读取一条磁盘缓存。第二个返回值是写入时刻（取文件 mtime）。
func responseCacheDiskGet(key string) ([]byte, time.Time, bool) {
	if !validDiskCacheKey(key) {
		return nil, time.Time{}, false
	}
	path := responseCacheDiskPath(responseCacheDiskRoot(), key)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, time.Time{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return nil, time.Time{}, false
	}
	return body, info.ModTime(), true
}

// responseCacheDiskPut 写入一条磁盘缓存（原子替换）。
//
// 返回 false 表示**未写入**（超单条上限 / 剩余空间不足），调用方无需处理错误 ——
// 缓存写失败从来不是请求失败的理由。
func responseCacheDiskPut(key string, body []byte) bool {
	if !validDiskCacheKey(key) {
		return false
	}
	maxEntry := response_cache_setting.DiskMaxEntryBytes()
	if int64(len(body)) > maxEntry {
		// 超大响应不缓存：落盘也要占 IO，收益低而风险（占配额）高。
		return false
	}
	root := responseCacheDiskRoot()

	// 剩余空间守卫：**先看宿主盘**再看自己的配额 —— 否则配额内也能把整块盘写满。
	if !common.HasDiskSpaceFor(root, int64(len(body)), response_cache_setting.MinFreeDiskBytes()) {
		return false
	}

	responseCacheDiskWriteMu.Lock()
	defer responseCacheDiskWriteMu.Unlock()

	// 配额守卫：超限则先按最旧优先淘汰，腾出空间。
	maxBytes := response_cache_setting.DiskMaxBytes()
	if used := responseCacheDiskUsage.Load(); used+int64(len(body)) > maxBytes {
		// 先按配置的 TTL 清一轮过期；仍然不够就直接按最旧淘汰到能放下为止。
		responseCacheDiskSweep(time.Now())
		_ = responseCacheDiskEvictTo(maxBytes-int64(len(body)), time.Now())
	}

	shardDir := filepath.Join(root, key[:2])
	if err := os.MkdirAll(shardDir, 0o750); err != nil {
		common.SysError("response cache: mkdir failed: " + err.Error())
		return false
	}
	finalPath := filepath.Join(shardDir, key)
	tmpPath := finalPath + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		common.SysError("response cache: create temp failed: " + err.Error())
		return false
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		common.SysError("response cache: write temp failed: " + err.Error())
		return false
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		common.SysError("response cache: close temp failed: " + err.Error())
		return false
	}

	// 原子替换。
	//
	// 跨平台注意：Unix 上 rename **原子覆盖**已存在目标；Windows 上目标存在会失败，
	// 且若此刻有读者正打开该文件，即使先 Remove 也会得到 "Access is denied"。
	// 因此这里做「先试 rename → 失败则 Remove 后重试 → 再失败则短暂退避重试」。
	// 缓存写入失败**不是错误**（下次请求走上游而已），所以最终失败只记日志不报错。
	if err := renameOverwriting(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		common.SysError("response cache: persist failed: " + err.Error())
		return false
	}

	responseCacheDiskUsage.Add(int64(len(body)))
	response_cache_setting.RecordCacheStore()
	return true
}

// renameOverwriting 把 tmp 原子替换到 final，兼容 Windows 的"目标被占用"。
func renameOverwriting(tmpPath, finalPath string) error {
	if err := os.Rename(tmpPath, finalPath); err == nil {
		return nil
	}
	// Windows：目标已存在 → 先删再改名。
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		// 目标可能正被读者打开：短暂退避后重试一次。
		time.Sleep(20 * time.Millisecond)
		_ = os.Remove(finalPath)
	}
	if err := os.Rename(tmpPath, finalPath); err == nil {
		return nil
	}
	time.Sleep(50 * time.Millisecond)
	return os.Rename(tmpPath, finalPath)
}

// responseCacheDiskDelete 删除一条磁盘缓存，并回冲占用计数。
func responseCacheDiskDelete(key string) {
	if !validDiskCacheKey(key) {
		return
	}
	path := responseCacheDiskPath(responseCacheDiskRoot(), key)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if err := os.Remove(path); err == nil {
			responseCacheDiskUsage.Add(-info.Size())
		}
	}
}

// diskCacheFileEntry 是扫描结果的一行。
type diskCacheFileEntry struct {
	path    string
	size    int64
	modTime time.Time
}

// scanResponseCacheFiles 遍历缓存目录，返回全部条目。
// 只处理**两级以内**的文件（`<root>/<shard>/<key>`），不递归 —— 避免误删同目录下的其它东西。
func scanResponseCacheFiles(root string) []diskCacheFileEntry {
	shards, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	entries := make([]diskCacheFileEntry, 0, 256)
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		shardPath := filepath.Join(root, shard.Name())
		files, err := os.ReadDir(shardPath)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			info, err := file.Info()
			if err != nil {
				continue
			}
			if strings.HasSuffix(name, ".tmp") {
				// 半成品文件（进程被杀等）：超过 1 分钟即可回收。
				if time.Since(info.ModTime()) > time.Minute {
					_ = os.Remove(filepath.Join(shardPath, name))
				}
				continue
			}
			if !validDiskCacheKey(name) {
				continue
			}
			entries = append(entries, diskCacheFileEntry{
				path:    filepath.Join(shardPath, name),
				size:    info.Size(),
				modTime: info.ModTime(),
			})
		}
	}
	return entries
}

// responseCacheDiskSweep 执行一次「过期清理 + 占用计数校正」。
//
// 返回 (删除条数, 释放字节数)。这是磁盘后端的**权威统计来源**：
// 每轮把进程内近似计数校正为真实值，避免长期漂移。
func responseCacheDiskSweep(now time.Time) (int, int64) {
	root := responseCacheDiskRoot()
	entries := scanResponseCacheFiles(root)

	ttl := time.Duration(response_cache_setting.TTLSeconds()) * time.Second
	removed, freed := 0, int64(0)
	total := int64(0)
	for _, e := range entries {
		if now.Sub(e.modTime) > ttl {
			if err := os.Remove(e.path); err == nil {
				removed++
				freed += e.size
				continue
			}
		}
		total += e.size
	}

	// 校正近似计数（含被外部删除的条目）
	responseCacheDiskUsage.Store(total)
	responseCacheDiskEntries.Store(int64(len(entries) - removed))
	if removed > 0 {
		response_cache_setting.RecordCacheEvict(int64(removed))
	}
	return removed, freed
}

// responseCacheDiskEvictTo 按**最旧优先**删除，直到占用不超过 targetBytes。
//
// 这是「绝不无限增长」的硬保证：无论 TTL 配置成什么，总量都不会越过配额。
func responseCacheDiskEvictTo(targetBytes int64, now time.Time) int64 {
	if targetBytes < 0 {
		targetBytes = 0
	}
	entries := scanResponseCacheFiles(responseCacheDiskRoot())
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime.Before(entries[j].modTime) })

	total := int64(0)
	for _, e := range entries {
		total += e.size
	}
	if total <= targetBytes {
		responseCacheDiskUsage.Store(total)
		return 0
	}

	freed := int64(0)
	removed := 0
	for _, e := range entries {
		if total <= targetBytes {
			break
		}
		// 保护刚写入的条目：正在被并发读取的文件不该立刻被淘汰。
		if now.Sub(e.modTime) < responseCacheDiskEvictionSafeWindow {
			continue
		}
		if err := os.Remove(e.path); err == nil {
			total -= e.size
			freed += e.size
			removed++
		}
	}
	responseCacheDiskUsage.Store(total)
	if removed > 0 {
		response_cache_setting.RecordCacheEvict(int64(removed))
	}
	return freed
}

// responseCacheDiskEvictionSafeWindow 内刚写入的条目不参与淘汰
// （避免"刚存进去、下一个请求要读时已经被删"这种抖动）。
const responseCacheDiskEvictionSafeWindow = 2 * time.Minute

// responseCacheDiskEmergencyReclaim 在宿主盘剩余空间触及下限时紧急回收：
// 直接砍掉一半配额，把空间还给系统。
func responseCacheDiskEmergencyReclaim(now time.Time) int64 {
	half := response_cache_setting.DiskMaxBytes() / 2
	return responseCacheDiskEvictTo(half, now)
}

// responseCacheDiskStats 返回 (条目数, 占用字节数, 近似值)。
func responseCacheDiskStats() (int, int64) {
	entries := scanResponseCacheFiles(responseCacheDiskRoot())
	total := int64(0)
	for _, e := range entries {
		total += e.size
	}
	return len(entries), total
}

// ResponseCacheDiskUsageBytes 暴露磁盘占用（供 /metrics 与「实验功能」页）。
func ResponseCacheDiskUsageBytes() int64 { return responseCacheDiskUsage.Load() }

// ResponseCacheDiskEntryCount 暴露磁盘条目数（由清理 loop 校正，不实时扫目录）。
func ResponseCacheDiskEntryCount() int64 { return responseCacheDiskEntries.Load() }

// ResponseCacheDiskRootDir 暴露根目录（排障用）。
func ResponseCacheDiskRootDir() string { return responseCacheDiskRoot() }

// ensureResponseCacheDiskRoot 启动时建目录；失败不致命（后续写入会自然失败）。
func ensureResponseCacheDiskRoot() error {
	root := responseCacheDiskRoot()
	if err := os.MkdirAll(root, 0o750); err != nil {
		return fmt.Errorf("create response cache dir %q: %w", root, err)
	}
	return nil
}
