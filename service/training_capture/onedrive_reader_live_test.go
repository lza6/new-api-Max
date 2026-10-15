package training_capture

import (
	"context"
	"strings"
	"testing"
	"time"
)

// OneDrive 读取能力的**真实**集成测试（SinkReader，B4/B7/B9 的基础）。
//
// 未配置凭据时跳过（与 onedrive_live_test.go 同策略）。
//
// 运行：
//
//	TRAINING_ONEDRIVE_TENANT=... TRAINING_ONEDRIVE_CLIENT_ID=... \
//	TRAINING_ONEDRIVE_CLIENT_SECRET=... TRAINING_ONEDRIVE_DRIVE_ID=... \
//	TRAINING_ONEDRIVE_ROOT_PATH=<root> \
//	go test ./service/training_capture/ -run TestOneDriveReaderLive -v
//
// 为什么要真测：读取走的是 **302 重定向 + Bearer token** 的组合，
// 这类行为无法用 mock 证明（mock 会假设「应该怎么工作」，
// 而真实 API 的坑恰恰在假设之外——上传侧的读回就踩过这个）。
func TestOneDriveReaderLive(t *testing.T) {
	cfg, err := LoadOneDriveConfigFromEnv()
	if err != nil {
		t.Skipf("OneDrive 未配置，跳过：%v", err)
	}
	sink := NewOneDriveSink(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 独立探测目录，不与真实采集数据混淆。
	probeDir := "2026-10-15/__reader_inttest"
	const partName = "part-0001.jsonl.gz"

	// 准备：写一个**压缩**分片（与生产格式一致）。
	payload := encodeJSONL([]Message{
		{Role: "user", Content: "读取器集成测试：用户原话"},
		{Role: "assistant", Content: "读取器集成测试：助手回复"},
	})
	compressed, err := gzipCompress(payload)
	if err != nil {
		t.Fatalf("压缩：%v", err)
	}
	if err := sink.EnsureDir(ctx, probeDir); err != nil {
		t.Fatalf("建目录：%v", err)
	}
	if _, err := sink.WritePart(ctx, probeDir+"/"+partName, compressed); err != nil {
		t.Fatalf("上传：%v", err)
	}

	t.Run("ReadFileReturnsExactBytes", func(t *testing.T) {
		got, err := sink.ReadFile(ctx, probeDir+"/"+partName, 1<<20)
		if err != nil {
			t.Fatalf("读取失败：%v", err)
		}
		if len(got) != len(compressed) {
			t.Fatalf("读回长度不一致：%d vs %d", len(got), len(compressed))
		}
		plain, err := gzipDecompress(got, 1<<20)
		if err != nil {
			t.Fatalf("读回的字节不是合法 gzip：%v", err)
		}
		if !strings.Contains(string(plain), "用户原话") {
			t.Fatalf("解压后内容不含原文：%q", plain)
		}
	})

	t.Run("ReadFileRejectsOversizeInsteadOfTruncating", func(t *testing.T) {
		// 传一个极小的 maxBytes：必须报 ErrFileTooLarge，**不能**返回半个文件
		// （半个文件会让分析端把残缺当完整）。
		_, err := sink.ReadFile(ctx, probeDir+"/"+partName, 8)
		if err != ErrFileTooLarge {
			t.Fatalf("超限必须 ErrFileTooLarge，实际 %v", err)
		}
	})

	t.Run("ReadFileMissingReturnsError", func(t *testing.T) {
		_, err := sink.ReadFile(ctx, probeDir+"/part-9999.jsonl.gz", 1<<20)
		if err == nil {
			t.Fatal("读取不存在的文件必须报错")
		}
	})

	t.Run("ListTreeReturnsFullRelativePaths", func(t *testing.T) {
		entries, err := sink.ListTree(ctx, cfg.RootPath, 500)
		if err != nil {
			t.Fatalf("ListTree：%v", err)
		}
		// 生产目录是三级（日期/用户名-用户ID/模型），
		// 这里断言**深层目录也被列到**（下钻前缀丢失的话这部分会缺）。
		var sawProbeFile bool
		for _, e := range entries {
			if e.IsDir {
				continue
			}
			if strings.HasSuffix(e.Path, probeDir+"/"+partName) {
				sawProbeFile = true
				if e.SizeBytes <= 0 {
					t.Errorf("文件大小应为正，实际 %d（path=%s）", e.SizeBytes, e.Path)
				}
				if e.ModifiedAt <= 0 {
					t.Errorf("修改时间应为正，实际 %d", e.ModifiedAt)
				}
			}
		}
		if !sawProbeFile {
			for _, e := range entries {
				t.Logf("  条目: %s dir=%v", e.Path, e.IsDir)
			}
			t.Fatalf("三级目录下的探测文件未被列出 —— 下钻前缀可能丢失（期望后缀 %s）",
				probeDir+"/"+partName)
		}
	})

	t.Run("ListTreeHonorsMaxEntries", func(t *testing.T) {
		entries, err := sink.ListTree(ctx, cfg.RootPath, 3)
		if err != nil {
			t.Fatalf("ListTree：%v", err)
		}
		if len(entries) > 3 {
			t.Fatalf("条目数超过硬顶：%d > 3", len(entries))
		}
	})

	t.Run("DeleteFileIsIdempotent", func(t *testing.T) {
		// 第一次删除成功；第二次（已不存在）也必须返回 nil ——
		// 管理端重复点击不该报错。
		if err := sink.DeleteFile(ctx, probeDir+"/"+partName); err != nil {
			t.Fatalf("首次删除失败：%v", err)
		}
		if err := sink.DeleteFile(ctx, probeDir+"/"+partName); err != nil {
			t.Fatalf("重复删除应幂等（返回 nil），实际 %v", err)
		}
		// 确认真的没了。
		if _, err := sink.ReadFile(ctx, probeDir+"/"+partName, 1<<20); err == nil {
			t.Fatal("删除后仍能读到文件")
		}
	})
}
