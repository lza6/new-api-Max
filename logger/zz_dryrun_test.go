package logger

import (
	"os"
	"path/filepath"
	"testing"
)

// 用生产日志目录的**真实形状**做一次 dry-run：只算不删。
func TestDryRunAgainstRealLogDirShape(t *testing.T) {
	dir := os.Getenv("REAL_LOG_DIR")
	if dir == "" {
		t.Skip("REAL_LOG_DIR 未设置")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("读取失败: %v", err)
	}
	var files []logFileInfo
	var total int64
	for _, e := range entries {
		if e.IsDir() || !isManagedLogFile(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		files = append(files, logFileInfo{
			path: filepath.Join(dir, e.Name()), name: e.Name(),
			size: info.Size(), modTime: info.ModTime(),
		})
	}
	doomed := selectLogFilesToDelete(files, logRetentionMaxFiles, logRetentionMaxBytes, logRetentionMinKeep, "")
	var freed int64
	for _, d := range doomed {
		freed += d.size
	}
	t.Logf("文件数=%d 总量=%.1fMB 上限=%d个/%.0fMB",
		len(files), float64(total)/(1<<20), logRetentionMaxFiles, float64(logRetentionMaxBytes)/(1<<20))
	t.Logf("将删除 %d 个文件，释放 %.1fMB，保留 %.1fMB",
		len(doomed), float64(freed)/(1<<20), float64(total-freed)/(1<<20))
	for _, d := range doomed {
		t.Logf("  DEL %s (%.1fMB)", d.name, float64(d.size)/(1<<20))
	}
}
