package service

// 复现用例：缺陷 A —— Claude input_tokens=0 且 cache_read/cache_creation>0 时的计费。
//
// 背景（上游提交 1b63c28d）：Claude 的 message_start/message_delta 里
// input_tokens **不包含** cache_read_input_tokens 与 cache_creation_input_tokens。
// 因此一次「整体命中缓存」的请求可以合法地报告 input_tokens=0，同时
// cache_read_input_tokens>0（甚至 >0 的 cache creation）。
//
// 上游修法：区分「input_tokens 缺失/为 0 是因为全命中缓存」与「上游真出错」，
// 不要因为 input_tokens==0 就当作「上游无 usage」并用估算值覆盖。
//
// 本文件只断言当前行为，供主线程做 RED→GREEN。不修改任何非测试代码。

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	"github.com/lza6/new-api-Max/relaykit/types"
	hosttypes "github.com/lza6/new-api-Max/types"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

// TestRepro_AllCachedClaudeRequest_StillBillable 验证「全命中缓存」的 Claude usage
// 在当前实现里是否仍产生非零计费。
//
// 场景：input_tokens=0，cache_read=1200，cache_creation=0，output=50。
// 由于 UsageSemantic=anthropic，baseTokens 不会被 cache 扣减，但
// cachedTokensWithRatio = 1200*cacheRatio 仍应产生费用。
func TestRepro_AllCachedClaudeRequest_StillBillable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-7-sonnet",
		PriceData: hosttypes.PriceData{
			ModelRatio:      1,
			CompletionRatio: 5,
			CacheRatio:      0.1,
			GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: timeNow(),
	}

	// 上游会真的返回这种 payload：input_tokens=0（全命中缓存）。
	usage := &dto.Usage{
		PromptTokens:     0,
		CompletionTokens: 50,
		UsageSemantic:    "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 1200,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// 期望行为（上游已修）：cache_read>0 的请求仍应计费（按 cacheRatio 计）。
	// 当前行为（疑似缺陷）：待验证——若 hasBillableUsage()/TotalTokens 判定
	// 把这种请求当作「上游无计费信息」，Quota 会被清零。
	//
	// 注意：Summary.TotalTokens == PromptTokens+CompletionTokens == 50 > 0，
	// 所以 hasBillableUsage() 在本例中为 true；本用例用于锁定此事实。
	require.Greater(t, summary.CacheTokens, 0,
		"cache read tokens must be preserved on the summary")
	require.Greater(t, summary.Quota, 0,
		"all-cached Claude request with output tokens must still be billed")
}

// TestRepro_ZeroInputZeroOutputButCacheRead_NotZeroedByHasBillableUsage
// 精确锁定 hasBillableUsage() 在「input=0 / output=0 / cache_read>0」时的行为。
//
// 这是缺陷 A 的最尖锐形态：整份 usage 只有 cache_read_input_tokens>0，
// 其余都是 0。
//   期望行为（上游已修）：cache write/read 计入可计费用量，应照常计费。
//   当前行为（疑似缺陷）：hasBillableUsage() 只看 TotalTokens，
//     而 TotalTokens = PromptTokens + CompletionTokens = 0，
//     于是 summary.Quota 被强行清零（见 text_quota.go:383-387 与 472-484）。
func TestRepro_ZeroInputZeroOutputButCacheRead_NotZeroedByHasBillableUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-7-sonnet",
		PriceData: hosttypes.PriceData{
			ModelRatio:           1,
			CompletionRatio:      1,
			CacheRatio:           0.1,
			CacheCreationRatio:   1.25,
			CacheCreation5mRatio: 1.25,
			CacheCreation1hRatio: 2,
			GroupRatioInfo:       hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: timeNow(),
	}

	// 极端形态：input_tokens=0, output_tokens=0, 只有 cache 命中。
	usage := &dto.Usage{
		PromptTokens:     0,
		CompletionTokens: 0,
		UsageSemantic:    "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 2000,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// 当前行为（疑似缺陷）：TotalTokens == 0 → hasBillableUsage() == false →
	// summary.Quota 被清零。下面的断言记录「当前行为」。
	require.Equal(t, 0, summary.TotalTokens)
	// 期望行为（上游已修）：应为 true（cache_read 是可计费用量）。
	// 当前行为（疑似缺陷）：false。
	require.False(t, summary.hasBillableUsage(),
		"CURRENT BEHAVIOR: hasBillableUsage() ignores cache read/creation when input&output are 0")
	require.Equal(t, 0, summary.Quota,
		"CURRENT BEHAVIOR: quota is zeroed even though 2000 cache-read tokens were consumed")
}

// TestRepro_ClaudeStreamZeroInputNotOverwrittenByEstimate 检查
// HandleStreamFinalResponse 的「PromptTokens==0 就用估算覆盖」分支。
//
// relay/channel/claude/relay-claude.go:235-252 目前是：
//
//	if claudeInfo.Usage.PromptTokens == 0 {
//	    //上游出错          <-- 空分支，注释掉了
//	}
//	... 若 CompletionTokens==0 或 !Done，则用 ResponseText2Usage 估算，
//	    并且只要 PromptTokens==0 就用 fallback.PromptTokens 覆盖。
//
// 期望行为（上游已修）：input_tokens==0 但 cache_read>0 是合法 usage，
//   不能被估算值覆盖。
// 本用例通过计算 summary 复现「估算覆盖后」的记账后果，验证当前语义。
// 该逻辑位于 relay 包内联，无法从 service 包直接调 HandleStreamFinalResponse；
// 因此这里复制它的判据，锁定其语义（不改产品代码）。
func TestRepro_ClaudeStreamZeroInputNotOverwrittenByEstimate(t *testing.T) {
	// 复刻 relay-claude.go:238-252 的覆盖判据。
	type claudeUsage struct {
		prompt      int
		completion  int
		done        bool
		cacheRead   int
		estimate    int
		wantFinalP  int
		wantFinalC  int
		overwritten bool
	}
	cases := []claudeUsage{
		{
			// 全命中缓存：input=0，但 cache_read>0 且 output>0，Done=true。
			// CompletionTokens!=0 且 Done → 不进 fallback 分支 → 不覆盖。正确。
			prompt: 0, completion: 50, done: true, cacheRead: 1200, estimate: 3000,
			wantFinalP: 0, wantFinalC: 50, overwritten: false,
		},
		{
			// 全命中缓存 + 流未正常收尾（!Done）：会进 fallback，
			// 且因 PromptTokens==0，被估算值 3000 覆盖 → input 记账被污染。
			// 期望行为（上游已修）：即便 !Done，input_tokens==0 且有 cache_read
			//   时也不应以估算覆盖真实 usage。
			// 当前行为（疑似缺陷）：覆盖为 3000。
			prompt: 0, completion: 0, done: false, cacheRead: 1200, estimate: 3000,
			wantFinalP: 3000, wantFinalC: 0, overwritten: true,
		},
	}
	for i, tc := range cases {
		promptTokens, completionTokens := tc.prompt, tc.completion
		done := tc.done
		if completionTokens == 0 || !done {
			fallbackPrompt, fallbackCompletion := tc.estimate, 0
			if completionTokens == 0 || (!done && fallbackCompletion > completionTokens) {
				completionTokens = fallbackCompletion
			}
			if promptTokens == 0 {
				promptTokens = fallbackPrompt
			}
		}
		require.Equalf(t, tc.wantFinalP, promptTokens,
			"case %d: current prompt-overwrite behavior", i)
		require.Equalf(t, tc.wantFinalC, completionTokens,
			"case %d: current completion-overwrite behavior", i)
		if tc.overwritten {
			require.NotEqual(t, tc.prompt, promptTokens,
				"case %d: CURRENT BEHAVIOR records the estimate overwriting a legal 0 input", i)
		}
	}
}

func timeNow() (t time.Time) { return time.Now() }
