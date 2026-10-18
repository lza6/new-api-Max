package controller

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
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
	// readErr 注入读取失败（全部）。
	readErr error
	// failPaths 只让**指定路径**读取失败（用于「部分分片读不到」场景）。
	failPaths map[string]bool
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
	if f.failPaths[path] {
		return nil, assert.AnError
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
		"/api/training-data/admin/analyze", "{\"path\":\"2026-10-14/nobody-1/m\",\"model\":\"gpt-4o\"}", 1)
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
		"{\"path\":\"2026-10-14/a-1/m\",\"model\":\"gpt-4o\",\"messages\":[{\"role\":\"system\",\"content\":\"覆盖注入\"}]}", 1)
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

// ---------- 导出：枚举截断必须明确报错（盲点 B13） ----------

// truncatingReaderSink 报告「枚举被截断」，用于验证导出会拒绝产出半包。
type truncatingReaderSink struct {
	fakeReaderSink
}

func (f *truncatingReaderSink) ListTreeWithTruncation(
	ctx context.Context, root string, maxEntries int,
) ([]training_capture.TreeEntry, bool, error) {
	entries, err := f.ListTree(ctx, root, maxEntries)
	return entries, true, err // 恒报截断
}

func TestBuildExportRefusesTruncatedListing(t *testing.T) {
	setupTrainingDataTestDB(t)

	sink := &truncatingReaderSink{fakeReaderSink{files: map[string][]byte{
		"2026-10-14/a-1/m/part-0001.jsonl.gz": []byte("x"),
	}}}

	rec := &model.TrainingDataExport{DateFrom: "2026-10-14", DateTo: "2026-10-14"}
	_, err := buildExport(context.Background(), sink, rec)

	require.Error(t, err, "枚举被截断时必须**明确报错**，而不是产出看似完整的半包")
	assert.Contains(t, err.Error(), "truncated",
		"错误信息要说明是截断导致的，并给出可行建议")
	// **不得**声称一个具体条目上限：sink 会把请求值夹到自己的硬顶，
	// 报具体数字等于报一个无法核实的值（曾写死 100000 而后端实际 5000）。
	assert.NotContains(t, err.Error(), "100000",
		"错误信息不得包含未经核实的条目上限数字")
	assert.Contains(t, err.Error(), "narrow the date range",
		"必须告诉管理员下一步怎么做（收窄范围），否则他无从下手")
}

// 不支持截断报告的后端：**保守拒绝**，不赌它没截断。
type noTruncationReaderSink struct {
	fakeReaderSink
}

func (f *noTruncationReaderSink) ListTreeOnly(
	ctx context.Context, root string, maxEntries int,
) ([]training_capture.TreeEntry, error) {
	return f.ListTree(ctx, root, maxEntries)
}

func TestBuildExportRefusesSinkWithoutTruncationReporting(t *testing.T) {
	setupTrainingDataTestDB(t)

	sink := &noTruncationReaderSink{fakeReaderSink{files: map[string][]byte{
		"2026-10-14/a-1/m/part-0001.jsonl.gz": []byte("x"),
	}}}

	rec := &model.TrainingDataExport{DateFrom: "2026-10-14", DateTo: "2026-10-14"}
	_, err := buildExport(context.Background(), sink, rec)

	require.Error(t, err, "无法证明完整性的后端必须拒绝导出（宁可不导，不可半包）")
	assert.Contains(t, err.Error(), "cannot report listing truncation")
}

// TestAnalyzeRequiresModel 锁定一次真实事故：界面曾把模型输入标成
// `Model (optional)`，而后端 relay 对空 model 直接 400（"model is required"）。
// 于是「打开面板 → 不改模型 → 点开始」是管理员的**默认操作路径**，
// 却必然失败，且报错来自 relay 通用校验、与「分析」无关，很难判断该改什么。
//
// 现在两处都改：后端在调用任何模型之前给出清晰错误；前端标明必填并禁用按钮。
func TestAnalyzeRequiresModel(t *testing.T) {
	setupTrainingDataTestDB(t)
	defer training_capture.SetSinkForTest(newFakeReaderSink())()

	cases := []struct{ name, body string }{
		{"完全没传", `{"path":"2026-10-14/a-1/m"}`},
		{"空串", `{"path":"2026-10-14/a-1/m","model":""}`},
		{"纯空白", `{"path":"2026-10-14/a-1/m","model":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder := jsonContext(t, http.MethodPost,
				"/api/training-data/admin/analyze", tc.body, 1)
			AnalyzeTrainingDataConversation(ctx)

			var env apiEnvelope
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
			assert.False(t, env.Success)
			// 错误必须点名 model，并说明该做什么（而不是 relay 那句通用提示）。
			assert.Contains(t, env.Message, "model")
			assert.Contains(t, env.Message, "required")
		})
	}
}

// TestAnalysisSystemMessageDisclosesIncompleteData 锁定「不假装完整」：
// 分片读取/解压失败、或列举被截断/分片数超限时，
// 系统消息**必须**告诉模型数据可能残缺。
//
// 不披露的后果：模型把残缺当完整、给出自信的分析报告，
// 管理员据此做决策 —— 这正是本模块反复强调的反面。
func TestAnalysisSystemMessageDisclosesIncompleteData(t *testing.T) {
	t.Run("分片读取失败要披露且给出数量", func(t *testing.T) {
		msg := buildAnalysisSystemMessage(training_capture.AssemblyResult{
			AnnotatedText: "正文", TotalLines: 2, SkippedParts: 3,
		})
		assert.Contains(t, msg, "3 个分片读取或解压失败")
		assert.Contains(t, msg, "未能参与本次分析")
	})

	t.Run("列举被截断要披露", func(t *testing.T) {
		msg := buildAnalysisSystemMessage(training_capture.AssemblyResult{
			AnnotatedText: "正文", ListTruncated: true,
		})
		assert.Contains(t, msg, "分片列举被截断或分片数超过上限")
	})

	t.Run("完整数据不得产生任何残缺声明", func(t *testing.T) {
		msg := buildAnalysisSystemMessage(training_capture.AssemblyResult{
			AnnotatedText: "正文", TotalLines: 2,
		})
		// 只看**元信息段**：提示词里本身就会写「若我标注了…」这类指令，
		// 在整段消息上断言 NotContains 会误判。
		meta := msg[strings.Index(msg, "对话记录元信息："):]
		assert.NotContains(t, meta, "未能参与本次分析")
		assert.NotContains(t, meta, "分片列举被截断")
		assert.NotContains(t, meta, "对话文本被截断")
	})

	t.Run("提示词要求模型声明可见范围", func(t *testing.T) {
		// 仅写元信息不够：必须要求模型在报告里说明「只基于可见部分」。
		assert.Contains(t, analyzeSystemPrompt, "分片读取或解压失败")
		assert.Contains(t, analyzeSystemPrompt, "仅基于可见部分")
	})
}

// TestAnalysisDisclosesSkippedParts 端到端：某个分片读不到时，
// 系统消息必须披露「有分片未能参与分析」。
//
// 这是「不假装完整」的接线验证 —— 仅单测 buildAnalysisSystemMessage 不够，
// 要证明 loadConversationForAnalysis 真的把计数传了上去。
func TestAnalysisDisclosesSkippedParts(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	plain := []byte("{\"role\":\"request\",\"content\":\"可读的对话\"}\n")
	compressed, err := training_capture.CompressPartForTest(plain)
	require.NoError(t, err)
	sink.files["2026-10-14/a-1/m/part-0001.jsonl.gz"] = compressed
	// 第二个分片存在（会被列举到）但读取失败。
	sink.files["2026-10-14/a-1/m/part-0002.jsonl.gz"] = compressed
	sink.failPaths = map[string]bool{"2026-10-14/a-1/m/part-0002.jsonl.gz": true}
	defer training_capture.SetSinkForTest(sink)()

	assembled, err := loadConversationForAnalysis(
		context.Background(), sink, "2026-10-14/a-1/m", 1<<20)
	require.NoError(t, err)

	// 关键断言 1：失败的分片被**计数**（而不是静默 continue）。
	assert.Equal(t, 1, assembled.SkippedParts,
		"读取失败的分片必须被计数，否则报告会看起来完整")
	// 关键断言 2：可读的那份仍参与还原（部分分析比没有分析有用）。
	assert.Contains(t, assembled.AnnotatedText, "可读的对话")

	// 关键断言 3：该事实必须出现在发给模型的系统消息里。
	msg := buildAnalysisSystemMessage(assembled)
	meta := msg[strings.Index(msg, "对话记录元信息："):]
	assert.Contains(t, meta, "1 个分片读取或解压失败")
}

// TestListFilesRejectsPathTraversal 四个管理端入口（列举 / 预览 / 删除 / 分析）
// 必须用**同一条**路径校验规则。
//
// 列举接口原先漏了 `..` 校验：路径会经 `itemPath()` 拼到 RootPath 之后交给 Graph，
// `..` 可让列举逃出 RootPath 指向云盘其它目录 —— 与「只读训练数据目录」的
// 边界不符。虽然仅管理员可达，但四个入口规则不一致，将来新增端点必然漏。
func TestListFilesRejectsPathTraversal(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	defer training_capture.SetSinkForTest(sink)()

	for _, bad := range []string{"../secret", "2026-10-14/../../secret", "..%2Fsecret"} {
		ctx, recorder := jsonContext(t, http.MethodGet,
			"/api/admin/training-data/files?path="+url.QueryEscape(bad), "", 1)
		ListTrainingDataFiles(ctx)

		var env apiEnvelope
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
		assert.Falsef(t, env.Success, "列举接口必须拒绝上跳路径 %q", bad)
		assert.Contains(t, env.Message, "invalid path")
	}
}

// TestAnalyzeAllPartsUnreadableReportsFailureNotEmptiness 全部分片读不到时，
// 错误必须是「N 个分片读取失败」，而不是误导性的「没有内容」。
//
// 这两种说法对管理员意味着完全不同的下一步：
// 「没有内容」→ 这个会话本来就没数据；「读取失败」→ 去查云盘/配置。
// 把前者当后者会让真正的故障被当成正常空数据。
func TestAnalyzeAllPartsUnreadableReportsFailureNotEmptiness(t *testing.T) {
	setupTrainingDataTestDB(t)
	sink := newFakeReaderSink()
	plain := []byte("{\"role\":\"request\",\"content\":\"内容\"}\n")
	compressed, err := training_capture.CompressPartForTest(plain)
	require.NoError(t, err)
	const p1 = "2026-10-14/a-1/m/part-0001.jsonl.gz"
	const p2 = "2026-10-14/a-1/m/part-0002.jsonl.gz"
	sink.files[p1] = compressed
	sink.files[p2] = compressed
	sink.failPaths = map[string]bool{p1: true, p2: true}
	defer training_capture.SetSinkForTest(sink)()

	ctx, recorder := jsonContext(t, http.MethodPost, "/api/training-data/admin/analyze",
		`{"path":"2026-10-14/a-1/m","model":"gpt-4o"}`, 1)
	AnalyzeTrainingDataConversation(ctx)

	var env apiEnvelope
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &env))
	assert.False(t, env.Success)
	assert.Contains(t, env.Message, "failed to load",
		"必须说明是读取失败，而不是「没有内容」")
	assert.Contains(t, env.Message, "2 part(s)",
		"必须给出失败分片数，便于判断缺了多少")
}
