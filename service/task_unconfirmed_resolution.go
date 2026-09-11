package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/logger"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/relay/channel/task/taskcommon"
	"github.com/lza6/new-api-Max/setting/task_setting"
)

// TaskUnconfirmedResolution 一次 unconfirmed 任务解析的结果。
type TaskUnconfirmedResolution string

const (
	UnconfirmedResolvedSuccess TaskUnconfirmedResolution = "success"
	UnconfirmedResolvedFailure TaskUnconfirmedResolution = "failure"
	UnconfirmedResolvedPending TaskUnconfirmedResolution = "pending"
	UnconfirmedResolvedRefund  TaskUnconfirmedResolution = "refunded_after_window"
)

// unconfirmedResolution 解析一条 unconfirmed 任务：
//
//   - remote_task_id_hint 非空 → 直接按 hint 查远端；远端终态则结算/退款。
//   - hint 为空 → 在 task.unconfirmed_window 窗口内反复查询（多次查询可累积
//     确认机会）；超窗仍未确认 → 退款并标记 resolution=refunded_after_window。
//
// 返回 true 表示本次已推进任务（终态或窗口期退款），false 表示保持 pending。
func unconfirmedResolution(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, windowMinutes int) bool {
	if task == nil || adaptor == nil {
		return false
	}

	var taskData map[string]any
	_ = task.GetData(&taskData)
	hint, _ := taskData["remote_task_id_hint"].(string)
	failedAt := int64(0)
	if v, ok := taskData["failed_at"].(float64); ok {
		failedAt = int64(v)
	}
	if failedAt == 0 {
		failedAt = task.SubmitTime
	}

	// 以数据库权威渠道元数据加载 key/baseURL/proxy（selectAll=true 才会读取
	// Key 列；false 会忽略 key 导致上游查询带空 key）。
	baseURL := ""
	channelType := -1
	key := task.PrivateData.Key
	proxy := ""
	if ch, err := model.GetChannelById(task.ChannelId, true); err == nil {
		channelType = ch.Type
		if ch.GetBaseURL() != "" {
			baseURL = ch.GetBaseURL()
		}
		if key == "" {
			key = ch.Key
		}
		proxy = ch.GetSetting().Proxy
	}
	if baseURL == "" && channelType >= 0 {
		baseURL = constant.GetChannelBaseURL(channelType)
	}

	if hint == "" {
		// 无 hint：有限窗口内反复查询，超窗退款。
		if failedAt > 0 && windowMinutes > 0 && time.Now().Unix() > failedAt+int64(windowMinutes)*60 {
			reason := fmt.Sprintf("unconfirmed task resolution window (%d minutes) expired", windowMinutes)
			return refundUnconfirmedTask(ctx, adaptor, task, reason, string(UnconfirmedResolvedRefund))
		}
		// 窗口内无 hint 时仍尝试按公开 task id 查询一次（多数平台公开 id 即上游 id）。
		if res := resolveWithUpstreamID(ctx, adaptor, task, task.GetUpstreamTaskID(), baseURL, key, proxy); res != "" {
			return finalizeUnconfirmed(ctx, adaptor, task, res)
		}
		return false
	}

	// 有 hint：直接按 hint 查远端。
	if res := resolveWithUpstreamID(ctx, adaptor, task, hint, baseURL, key, proxy); res != "" {
		return finalizeUnconfirmed(ctx, adaptor, task, res)
	}

	// hint 查询未得终态：同样检查窗口。
	if windowMinutes > 0 && failedAt > 0 && time.Now().Unix() > failedAt+int64(windowMinutes)*60 {
		reason := fmt.Sprintf("unconfirmed task resolution window (%d minutes) expired", windowMinutes)
		return refundUnconfirmedTask(ctx, adaptor, task, reason, string(UnconfirmedResolvedRefund))
	}
	return false
}

// resolveWithUpstreamID 按给定上游 id 查询一次任务状态。
// 返回终态解析结果（"" 表示仍 pending / 查询失败）。
func resolveWithUpstreamID(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, upstreamID, baseURL, key, proxy string) string {
	if upstreamID == "" {
		return ""
	}
	if task.PrivateData.UpstreamTaskID == "" {
		task.PrivateData.UpstreamTaskID = upstreamID
	}
	resp, err := adaptor.FetchTask(baseURL, key, task, proxy)
	if err != nil || resp == nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// 查询端点明确拒绝(4xx)/服务端故障(5xx): 本次不推进, 保持 unconfirmed。
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	taskResult, err := adaptor.ParseTaskResult(task, resp, body)
	if err != nil || taskResult == nil {
		// 响应不可解析: 本次不推进, 保持 unconfirmed, 触发方会继续窗口期重试。
		return ""
	}
	status := model.TaskStatus(taskResult.Status)
	if status != model.TaskStatusSuccess && status != model.TaskStatusFailure {
		// 仍在途/未知：更新进度但不结束。
		if taskResult.Progress != "" {
			task.Progress = taskResult.Progress
		}
		_, _ = task.UpdateWithStatus(model.TaskStatusUnconfirmed)
		return ""
	}
	// 状态已确定：先 CAS 从 UNCONFIRMED 推进到终态；CAS 失败说明其他进程已推进，
	// 直接返回对应终态标记，不再重复结算/退款。
	switch {
	case status == model.TaskStatusSuccess:
		won, _ := task.UpdateWithStatus(model.TaskStatusUnconfirmed)
		if !won {
			return string(UnconfirmedResolvedSuccess)
		}
		task.Status = model.TaskStatusSuccess
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = time.Now().Unix()
		}
		if taskResult.Url != "" {
			task.PrivateData.ResultURL = taskResult.Url
		}
		task.Data = redactVideoResponseBody(body)
		// 结算语义与 updateVideoSingleTask 一致：成功任务在无法差额结算时
		// 保留预扣额度作为最终扣费；只有失败任务才补做全额退款。
		settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
		return string(UnconfirmedResolvedSuccess)
	case status == model.TaskStatusFailure:
		won, _ := task.UpdateWithStatus(model.TaskStatusUnconfirmed)
		if !won {
			return string(UnconfirmedResolvedFailure)
		}
		task.Status = model.TaskStatusFailure
		task.Progress = taskcommon.ProgressComplete
		if task.FinishTime == 0 {
			task.FinishTime = time.Now().Unix()
		}
		if taskResult.Reason != "" {
			task.FailReason = taskResult.Reason
		}
		billingSettled := settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)
		if !billingSettled && task.Quota != 0 {
			RefundTaskQuota(ctx, task, task.FailReason)
		}
		return string(UnconfirmedResolvedFailure)
	default:
		return ""
	}
}

// finalizeUnconfirmed 将已解析终态标记写入 Task.Data 的 resolution 字段。
func finalizeUnconfirmed(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, resolution string) bool {
	if resolution == "" {
		return false
	}
	var data map[string]any
	_ = task.GetData(&data)
	if data == nil {
		data = map[string]any{}
	}
	data["resolution"] = resolution
	data["resolution_at"] = time.Now().Unix()
	task.SetData(data)
	_ = task.Update()
	logger.LogInfo(ctx, fmt.Sprintf("task %s unconfirmed resolved as %s", task.TaskID, resolution))
	return true
}

// refundUnconfirmedTask 将超窗仍未确认的任务按窗口退款，并标记 resolution。
func refundUnconfirmedTask(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, reason string, resolution string) bool {
	snap := task.Snapshot()
	task.Status = model.TaskStatusFailure
	task.Progress = taskcommon.ProgressComplete
	if task.FinishTime == 0 {
		task.FinishTime = time.Now().Unix()
	}
	task.FailReason = reason
	won, err := task.UpdateWithStatus(snap.Status)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("refundUnconfirmedTask CAS update error task %s: %v", task.TaskID, err))
		return false
	}
	if !won {
		return false
	}
	var data map[string]any
	_ = task.GetData(&data)
	if data == nil {
		data = map[string]any{}
	}
	data["resolution"] = string(resolution)
	data["resolution_at"] = time.Now().Unix()
	task.SetData(data)
	_ = task.Update()
	if task.Quota != 0 {
		RefundTaskQuota(ctx, task, reason)
	}
	logger.LogInfo(ctx, fmt.Sprintf("task %s unconfirmed refunded after window (%s)", task.TaskID, reason))
	return true
}

// ResolveUnconfirmedTasks 对数据库中所有 UNCONFIRMED 且未完成的任务执行一次解析。
// 由轮询调度每 15 秒触发；一次最多处理 constant.TaskQueryLimit 条。
func ResolveUnconfirmedTasks(ctx context.Context) error {
	if GetTaskAdaptorFunc == nil {
		return nil
	}
	tasks := model.GetUnconfirmedUnfinishedTasks(constant.TaskQueryLimit)
	if len(tasks) == 0 {
		return nil
	}
	windowMinutes := getUnconfirmedWindowMinutes()
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		adaptor := GetTaskAdaptorFunc(task.Platform)
		if adaptor == nil {
			continue
		}
		// 以数据库最新状态为基准加载渠道元数据，避免内存缓存未预热导致解析静默跳过。
		task = loadUnconfirmedTaskWithChannel(task)
		unconfirmedResolution(ctx, adaptor, task, windowMinutes)
	}
	return nil
}

// loadUnconfirmedTaskWithChannel 用数据库渠道元数据补齐 task 的 key，
// 供 unconfirmedResolution 直接使用（渠道可能未在内存缓存中预热）。
// baseURL/proxy 在 unconfirmedResolution 内以数据库权威渠道重新加载。
func loadUnconfirmedTaskWithChannel(task *model.Task) *model.Task {
	if task == nil {
		return nil
	}
	if task.PrivateData.Key != "" {
		return task
	}
	ch, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return task
	}
	task.PrivateData.Key = ch.Key
	return task
}

func getUnconfirmedWindowMinutes() int {
	return task_setting.GetUnconfirmedWindowMinutes()
}
