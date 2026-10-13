package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
)

// 日志文件保留策略。
//
// ## 为什么需要它（生产实测）
//
// `SetupLogger` 每累计 100 万行就**新建**一个 `oneapi-<时间戳>.log`，但**从不删除**
// 旧文件。生产实例实测：24 个文件、单个最大 **173MB**、目录合计 **865MB**，
// 且只增不减——这是服务器磁盘增长的真实来源（不是图片、不是镜像）。
//
// 无上限的增长迟早写满磁盘，而日志属于「排障用、过期即无用」的数据，
// 保留窗口足够即可。
//
// ## 两条独立的上限（缺一条都不够）
//
//  1. **按数量**：只保留最近 N 个文件。防「高频轮转产生大量小文件」。
//  2. **按总字节**：目录总量超上限时从最旧开始删到低于上限的 80%。
//     防「单个文件巨大」（生产那个 173MB 的文件，光靠数量上限拦不住）。
//
// 两条都设：任一条单独都会留下明显浪费。
const (
	// logRetentionMaxFiles 最多保留的日志文件数。
	logRetentionMaxFiles = 14
	// logRetentionMaxBytes 日志目录总字节上限（1GB）。
	logRetentionMaxBytes = int64(1) << 30
	// logRetentionTargetRatio 按字节回收时削到的目标比例。
	// 削到刚好等于上限会导致「下一行日志又超限→再删一次」，故一次多削一些。
	logRetentionTargetRatio = 0.8
	// logRetentionMinKeep 无论上限如何，至少保留最新这几个文件——
	// 排障时「当前日志」永远不能被删掉。
	logRetentionMinKeep = 2
)

// logFileInfo 用于排序的日志文件元信息。
type logFileInfo struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

// logRetentionPattern 只匹配本程序产生的日志文件名（`oneapi-<时间戳>.log`）。
// **刻意不删任何其它文件**：管理员可能把别的数据放在同一目录，
// 一个「清理日志」的功能误删无关数据是不可接受的。
const logFilePrefix = "oneapi-"
const logFileSuffix = ".log"

func isManagedLogFile(name string) bool {
	return strings.HasPrefix(name, logFilePrefix) && strings.HasSuffix(name, logFileSuffix)
}

// selectLogFilesToDelete 是**纯函数**：给定文件列表，算出该删哪些。
//
// 抽成纯函数是为了能不碰真实文件系统就把边界测穷举——
// 这个函数一旦算错就是删错用户的文件。
//
// `activePath` 是当前正在写入的文件，**永不出现在返回值里**。
func selectLogFilesToDelete(
	files []logFileInfo,
	maxFiles int,
	maxBytes int64,
	minKeep int,
	activePath string,
) []logFileInfo {
	// 按修改时间**从新到旧**排序（同名时间戳下退化为按名字比较，保证确定性）。
	ordered := make([]logFileInfo, 0, len(files))
	for _, f := range files {
		ordered = append(ordered, f)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].modTime.Equal(ordered[j].modTime) {
			return ordered[i].name > ordered[j].name
		}
		return ordered[i].modTime.After(ordered[j].modTime)
	})

	// 保护名单：正在写入的 + 最新 minKeep 个。
	protected := make(map[string]bool, minKeep+1)
	if activePath != "" {
		protected[activePath] = true
	}
	for i := range ordered {
		if i < minKeep {
			protected[ordered[i].path] = true
		}
	}

	doomed := make(map[string]logFileInfo)
	// 规则 1：数量上限。
	kept := 0
	for _, f := range ordered {
		if protected[f.path] {
			kept++
			continue
		}
		if kept < maxFiles {
			kept++
			continue
		}
		doomed[f.path] = f
	}

	// 规则 2：总字节上限。基于「保留集」计算，避免与规则 1 重复计数。
	var keptBytes int64
	remaining := make([]logFileInfo, 0, len(ordered))
	for _, f := range ordered {
		if _, bad := doomed[f.path]; bad {
			continue
		}
		keptBytes += f.size
		remaining = append(remaining, f)
	}
	if maxBytes > 0 && keptBytes > maxBytes {
		target := int64(float64(maxBytes) * logRetentionTargetRatio)
		// 从**最旧**开始继续删，直到低于目标值。protected 同样跳过。
		for i := len(remaining) - 1; i >= 0 && keptBytes > target; i-- {
			f := remaining[i]
			if protected[f.path] {
				continue
			}
			doomed[f.path] = f
			keptBytes -= f.size
		}
	}

	out := make([]logFileInfo, 0, len(doomed))
	for _, f := range doomed {
		out = append(out, f)
	}
	// 稳定顺序便于测试与日志。
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// cleanupOldLogFiles 按保留策略删除过期日志。返回删除的文件数。
//
// 出错只记日志不返回错误：清理失败绝不该影响主流程（日志是辅助设施）。
func cleanupOldLogFiles(dir string, activePath string) int {
	if dir == "" {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	files := make([]logFileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isManagedLogFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, logFileInfo{
			path:    filepath.Join(dir, entry.Name()),
			name:    entry.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}
	doomed := selectLogFilesToDelete(files, logRetentionMaxFiles, logRetentionMaxBytes, logRetentionMinKeep, activePath)
	removed := 0
	for _, f := range doomed {
		if err := os.Remove(f.path); err != nil {
			continue
		}
		removed++
	}
	if removed > 0 {
		common.SysLog(fmt.Sprintf("log retention: removed %d old log file(s) from %s", removed, dir))
	}
	return removed
}
