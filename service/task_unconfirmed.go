/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// 任务"unconfirmed"三态提交 —— 提交结果不可确认时的资损防线。
//
// 背景（N10 侦察证据 MoneyPrinterTurbo volcengine_seedance.py:251）：
// 任务提交接口超时 / 5xx / 响应不可读时，远端可能已经创建了付费任务。
// 旧逻辑直接走 BillingSession.Refund 退预扣费，导致"用户已付费、本端却退款"的资损。
//
// 本模块将这种提交结果不可确认的情况建模为一等状态：
//   - RelayTaskSubmit 失败且属于不可确认错误（网络错/5xx/响应不可读）时，
//     通过 SubmitUnconfirmedInfo 把本次失败标记进 gin.Context，控制器收到
//     该标记后不再 Refund 预扣费，而是落库一个 unconfirmed 任务行（见
//     MarkUnconfirmedOnTask / AppendUnconfirmedSubmitMarker）。
//   - 若适配器在失败响应中还能提取出 remote task id，则一并记录到
//     Task.Data 的 remote_task_id_hint，供轮询器按 id 兜底查询。
//   - 轮询器对 unconfirmed 任务：有 hint 直接查远端；无 hint 在
//     task.unconfirmed_window 窗口内反复查询；超窗仍未确认 → 退款并在
//     Task.Data 标记 resolution=refunded_after_window（见 task_polling.go）。
package service

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	taskdto "github.com/lza6/new-api-Max/dto"
	"github.com/lza6/new-api-Max/model"
)

// SubmitUnconfirmedInfo 一次提交失败中"结果不可确认"的判定结果。
type SubmitUnconfirmedInfo struct {
	// Unconfirmed 表示本次提交失败属于不可确认错误，应当落 unconfirmed 任务
	// 而非退款。
	Unconfirmed bool
	// RemoteTaskIDHint 响应中若能提取到的远端任务 id（可为空）。
	RemoteTaskIDHint string
	// FailedAt 失败发生时间（unix 秒）。
	FailedAt int64
}

// ClassifySubmitFailure 判定一次任务提交失败是否属于"结果不可确认"。
//
// 复用 B2-1 的公共错误分类器（aisix "cooldown 与 retryability 解耦"模型）：
//   - 网络层错误（status<=0）、408、5xx → 结果不可确认（Timeout/ServerError 类），
//     远端可能已创建付费任务，Unconfirmed=true。
//   - 2xx 但响应体为空（ParseResponse 失败路径）→ 响应不可读，Unconfirmed=true。
//   - 其余（含 4xx 明确拒绝）→ 远端明确给出结论，结果可确认，Unconfirmed=false。
func ClassifySubmitFailure(statusCode int, body []byte, upstreamErr error) SubmitUnconfirmedInfo {
	info := SubmitUnconfirmedInfo{FailedAt: time.Now().Unix()}
	if statusCode >= 200 && statusCode < 300 {
		// 2xx 但响应体为空（ParseResponse 失败路径）：响应不可读 → 不可确认。
		info.Unconfirmed = len(body) == 0
		return info
	}
	_, _, class := ClassifyHTTPStatus(statusCode, "")
	switch class {
	case ErrClassTimeout, ErrClassServerError:
		info.Unconfirmed = true
	case ErrClassUnknown:
		// 网络层错误在分类器中归 Timeout；走到 Unknown 说明响应形态异常，
		// 保守视为不可确认（宁可不退，不可错退）。
		info.Unconfirmed = upstreamErr != nil
	}
	return info
}

// ExtractRemoteTaskIDHint 从失败响应体中尽力提取远端任务 id。
// 目前从 `data.task_id`、`output.task_id`、顶层 `task_id`、顶层 `id` 提取。
// 各平台适配器可在 DoRequest 失败时把响应体透传给本函数。
func ExtractRemoteTaskIDHint(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parsed map[string]any
	if err := common.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	for _, candidate := range []string{"task_id", "id"} {
		if v, ok := parsed[candidate].(string); ok && v != "" {
			return v
		}
	}
	if data, ok := parsed["data"].(map[string]any); ok {
		if v, ok := data["task_id"].(string); ok && v != "" {
			return v
		}
	}
	if output, ok := parsed["output"].(map[string]any); ok {
		if v, ok := output["task_id"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// MarkUnconfirmedOnError stamps the classification result onto a TaskError so
// controller/relay.go can decide whether to refund or to persist unconfirmed.
func MarkUnconfirmedOnError(taskErr *taskdto.TaskError, info SubmitUnconfirmedInfo) {
	if taskErr == nil {
		return
	}
	if !info.Unconfirmed {
		return
	}
	if info.RemoteTaskIDHint == "" {
		msg := []byte(taskErr.Message)
		info.RemoteTaskIDHint = ExtractRemoteTaskIDHint(msg)
	}
	taskErr.Data = taskErrDataWithUnconfirmed(info)
}

// MarkUnconfirmedOnContext stores the unconfirmed marker in the gin.Context so
// controller/relay.go can read it in the deferred refund path and instead
// persist an unconfirmed task row.
func MarkUnconfirmedOnContext(c *gin.Context, info SubmitUnconfirmedInfo) {
	common.SetContextKey(c, constant.ContextKeySubmitUnconfirmed, info)
}

// ReadUnconfirmedFromContext returns the unconfirmed marker previously stored
// by MarkUnconfirmedOnContext, or zero if none.
func ReadUnconfirmedFromContext(c *gin.Context) (SubmitUnconfirmedInfo, bool) {
	v, ok := common.GetContextKeyType[SubmitUnconfirmedInfo](c, constant.ContextKeySubmitUnconfirmed)
	return v, ok
}

// ClearUnconfirmedFromContext removes the marker so a later attempt that ends
// with a deterministic outcome (e.g. explicit 4xx) never inherits the previous
// attempt's unconfirmed classification.
func ClearUnconfirmedFromContext(c *gin.Context) {
	c.Set(string(constant.ContextKeySubmitUnconfirmed), SubmitUnconfirmedInfo{})
}

// AppendUnconfirmedSubmitMarker merges unconfirmed metadata into task data so
// the polling loop can drive the unconfirmed resolution path.
func AppendUnconfirmedSubmitMarker(task *model.Task, info SubmitUnconfirmedInfo) {
	if task == nil || !info.Unconfirmed {
		return
	}
	var data map[string]any
	_ = task.GetData(&data)
	if data == nil {
		data = map[string]any{}
	}
	data["submit_state"] = "unconfirmed"
	if info.RemoteTaskIDHint != "" {
		data["remote_task_id_hint"] = info.RemoteTaskIDHint
	}
	data["failed_at"] = info.FailedAt
	task.SetData(data)
}

// taskErrDataWithUnconfirmed serializes unconfirmed metadata into the TaskError
// Data field so clients can render it.
func taskErrDataWithUnconfirmed(info SubmitUnconfirmedInfo) any {
	out := map[string]any{
		"submit_state": "unconfirmed",
		"failed_at":    info.FailedAt,
	}
	if info.RemoteTaskIDHint != "" {
		out["remote_task_id_hint"] = info.RemoteTaskIDHint
	}
	return out
}

// IsUnconfirmedTask reports whether a task is in the unconfirmed submit state.
func IsUnconfirmedTask(task *model.Task) bool {
	if task == nil {
		return false
	}
	return strings.EqualFold(string(task.Status), "unconfirmed")
}
