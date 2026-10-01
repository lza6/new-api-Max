package common

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withDiskCache 启用磁盘缓存并指向一个测试临时目录，测试结束还原全局配置。
func withDiskCache(t *testing.T, thresholdMB, maxMB int) string {
	t.Helper()
	prev := GetDiskCacheConfig()
	prevFiles := GetDiskCacheStats().ActiveDiskFiles
	prevUsage := GetDiskCacheStats().CurrentDiskUsageBytes
	dir := t.TempDir()
	SetDiskCacheConfig(DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: thresholdMB,
		MaxSizeMB:   maxMB,
		Path:        dir,
	})
	t.Cleanup(func() {
		SetDiskCacheConfig(prev)
		ResetDiskCacheUsage()
		_ = prevFiles
		_ = prevUsage
	})
	return dir
}

// TestEnsureDiskCacheSpaceEvictsOldestToMakeRoom 生产诉求（2026-09-30）：
// 磁盘缓存达上限时必须「先淘汰最旧文件再落盘」，而不是把大请求体回退内存。
func TestEnsureDiskCacheSpaceEvictsOldestToMakeRoom(t *testing.T) {
	withDiskCache(t, 1, 1) // 上限 1 MiB

	cacheDir := GetDiskCacheDir()
	require.NoError(t, EnsureDiskCacheDir())

	// 造两个「旧」文件（把 ModTime 拨到安全窗口之外）占满容量。
	oldA := filepath.Join(cacheDir, "body-old-a.tmp")
	oldB := filepath.Join(cacheDir, "body-old-b.tmp")
	require.NoError(t, os.WriteFile(oldA, bytes.Repeat([]byte("a"), 400*1024), 0600))
	require.NoError(t, os.WriteFile(oldB, bytes.Repeat([]byte("b"), 400*1024), 0600))
	oldTime := time.Now().Add(-10 * time.Minute)
	require.NoError(t, os.Chtimes(oldA, oldTime, oldTime))
	require.NoError(t, os.Chtimes(oldB, oldTime, oldTime))

	// 统计口径与磁盘一致（模拟运行中累计）。
	ResetDiskCacheUsage()
	IncrementDiskFiles(400 * 1024)
	IncrementDiskFiles(400 * 1024)

	// 现在请求 400 KiB：0.8MB + 0.4MB = 1.2MB > 1MB，必须淘汰最旧文件腾地方。
	assert.True(t, EnsureDiskCacheSpace(400*1024), "淘汰后应能落盘")

	remaining, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(remaining), 1, "至少淘汰了一个最旧文件")

	// 淘汰后统计不应超过上限。
	stats := GetDiskCacheStats()
	assert.LessOrEqual(t, stats.CurrentDiskUsageBytes, GetDiskCacheMaxSizeBytes())
}

// TestEnsureDiskCacheSpaceKeepsInFlightFiles 安全窗口内的文件（可能仍在被读）
// 不得被淘汰；此时如需更多空间只能如实返回 false，交由调用方拒绝请求。
func TestEnsureDiskCacheSpaceKeepsInFlightFiles(t *testing.T) {
	withDiskCache(t, 1, 1)
	cacheDir := GetDiskCacheDir()
	require.NoError(t, EnsureDiskCacheDir())

	fresh := filepath.Join(cacheDir, "body-inflight.tmp")
	require.NoError(t, os.WriteFile(fresh, bytes.Repeat([]byte("x"), 900*1024), 0600))
	// ModTime 保持为「现在」，落在安全窗口内。

	ResetDiskCacheUsage()
	IncrementDiskFiles(900 * 1024)

	assert.False(t, EnsureDiskCacheSpace(400*1024), "在途文件不可删，必须如实返回 false")
	_, err := os.Stat(fresh)
	assert.NoError(t, err, "安全窗口内的文件必须保留")
}

// TestCreateBodyStorageRefusesInsteadOfFallingBackToMemory 关键语义回归：
// 启用磁盘缓存且请求体 ≥ 阈值时，容量不足必须**返回错误**，不得回退内存。
func TestCreateBodyStorageRefusesInsteadOfFallingBackToMemory(t *testing.T) {
	withDiskCache(t, 1, 1)
	cacheDir := GetDiskCacheDir()
	require.NoError(t, EnsureDiskCacheDir())

	fresh := filepath.Join(cacheDir, "body-inflight.tmp")
	require.NoError(t, os.WriteFile(fresh, bytes.Repeat([]byte("x"), 900*1024), 0600))
	ResetDiskCacheUsage()
	IncrementDiskFiles(900 * 1024)

	big := bytes.Repeat([]byte("y"), 2*1024*1024) // 2 MiB ≥ 1 MiB 阈值
	storage, err := CreateBodyStorage(big)
	require.Error(t, err, "容量不足必须明确失败，而不是回退内存")
	assert.Nil(t, storage)
	assert.ErrorIs(t, err, ErrRequestBodyTooLarge)
}

// TestCreateBodyStorageUsesMemoryBelowThreshold 小请求体（< 阈值）走内存，
// 这是有意设计：对小对象落盘是负优化。
func TestCreateBodyStorageUsesMemoryBelowThreshold(t *testing.T) {
	withDiskCache(t, 10, 100)

	small := []byte(`{"model":"x","input":"hi"}`)
	storage, err := CreateBodyStorage(small)
	require.NoError(t, err)
	defer storage.Close()
	assert.False(t, storage.IsDisk(), "小于阈值的请求体应走内存")
}

// TestEnforceDiskCacheCapacityRespectsSafeWindow 周期维护（reserve 0）同样不得
// 触碰安全窗口内的文件。
func TestEnforceDiskCacheCapacityRespectsSafeWindow(t *testing.T) {
	withDiskCache(t, 1, 1)
	cacheDir := GetDiskCacheDir()
	require.NoError(t, EnsureDiskCacheDir())

	fresh := filepath.Join(cacheDir, "body-fresh.tmp")
	require.NoError(t, os.WriteFile(fresh, bytes.Repeat([]byte("x"), 2*1024*1024), 0600))
	ResetDiskCacheUsage()
	IncrementDiskFiles(2 * 1024 * 1024)

	removed, err := EnforceDiskCacheCapacity(DiskCacheEvictionSafeWindow, 0)
	require.NoError(t, err)
	assert.Zero(t, removed, "安全窗口内的文件不得被周期维护删除")
	_, statErr := os.Stat(fresh)
	assert.NoError(t, statErr)
}

// TestCreateBodyStorageSpillsChunkedBodyToDisk 长度未知（chunked）的大请求体
// 必须溢写落盘，而不是整体读进内存。
func TestCreateBodyStorageSpillsChunkedBodyToDisk(t *testing.T) {
	withDiskCache(t, 1, 10)
	require.NoError(t, EnsureDiskCacheDir())

	payload := strings.Repeat("z", 2*1024*1024) // 2 MiB > 1 MiB 阈值
	maxBytes := int64(128 << 20)
	storage, err := CreateBodyStorageFromReader(strings.NewReader(payload), -1, maxBytes)
	require.NoError(t, err)
	defer storage.Close()
	assert.True(t, storage.IsDisk(), "超过阈值的 chunked 请求体必须落盘")
	assert.EqualValues(t, len(payload), storage.Size())
}
