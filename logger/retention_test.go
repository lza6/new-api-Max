package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 这个函数一旦算错就是**删掉不该删的文件**，所以边界要穷举——
// 尤其是「当前正在写入的文件」与「不属于本程序的文件」这两条绝对不能碰。

func mkFile(name string, size int64, age time.Duration) logFileInfo {
	return logFileInfo{
		path:    "/logs/" + name,
		name:    name,
		size:    size,
		modTime: time.Now().Add(-age),
	}
}

func names(files []logFileInfo) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.name)
	}
	return out
}

func TestSelectLogFilesToDeleteKeepsNewestWithinCountLimit(t *testing.T) {
	files := []logFileInfo{
		mkFile("oneapi-a.log", 10, 5*time.Hour),
		mkFile("oneapi-b.log", 10, 4*time.Hour),
		mkFile("oneapi-c.log", 10, 3*time.Hour),
		mkFile("oneapi-d.log", 10, 2*time.Hour),
		mkFile("oneapi-e.log", 10, 1*time.Hour),
	}

	// 上限 3 个、无字节上限、minKeep=0（隔离数量规则）。
	doomed := selectLogFilesToDelete(files, 3, 0, 0, "")

	assert.Equal(t, []string{"oneapi-a.log", "oneapi-b.log"}, names(doomed),
		"应删最旧的两个，保留最新三个")
}

func TestSelectLogFilesToDeleteNeverDeletesActiveFile(t *testing.T) {
	files := []logFileInfo{
		mkFile("oneapi-old.log", 10, 5*time.Hour),
		mkFile("oneapi-active.log", 10, 10*time.Minute),
	}

	// 极端配置：上限 0、minKeep 0 —— 仍不得删除正在写入的文件。
	doomed := selectLogFilesToDelete(files, 0, 0, 0, "/logs/oneapi-active.log")

	assert.Equal(t, []string{"oneapi-old.log"}, names(doomed),
		"当前日志文件必须被保护")
}

func TestSelectLogFilesToDeleteRespectsMinKeep(t *testing.T) {
	files := []logFileInfo{
		mkFile("oneapi-a.log", 10, 5*time.Hour),
		mkFile("oneapi-b.log", 10, 4*time.Hour),
		mkFile("oneapi-c.log", 10, 3*time.Hour),
	}

	// 上限 0 但要求至少保留 2 个：只能删最新的那个之外……
	// 实际语义是「保护最新 2 个」，于是只剩最旧那个可删。
	doomed := selectLogFilesToDelete(files, 0, 0, 2, "")

	assert.Equal(t, []string{"oneapi-a.log"}, names(doomed))
}

func TestSelectLogFilesToDeleteTrimsByBytesFromOldest(t *testing.T) {
	// 4 × 100MB = 400MB，上限 250MB → 需削到 200MB（80%），即删掉最旧的两个。
	files := []logFileInfo{
		mkFile("oneapi-1.log", 100<<20, 4*time.Hour),
		mkFile("oneapi-2.log", 100<<20, 3*time.Hour),
		mkFile("oneapi-3.log", 100<<20, 2*time.Hour),
		mkFile("oneapi-4.log", 100<<20, 1*time.Hour),
	}

	// 数量上限给足（不触发），只看字节规则。
	doomed := selectLogFilesToDelete(files, 100, 250<<20, 0, "")

	assert.Equal(t, []string{"oneapi-1.log", "oneapi-2.log"}, names(doomed),
		"从最旧开始删到低于上限的 80%")
}

func TestSelectLogFilesToDeleteSingleHugeFileIsCaughtByByteLimit(t *testing.T) {
	// 生产实测形态：只有 24 个文件但单个 173MB——光靠数量上限拦不住。
	files := []logFileInfo{
		mkFile("oneapi-huge.log", 173<<20, 2*time.Hour),
		mkFile("oneapi-new.log", 1<<20, 1*time.Hour),
	}

	doomed := selectLogFilesToDelete(files, 100, 100<<20, 0, "")

	assert.Equal(t, []string{"oneapi-huge.log"}, names(doomed),
		"单个超大文件必须能被字节上限单独拦下")
}

func TestSelectLogFilesToDeleteDoesNotDeleteWhenUnderAllLimits(t *testing.T) {
	files := []logFileInfo{
		mkFile("oneapi-a.log", 1<<20, 2*time.Hour),
		mkFile("oneapi-b.log", 1<<20, 1*time.Hour),
	}

	doomed := selectLogFilesToDelete(files, 14, 1<<30, 2, "")

	assert.Empty(t, doomed, "未超限时不得删除任何文件")
}

func TestSelectLogFilesToDeleteHandlesEmptyInput(t *testing.T) {
	assert.Empty(t, selectLogFilesToDelete(nil, 14, 1<<30, 2, ""))
	assert.Empty(t, selectLogFilesToDelete([]logFileInfo{}, 14, 1<<30, 2, ""))
}

func TestSelectLogFilesToDeleteZeroByteLimitMeansUnlimited(t *testing.T) {
	// <=0 表示不限制字节（与项目既有「0 = 不限制」约定一致）。
	files := []logFileInfo{
		mkFile("oneapi-a.log", 1<<40, 2*time.Hour),
		mkFile("oneapi-b.log", 1<<40, 1*time.Hour),
	}

	doomed := selectLogFilesToDelete(files, 100, 0, 0, "")

	assert.Empty(t, doomed)
}

func TestSelectLogFilesToDeleteIsDeterministicOnEqualTimestamps(t *testing.T) {
	// 同一秒内轮转两次会产生相同 modTime；结果必须稳定（不能依赖 map 迭代序）。
	same := time.Now()
	files := []logFileInfo{
		{path: "/logs/oneapi-x.log", name: "oneapi-x.log", size: 1, modTime: same},
		{path: "/logs/oneapi-y.log", name: "oneapi-y.log", size: 1, modTime: same},
		{path: "/logs/oneapi-z.log", name: "oneapi-z.log", size: 1, modTime: same},
	}

	first := names(selectLogFilesToDelete(files, 1, 0, 0, ""))
	for range 10 {
		assert.Equal(t, first, names(selectLogFilesToDelete(files, 1, 0, 0, "")),
			"相同 modTime 下结果必须可复现")
	}
}

func TestIsManagedLogFileOnlyMatchesOwnLogs(t *testing.T) {
	// 管理员可能把别的数据放在同一目录——「清理日志」误删无关数据不可接受。
	assert.True(t, isManagedLogFile("oneapi-20261013101143.log"))
	assert.False(t, isManagedLogFile("backup.sql"))
	assert.False(t, isManagedLogFile("oneapi-20261013101143.log.gz"))
	assert.False(t, isManagedLogFile("my-oneapi-x.log"))
	assert.False(t, isManagedLogFile("oneapi-x.txt"))
	assert.False(t, isManagedLogFile(""))
}

func TestCleanupOldLogFilesOnlyTouchesManagedFiles(t *testing.T) {
	dir := t.TempDir()

	// 造出**超过数量上限**的日志文件（上限 14 → 造 16 个），
	// 否则不会触发删除，用例就测不到「只删自己的文件」这件事。
	for i := range 16 {
		name := filepath.Join(dir, "oneapi-"+string(rune('a'+i))+".log")
		require.NoError(t, os.WriteFile(name, []byte("x"), 0o644))
	}
	unrelated := filepath.Join(dir, "important-data.json")
	require.NoError(t, os.WriteFile(unrelated, []byte("{}"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "oneapi-subdir.log"), 0o755))

	active := filepath.Join(dir, "oneapi-zzz.log")
	require.NoError(t, os.WriteFile(active, []byte("x"), 0o644))

	removed := cleanupOldLogFiles(dir, active)

	assert.Greater(t, removed, 0, "超出数量上限应删除旧日志")

	// 无关文件与子目录必须完好。
	_, err := os.Stat(unrelated)
	assert.NoError(t, err, "无关文件绝不能被删除")
	_, err = os.Stat(filepath.Join(dir, "oneapi-subdir.log"))
	assert.NoError(t, err, "同名的目录不能被当成日志文件删除")

	// 当前日志文件必须仍在。
	_, err = os.Stat(active)
	assert.NoError(t, err, "当前日志文件必须保留")

	// 删除后目录内的日志文件数应收敛到上限附近。
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	logs := 0
	for _, e := range entries {
		if !e.IsDir() && isManagedLogFile(e.Name()) {
			logs++
		}
	}
	assert.LessOrEqual(t, logs, logRetentionMaxFiles+1, "轮转后应回落到上限附近")
}

func TestCleanupOldLogFilesMissingDirectoryIsNotAnError(t *testing.T) {
	// 目录不存在/不可读时只返回 0，绝不 panic 或阻塞启动。
	assert.Equal(t, 0, cleanupOldLogFiles(filepath.Join(t.TempDir(), "nope"), ""))
	assert.Equal(t, 0, cleanupOldLogFiles("", ""))
}
