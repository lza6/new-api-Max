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

请输出一份结构化的中文分析报告，包含：
1. **对话主题**：这段对话在聊什么（3-5 句概述）。
2. **用户原话摘录**：至少引用 5 条用户的**原始发言**（逐字引用，不要改写）。
3. **用户意图与画像**：用户想解决什么问题、技术水平如何、可能的身份/行业。
4. **情绪与满意度**：用户的情绪走向，是否有不满或抱怨的迹象。
5. **产品改进建议**：基于这段对话，给产品/运营的具体可执行建议。

注意：
- 若我标注了「对话文本被截断」，请在报告开头说明你的分析仅基于可见部分。
- 不要编造记录里没有的内容。引用原话时保持逐字准确。`

// AnalyzeTrainingDataConversation 管理员对某个对话记录做深度分析 / 继续追问。
//
// POST /api/training-data/admin/analyze
func AnalyzeTrainingDataConversation(c *gin.Context) {
	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request body: "+err.Error())
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
	if strings.TrimSpace(assembly.Text) == "" {
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

	if strings.HasSuffix(path, ".jsonl.gz") || strings.HasSuffix(path, ".jsonl") {
		// 单文件
		objectPaths = []string{path}
	} else {
		// 目录：取其下全部条目，只保留分片文件。
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
		if len(objectPaths) == 0 {
			return training_capture.AssemblyResult{}, nil
		}
		// 按分片序号升序，并限制数量（超出的部分不参与，避免读几百个分片）。
		sortPartPaths(objectPaths)
		if len(objectPaths) > analyzeMaxParts {
			objectPaths = objectPaths[:analyzeMaxParts]
		}
	}

	// 读取 + 解压。
	objects := make(map[string][]byte, len(objectPaths))
	for _, p := range objectPaths {
		raw, err := reader.ReadFile(ctx, p, 64<<20)
		if err != nil {
			// 单个分片读失败不中断整体（部分分析比没有分析有用）。
			continue
		}
		plain := raw
		if len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			if dec, derr := training_capture.DecompressPart(raw); derr == nil {
				plain = dec
			} else {
				continue // 解压失败：跳过该分片，而不是把压缩字节当文本喂给模型
			}
		}
		// 只取文件名（Assembly 内部按分片序号排序）。
		name := p
		if idx := strings.LastIndex(p, "/"); idx >= 0 {
			name = p[idx+1:]
		}
		objects[name] = plain
	}

	return training_capture.AssembleConversation(objects, maxBytes), nil
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
	sb.WriteString("\n\n---\n对话记录正文：\n")
	sb.WriteString(a.Text)
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
	return true
}
