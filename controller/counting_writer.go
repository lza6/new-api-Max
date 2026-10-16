package controller

import (
	"github.com/gin-gonic/gin"
)

// countingResponseWriter 统计写入客户端的响应体字节数（实时累加到目标指针，
// 供 consume log 在 handler 内部记录时读取；不影响响应行为）。
//
// [扩展] 同时可选地 **tee 一份响应体**供训练数据采集提取助手回复。
// `capture` 为 nil 时行为与原先完全一致（只多一次 nil 判断）——
// 采集未开启的部署不付任何代价。
type countingResponseWriter struct {
	gin.ResponseWriter
	bytes   *int64
	capture *responseCapture
}

func (w *countingResponseWriter) Write(data []byte) (int, error) {
	// 先 tee 再写：即使写客户端失败（连接断开），
	// 我们仍然拿到了模型已经产出的文本 —— 那正是采集想要的。
	// 反过来的顺序在断连时会丢掉整段回复。
	if w.capture != nil {
		w.capture.append(data)
	}
	n, err := w.ResponseWriter.Write(data)
	if w.bytes != nil {
		*w.bytes += int64(n)
	}
	return n, err
}
