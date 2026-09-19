package controller

import (
	"github.com/gin-gonic/gin"
)

// countingResponseWriter 统计写入客户端的响应体字节数（实时累加到目标指针，
// 供 consume log 在 handler 内部记录时读取；不影响响应行为）。
type countingResponseWriter struct {
	gin.ResponseWriter
	bytes *int64
}

func (w *countingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if w.bytes != nil {
		*w.bytes += int64(n)
	}
	return n, err
}
