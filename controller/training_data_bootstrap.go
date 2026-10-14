package controller

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service/training_capture"
)

// 训练数据采集的启动装配（Spec-008）。
//
// 拆成独立文件而不是塞进 main.go：装配含「读环境变量 → 选后端 →
// 同步开关 → 启动 worker → 起清理任务」五个步骤，任一漏掉都会表现为
// 「功能静默不工作」（本项目踩过的死代码坑），值得集中一处审查。

// EnvTrainingDataSink 选择存储后端。取值：onedrive / disabled（默认）。
//
// 默认 **disabled**：不支持「配了凭据就自动往云盘传用户对话」。
// 采集涉及用户数据出境，必须由部署者显式打开。
const EnvTrainingDataSink = "TRAINING_DATA_SINK"

// StartTrainingDataCapture 在进程启动时装配并启动采集。
//
// **永远不返回错误、永远不阻塞启动**：采集是辅助功能，
// 它的任何配置问题都不该让整个网关起不来。
func StartTrainingDataCapture() {
	// 注入落库函数，让 service 层不必依赖 options 表的具体实现。
	training_capture.SetDBUpdater(model.UpdateOption)

	sinkName := strings.TrimSpace(os.Getenv(EnvTrainingDataSink))

	if sinkName == "" || strings.EqualFold(sinkName, "disabled") {
		// 显式保持关闭：同步热路径标志，让请求路径走最省的分支。
		training_capture.SetGatewayEnabled(false)
		common.SysLog("training data capture: disabled (set " + EnvTrainingDataSink + "=onedrive to enable)")
		return
	}

	if err := training_capture.ConfigureSink(sinkName); err != nil {
		// 明确报错而不是静默降级：配错名字时静默 disabled 会让管理员
		// 以为在采集，直到某天发现云盘是空的。
		common.SysError("training data capture: failed to configure sink " + sinkName + ": " + err.Error())
		training_capture.SyncHotPathFlag()
		return
	}

	// 后端就绪后，把总开关对齐到持久化配置（管理员上次设的状态）。
	// 注意顺序：必须先 ConfigureSink，否则 SetGatewayEnabled 里的
	// SyncHotPathFlag 会因「云盘未配」而把热路径判为关闭。
	training_capture.LoadGatewayEnabledFromDB()
	training_capture.SyncHotPathFlag()

	if !training_capture.HotPathEnabled() {
		reason := "gateway switch is off"
		if !training_capture.CanEnableGateway() {
			reason = "sink not configured"
		}
		common.SysLog("training data capture: sink ready (" + sinkName + ") but capture inactive: " + reason)
		return
	}

	training_capture.StartWorker()
	startTrainingDataCleanupLoop()

	common.SysLog("training data capture: worker started (sink=" + sinkName + ")")
}

// trainingDataCleanupInterval 水位清理周期。
//
// 水位是会话级状态，会话不再活跃后行会一直留着。清理很便宜
// （一条带索引的 DELETE），一天一次足够；调快没有收益。
const trainingDataCleanupInterval = 24 * time.Hour

// startTrainingDataCleanupLoop 定期清理过期水位。
//
// 不加这个循环，`training_data_watermarks` 会随会话数无界增长 ——
// 这是「表无界增长」类问题的典型来源，且初期完全不可见。
func startTrainingDataCleanupLoop() {
	go func() {
		// 启动后先等一个周期再清理：避免与启动期的迁移/首次请求抢 DB。
		ticker := time.NewTicker(trainingDataCleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			count, err := training_capture.CleanupStaleWatermarks()
			if err != nil {
				common.SysError("training data capture: watermark cleanup failed: " + err.Error())
				continue
			}
			if count > 0 {
				common.SysLog("training data capture: cleaned " + strconv.FormatInt(count, 10) + " stale watermarks")
			}
		}
	}()
}
