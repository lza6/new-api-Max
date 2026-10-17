package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relay/channel"
	relaycommon "github.com/lza6/new-api-Max/relay/common"
	"github.com/lza6/new-api-Max/relaykit/dto"
	relaykittypes "github.com/lza6/new-api-Max/relaykit/types"
	"github.com/lza6/new-api-Max/service"
	hosttypes "github.com/lza6/new-api-Max/types"
)

// OpenAI 运维类端点（/v1/files、/v1/fine-tunes、/v1/batches）的转发实现。
//
// 设计约束（与项目既有事故教训一致）：
//   - **不绕过 relay 生命周期**：仍由 controller.Relay 做鉴权、选渠道、预扣费、
//     失败退款与日志；本文件只负责「发出去 + 回传 + 成功结算」。
//   - **不清楚的收费就不收**：文件元数据/列表/查询是免费的；fine-tune 创建与
//     batch 创建会产生上游真实费用，必须由管理员显式配置
//     `OPENAI_OPS_FLAT_QUOTA`（按次额度）；未配置时明确拒绝，绝不免费放行。
//   - **原始 body 透传**：files 是 multipart，batches/fine-tunes 是 JSON；
//     不做 chat 协议级解析，避免把合法请求改坏。

const openAIOpsMaxErrorBodyBytes = 64 << 10

// OpenAIOpsIsPaid 报告本次操作是否会产生上游费用、需要预扣/结算。
//
// 只有两类创建操作是付费的：
//   - POST /v1/fine-tunes
//   - POST /v1/batches
//
// 列表/查询/下载/删除/cancel 都按免费处理（上游可能收存储费，但网关不代收；
// 这比按错误口径收费更安全，且不会出现「用户被静默扣费」）。
//
// 必须传入**解码后的** `c.Request.URL.Path`。不能用 `URL.String()`：后者保留
// `%2D`/`%62` 等百分号编码，`POST /v1/fine%2Dtunes` 会被 Gin 路由到 fine-tunes，
// 但字面量比较不相等 → 被误判为免费，形成计费绕过。
func OpenAIOpsIsPaid(method, decodedPath string) bool {
	if !strings.EqualFold(method, http.MethodPost) {
		return false
	}
	switch strings.TrimRight(decodedPath, "/") {
	case "/v1/fine-tunes", "/v1/batches":
		return true
	default:
		return false
	}
}

// OpenAIOpsPathSupported 只允许明确列出的运维端点（含子资源）。
// 防止通过额外转义/多段路径把请求送进未预期的上游路径。
func OpenAIOpsPathSupported(decodedPath string) bool {
	path := strings.TrimRight(decodedPath, "/")
	if path == "/v1/files" || path == "/v1/fine-tunes" || path == "/v1/batches" {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" {
		return false
	}
	switch parts[1] {
	case "files":
		return len(parts) == 3 || (len(parts) == 4 && parts[3] == "content")
	case "fine-tunes":
		return len(parts) == 3 || (len(parts) == 4 && (parts[3] == "cancel" || parts[3] == "events"))
	case "batches":
		return len(parts) == 3 || (len(parts) == 4 && parts[3] == "cancel")
	default:
		return false
	}
}

// OpenAIOpsPriceData 为运维类端点构造价格数据。
//
// 免费操作返回 FreeModel；付费操作要求 OPENAI_OPS_FLAT_QUOTA > 0，
// 否则返回可解释错误（由 controller.Relay 转成 400），不猜测价格。
func OpenAIOpsPriceData(method, decodedPath string) (hosttypes.PriceData, error) {
	priceData := hosttypes.PriceData{}
	if !OpenAIOpsIsPaid(method, decodedPath) {
		priceData.FreeModel = true
		return priceData, nil
	}
	flat := common.GetEnvOrDefault("OPENAI_OPS_FLAT_QUOTA", 0)
	if flat <= 0 {
		return priceData, errors.New("fine-tune/batch creation pricing is not configured: set OPENAI_OPS_FLAT_QUOTA to the per-request quota")
	}
	// 单请求额度必须落在 int32 边界内，避免批量累计接近 64 位回绕。
	if int64(flat) > int64(common.MaxQuota) {
		return priceData, fmt.Errorf("OPENAI_OPS_FLAT_QUOTA=%d exceeds the safe per-request quota %d", flat, common.MaxQuota)
	}
	priceData.UsePrice = true
	priceData.ModelPrice = float64(flat) / common.QuotaPerUnit
	priceData.Quota = flat
	priceData.QuotaToPreConsume = flat
	return priceData, nil
}

// OpenAIOpsHandler 把请求原样转发到上游并回传响应。
//
// 成功（2xx）时：复制响应体；付费操作按配置的固定额度结算并记录消费日志。
// 上游非 2xx 时：读取有限错误体并返回 NewAPIError，由 controller.Relay 统一
// 输出错误并触发预扣额度退款。
func OpenAIOpsHandler(c *gin.Context, info *relaycommon.RelayInfo) *relaykittypes.NewAPIError {
	// 只接受明确列出的运维端点；解码后的路径做白名单校验，
	// 防止 `%2D`/多段转义把请求送进未预期路径。
	if !OpenAIOpsPathSupported(c.Request.URL.Path) {
		return relaykittypes.NewErrorWithStatusCode(
			fmt.Errorf("unsupported OpenAI ops path: %s", c.Request.URL.Path),
			relaykittypes.ErrorCodeInvalidRequest, http.StatusBadRequest,
			relaykittypes.ErrOptionWithSkipRetry())
	}
	// 与其他 handler 一致：先初始化 ChannelMeta（携带 base_url/api_type/api_key）。
	// 不调用会 nil dereference —— ChannelMeta 是嵌入指针，GenRelayInfo 阶段为空。
	info.InitChannelMeta(c)
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return relaykittypes.NewError(errors.New("no adaptor for channel type"), relaykittypes.ErrorCodeGetChannelFailed, relaykittypes.ErrOptionWithSkipRetry())
	}
	if _, ok := info.Request.(*dto.OpenAIOpsRequest); !ok {
		return relaykittypes.NewError(errors.New("invalid OpenAI ops request"), relaykittypes.ErrorCodeInvalidRequest, relaykittypes.ErrOptionWithSkipRetry())
	}
	// 直接用 body storage：大文件已溢写磁盘，不能先 ReadAll 进堆。
	storage, storageErr := common.GetBodyStorage(c)
	if storageErr != nil {
		return relaykittypes.NewError(storageErr, relaykittypes.ErrorCodeReadRequestBodyFailed, relaykittypes.ErrOptionWithSkipRetry())
	}

	ref := parseOpenAIOpsObjectRef(c.Request.URL.Path, c.Request.Method)
	if ref.Id != "" && !ref.IsCollection {
		owned, ownErr := model.OpenAIOpsObjectOwned(info.UserId, ref.Kind, ref.Id)
		if ownErr != nil {
			return relaykittypes.NewError(ownErr, relaykittypes.ErrorCodeQueryDataError, relaykittypes.ErrOptionWithSkipRetry())
		}
		if !owned {
			// 不确认对象是否存在，统一 404。
			return relaykittypes.NewErrorWithStatusCode(
				fmt.Errorf("object not found"), relaykittypes.ErrorCode("object_not_found"),
				http.StatusNotFound, relaykittypes.ErrOptionWithSkipRetry())
		}
	}

	resp, err := channel.DoApiRequest(adaptor, c, info, storage)
	if err != nil {
		return relaykittypes.NewError(err, relaykittypes.ErrorCode("openai_ops_request_failed"), relaykittypes.ErrOptionWithSkipRetry())
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, openAIOpsMaxErrorBodyBytes))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return relaykittypes.NewErrorWithStatusCode(
			fmt.Errorf("upstream returned %d: %s", resp.StatusCode, message),
			relaykittypes.ErrorCode("openai_ops_upstream_error"),
			resp.StatusCode,
			relaykittypes.ErrOptionWithSkipRetry(),
		)
	}

	// 只复制与客户端有关的响应头；不复制 Content-Length/Content-Encoding，
	// 避免 Go transport 已解压后仍声明 gzip 导致客户端解析失败。
	for _, key := range []string{"Content-Type", "Content-Disposition", "Cache-Control", "ETag", "OpenAI-Processing-Ms", "OpenAI-Version"} {
		if values := resp.Header.Values(key); len(values) > 0 {
			c.Writer.Header()[key] = values
		}
	}
	var copyErr error
	needBuffer := ref.Action == "list" || ref.Action == "create"
	if needBuffer {
		buffered, readErr := io.ReadAll(io.LimitReader(resp.Body, openAIOpsMaxBufferBytes))
		if readErr != nil {
			return relaykittypes.NewError(readErr, relaykittypes.ErrorCode("openai_ops_response_read_failed"), relaykittypes.ErrOptionWithSkipRetry())
		}
		if ref.Action == "list" {
			buffered = filterOwnedOpsList(buffered, info.UserId, ref.Kind)
		} else if id := extractOpsObjectID(buffered); id != "" {
			if _, recordErr := model.RecordOpenAIOpsObject(info.UserId, ref.Kind, id); recordErr != nil {
				common.SysError("record OpenAI ops object ownership failed: " + recordErr.Error())
			}
		}
		c.Writer.WriteHeader(resp.StatusCode)
		_, copyErr = c.Writer.Write(buffered)
	} else {
		c.Writer.WriteHeader(resp.StatusCode)
		_, copyErr = io.Copy(c.Writer, resp.Body)
	}
	if copyErr == nil && (ref.Action == "delete" || ref.Action == "cancel") {
		_ = model.DeleteOpenAIOpsObject(info.UserId, ref.Kind, ref.Id)
	}

	quota := 0
	if OpenAIOpsIsPaid(c.Request.Method, c.Request.URL.Path) {
		// 实际费用必须由**配置的按次额度**决定，而不是预扣额度：
		// 信任用户（trust quota）与订阅计费路径的 preConsumed 可能为 0，
		// 用 preConsumed 当实际费用会让付费操作静默免费。
		priceData, priceErr := OpenAIOpsPriceData(c.Request.Method, c.Request.URL.Path)
		if priceErr != nil {
			// 价格在预扣阶段已校验过；这里再失败说明配置在请求期间被改动，
			// 按「宁可少收也不乱收」返回明确错误，并把已预扣额度退回。
			if info.Billing != nil {
				info.Billing.Refund(c)
			}
			return relaykittypes.NewError(priceErr, relaykittypes.ErrorCodeModelPriceError,
				relaykittypes.ErrOptionWithSkipRetry())
		}
		quota = priceData.Quota
	}
	settleErr := service.SettleBilling(c, info, quota)
	if settleErr != nil {
		common.SysError("settle OpenAI ops billing failed: " + settleErr.Error())
		// 结算失败时不能把统计记成已收费：日志保留，但 quota/用量按 0 记。
		quota = 0
	}

	tokenName := c.GetString("token_name")
	path := strings.SplitN(info.RequestURLPath, "?", 2)[0]
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId:      info.ChannelId,
		ModelName:      info.OriginModelName,
		TokenName:      tokenName,
		Quota:          quota,
		Content:        fmt.Sprintf("%s %s", c.Request.Method, path),
		TokenId:        info.TokenId,
		Group:          info.UsingGroup,
		RequestBytes:   storage.Size(),
		ResponseBytes:  info.ResponseBytes,
		UseTimeSeconds: int(time.Since(info.StartTime).Seconds()),
	})
	if quota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, quota)
		model.UpdateChannelUsedQuota(info.ChannelId, quota)
	}

	if copyErr != nil {
		// 响应已开始写出，不能再写第二份错误体；只记录。
		common.SysError("openai ops response copy failed: " + copyErr.Error())
	}
	return nil
}

const openAIOpsMaxBufferBytes = 8 << 20

type openAIOpsObjectRef struct {
	Kind         string // file / fine-tune / batch
	Id           string
	Action       string // list/create/get/delete/content/cancel/events
	IsCollection bool
}

// parseOpenAIOpsObjectRef 解析对象归属检查所需的 kind/id/action。
func parseOpenAIOpsObjectRef(path, method string) openAIOpsObjectRef {
	parts := strings.Split(strings.TrimPrefix(strings.TrimRight(path, "/"), "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" {
		return openAIOpsObjectRef{}
	}
	kind := ""
	switch parts[1] {
	case "files":
		kind = "file"
	case "fine-tunes":
		kind = "fine-tune"
	case "batches":
		kind = "batch"
	default:
		return openAIOpsObjectRef{}
	}
	ref := openAIOpsObjectRef{Kind: kind}
	switch len(parts) {
	case 2:
		ref.IsCollection = true
		if strings.EqualFold(method, http.MethodGet) {
			ref.Action = "list"
		} else {
			ref.Action = "create"
		}
	case 3:
		ref.Id = parts[2]
		if strings.EqualFold(method, http.MethodDelete) {
			ref.Action = "delete"
		} else {
			ref.Action = "get"
		}
	case 4:
		ref.Id = parts[2]
		ref.Action = parts[3]
	}
	return ref
}

// filterOwnedOpsList 只保留当前用户拥有的对象，避免共享渠道 key 下
// 列表接口泄漏其他用户的上游对象。
func filterOwnedOpsList(body []byte, userId int, kind string) []byte {
	var m map[string]any
	if common.Unmarshal(body, &m) != nil {
		return body
	}
	data, ok := m["data"].([]any)
	if !ok {
		return body
	}
	owned, err := model.OpenAIOpsObjectIDs(userId, kind)
	if err != nil {
		owned = map[string]struct{}{}
	}
	filtered := make([]any, 0, len(data))
	for _, item := range data {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := obj["id"].(string)
		if _, ok := owned[id]; ok {
			filtered = append(filtered, item)
		}
	}
	m["data"] = filtered
	out, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// extractOpsObjectID 从创建响应里取上游对象 id（仅用于归属登记）。
func extractOpsObjectID(body []byte) string {
	var m map[string]any
	if common.Unmarshal(body, &m) != nil {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}
