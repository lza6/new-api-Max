package training_capture

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实 OneDrive sink 的**路径契约**测试（用假 Graph，不联网）。
//
// ## 为什么必须用真 sink 测
//
// `fakeReaderSink`（controller 层）返回的始终是完整路径，
// 而真实 sink 的行为取决于它自己的实现 —— 一旦真实 sink 返回「相对 root」，
// 所有调用方（前端下钻 / 预览 / 删除 / 分析）都会拿到不存在的路径，
// 而失败会**伪装成「目录是空的」**（Graph 404 被当空目录），
// 极难定位。所以这里直接对真 sink 断言。

// roundTripFunc 让测试用函数替代 http.Client.Transport。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// jsonResponse 构造一个最小 HTTP 响应。
func jsonResponse(body string, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// fakeGraphSink 构造一个用假 Graph 后端的 OneDrive sink。
//
// 目录结构：
//
//	/ (root)
//	└── 2026-10-14/
//	    └── alice-1/
//	        └── gpt-4o/
//	            └── part-0001.jsonl.gz
func fakeGraphSink(t *testing.T) *oneDriveSink {
	t.Helper()
	sink := NewOneDriveSink(OneDriveConfig{
		Tenant: "t", ClientID: "c", ClientSecret: "s", DriveID: "drive",
	})
	sink.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// 令牌端点
		if strings.Contains(r.URL.Host, "login.microsoftonline.com") {
			return jsonResponse(`{"access_token":"tok","expires_in":3600}`, http.StatusOK), nil
		}
		path := r.URL.EscapedPath() + "?" + r.URL.RawQuery
		switch {
		// 根目录
		case strings.Contains(path, "/root/children"):
			return jsonResponse(`{"value":[
				{"name":"2026-10-14","folder":{"childCount":1}}
			]}`, http.StatusOK), nil
		// 第一层：日期目录
		case strings.Contains(path, "/root:/2026-10-14:/children"):
			return jsonResponse(`{"value":[
				{"name":"alice-1","folder":{"childCount":1}}
			]}`, http.StatusOK), nil
		// 第二层：用户目录
		case strings.Contains(path, "/root:/2026-10-14/alice-1:/children"):
			return jsonResponse(`{"value":[
				{"name":"gpt-4o","folder":{"childCount":1}}
			]}`, http.StatusOK), nil
		// 第三层：模型目录（分片文件）
		case strings.Contains(path, "/root:/2026-10-14/alice-1/gpt-4o:/children"):
			return jsonResponse(`{"value":[
				{"name":"part-0001.jsonl.gz","size":123}
			]}`, http.StatusOK), nil
		default:
			// 其余路径视为不存在（Graph 的 404 → fetchEntries 返回空）。
			return jsonResponse(`{}`, http.StatusNotFound), nil
		}
	})
	return sink
}

// TestListTreeAlwaysReturnsDriveRelativePaths 锁定核心契约：
// 无论 root 传什么，返回的 Path 都是**可直接回传**给
// ReadFile / DeleteFile / 再次 ListTree 的完整路径。
//
// [修复防御] 早期实现把返回值裁成「相对 root」。看起更干净，
// 但没有任何调用方遵守该契约：
//   - 前端把 entry.path 当下一层的 path 参数 → 少一层前缀 → 404 → 显示"空目录"；
//   - 分析把 e.Path 直接传给 ReadFile → 404 → 全部计入 SkippedParts → 报"没有内容"。
//
// 两种失败都伪装成「数据为空」，是极难定位的一类问题。
func TestListTreeAlwaysReturnsDriveRelativePaths(t *testing.T) {
	sink := fakeGraphSink(t)
	ctx := context.Background()

	t.Run("root 为空：完整路径", func(t *testing.T) {
		entries, err := sink.ListTree(ctx, "", 0)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		for _, e := range entries {
			assert.NotEmpty(t, e.Path)
		}
		found := false
		for _, e := range entries {
			if e.Path == "2026-10-14" {
				found = true
			}
			// 根目录下的条目不应带任何前缀。
			assert.False(t, strings.HasPrefix(e.Path, "/"), "路径不应以 / 开头：%q", e.Path)
		}
		assert.True(t, found, "应能列出日期目录，实际：%+v", entries)
	})

	t.Run("root 非空：仍返回带前缀的完整路径", func(t *testing.T) {
		entries, err := sink.ListTree(ctx, "2026-10-14", 0)
		require.NoError(t, err)
		require.NotEmpty(t, entries)

		for _, e := range entries {
			// 核心断言：必须带 root 前缀，否则调用方回传时会 404。
			assert.Truef(t, strings.HasPrefix(e.Path, "2026-10-14/"),
				"root=2026-10-14 时返回了相对路径 %q —— 调用方回传会 404 并伪装成空目录", e.Path)
		}
		var found bool
		for _, e := range entries {
			if e.Path == "2026-10-14/alice-1" {
				found = true
			}
		}
		assert.True(t, found, "应返回 2026-10-14/alice-1，实际：%+v", entries)
	})

	t.Run("逐层下钻的路径可直接回传", func(t *testing.T) {
		// 模拟前端：用上一层的 entry.Path 作为下一层的 root。
		level1, err := sink.ListTree(ctx, "", 0)
		require.NoError(t, err)
		require.NotEmpty(t, level1)

		var dateDir string
		for _, e := range level1 {
			if e.IsDir {
				dateDir = e.Path
				break
			}
		}
		require.NotEmpty(t, dateDir, "应有一个目录可供下钻")

		level2, err := sink.ListTree(ctx, dateDir, 0)
		require.NoError(t, err)
		require.NotEmptyf(t, level2,
			"用上层返回的 %q 作为 root 必须能列出内容（否则前端下钻会显示空目录）", dateDir)
		for _, e := range level2 {
			assert.Truef(t, strings.HasPrefix(e.Path, dateDir+"/"),
				"下钻后的路径必须带完整前缀，实际 %q", e.Path)
		}
	})
}
