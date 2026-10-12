package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/types"
	"golang.org/x/net/http/httpguts"
)

// 产物字节抓取 —— G6 §8.2.1「接线 Persist」。
//
// 背景（审计硬事实，`已验证`）：在本次接线之前，全仓**没有任何地方下载产物字节**。
// 任务轮询只把上游 URL 存进 `task.PrivateData.ResultURL`，因此
// `TaskArtifactStore.Persist` 在生产代码里零调用方——站内图床
// （`TASK_ARTIFACT_STORE_MODE=local`）只能读、从不写。
//
// 关键设计约束（决定「抓什么」）：读取侧 `controller/task.go` 只在**插件任务**分支
// 调用 `Resolve(task, artifactKey)`，且 `artifactKey` 来自插件 `ListArtifacts` 的声明；
// legacy 任务分支（`controller/task.go:299`）直接代理上游、**完全不查 store**。
// 所以只抓「读取侧真的会解析到的 key」——否则写进磁盘的字节永远无人读取，
// 等于把「读了没人写」换成「写了没人读」，仍是伪闭环。
//
// 三条不变量（与开关语义绑定）：
//  1. `Enabled()==false`（默认 `upstream` 模式）→ **零行为变化**：不发任何出站请求，
//     直接返回。这是本改动最大的风险面，由用例
//     `TestCaptureTaskArtifactsDisabledStoreIsNoop` 保护。
//  2. 抓取失败（URL 过期/不可达/超限/被 SSRF 拦）**不得影响任务状态与计费**：
//     产物落盘是尽力而为，只记日志，上游代理路径保持可用。
//  3. 产物 URL 来自上游/插件，属**外部输入** → 必须走 SSRF 防护
//     （`ValidateSSRFProtectedFetchURL` + `GetSSRFProtectedHTTPClient`，后者含拨号级校验）。

const (
	// artifactCaptureTimeout 单次产物抓取的墙钟上限。视频产物可能较大，
	// 但不能让抓取 goroutine 被慢上游长期占用。
	artifactCaptureTimeout = 5 * time.Minute
	// artifactCaptureMaxArtifacts 单任务单轮抓取的产物数上限（插件契约上限是 64，
	// 抓取是带宽密集动作，这里更保守；超出部分留给下一轮）。
	artifactCaptureMaxArtifacts = 16
	// artifactCaptureMaxHeaderBytes 单个产物请求头长度上限（插件声明值不可信）。
	artifactCaptureMaxHeaderBytes = 8192
)

// TaskArtifactFetchPlan 描述「去哪里取某个产物 key 的字节」。
// 由 main 包从任务插件（relay 层）解析后注入，避免 service -> relay 的导入环。
type TaskArtifactFetchPlan struct {
	Artifact types.TaskArtifact
	URL      string
	Method   string
	Headers  map[string]string
	Body     []byte
}

// TaskArtifactFetchPlanner 由 main 包注入：返回该任务需要落盘的产物抓取计划。
// 未注入、任务非插件任务、或插件未声明产物时返回空 → 捕获为 no-op。
var TaskArtifactFetchPlanner func(task *model.Task) ([]TaskArtifactFetchPlan, error)

// artifactMaxFileBytes 单产物上限的唯一来源（buildTaskArtifactStore 与抓取侧共用，
// 避免两处各写一份 env 解析而漂移）。
func artifactMaxFileBytes() int64 {
	maxBytes := int64(common.GetEnvOrDefault("TASK_ARTIFACT_MAX_FILE_MB", defaultArtifactMaxFileBytes>>20)) << 20
	if maxBytes <= 0 {
		return defaultArtifactMaxFileBytes
	}
	return maxBytes
}

// captureTaskArtifacts 在任务成功终态后抓取产物并落盘（尽力而为、异步）。
//
// 挂点选择理由（读代码得出，非推测）：
//   - 产物身份（key/type）与取字节的位置都只能从**插件**解析得到；插件任务上
//     `task.PrivateData.ResultURL` 通常是站内代理 URL（不是上游地址），
//     拿它当抓取源会自请求回环。
//   - 必须放在 `UpdateWithStatus` CAS **胜出之后**：CAS 失败说明另一进程/另一轮
//     已推进该任务，此时再抓取会重复下载同一产物（并发轮询同一任务）。
//   - 必须放在终态（SUCCESS）**且结算之后**：抓取失败绝不能反过来影响计费。
//   - 网络抓取经 gopool 异步派发：产物可达数十 MB，同步执行会拖住 15s 一轮的轮询
//     循环。计划解析（可能触发插件 JS 引擎、读 task 字段）在**调用方 goroutine**上
//     同步完成，避免与轮询后续对 task 的写入并发。
func captureTaskArtifacts(ctx context.Context, task *model.Task) {
	store := GetTaskArtifactStore()
	// 不变量 1：默认 upstream 模式零行为变化。必须先于任何网络/插件动作返回。
	if store == nil || !store.Enabled() || task == nil || TaskArtifactFetchPlanner == nil {
		return
	}
	plans, err := TaskArtifactFetchPlanner(task)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task %s artifact plan resolution failed: %v", task.TaskID, err))
		return
	}
	if len(plans) == 0 {
		return
	}
	if len(plans) > artifactCaptureMaxArtifacts {
		plans = plans[:artifactCaptureMaxArtifacts]
	}
	plans = append([]TaskArtifactFetchPlan(nil), plans...)

	gopool.Go(func() {
		// 脱离轮询 ctx：一轮结束即取消 ctx，会把下载中途掐断。
		captureCtx, cancel := context.WithTimeout(context.Background(), artifactCaptureTimeout)
		defer cancel()
		persistTaskArtifactPlans(captureCtx, store, task, plans)
	})
}

// persistTaskArtifactPlans 逐条抓取并落盘；任一失败只记日志，不影响其他产物。
func persistTaskArtifactPlans(ctx context.Context, store TaskArtifactStore, task *model.Task, plans []TaskArtifactFetchPlan) {
	for _, plan := range plans {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		if err := persistTaskArtifactPlan(ctx, store, task, plan); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf(
				"task %s artifact %q capture failed (delivery stays on upstream proxy): %v",
				task.TaskID, plan.Artifact.Key, err,
			))
		}
	}
}

// persistTaskArtifactPlan 抓取单个产物并把字节流交给 store.Persist。
// 失败只返回错误（由调用方记日志），绝不改动任务状态与计费字段。
func persistTaskArtifactPlan(ctx context.Context, store TaskArtifactStore, task *model.Task, plan TaskArtifactFetchPlan) error {
	if task == nil || store == nil {
		return errors.New("artifact capture is not configured")
	}
	rawURL := strings.TrimSpace(plan.URL)
	if rawURL == "" {
		return errors.New("artifact fetch plan has no url")
	}
	if isSelfTaskMediaPath(rawURL, task.TaskID) {
		return errors.New("artifact fetch plan points back at this gateway")
	}

	body, release, err := openTaskArtifactStream(ctx, plan, rawURL)
	if err != nil {
		return err
	}
	defer release()

	// 上限：local store 用 maxBytes+1 探界并**报错**（绝不静默截断）；这里再套同一上限，
	// 防止非 local 实现被喂无界流。
	limit := artifactMaxFileBytes() + 1
	ref, err := store.Persist(ctx, task, types.TaskArtifact{
		Key:      plan.Artifact.Key,
		Type:     plan.Artifact.Type,
		MimeType: plan.Artifact.MimeType,
	}, io.LimitReader(body, limit))
	if err != nil {
		return fmt.Errorf("artifact persist failed: %w", err)
	}
	if ref == nil {
		return errors.New("artifact store returned no reference")
	}
	return nil
}

// openTaskArtifactStream 按计划打开产物字节流。返回的 release 必须被调用。
func openTaskArtifactStream(ctx context.Context, plan TaskArtifactFetchPlan, rawURL string) (io.Reader, func(), error) {
	noop := func() {}
	if strings.HasPrefix(rawURL, "data:") {
		// 纯函数型插件在沙箱内直接生成字节并回传 data: URL（无上游可指）。
		reader, err := decodeTaskArtifactDataURL(rawURL)
		if err != nil {
			return nil, noop, err
		}
		return reader, noop, nil
	}

	// 不变量 3：URL 预校验走 SSRF 防护（域/端口/IP 过滤与私网拒绝）。
	// 先做本地可判定的形状/方法校验，再走会触发 DNS 解析的 SSRF 校验——否则一个
	// 方法错误的计划会先花掉一次 DNS 查询才被拒绝。
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, noop, errors.New("artifact url is not an absolute http(s) url")
	}

	method := strings.ToUpper(strings.TrimSpace(plan.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost:
	default:
		// HEAD 没有字节可落盘；其余方法不是抓取语义。
		return nil, noop, fmt.Errorf("artifact fetch method %q is not supported", method)
	}

	if err := ValidateSSRFProtectedFetchURL(rawURL); err != nil {
		return nil, noop, fmt.Errorf("artifact url rejected: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(plan.Body))
	if err != nil {
		return nil, noop, fmt.Errorf("artifact request build failed: %w", err)
	}
	for name, value := range plan.Headers {
		if err := applyTaskArtifactHeader(req.Header, name, value); err != nil {
			return nil, noop, err
		}
	}

	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		return nil, noop, errors.New("ssrf protected http client is unavailable")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, noop, fmt.Errorf("artifact fetch failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, noop, fmt.Errorf("artifact upstream returned status %d", resp.StatusCode)
	}
	return resp.Body, func() { _ = resp.Body.Close() }, nil
}

// applyTaskArtifactHeader 只放行安全的请求头；插件声明值不可信，禁止覆盖
// 主机/长度/连接类头（防请求走私与把凭据发到被改写的目标）。
func applyTaskArtifactHeader(destination http.Header, name, value string) error {
	name = strings.TrimSpace(name)
	if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) ||
		len(value) > artifactCaptureMaxHeaderBytes {
		return errors.New("artifact request header was rejected")
	}
	switch strings.ToLower(name) {
	case "host", "content-length", "accept-encoding", "connection", "proxy-connection",
		"keep-alive", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return fmt.Errorf("artifact request header %q is not allowed", name)
	}
	destination.Set(name, value)
	return nil
}

// decodeTaskArtifactDataURL 解析纯函数型插件返回的 data: URL。
func decodeTaskArtifactDataURL(rawURL string) (io.Reader, error) {
	header, payload, ok := strings.Cut(rawURL, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.Contains(header, ";base64") {
		return nil, errors.New("artifact data url is not base64-encoded")
	}
	// 用完整 payload 判定编码表（RawStdEncoding 对含 '=' 的输入会失败，反之亦然）。
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if _, err := encoding.DecodeString(payload); err == nil {
			return base64.NewDecoder(encoding, strings.NewReader(payload)), nil
		}
	}
	return nil, errors.New("artifact data url payload is not valid base64")
}

// isSelfTaskMediaPath 判断 URL 是否指向本网关的任务媒体代理路径（防抓取回环）。
func isSelfTaskMediaPath(rawURL, taskID string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || taskID == "" {
		return false
	}
	path, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		path = parsed.Path
	}
	if path == "/v1/videos/"+taskID+"/content" {
		return true
	}
	prefix := "/v1/tasks/" + taskID + "/artifacts/"
	return strings.HasPrefix(path, prefix) && strings.HasSuffix(path, "/content")
}
