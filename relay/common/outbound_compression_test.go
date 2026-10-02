package common

import (
	"bytes"
	"compress/gzip"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipDecompress(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer zr.Close()
	out, err := io.ReadAll(zr)
	require.NoError(t, err)
	return out
}

func compressionTestInfo(enabled bool) *RelayInfo {
	return &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{RequestCompression: enabled},
		},
	}
}

// 渠道未开启压缩时，无论 body 多大都应原样返回、不压缩。
func TestMaybeCompressOutboundBody_Disabled(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("x", 3<<20) + `"}]}`)
	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(false), body)
	assert.False(t, compressed)
	assert.Nil(t, outCloser)
	assert.True(t, out == body, "disabled channel must return the original body unchanged")
}

// 大 JSON 请求体应被 gzip 压缩，且压缩结果可解压回原文、可回放。
func TestMaybeCompressOutboundBody_CompressesLargeJSON(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 200000) + `"}]}`)
	require.Greater(t, len(payload), 256<<10, "fixture must exceed the 256KB default threshold")

	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	require.True(t, compressed)
	require.NotNil(t, outCloser)
	defer outCloser.Close()

	compressedBytes, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Less(t, len(compressedBytes), len(payload), "compressed must be smaller than the original")
	assert.Equal(t, payload, gzipDecompress(t, compressedBytes))

	// 压缩体必须可回放（HTTP/2 透明重试依赖 GetBody）。
	replayable, ok := out.(common.ReplayableBody)
	require.True(t, ok)
	rc, err := replayable.NewReader()
	require.NoError(t, err)
	defer rc.Close()
	replay, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, compressedBytes, replay)
}

// 低于阈值的小请求体不压缩（收益不足，避免无谓 CPU 开销）。
func TestMaybeCompressOutboundBody_SkipsSmallBody(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	assert.False(t, compressed)
	assert.Nil(t, outCloser)
	assert.True(t, out == body)
}

// 已压缩内容（不可再压）时保持明文，绝不因压缩无收益而拒绝请求。
func TestMaybeCompressOutboundBody_IncompressibleStaysPlain(t *testing.T) {
	t.Parallel()
	// 随机字节不可压缩：压缩后必然 >= 原文，应回退明文。
	incompressible := make([]byte, 2<<20)
	rng := rand.New(rand.NewSource(1))
	_, err := rng.Read(incompressible)
	require.NoError(t, err)
	body, closer, err := NewOutboundJSONBody(incompressible)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	assert.False(t, compressed, "incompressible body must stay plain")
	assert.Nil(t, outCloser)
	got, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Equal(t, incompressible, got)
}

// 全局 kill-switch 关闭时，即使渠道开启也不压缩。
func TestMaybeCompressOutboundBody_GlobalKillSwitch(t *testing.T) {
	prev := common.RelayRequestCompressionEnabled
	common.RelayRequestCompressionEnabled = false
	defer func() { common.RelayRequestCompressionEnabled = prev }()

	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello ", 300000) + `"}]}`)
	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	assert.False(t, compressed)
	assert.Nil(t, outCloser)
	assert.True(t, out == body)
}

// 非可回放 body（如 *bytes.Reader）也应支持压缩，返回可回放结果。
func TestMaybeCompressOutboundBody_NonReplayableInput(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 200000) + `"}]}`)

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), bytes.NewReader(payload))
	require.True(t, compressed)
	require.NotNil(t, outCloser)
	defer outCloser.Close()

	compressedBytes, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Equal(t, payload, gzipDecompress(t, compressedBytes))
}

// 端到端契约：压缩后的 body + Content-Encoding: gzip 经真实 http.Client 送达
// 上游时，上游能正确解压回原文。这验证了 DoApiRequest 依赖的完整契约
// （压缩 → 设 header → 发送 → 上游解压）。
func TestOutboundCompression_EndToEndContract(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 200000) + `"}]}`)

	var gotEncoding string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		var reader io.Reader = r.Body
		if gotEncoding == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			require.NoError(t, err)
			reader = zr
		}
		b, err := io.ReadAll(reader)
		require.NoError(t, err)
		gotBody = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	require.True(t, compressed)
	require.NotNil(t, outCloser)
	defer outCloser.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, out)
	require.NoError(t, err)
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "gzip", gotEncoding, "上游必须收到 Content-Encoding: gzip")
	assert.Equal(t, payload, gotBody, "上游解压后必须得到原始请求体")
}

// 存储不可用时（磁盘缓存目录无法创建）必须回退明文，绝不返回 nil body 或报错。
func TestMaybeCompressOutboundBody_StorageFailureFallsBackToPlain(t *testing.T) {
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 200000) + `"}]}`)
	// 先用默认配置建 body（内存存储），再破坏磁盘配置，确保只有压缩存储这一步失败。
	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	prev := common.GetDiskCacheConfig()
	// Path 指向一个「文件」而非目录 → 磁盘缓存目录 MkdirAll 失败 →
	// CreateBodyStorage 必然报错，强制走存储失败回退分支。
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0600))
	common.SetDiskCacheConfig(common.DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 0, // 阈值 0：任意大小都尝试落盘
		MaxSizeMB:   64,
		Path:        blocker,
	})
	defer common.SetDiskCacheConfig(prev)

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	require.NotNil(t, out, "must never return nil body")
	assert.False(t, compressed, "storage failure must fall back to plain")
	assert.Nil(t, outCloser)
	// 上游必须能拿到完整原文（原 body 未受扰动）。
	got, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// 非可回放 body 未压缩时返回 *bytes.Reader（net/http 自动补 GetBody）。
func TestMaybeCompressOutboundBody_NonReplayableUncompressedReturnsReplayableReader(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), bytes.NewReader(payload))
	assert.False(t, compressed)
	assert.Nil(t, outCloser)
	got, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// 300KB 请求在 256KB 默认阈值下应被压缩（此前 1MB 阈值会跳过）。
func TestMaybeCompressOutboundBody_300KBCompressedAtNewThreshold(t *testing.T) {
	// 不 t.Parallel：读取全局阈值默认值。
	assert.Equal(t, 256, common.RelayRequestCompressionThresholdKB, "默认阈值应为 256KB")

	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 25000) + `"}]}`)
	require.Greater(t, len(payload), 256<<10)
	require.Less(t, len(payload), 1<<20, "fixture 应落在 256KB-1MB 区间")

	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	out, compressed, outCloser, _, _ := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	require.NotNil(t, out)
	if outCloser != nil {
		defer outCloser.Close()
	}
	assert.True(t, compressed, "300KB 请求在 256KB 阈值下应被压缩")
	got, err := io.ReadAll(out)
	require.NoError(t, err)
	assert.Equal(t, payload, gzipDecompress(t, got))
}

// 尺寸契约：返回的 originalBytes/outBytes 必须准确反映压缩前后字节，
// 非可回放 body 也要有正确尺寸（此前 bodySize 对非可回放返回 0 → 面板显示 0/Infinity）。
func TestMaybeCompressOutboundBody_SizeContract(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("hello world ", 200000) + `"}]}`)

	// 可回放输入：原始尺寸 = payload 长度，压缩后 < 原始。
	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()
	_, compressed, outCloser, orig, out := MaybeCompressOutboundBody(nil, compressionTestInfo(true), body)
	if outCloser != nil {
		defer outCloser.Close()
	}
	require.True(t, compressed)
	assert.EqualValues(t, len(payload), orig, "可回放：原始尺寸准确")
	assert.Less(t, out, orig, "压缩后更小")

	// 非可回放输入（*bytes.Reader，模拟透传+reasoning_effort 路径）：尺寸也必须准确，
	// 不能是 0。
	_, compressed2, outCloser2, orig2, out2 := MaybeCompressOutboundBody(nil, compressionTestInfo(true), bytes.NewReader(payload))
	if outCloser2 != nil {
		defer outCloser2.Close()
	}
	require.True(t, compressed2)
	assert.EqualValues(t, len(payload), orig2, "非可回放：原始尺寸也必须准确（非 0）")
	assert.Less(t, out2, orig2)

	// 渠道未开压缩：返回 -1（调用方据此跳过展示，不显示 0）。
	_, _, _, orig3, out3 := MaybeCompressOutboundBody(nil, compressionTestInfo(false), bytes.NewReader(payload))
	assert.EqualValues(t, -1, orig3)
	assert.EqualValues(t, -1, out3)
}
