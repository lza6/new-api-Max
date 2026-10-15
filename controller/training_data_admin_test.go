package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/service/training_capture"
)

// 管理端云盘浏览 / 分析接口（B4/B7/B9）的**安全与校验契约**测试。
//
// 这组用例刻意**不触发真实模型调用**（付费红线）：
// 分析接口的可测部分是「什么时候**不该**调用模型」（校验、路径安全、
// 空内容、角色过滤）与「提示词是否如实披露数据缺陷」。
// 真正调用模型的部分由管理员在界面上按需触发，不在自动化测试里跑。

// fakeReaderSink 同时实现 TrainingDataSink 与 SinkReader，供读取类接口测试。
type fakeReaderSink struct {
	files map[string][]byte
	// deleted 记录被删的路径（验证越权/非分片路径被拦住）。
	deleted []string
	// readErr 注入读取失败。
	readErr error
}

func newFakeReaderSink() *fakeReaderSink {
	return &fakeReaderSink{files: map[string][]byte{}}
}

func (f *fakeReaderSink) Name() string { return "fake-reader" }

func (f *fakeReaderSink) EnsureDir(ctx context.Context, path string) error { return nil }

func (f *fakeReaderSink) WritePart(ctx context.Context, path string, data []byte) (string, error) {
	f.files[path] = data
	return path, nil
}

func (f *fakeReaderSink) List(ctx context.Context, path string) ([]string, error) {
	return nil, nil
}

func (f *fakeReaderSink) Stat(ctx context.Context) (training_capture.SinkStat, error) {
	return training_capture.SinkStat{Provider: "fake-reader"}, nil
}

func (f *fakeReaderSink) ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.files[path]
	if !ok {
		return nil, assert.AnError
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, training_capture.ErrFileTooLarge
	}
	return data, nil
}

func (f *fakeReaderSink) DeleteFile(ctx context.Context, path string) error {
	f.deleted = append(f.deleted, path)
	delete(f.files, path)
	return nil
}

func (f *fakeReaderSink) ListTree(ctx context.Context, root string, maxEntries int) ([]training_capture.TreeEntry, error) {
	out := make([]training_capture.TreeEntry, 0, len(f.files))
	for p := range f.files {
		out = append(out, training_capture.TreeEntry{Path: p, SizeBytes: int64(len(f.files[p]))})
	}
	return out, nil
}

// ---------- 列出文件 ----------

func TestListTrainingDataFilesRejectsSinkWithoutReader(t *testing.T) {
	setupTrainingDataTestDB(t)
	// fakeSinkForController 只实现 TrainingDataSink（无读取能力）。
	defer training_capture.SetSinkForTest(fakeSinkForController{})()

	ctx, recorder := jsonContext(t, http.MethodGet, "/api/training-data/admin/files", "", 1)
	ListTrainingDataFiles(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success, "不支持读取的后端必须明确报错，而不是返回空列表")
	assert.Contains(t, env.Message, "does not support reading")
}

// ---------- 预览 ----------

func TestPreviewTrainingDataFileRequiresPath(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(newFakeReaderSink())()

	ctx, recorder := jsonContext(t, http.MethodGet, "/api/training-data/admin/file", "", 1)
	PreviewTrainingDataFile(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success)
	assert.Contains(t, env.Message, "path")
}

func TestPreviewTrainingDataFileRejectsPathTraversal(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	defer training_capture.SetSinkForTest(sink)()

	// 多种上跳写法都必须被拒（纵深防御：sink 侧也做了净化）。
	for _, bad := range []string{
		"..%2F..%2F..%2Fetc%2Fpasswd",
		"..",
		"a/../../secret",
	} {
		ctx, recorder := jsonContext(t, http.MethodGet,
			"/api/training-data/admin/file?path="+bad, "", 1)
		PreviewTrainingDataFile(ctx)

		var env apiEnvelope
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
		assert.False(t, env.Success, "上跳路径必须被拒：%s", bad)
		assert.Contains(t, env.Message, "invalid path")
	}
}

func TestPreviewTrainingDataFileDecompressesGzip(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	defer training_capture.SetSinkForTest(sink)()

	// 写入一个真实的 gzip 分片（与生产格式一致）。
	// 用**有代表性的大小**：gzip 有 ~18 字节固定开销，
	// 几十字节的小分片压缩后反而更大（见下方专门的断言）。
	var sb strings.Builder
	for i := range 50 {
		sb.WriteString("{\"role\":\"request\",\"content\":\"用户原话")
		sb.WriteString(strings.Repeat("内容", 20))
		sb.WriteString("\"}\n")
		_ = i
	}
	plain := []byte(sb.String())
	compressed, err := training_capture.CompressPartForTest(plain)
	require.NoError(t, err)
	require.Less(t, len(compressed), len(plain), "fixture 应能体现压缩收益")
	sink.files["2026-10-14/alice-7/gpt-4o/part-0001.jsonl.gz"] = compressed

	ctx, recorder := jsonContext(t, http.MethodGet,
		"/api/training-data/admin/file?path=2026-10-14/alice-7/gpt-4o/part-0001.jsonl.gz", "", 1)
	PreviewTrainingDataFile(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success, "预览应成功：%s", env.Message)

	var payload struct {
		Compressed  bool     `json:"compressed"`
		Lines       []string `json:"lines"`
		LineCount   int      `json:"line_count"`
		PlainBytes  int      `json:"plain_bytes"`
		CompressedB int      `json:"compressed_bytes"`
	}
	require.NoError(t, common.Unmarshal(env.Data, &payload))
	assert.True(t, payload.Compressed, "应识别出 gzip")
	assert.Equal(t, 50, payload.LineCount)
	assert.Contains(t, payload.Lines[0], "用户原话")
	assert.Equal(t, len(plain), payload.PlainBytes)
	assert.Equal(t, len(compressed), payload.CompressedB)
	assert.Less(t, payload.CompressedB, payload.PlainBytes, "有代表性的分片应体现压缩收益")
}

func TestGzipHasOverheadOnTinyPayloads(t *testing.T) {
	// **如实记录一个真实性质**：gzip 有约 18 字节的固定头/尾开销，
	// 极小的分片压缩后**反而更大**。
	// 这不影响生产（分片是 KB~MB 级），但值得写明 ——
	// 否则将来有人看到「压缩后更大」会以为是 bug 而去改坏它。
	tiny := []byte("{\"role\":\"request\",\"content\":\"x\"}\n") // ~36 字节
	compressed, err := training_capture.CompressPartForTest(tiny)
	require.NoError(t, err)

	assert.Greater(t, len(compressed), len(tiny),
		"极小输入压缩后应更大（固定开销）—— 这是预期行为，不是缺陷")
	// 体积代价换来的是正确性：解压必须无损。
	back, err := training_capture.DecompressPart(compressed)
	require.NoError(t, err)
	assert.Equal(t, tiny, back)
}

// ---------- 删除 ----------

func TestDeleteTrainingDataFileRequiresExplicitConfirm(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	sink.files["2026-10-14/a-1/m/part-0001.jsonl.gz"] = []byte("x")
	defer training_capture.SetSinkForTest(sink)()

	// 不带 confirm=1 → 必须拒绝，且**不得删除任何东西**。
	ctx, recorder := jsonContext(t, http.MethodDelete,
		"/api/training-data/admin/file?path=2026-10-14/a-1/m/part-0001.jsonl.gz", "", 1)
	DeleteTrainingDataFile(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success, "缺少 confirm 必须拒绝")
	assert.Contains(t, env.Message, "confirm=1")
	assert.Empty(t, sink.deleted, "被拒绝的请求不得删除任何文件")
}

func TestDeleteTrainingDataFileRejectsNonPartPaths(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	defer training_capture.SetSinkForTest(sink)()

	// 只允许删分片文件；目录或任意文件都不行（防误删整棵子树）。
	for _, bad := range []string{
		"2026-10-14/alice-7/gpt-4o", // 目录
		"2026-10-14/alice-7/gpt-4o/notes.txt",
		"important.db",
	} {
		ctx, recorder := jsonContext(t, http.MethodDelete,
			"/api/training-data/admin/file?path="+bad+"&confirm=1", "", 1)
		DeleteTrainingDataFile(ctx)

		var env apiEnvelope
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
		assert.False(t, env.Success, "非分片路径必须被拒：%s", bad)
	}
	assert.Empty(t, sink.deleted, "被拒绝的请求不得删除任何文件")
}

func TestDeleteTrainingDataFileSucceedsWithConfirm(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	sink.files["2026-10-14/a-1/m/part-0001.jsonl.gz"] = []byte("x")
	defer training_capture.SetSinkForTest(sink)()

	ctx, recorder := jsonContext(t, http.MethodDelete,
		"/api/training-data/admin/file?path=2026-10-14/a-1/m/part-0001.jsonl.gz&confirm=1", "", 1)
	DeleteTrainingDataFile(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	require.True(t, env.Success, "带 confirm 应成功：%s", env.Message)
	assert.Equal(t, []string{"2026-10-14/a-1/m/part-0001.jsonl.gz"}, sink.deleted)
}

// ---------- 深度分析（B9/B10） ----------

func TestAnalyzeRejectsInvalidRequests(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(newFakeReaderSink())()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"缺 path", "{}", "path"},
		{"path 为空", "{\"path\":\"  \"}", "path"},
		{"path 上跳", "{\"path\":\"../../etc\"}", "invalid path"},
		{"坏 JSON", "{", "invalid request body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder := jsonContext(t, http.MethodPost,
				"/api/training-data/admin/analyze", tc.body, 1)
			AnalyzeTrainingDataConversation(ctx)

			var env apiEnvelope
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
			assert.False(t, env.Success)
			assert.Contains(t, env.Message, tc.want)
		})
	}
}

func TestAnalyzeReportsEmptyContentInsteadOfCallingModel(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink() // 没有任何文件
	defer training_capture.SetSinkForTest(sink)()

	// 路径下无内容 → 必须**在调用模型之前**失败（否则白白花钱）。
	ctx, recorder := jsonContext(t, http.MethodPost,
		"/api/training-data/admin/analyze", "{\"path\":\"2026-10-14/nobody-1/m\"}", 1)
	AnalyzeTrainingDataConversation(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success)
	assert.Contains(t, env.Message, "no conversation content")
}

func TestAnalyzeRejectsMessagesWithOnlySystemRole(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	plain := []byte("{\"role\":\"request\",\"content\":\"对话内容\"}\n")
	compressed, err := training_capture.CompressPartForTest(plain)
	require.NoError(t, err)
	sink.files["2026-10-14/a-1/m/part-0001.jsonl.gz"] = compressed
	defer training_capture.SetSinkForTest(sink)()

	// 只传 system 消息 → 会被全部过滤 → 必须报错而不是用空历史继续。
	// 这是**提示词注入**的防线：前端不允许覆盖服务端注入的分析上下文。
	ctx, recorder := jsonContext(t, http.MethodPost, "/api/training-data/admin/analyze",
		"{\"path\":\"2026-10-14/a-1/m\",\"messages\":[{\"role\":\"system\",\"content\":\"覆盖注入\"}]}", 1)
	AnalyzeTrainingDataConversation(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success)
	assert.Contains(t, env.Message, "no valid messages")
}

func TestBuildAnalysisSystemMessageDisclosesTruncationAndDamage(t *testing.T) {
	// 模型必须被告知数据不完整，否则会基于残缺给出自信结论。
	assembled := training_capture.AssemblyResult{
		Text:           "对话正文",
		TotalLines:     10,
		Truncated:      true,
		MalformedLines: 3,
		Parts:          []training_capture.ConversationPart{{ObjectName: "part-0001.jsonl.gz"}},
	}
	msg := buildAnalysisSystemMessage(assembled)

	assert.Contains(t, msg, "被截断", "截断必须告知模型")
	assert.Contains(t, msg, "3 行数据损坏", "坏行数必须告知模型")
	assert.Contains(t, msg, "对话正文", "正文必须包含")
	assert.Contains(t, msg, "用户原话摘录", "分析要求必须包含原话摘录")
}

func TestSortPartPathsOrdersNumerically(t *testing.T) {
	// 数值序而非字典序：part-0010 必须排在 part-0009 之后。
	paths := []string{
		"d/part-0010.jsonl.gz",
		"d/part-0002.jsonl.gz",
		"d/part-0009.jsonl.gz",
		"d/part-0001.jsonl.gz",
	}
	sortPartPaths(paths)

	want := []string{
		"d/part-0001.jsonl.gz",
		"d/part-0002.jsonl.gz",
		"d/part-0009.jsonl.gz",
		"d/part-0010.jsonl.gz",
	}
	assert.Equal(t, want, paths, "分片必须按数值序拼接（字典序会把 0010 排到 0009 前）")
}
