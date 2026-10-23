package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/middleware"
	"github.com/lza6/new-api-Max/model"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service/training_capture"
)

// 对话记录深度分析（增量需求 B9）+ 继续追问（B10）。
//
// ## 为什么复用 relay 栈而不是自己发 HTTP
//
// 分析要调用模型，最省事、最一致的做法是**走本站自己的 relay 栈**：
// 渠道选择、负载均衡、重试、计费、消费日志、审计全部自动生效。
// `controller.Playground` 已经证明了这条路径可行（合成上下文 + 调用 Relay）。
// 自己拼一个 HTTP 请求打上游会绕过计费与日志，且要重复实现渠道选择。
//
// 与 Playground 的唯一差别：**不新建临时 token**，直接以发起分析的管理员
// 身份计费 —— 运营分析属于管理行为，费用记在管理员账上最便于审计。

const (
	// analyzeMaxConversationBytes 送进模型的对话文本上限（默认 256KB）。
	//
	// 为什么要有上限：一个分片解压后可达数 MB（压缩率 ~10%），
	// 无限塞进 prompt 会让**分析这一次调用**消耗巨额 token（真金白银），
	// 且多数模型的上下文窗口也放不下。截断并**如实披露**比失败更实用。
	analyzeMaxConversationBytes = 256 << 10

	// analyzeMaxParts 一次分析最多拼接的分片数。
	// 超过的部分不参与（并在响应里说明），避免读了 500 个分片才失败。
	analyzeMaxParts = 50

	// analyzeReadTimeout 读取云盘分片的总超时。
	analyzeReadTimeout = 3 * time.Minute
)

// analyzeRequest 分析请求体。
type analyzeRequest struct {
	// Path 云盘上的**目录**（分析该会话全部分片）或单个分片文件。
	Path string `json:"path"`
	// Model 指定用于分析的模型；为空时用站点默认（由 relay 栈决定）。
	Model string `json:"model,omitempty"`
	// Messages 追问历史。为空表示「首次分析」（用内置分析提示词）。
	// 传入上一轮的 assistant 回复 + 新的 user 问题即可继续对话。
	Messages []analyzeMessage `json:"messages,omitempty"`
	// MaxBytes 覆盖对话文本上限（字节）。
	MaxBytes int `json:"max_bytes,omitempty"`
}

type analyzeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// analyzeSystemPrompt 首次分析用的系统提示词。
//
// 明确要求「引用用户原话」——那是运营方最关心的（用户到底在问什么），
// 并明确要求「若对话文本被截断要说明」，避免模型基于残缺给出自信结论。
const analyzeSystemPrompt = `你是一名资深的产品与运营分析师。下面是从 AI 网关采集到的一段真实用户对话记录。

记录正文的每行都带角色标注，请据此区分发言方：
- 【请求侧（含历史各轮拼接，逐轮角色不可靠）】= 用户发给模型的请求内容。
  注意：这一行内部把历史多轮的用户与助手文本**按顺序拼在了一起**，
  逐轮边界不可靠，但它**一定属于用户发起的请求侧**，不是模型生成的。
- 【模型回复】= 模型生成的回复内容。
- 【其它标注】= 其它角色（如 system/tool）。

请输出一份结构化的中文分析报告，包含：
1. **对话主题**：这段对话在聊什么（3-5 句概述）。
2. **用户原话摘录**：至少引用 5 条**用户侧**的原始发言（逐字引用，不要改写）。
   **只能引用【请求侧】的内容**；【模型回复】里的文字不得当作"用户原话"引用。
3. **用户意图与画像**：用户想解决什么问题、技术水平如何、可能的身份/行业。
4. **情绪与满意度**：用户的情绪走向，是否有不满或抱怨的迹象。
5. **产品改进建议**：基于这段对话，给产品/运营的具体可执行建议。

注意：
- 若我标注了「对话文本被截断」「记录行数被截断」「有分片读取或解压失败」
  或「分片列举被截断」，请在报告开头说明你的分析仅基于可见部分，
  并指出可能缺失哪类内容（不要假装你看到了全部对话）。
- **逐轮问答的配对不可靠**：请求侧那一行是拼接文本，不要断言"用户第 N 轮说了 X 之后模型回了 Y"这种精确对齐。
- 不要编造记录里没有的内容。引用原话时保持逐字准确。`

// AnalyzeTrainingDataConversation 管理员对某个对话记录做深度分析 / 继续追问。
//
// POST /api/admin/training-data/analyze
func AnalyzeTrainingDataConversation(c *gin.Context) {
	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsgLogged(c, "invalid request body", err)
		return
	}
	path := strings.Trim(strings.TrimSpace(req.Path), "/")
	if path == "" {
		common.ApiErrorMsg(c, "missing required field: path")
		return
	}
	if strings.Contains(path, "..") {
		common.ApiErrorMsg(c, "invalid path")
		return
	}
	// **模型是必填**，在这里就给出清晰错误。
	//
	// 交给 relay 也能拦住（`valid_request.go` 会返回 400 "model is required"），
	// 但那条错误来自 relay 通用校验，管理员看到的是一句与「分析」无关的提示，
	// 难以判断该改什么。界面曾把该输入框标成 `Model (optional)`，
	// 于是「不改直接点开始」→ 必然 400，且提示对不上。
	if strings.TrimSpace(req.Model) == "" {
		common.ApiErrorMsg(c, "missing required field: model (choose the model used for analysis)")
		return
	}

	reader, err := trainingDataReader()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// ---- 1. 读取并还原对话文本 ----
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = analyzeMaxConversationBytes
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), analyzeReadTimeout)
	defer cancel()

	assembly, err := loadConversationForAnalysis(ctx, reader, path, maxBytes)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 两个视图由同一次扫描产出，正常情况同空同非空；但**判据要与真正
	// 下发的那份对齐**（下发的是 AnnotatedText），否则将来两者一旦分叉，
	// 就会出现「内容非空却报了空」或「空内容却发了空正文给模型」。
	if strings.TrimSpace(assembly.AnnotatedText) == "" {
		// [修复防御] 内容为空时**必须区分「真的没有」与「读失败了」**。
		//
		// 若所有分片都因路径不匹配/读取/解压失败被跳过，直接回
		// "no conversation content found" 会让管理员以为「这个会话没数据」，
		// 而事实是「N 个分片读不到」—— 与 N1 的披露初衷正好相反。
		if assembly.SkippedParts > 0 || assembly.ListTruncated {
			common.ApiErrorMsg(c, fmt.Sprintf(
				"no readable conversation content: %d part(s) failed to load%s",
				assembly.SkippedParts,
				func() string {
					if assembly.ListTruncated {
						return "; the listing was truncated or exceeded the part limit"
					}
					return ""
				}()))
			return
		}
		common.ApiErrorMsg(c, "no conversation content found at the given path")
		return
	}

	// ---- 2. 组装发给模型的消息 ----
	systemContent := buildAnalysisSystemMessage(assembly)
	messages := make([]analyzeMessage, 0, len(req.Messages)+2)
	messages = append(messages, analyzeMessage{Role: "system", Content: systemContent})

	if len(req.Messages) == 0 {
		// 首次分析：内置提示词。
		messages = append(messages, analyzeMessage{
			Role:    "user",
			Content: "请按系统提示的要求，对上面的对话记录输出分析报告。",
		})
	} else {
		// 追问（B10）：只接受 user/assistant 角色，且**过滤 system**——
		// 否则前端可以覆盖我们注入的分析上下文（提示词注入面）。
		for _, m := range req.Messages {
			role := strings.ToLower(strings.TrimSpace(m.Role))
			if role != "user" && role != "assistant" {
				continue
			}
			messages = append(messages, analyzeMessage{Role: role, Content: m.Content})
		}
		if len(messages) == 1 { // 只有 system，说明追问全被过滤了
			common.ApiErrorMsg(c, "no valid messages: only user/assistant roles are accepted")
			return
		}
	}

	body, err := common.Marshal(gin.H{
		"model":    req.Model,
		"messages": messages,
		"stream":   false, // 非流式：分析是「一次拿完整报告」，流式只会让前端复杂化
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// ---- 3. 合成 relay 请求并复用整条栈 ----
	if !synthesizeRelayRequest(c, body, req.Model) {
		common.ApiErrorMsg(c, "failed to prepare relay request")
		return
	}

	// 与 Playground 同样的上下文准备（用户缓存 + 临时 token 语义）。
	// 这里用发起分析的管理员身份，不新建 token。
	userId := c.GetInt("id")
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userCache.WriteContext(c)

	if relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil); err == nil {
		_ = middleware.SetupContextForToken(c, &model.Token{
			UserId: userId,
			Name:   fmt.Sprintf("training-data-analysis-%s", relayInfo.UsingGroup),
			Group:  relayInfo.UsingGroup,
		})
	}

	common.SysLog(fmt.Sprintf(
		"training data capture: admin analysis requested by user=%d path=%s parts=%d lines=%d truncated=%v",
		userId, path, len(assembly.Parts), assembly.TotalLines, assembly.Truncated))

	// 交给 relay 栈：渠道选择/计费/日志/重试全部由它处理，
	// 响应直接写回给发起分析的管理员。
	Relay(c, types.RelayFormatOpenAI)
}

// loadConversationForAnalysis 读云盘并还原对话文本。
//
// 支持两种 path：目录（拼接该目录下全部分片）与单个分片文件。
func loadConversationForAnalysis(
	ctx context.Context,
	reader training_capture.SinkReader,
	path string,
	maxBytes int,
) (training_capture.AssemblyResult, error) {
	var objectPaths []string
	// 三个「不完整」信号：列举被截断 / 分片数超上限 / 单个分片读失败。
	// 任何一项都必须传给模型，否则报告看起来完整、实际缺了整段对话。
	var listTruncated bool
	var partsOverLimit bool
	var skippedParts int

	if strings.HasSuffix(path, ".jsonl.gz") || strings.HasSuffix(path, ".jsonl") {
		// 单文件
		objectPaths = []string{path}
	} else {
		// 目录：取其下全部条目，只保留分片文件。
		//
		// 用**带截断报告**的列举：`ListTree` 会把结果夹到内部硬顶而不告知，
		// 于是我们既不知道少列了，也没法告诉模型。
		if lister, ok := reader.(interface {
			ListTreeWithTruncation(ctx context.Context, root string, maxEntries int) ([]training_capture.TreeEntry, bool, error)
		}); ok {
			entries, truncated, err := lister.ListTreeWithTruncation(ctx, path, analyzeMaxParts*4)
			if err != nil {
				return training_capture.AssemblyResult{}, err
			}
			listTruncated = truncated
			for _, e := range entries {
				if e.IsDir {
					continue
				}
				if strings.HasSuffix(e.Path, ".jsonl.gz") || strings.HasSuffix(e.Path, ".jsonl") {
					objectPaths = append(objectPaths, e.Path)
				}
			}
		} else {
			entries, err := reader.ListTree(ctx, path, analyzeMaxParts*4)
			if err != nil {
				return training_capture.AssemblyResult{}, err
			}
			for _, e := range entries {
				if e.IsDir {
					continue
				}
				if strings.HasSuffix(e.Path, ".jsonl.gz") || strings.HasSuffix(e.Path, ".jsonl") {
					objectPaths = append(objectPaths, e.Path)
				}
			}
		}
		if len(objectPaths) == 0 {
			return training_capture.AssemblyResult{}, nil
		}
		// 按分片序号升序，并限制数量（超出的部分不参与，避免读几百个分片）。
		sortPartPaths(objectPaths)
		if len(objectPaths) > analyzeMaxParts {
			partsOverLimit = true
			objectPaths = objectPaths[:analyzeMaxParts]
		}
	}

	// 读取 + 解压。
	objects := make(map[string][]byte, len(objectPaths))
	for _, p := range objectPaths {
		raw, err := reader.ReadFile(ctx, p, 64<<20)
		if err != nil {
			// 单个分片读失败不中断整体（部分分析比没有分析有用），
			// 但**必须计数**：否则报告看起来完整、实际缺了整段对话。
			skippedParts++
			continue
		}
		plain := raw
		if len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			if dec, derr := training_capture.DecompressPart(raw); derr == nil {
				plain = dec
			} else {
				// 解压失败：跳过该分片（不把压缩字节当文本喂给模型），
				// 同样必须计数并披露。
				skippedParts++
				continue
			}
		}
		// 只取文件名（Assembly 内部按分片序号排序）。
		name := p
		if idx := strings.LastIndex(p, "/"); idx >= 0 {
			name = p[idx+1:]
		}
		objects[name] = plain
	}

	assembled := training_capture.AssembleConversation(objects, maxBytes)
	assembled.SkippedParts = skippedParts
	// 列举被截断或分片数超上限 → 都可能存在**没被读到的分片**，
	// 与「文本被截断」一样必须让模型知道。
	assembled.ListTruncated = listTruncated || partsOverLimit
	return assembled, nil
}

// sortPartPaths 按分片序号排序（与 AssembleConversation 同口径）。
func sortPartPaths(paths []string) {
	seqOf := func(p string) int {
		base := p
		if idx := strings.LastIndex(p, "/"); idx >= 0 {
			base = p[idx+1:]
		}
		return training_capture.ParsePartSeq(base)
	}
	for i := 1; i < len(paths); i++ {
		for j := i; j > 0 && seqOf(paths[j-1]) > seqOf(paths[j]); j-- {
			paths[j-1], paths[j] = paths[j], paths[j-1]
		}
	}
}

// buildAnalysisSystemMessage 组装系统消息（含对话正文与边界声明）。
func buildAnalysisSystemMessage(a training_capture.AssemblyResult) string {
	var sb strings.Builder
	sb.WriteString(analyzeSystemPrompt)
	sb.WriteString("\n\n---\n对话记录元信息：")
	sb.WriteString(fmt.Sprintf(
		"\n- 分片数：%d，记录行数：%d", len(a.Parts), a.TotalLines))
	if a.Truncated {
		sb.WriteString("\n- **对话文本被截断**（超出长度上限），你的分析仅基于可见部分。")
	}
	if a.MalformedLines > 0 {
		sb.WriteString(fmt.Sprintf(
			"\n- 有 %d 行数据损坏无法解析，可能缺失内容。", a.MalformedLines))
	}
	// [修复防御] 分片缺失必须告诉模型。
	//
	// 读取失败/解压失败的分片、以及「列举被截断/分片数超上限」导致的未读分片，
	// 都意味着**整段对话可能根本没进来**。不披露的后果：
	// 报告看起来完整，实际基于残缺数据，管理员据此做决策 ——
	// 这正是本模块反复强调的「不假装完整」的反面。
	if a.SkippedParts > 0 {
		sb.WriteString(fmt.Sprintf(
			"\n- **有 %d 个分片读取或解压失败，未能参与本次分析**，你可能看不到那部分对话。",
			a.SkippedParts))
	}
	if a.ListTruncated {
		sb.WriteString(
			"\n- **分片列举被截断或分片数超过上限**，可能存在未被读取的分片。")
	}
	sb.WriteString("\n\n---\n对话记录正文：\n")
	// 用**角色标注版**而不是保真原文：提示词要求「逐字引用用户原话」，
	// 而无标注文本把用户侧与模型回复混在一起 —— 模型会把**自己这一侧的回复**
	// 当成用户原话引用，报告从根上不可信且管理员看不出来。
	// 标注来自记录的 role 字段（数据结构事实），不是猜测。
	sb.WriteString(a.AnnotatedText)
	return sb.String()
}

// synthesizeRelayRequest 把合成出来的 chat 请求塞进 gin 上下文，
// 让 Relay 把它当成一次正常的 /v1/chat/completions 请求处理。
//
// Relay 通过 `helper.GetAndValidateRequest` 读 `c.Request.Body`，
// 并通过 `c.Request.URL.Path` 推断 relay 模式 —— 两者都必须正确设置，
// 否则会出现「模型拿到空消息」或「走了错误的 relay 分支」这类
// 难以定位的问题（因此这里返回 bool 让调用方显式处理失败）。
func synthesizeRelayRequest(c *gin.Context, body []byte, modelName string) bool {
	if c.Request == nil {
		return false
	}
	// 用 httptest 构造一个**独立的** Request，避免改动原请求的
	// URL/Header 结构时把管理端自身的路由信息搞乱。
	req := httptest.NewRequest(
		http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = c.Request.RemoteAddr

	// 保留原 context（取消信号、超时、request id 都在里面）。
	req = req.WithContext(c.Request.Context())

	c.Request = req
	if modelName != "" {
		c.Set("model", modelName)
	}

	// [修复防御] 把这次请求标记为**本站内部合成请求**，让采集路径跳过它。
	//
	// 不加这个标记会形成一条闭环：分析 → Relay() → 成功 → 采集
	// → 把**整段用户对话**当成「发起分析的管理员的对话」再上传到云盘。
	// 后果有三重：数据重复入库、数据归属错乱（别人的对话落到管理员目录）、
	// 以及用户从未同意过「自己的对话会被再次采集到另一个目录」。
	//
	// 这是一个真实的闭环缺陷（盲点扫描发现），不是理论风险。
	SetInternalSynthesizedRequest(c)
	return true
}

// internalRequestContextKey 标记「本站内部合成请求」的 context key。
//
// 用常量而不是裸字符串：采集侧与合成侧必须用**同一个**拼写，
// 拼错会静默失效（两边都不报错，但标记永远不生效）。
const internalRequestContextKey = "training_capture_internal_request"

// SetInternalSynthesizedRequest 标记当前请求是本站内部合成的。
func SetInternalSynthesizedRequest(c *gin.Context) {
	if c != nil {
		c.Set(internalRequestContextKey, true)
	}
}

// IsInternalSynthesizedRequest 判断当前请求是否为本站内部合成。
//
// 采集路径见 `maybeCaptureTrainingData`：命中则不采集。
func IsInternalSynthesizedRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(internalRequestContextKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}
