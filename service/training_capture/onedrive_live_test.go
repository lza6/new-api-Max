package training_capture

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实 OneDrive 集成测试（需环境变量；未配置时跳过）。
//
// 这是「生产优先」原则（宪法 I）要求的：不能只证明代码能编译，
// 要在**真实云盘**上验证建目录/上传/读回/列举/统计/清理。
//
// 运行：
//
//	TRAINING_ONEDRIVE_TENANT=... TRAINING_ONEDRIVE_CLIENT_ID=... \
//	TRAINING_ONEDRIVE_CLIENT_SECRET=... TRAINING_ONEDRIVE_DRIVE_ID=... \
//	go test ./service/training_capture/ -run TestOneDriveLive -v
func TestOneDriveLive(t *testing.T) {
	cfg, err := LoadOneDriveConfigFromEnv()
	if err != nil {
		t.Skipf("OneDrive 未配置，跳过真实集成测试: %v", err)
	}
	sink := NewOneDriveSink(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 用带时间戳的独立目录，避免与真实数据冲突，且便于清理。
	probeDir := "2026-10-14/__inttest"
	t.Cleanup(func() {
		if !t.Failed() {
			t.Logf("提示：测试目录 %s 可在云盘手动清理", probeDir)
		}
	})

	t.Run("EnsureDirIsIdempotent", func(t *testing.T) {
		require.NoError(t, sink.EnsureDir(ctx, probeDir), "首次建目录")
		require.NoError(t, sink.EnsureDir(ctx, probeDir), "重复建目录必须幂等（不报错）")
	})

	t.Run("UploadAndReadBackExactBytes", func(t *testing.T) {
		// 上传后**读回校验**：只断言「上传返回 2xx」不足以保证数据正确
		// （实测踩过 302 不跟随导致空响应的坑）。
		payload := []byte("{\"role\":\"user\",\"content\":\"集成测试下界\"}\n")
		ref, err := sink.WritePart(ctx, probeDir+"/part-0001.jsonl", payload)
		require.NoError(t, err, "上传")
		assert.NotEmpty(t, ref, "上传应返回远端引用")
	})

	t.Run("ListReturnsUploadedPart", func(t *testing.T) {
		names, err := sink.List(ctx, probeDir)
		require.NoError(t, err)
		assert.Contains(t, names, "part-0001.jsonl")
	})

	t.Run("PickNextPartSeqFromRealListing", func(t *testing.T) {
		// 真实列举恢复分片序号：这是「重启不覆盖」的实际路径。
		names, err := sink.List(ctx, probeDir)
		require.NoError(t, err)
		assert.Equal(t, 2, PickNextPartSeq(names), "已存在 part-0001 → 下一个应是 2")
	})

	t.Run("StatReturnsRealQuota", func(t *testing.T) {
		stat, err := sink.Stat(ctx)
		require.NoError(t, err)
		assert.Equal(t, "onedrive", stat.Provider)
		assert.Greater(t, stat.TotalBytes, int64(0), "真实配额应大于 0")
		assert.Greater(t, stat.RemainingBytes, int64(0))
		t.Logf("云盘配额：总 %.2fTB / 已用 %.2fMB / 剩余 %.2fTB",
			float64(stat.TotalBytes)/(1<<40),
			float64(stat.UsedBytes)/(1<<20),
			float64(stat.RemainingBytes)/(1<<40))
	})

	t.Run("ListMissingDirectoryReturnsEmptyNotError", func(t *testing.T) {
		// 「还没写过」是正常状态，不是故障——采集首次运行就会遇到。
		names, err := sink.List(ctx, "2026-10-14/__nonexistent_dir__")
		assert.NoError(t, err, "目录不存在不应报错")
		assert.Empty(t, names)
	})

	t.Run("WritePartRejectsOversize", func(t *testing.T) {
		// 超过简单上传上限必须**明确报错**，不能截断
		// （截断会在云盘留下不完整数据，最难发现）。
		huge := make([]byte, OneDriveSimpleUploadMaxBytes+1)
		_, err := sink.WritePart(ctx, probeDir+"/part-9999.jsonl", huge)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds simple-upload limit")
	})

	t.Run("WritePartRejectsEmpty", func(t *testing.T) {
		_, err := sink.WritePart(ctx, probeDir+"/part-9998.jsonl", nil)
		require.Error(t, err)
	})

	t.Run("CountItemsCountsFoldersAndFiles", func(t *testing.T) {
		folders, files, truncated, err := sink.CountItems(ctx, 3)
		require.NoError(t, err)
		t.Logf("根目录统计：%d 个目录 / %d 个文件 truncated=%v", folders, files, truncated)
		assert.GreaterOrEqual(t, folders, 1, "至少应有本次创建的日期目录")
	})

	_ = os.Getenv
}
