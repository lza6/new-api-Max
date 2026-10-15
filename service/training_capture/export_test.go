package training_capture

import (
	"archive/tar"
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 导出模块的纯逻辑测试：Range 解析、日期范围校验、签名、格式/分片选择。
//
// 这些是**真实业务不变量**（不是覆盖率装饰）：
//   - Range 边界直接决定断点续传是否正确（错了会拼出损坏文件）；
//   - 日期范围校验防「误填把整云盘拉进一次导出」；
//   - 签名决定直链的安全（错了 = 任意人可下载用户数据）。

func TestParseSingleRange(t *testing.T) {
	const size = 1000

	tests := []struct {
		name    string
		header  string
		want    ByteRange
		wantErr error
	}{
		{"explicit range", "bytes=0-499", ByteRange{0, 499}, nil},
		{"middle range", "bytes=500-999", ByteRange{500, 999}, nil},
		{"open ended", "bytes=500-", ByteRange{500, 999}, nil},
		{"suffix range", "bytes=-200", ByteRange{800, 999}, nil},
		{"suffix larger than size clamps", "bytes=-5000", ByteRange{0, 999}, nil},
		{"end beyond size clamps", "bytes=900-5000", ByteRange{900, 999}, nil},
		{"single byte", "bytes=0-0", ByteRange{0, 0}, nil},
		{"last byte", "bytes=999-999", ByteRange{999, 999}, nil},
		{"start equal size not satisfiable", "bytes=1000-", ByteRange{}, ErrRangeNotSatisfiable},
		{"start beyond size not satisfiable", "bytes=2000-3000", ByteRange{}, ErrRangeNotSatisfiable},
		{"zero suffix not satisfiable", "bytes=-0", ByteRange{}, ErrRangeNotSatisfiable},
		{"start after end not satisfiable", "bytes=600-500", ByteRange{}, ErrRangeNotSatisfiable},
		{"multi-range rejected", "bytes=0-99,200-299", ByteRange{}, ErrRangeMalformed},
		{"non bytes unit", "items=0-99", ByteRange{}, ErrRangeMalformed},
		{"empty", "", ByteRange{}, ErrRangeMalformed},
		{"no equals", "bytes 0-99", ByteRange{}, ErrRangeMalformed},
		{"empty spec", "bytes=", ByteRange{}, ErrRangeMalformed},
		{"negative start", "bytes=-100-200", ByteRange{}, ErrRangeMalformed},
		{"non numeric", "bytes=abc-def", ByteRange{}, ErrRangeMalformed},
		{"only dash", "bytes=-", ByteRange{}, ErrRangeMalformed},
		{"uppercase unit", "BYTES=0-9", ByteRange{0, 9}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSingleRange(tc.header, size)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseSingleRangeZeroSize(t *testing.T) {
	// 空文件任何区间都不可满足（避免对 0 长度做 size-1 得到 -1）。
	_, err := ParseSingleRange("bytes=0-", 0)
	require.ErrorIs(t, err, ErrRangeNotSatisfiable)
	_, err = ParseSingleRange("bytes=-10", 0)
	require.ErrorIs(t, err, ErrRangeNotSatisfiable)
}

func TestValidateExportRange(t *testing.T) {
	t.Run("valid range normalized", func(t *testing.T) {
		from, to, err := ValidateExportRange("2026-10-01", "2026-10-14", 366)
		require.NoError(t, err)
		assert.Equal(t, "2026-10-01", from)
		assert.Equal(t, "2026-10-14", to)
	})

	t.Run("single day", func(t *testing.T) {
		from, to, err := ValidateExportRange("2026-10-14", "2026-10-14", 366)
		require.NoError(t, err)
		assert.Equal(t, "2026-10-14", from)
		assert.Equal(t, "2026-10-14", to)
	})

	t.Run("reversed range rejected", func(t *testing.T) {
		_, _, err := ValidateExportRange("2026-10-14", "2026-10-01", 366)
		require.ErrorIs(t, err, ErrExportRangeInvalid)
	})

	t.Run("missing date rejected", func(t *testing.T) {
		_, _, err := ValidateExportRange("", "2026-10-01", 366)
		require.ErrorIs(t, err, ErrExportRangeInvalid)
	})

	t.Run("bad format rejected", func(t *testing.T) {
		_, _, err := ValidateExportRange("2026/10/01", "2026-10-14", 366)
		require.ErrorIs(t, err, ErrExportRangeInvalid)
	})

	t.Run("span over limit rejected", func(t *testing.T) {
		_, _, err := ValidateExportRange("2020-01-01", "2026-10-14", 366)
		require.ErrorIs(t, err, ErrExportRangeInvalid)
	})

	t.Run("span exactly at limit allowed", func(t *testing.T) {
		// 2026-01-01 .. 2026-12-31 = 365 天；上限 366 应通过。
		_, _, err := ValidateExportRange("2026-01-01", "2026-12-31", 366)
		require.NoError(t, err)
	})
}

func TestSelectExportParts(t *testing.T) {
	entries := []TreeEntry{
		{Path: "2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz"},
		{Path: "2026-10-01/alice-7/gpt-4o/part-0002.jsonl.gz"},
		{Path: "2026-10-14/bob-9/claude-3/part-0001.jsonl.gz"},
		{Path: "2026-10-15/carol-3/gpt-4o/part-0001.jsonl.gz"},
		{Path: "2026-10-1/short-1/gpt-4o/part-0001.jsonl.gz"}, // 非规范日期（会被 < 比较排除）
		{Path: "2026-10-01/alice-7/gpt-4o/notes.txt"},         // 非分片
		{Path: "2026-10-01/alice-7", IsDir: true},             // 目录
		{Path: "2026-10-01/too-shallow.jsonl.gz"},             // 段数不足
	}

	t.Run("date range only", func(t *testing.T) {
		got := SelectExportParts(entries, ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-14"})
		paths := partPaths(got)
		assert.Equal(t, []string{
			"2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz",
			"2026-10-01/alice-7/gpt-4o/part-0002.jsonl.gz",
			"2026-10-14/bob-9/claude-3/part-0001.jsonl.gz",
		}, paths)
	})

	t.Run("filter by user", func(t *testing.T) {
		got := SelectExportParts(entries, ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-31", UserId: 9})
		paths := partPaths(got)
		assert.Equal(t, []string{"2026-10-14/bob-9/claude-3/part-0001.jsonl.gz"}, paths)
	})

	t.Run("filter by model", func(t *testing.T) {
		got := SelectExportParts(entries, ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-31", Model: "gpt-4o"})
		paths := partPaths(got)
		assert.Equal(t, []string{
			"2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz",
			"2026-10-01/alice-7/gpt-4o/part-0002.jsonl.gz",
			"2026-10-15/carol-3/gpt-4o/part-0001.jsonl.gz",
		}, paths)
	})

	t.Run("user and model combined", func(t *testing.T) {
		got := SelectExportParts(entries, ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-31", UserId: 7, Model: "gpt-4o"})
		paths := partPaths(got)
		assert.Equal(t, []string{
			"2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz",
			"2026-10-01/alice-7/gpt-4o/part-0002.jsonl.gz",
		}, paths)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := SelectExportParts(entries, ExportSelection{DateFrom: "2030-01-01", DateTo: "2030-12-31"})
		assert.Empty(t, got)
	})
}

func partPaths(parts []ExportPart) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, p.Path)
	}
	return out
}

// flipLast 把 token 的最后一个 base64 字符换成不同的字符（篡改签名用）。
func flipLast(s string) string {
	if s == "" {
		return "A"
	}
	last := s[len(s)-1]
	repl := byte('A')
	if last == 'A' {
		repl = 'B'
	}
	return string(repl)
}

func TestExportAccessSignAndVerify(t *testing.T) {
	// CryptoSecret 是包级变量，测试里显式设置并恢复。
	prev := common.CryptoSecret
	common.CryptoSecret = "unit-test-export-secret"
	defer func() { common.CryptoSecret = prev }()

	now := int64(1_700_000_000)
	const issuer = 7
	token, expiresAt, err := IssueExportAccess(42, issuer, time.Hour, now)
	require.NoError(t, err)
	assert.Equal(t, now+3600, expiresAt)

	t.Run("valid token verifies", func(t *testing.T) {
		assert.True(t, VerifyExportAccess(token, 42, issuer, now))
	})

	t.Run("wrong export id fails", func(t *testing.T) {
		assert.False(t, VerifyExportAccess(token, 43, issuer, now))
	})

	t.Run("wrong issuer fails", func(t *testing.T) {
		// 改 iss 嫁祸他人 → 签名失配。
		assert.False(t, VerifyExportAccess(token, 42, 8, now))
		assert.False(t, VerifyExportAccess(token, 42, 0, now))
	})

	t.Run("expired token fails", func(t *testing.T) {
		assert.False(t, VerifyExportAccess(token, 42, issuer, expiresAt))
		assert.False(t, VerifyExportAccess(token, 42, issuer, expiresAt+1))
	})

	t.Run("one second before expiry ok", func(t *testing.T) {
		assert.True(t, VerifyExportAccess(token, 42, issuer, expiresAt-1))
	})

	t.Run("tampered signature fails", func(t *testing.T) {
		bad := token[:len(token)-1] + flipLast(token)
		assert.False(t, VerifyExportAccess(bad, 42, issuer, now))
	})

	t.Run("tampered expiry fails", func(t *testing.T) {
		// 把过期时间改大（希望延长有效期）→ 签名不再匹配。
		forged := "99999999999." + strings.SplitN(token, ".", 2)[1]
		assert.False(t, VerifyExportAccess(forged, 42, issuer, now))
	})

	t.Run("garbage token fails", func(t *testing.T) {
		assert.False(t, VerifyExportAccess("not-a-token", 42, issuer, now))
		assert.False(t, VerifyExportAccess("", 42, issuer, now))
		assert.False(t, VerifyExportAccess("123.", 42, issuer, now))
		assert.False(t, VerifyExportAccess(".abc", 42, issuer, now))
	})

	t.Run("zero export id rejected on issue", func(t *testing.T) {
		_, _, err := IssueExportAccess(0, issuer, time.Hour, now)
		require.ErrorIs(t, err, ErrExportAccessInvalid)
	})
}

func TestClampExportTTL(t *testing.T) {
	assert.Equal(t, time.Duration(0), ClampExportTTL(0))
	assert.Equal(t, time.Duration(0), ClampExportTTL(-5))
	assert.Equal(t, 60*time.Second, ClampExportTTL(60))
	assert.Equal(t, exportAccessMaxTTL, ClampExportTTL(int(exportAccessMaxTTL/time.Second)+1000))
}

func TestIfRangeMatches(t *testing.T) {
	mod := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	etag := "\"abc-123\""

	assert.True(t, IfRangeMatches("", mod, etag), "empty If-Range matches")
	assert.True(t, IfRangeMatches(etag, mod, etag), "same etag matches")
	assert.False(t, IfRangeMatches("\"other\"", mod, etag), "different etag fails")

	// 日期：资源修改时间早于/等于条件时间 → 未变 → 匹配。
	assert.True(t, IfRangeMatches(mod.Format(http.TimeFormat), mod, etag))
	assert.True(t, IfRangeMatches(mod.Add(time.Hour).Format(http.TimeFormat), mod, etag),
		"condition later than mtime => unchanged => match")
	assert.False(t, IfRangeMatches(mod.Add(-time.Hour).Format(http.TimeFormat), mod, etag),
		"condition earlier than mtime => changed => no match")
	assert.False(t, IfRangeMatches("not-a-date", mod, etag), "unparsable fails closed")
}

// TestExportBuilderBuild 用内存 fake sink 端到端验证打包产物是一个
// 可被 `archive/tar` 正常解开的归档，且只读选中分片、不读其它。
func TestExportBuilderBuild(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "export-1.tar")

	gzPart := gzipMust(t, "line-a\nline-b\n")
	reader := &fakeReader{
		files: map[string][]byte{
			"2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz": gzPart,
		},
	}
	builder := &ExportBuilder{Reader: reader, Provider: "fake", Now: func() int64 { return 1_700_000_000 }}

	parts := []ExportPart{{Path: "2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz", SizeBytes: int64(len(gzPart))}}
	sel := ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	n, manifest, err := builder.Build(context.Background(), sel, parts, dst)
	require.NoError(t, err)
	require.Greater(t, n, int64(0))
	assert.Equal(t, 1, manifest.PartCount)
	assert.Equal(t, int64(len(gzPart)), manifest.TotalBytes)

	// 解开归档，成员必须是 manifest.json + 原始分片路径。
	f, err := os.Open(dst)
	require.NoError(t, err)
	defer f.Close()
	tr := tar.NewReader(f)
	names := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(tr)
		names[hdr.Name] = buf.Bytes()
	}
	require.Contains(t, names, "manifest.json")
	require.Contains(t, names, "2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz")
	// 分片字节必须**原样**搬运（不重压、不解压）。
	assert.Equal(t, gzPart, names["2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz"])
	assert.Contains(t, string(names["manifest.json"]), "\"part_count\": 1")
}

func TestExportBuilderBuildRemovesPartialOnError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "export-2.tar")
	reader := &fakeReader{failOn: "2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz"}
	builder := &ExportBuilder{Reader: reader, Now: func() int64 { return 1 }}

	parts := []ExportPart{{Path: "2026-10-01/alice-7/gpt-4o/part-0001.jsonl.gz"}}
	_, _, err := builder.Build(context.Background(), ExportSelection{DateFrom: "2026-10-01", DateTo: "2026-10-01"}, parts, dst)
	require.Error(t, err)
	// 失败后不得留下半截产物。
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr), "partial archive must be removed on failure")
}

// fakeReader 是 SinkReader 的最小内存实现（测试用）。
type fakeReader struct {
	files  map[string][]byte
	failOn string
}

func (f *fakeReader) ReadFile(_ context.Context, path string, _ int64) ([]byte, error) {
	if path == f.failOn {
		return nil, os.ErrNotExist
	}
	data, ok := f.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *fakeReader) DeleteFile(context.Context, string) error { return nil }

func (f *fakeReader) ListTree(context.Context, string, int) ([]TreeEntry, error) {
	out := make([]TreeEntry, 0, len(f.files))
	for p := range f.files {
		out = append(out, TreeEntry{Path: p})
	}
	return out, nil
}
