package service

import (
	"context"
	"time"

	relaycommon "github.com/lza6/new-api-Max/relay/common"

	"github.com/lza6/new-api-Max/model"
)

// 压缩统计周期落库间隔（与 batchUpdate 同频，5s）。
const modelCompressionFlushInterval = 5 * time.Second

var compressionStatFlushLoop backgroundLoop

// init 注册按模型压缩采集回调（relay/common → model 解耦）。
func init() {
	relaycommon.ModelCompressionRecorder = model.RecordModelCompression
}

// StartModelCompressionStatFlush 启动压缩统计周期落库 loop（进程重启不丢）。
func StartModelCompressionStatFlush() {
	compressionStatFlushLoop.start(func(ctx context.Context) {
		ticker := time.NewTicker(modelCompressionFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				model.FlushModelCompressionStats()
				return
			case <-ticker.C:
				model.FlushModelCompressionStats()
			}
		}
	})
}

// StopModelCompressionStatFlush 停止 loop 并排空（优雅关闭/测试用）。
func StopModelCompressionStatFlush() {
	compressionStatFlushLoop.stop()
	model.FlushModelCompressionStats()
}
