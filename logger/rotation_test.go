package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 日志按大小分片（T6.3）的测试。
//
// 重点是两条容易出错、且出错后**不会报错**（静默失效）的性质：
//  1. 分片文件名必须仍被 `isManagedLogFile()` 匹配 —— 否则 retention
//     永远清不掉它们，磁盘照样涨（"轮转了但没被清理"）。
//  2. 同一秒内的连续轮转必须产生**不同**文件名 —— 否则 O_APPEND
//     会继续写原文件，"轮转"完全失效且日志里看不出异常。

// ---------- shouldRotate ----------

func TestShouldRotateOnLineCount(t *testing.T) {
	// 行数超限触发（原行为保持）。
	if !shouldRotate(1_000_001, 0, 1_000_000, 0) {
		t.Fatal("行数超限应触发轮转")
	}
	if shouldRotate(1_000_000, 0, 1_000_000, 0) {
		t.Fatal("恰好等于阈值不应触发（避免边界抖动）")
	}
}

func TestShouldRotateOnByteSize(t *testing.T) {
	// **新增行为**：字节数超限触发。
	// 这是本任务的核心 —— 行数与字节数无关，一行可以是 80B 也可以是 200KB。
	const maxBytes = 256 << 20
	if !shouldRotate(1, maxBytes+1, 0, maxBytes) {
		t.Fatal("字节数超限应触发轮转")
	}
	if shouldRotate(1, maxBytes, 0, maxBytes) {
		t.Fatal("恰好等于字节阈值不应触发")
	}
}

func TestShouldRotateEitherDimensionTriggers(t *testing.T) {
	// 两个维度**任一**超限都触发（不是必须同时满足）。
	if !shouldRotate(2_000_000, 10, 1_000_000, 1<<20) {
		t.Fatal("只有行数超限也应轮转")
	}
	if !shouldRotate(10, 1<<21, 1_000_000, 1<<20) {
		t.Fatal("只有字节数超限也应轮转")
	}
	if shouldRotate(10, 10, 1_000_000, 1<<20) {
		t.Fatal("两个维度都未超限不应轮转")
	}
}

func TestShouldRotateTreatsNonPositiveLimitAsUnlimited(t *testing.T) {
	// 传 0 <= 视为「该维度不限制」。若把 0 当阈值，就变成
	// 「每条日志都轮转」—— 那会瞬间刷爆目录，比不轮转更糟。
	//
	// 注意：只让**被关掉的那个维度**取值很大；另一个维度保持在阈值内，
	// 否则会因为它超限而触发（那是对的行为，但会掩盖本用例的意图）。
	if shouldRotate(1<<40, 1<<40, 0, 0) {
		t.Fatal("两个维度都为 0 时应视为不限制")
	}
	// 行数维度关闭、字节维度在限内 → 巨大的行数不应触发。
	if shouldRotate(1<<40, 10, 0, 1<<30) {
		t.Fatal("行数维度为 0 时不应因行数触发")
	}
	// 字节维度关闭、行数在限内 → 巨大的字节数不应触发。
	if shouldRotate(10, 1<<40, 1_000_000, 0) {
		t.Fatal("字节维度为 0 时不应因字节触发")
	}
}

// ---------- nextLogFileName ----------

func TestNextLogFileNameBasic(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)
	got := nextLogFileName(dir, now, func(string) bool { return false })

	want := filepath.Join(dir, "oneapi-20261015123456.log")
	if got != want {
		t.Fatalf("期望 %q，实际 %q", want, got)
	}
}

func TestNextLogFileNameIsAlwaysManagedByRetention(t *testing.T) {
	// **关键**：无论走了哪条命名分支，文件名都必须被 isManagedLogFile 匹配。
	// 不匹配意味着 retention 永远清不掉分片 → 磁盘照样涨。
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)

	cases := []struct {
		name   string
		exists func(string) bool
	}{
		{"无冲突", func(string) bool { return false }},
		{"首个冲突", func(p string) bool { return strings.HasSuffix(p, "oneapi-20261015123456.log") }},
		{"连续冲突", func(p string) bool {
			return strings.Contains(p, "oneapi-20261015123456")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextLogFileName(dir, now, tc.exists)
			name := filepath.Base(got)
			if !isManagedLogFile(name) {
				t.Fatalf("文件名 %q 不被 isManagedLogFile 匹配 —— retention 将无法清理它", name)
			}
		})
	}
}

func TestNextLogFileNameAvoidsSameSecondCollision(t *testing.T) {
	// 同一秒内连续轮转必须得到**不同**文件名。
	// 否则 O_APPEND 会继续写原文件，轮转静默失效。
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)

	// 模拟「文件系统里已经有这些名字」。
	taken := map[string]bool{}
	exists := func(p string) bool { return taken[p] }

	first := nextLogFileName(dir, now, exists)
	taken[first] = true

	second := nextLogFileName(dir, now, exists)
	if second == first {
		t.Fatalf("同一秒的第二次轮转复用了同一文件名 %q —— 会继续写原文件", first)
	}
	taken[second] = true

	third := nextLogFileName(dir, now, exists)
	if third == first || third == second {
		t.Fatalf("第三次轮转仍冲突：%q", third)
	}
	if !isManagedLogFile(filepath.Base(third)) {
		t.Fatalf("冲突分支产出的名字不受 retention 管辖：%q", third)
	}
}

func TestNextLogFileNameFallsBackAfterManyCollisions(t *testing.T) {
	// 极端：目录里同秒文件多到超过序号尝试上限 —— 必须仍能给出唯一名字，
	// 而不是死循环或返回一个冲突的名字。
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)

	var mu sync.Mutex
	taken := map[string]bool{}
	exists := func(p string) bool {
		mu.Lock()
		defer mu.Unlock()
		return taken[p]
	}

	var names []string
	for range maxLogSizeSuffixAttempts + 5 {
		name := nextLogFileName(dir, now, exists)
		mu.Lock()
		taken[name] = true
		mu.Unlock()
		names = append(names, name)
	}

	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("产生了重复文件名 %q —— 该文件会被覆盖", n)
		}
		seen[n] = true
		if !isManagedLogFile(filepath.Base(n)) {
			t.Fatalf("兜底分支产出的名字不受 retention 管辖：%q", n)
		}
	}
}

func TestNextLogFileNameWithRealFilesystem(t *testing.T) {
	// 真实文件系统路径（exists 传 nil → 用 os.Stat）。
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)

	first := nextLogFileName(dir, now, nil)
	if err := os.WriteFile(first, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	second := nextLogFileName(dir, now, nil)
	if second == first {
		t.Fatalf("已存在的文件被复用：%q", first)
	}
	// 确认第一步生成的就是我们期望的基础名。
	if filepath.Base(first) != "oneapi-20261015123456.log" {
		t.Fatalf("基础名不对：%q", filepath.Base(first))
	}
}

// ---------- 环境变量解析 ----------

func TestResolveMaxLogSizeBytesDefault(t *testing.T) {
	t.Setenv(envLogMaxSizeMB, "")
	got := resolveMaxLogSizeBytes()
	if want := int64(defaultLogMaxSizeMB) << 20; got != want {
		t.Fatalf("期望默认 %d，实际 %d", want, got)
	}
}

func TestResolveMaxLogSizeBytesOverride(t *testing.T) {
	t.Setenv(envLogMaxSizeMB, "64")
	got := resolveMaxLogSizeBytes()
	if want := int64(64) << 20; got != want {
		t.Fatalf("期望 %d，实际 %d", want, got)
	}
}

func TestResolveMaxLogSizeBytesIgnoresInvalidValues(t *testing.T) {
	// 配置写错时回退默认 —— 不能因为拿到 0 而变成「每条日志都轮转」
	// （那会瞬间刷爆目录，比不轮转更糟）。
	cases := []struct{ name, raw string }{
		{"非数字", "abc"},
		{"零", "0"},
		{"负数", "-5"},
		{"小数", "1.5"},
		{"空字符串", "  "},
	}
	want := int64(defaultLogMaxSizeMB) << 20
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envLogMaxSizeMB, tc.raw)
			if got := resolveMaxLogSizeBytes(); got != want {
				t.Fatalf("非法值 %q 应回退默认 %d，实际 %d", tc.raw, want, got)
			}
		})
	}
}

// ---------- 与 retention 的衔接 ----------

func TestRotationOutputIsCleanedByRetention(t *testing.T) {
	// 端到端衔接：轮转产生的文件名必须能被 retention 纳入清理范围。
	dir := t.TempDir()
	now := time.Date(2026, 10, 15, 12, 34, 56, 0, time.UTC)

	// 造出「基础名 + 3 个冲突名」这批轮转产物。
	taken := map[string]bool{}
	exists := func(p string) bool { return taken[p] }
	var produced []string
	for range 4 {
		name := nextLogFileName(dir, now, exists)
		taken[name] = true
		produced = append(produced, name)
		// 真建出文件，让 retention 能扫到。
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// retention 的清理必须能识别并删除**非活动**的那些。
	active := produced[len(produced)-1]
	deleted := cleanupOldLogFiles(dir, active)

	// 小目录不会触发删除（未超上限），这里只断言「识别为受管文件」
	// ——那才是 retention 能清理的前提。
	for _, p := range produced {
		if !isManagedLogFile(filepath.Base(p)) {
			t.Fatalf("%q 不被识别为受管日志文件", filepath.Base(p))
		}
	}
	// 活动文件必须还在。
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("活动日志文件被删了：%v", err)
	}
	_ = deleted
}
