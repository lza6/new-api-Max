package training_capture

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 导出产物清理（盲点 B7）。这是纯函数测试 —— 算错就是删错文件，
// 或该删的没删（磁盘照样涨）。两种错误都要有用例。
func TestSelectExportArtifactsToDelete(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	hour := time.Hour
	day := 24 * hour

	f := func(name string, sizeMB int64, age time.Duration) exportArtifactInfo {
		return exportArtifactInfo{
			name: name, path: "/x/" + name,
			size: sizeMB << 20, modTime: now.Add(-age),
		}
	}

	t.Run("TTL 过期的被删，未过期的保留", func(t *testing.T) {
		files := []exportArtifactInfo{
			f("export-1.tar", 10, 10*day), // 过期
			f("export-2.tar", 10, 1*day),  // 未过期
		}
		doomed := selectExportArtifactsToDelete(files, now, 7*day, 0, 0.8)
		if len(doomed) != 1 || doomed[0].name != "export-1.tar" {
			t.Fatalf("应只删过期的 export-1，实际 %+v", doomed)
		}
	})

	t.Run("总量超限时从最旧的开始删到回落线", func(t *testing.T) {
		// 四个各 1GB、都未过期；上限 3GB → 目标是 2.4GB → 需删到剩 2 个。
		files := []exportArtifactInfo{
			f("export-a.tar", 1024, 4*day),
			f("export-b.tar", 1024, 3*day),
			f("export-c.tar", 1024, 2*day),
			f("export-d.tar", 1024, 1*day),
		}
		const maxBytes = int64(3) << 30
		doomed := selectExportArtifactsToDelete(files, now, 0, maxBytes, 0.8)

		// 释放量必须够：删掉的 ≥ 1GB。
		var freed int64
		for _, d := range doomed {
			freed += d.size
		}
		if freed < int64(1)<<30 {
			t.Fatalf("释放不足，只删了 %d 字节", freed)
		}
		// **必须是最旧的先删** —— 否则会把刚生成的产物删掉、留着老文件。
		if len(doomed) == 0 || doomed[0].name != "export-a.tar" {
			t.Fatalf("应从最旧的 export-a 开始删，实际 %+v", doomed)
		}
	})

	t.Run("TTL 与总量都不触发时不删任何东西", func(t *testing.T) {
		files := []exportArtifactInfo{
			f("export-1.tar", 1, 1*day),
			f("export-2.tar", 1, 2*day),
		}
		doomed := selectExportArtifactsToDelete(files, now, 7*day, 100<<30, 0.8)
		if len(doomed) != 0 {
			t.Fatalf("不该删任何文件，实际 %+v", doomed)
		}
	})

	t.Run("空输入不 panic", func(t *testing.T) {
		if got := selectExportArtifactsToDelete(nil, now, day, 1<<30, 0.8); got != nil {
			t.Fatalf("空输入应返回 nil，实际 %+v", got)
		}
	})

	t.Run("ttl 为 0 表示不按时间清理", func(t *testing.T) {
		files := []exportArtifactInfo{f("export-old.tar", 1, 365*day)}
		if got := selectExportArtifactsToDelete(files, now, 0, 0, 0.8); len(got) != 0 {
			t.Fatalf("ttl<=0 且无总量限制时不该删，实际 %+v", got)
		}
	})

	t.Run("清理结果互不重叠（不会既删又留）", func(t *testing.T) {
		files := []exportArtifactInfo{
			f("export-1.tar", 2048, 10*day), // TTL 过期
			f("export-2.tar", 2048, 2*day),
			f("export-3.tar", 2048, 1*day),
		}
		const maxBytes = int64(3) << 30
		doomed := selectExportArtifactsToDelete(files, now, 7*day, maxBytes, 0.8)

		seen := map[string]bool{}
		for _, d := range doomed {
			if seen[d.name] {
				t.Fatalf("%s 被重复列入删除清单", d.name)
			}
			seen[d.name] = true
		}
		// export-1 必定在（TTL 过期）；且它只出现一次。
		if !seen["export-1.tar"] {
			t.Fatal("TTL 过期的文件必须被删")
		}
	})
}

// 只清自己的产物：目录可被运维指向共享路径，
// 删别人的文件是不可接受的风险。
func TestCleanupOnlyTouchesOwnArtifacts(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, exportFilePrefix+"cleanup-test.tar")
	foreign := filepath.Join(dir, "someone-elses-important-file.dat")
	for _, p := range []string{own, foreign} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// 把 mtime 设成很久以前，让 TTL 一定会命中。
	old := time.Now().Add(-365 * 24 * time.Hour)
	for _, p := range []string{own, foreign} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	restore := setExportStoreForTest(dir)
	t.Cleanup(restore)

	deleted, _, err := CleanupExportArtifacts(time.Now())
	if err != nil {
		t.Fatalf("清理报错：%v", err)
	}
	if deleted != 1 {
		t.Fatalf("应只删 1 个自己的产物，实际 %d", deleted)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatal("自己的过期产物应被删除")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("**非本模块的文件绝不能被删** —— 目录可能指向共享路径")
	}
}

// 回归：清理**不得**碰到构建中的 `.tmp` 产物。
//
// 构建期先写 `export-<id>.tar.tmp` 再 rename。若清理把它算进来，
// 一次长时间导出（目录恰好超总量上限时）可能写到一半被删：
// Linux 上 unlink 后 rename 失败 → 数小时导出白做，且两平台行为不一致。
func TestCleanupSkipsInProgressTempArtifacts(t *testing.T) {
	dir := t.TempDir()
	done := filepath.Join(dir, "export-1.tar")
	inProgress := filepath.Join(dir, "export-2.tar.tmp")
	foreign := filepath.Join(dir, "notes.txt")
	for _, p := range []string{done, inProgress, foreign} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-365 * 24 * time.Hour)
	for _, p := range []string{done, inProgress, foreign} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	restore := setExportStoreForTest(dir)
	t.Cleanup(restore)

	deleted, _, err := CleanupExportArtifacts(time.Now())
	if err != nil {
		t.Fatalf("清理报错：%v", err)
	}
	if deleted != 1 {
		t.Fatalf("应只删 1 个已完成产物，实际 %d", deleted)
	}
	if _, err := os.Stat(inProgress); err != nil {
		t.Fatal("**构建中的 .tmp 绝不能被清理删掉** —— 会毁掉进行中的导出")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("非本模块的文件不能被删")
	}
	if _, err := os.Stat(done); !os.IsNotExist(err) {
		t.Fatal("已完成的过期产物应被删除")
	}
}
