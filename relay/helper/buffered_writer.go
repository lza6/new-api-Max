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

// B3-2 流式首包缓冲 fallover（N2 free-router 证据）：
// 流式请求一旦开始（响应头上线）就无法切换渠道，空响应/瞬时失败只能让用户
// 看到错误。本 Writer 把「客户端写」挡在缓冲后面，直到上游发出首个有效
// data 块才一次性提交；首包超时则整体放弃本次 attempt，交还渠道重试链。
//
// 性能契约：缓冲用单 bytes.Buffer 顺序追加（单次内存拷贝），上限
// maxFirstPacketBufferBytes（1MB）超过即放行（防异常响应占内存）。
// 开关 relay.stream_fallover（默认 off）：off 时完全不安装本 Writer。

package helper

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// maxFirstPacketBufferBytes 缓冲上限：超过直接放行（防恶意/异常响应占内存）。
const maxFirstPacketBufferBytes = 1 << 20

// FirstPacketBufferWriter 包装 gin.ResponseWriter，在 Commit 之前把所有
// 客户端写入缓冲在内存里（响应头不上线），Commit 后全部直通。
type FirstPacketBufferWriter struct {
	gin.ResponseWriter
	mu        sync.Mutex
	buf       bytes.Buffer
	committed bool
	dropped   bool
}

// NewFirstPacketBufferWriter 包装当前 writer。禁用 ping 保活由调用方负责：
// PING 注释行会在缓冲期直接上线提前提交响应头。
func NewFirstPacketBufferWriter(w gin.ResponseWriter) *FirstPacketBufferWriter {
	return &FirstPacketBufferWriter{ResponseWriter: w}
}

// Commit 把缓冲字节一次性写穿并进入直通模式。之后的写入行为与原始
// Writer 完全一致。
func (w *FirstPacketBufferWriter) Commit() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		return
	}
	w.committed = true
	if !w.dropped && w.buf.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.buf.Bytes())
	}
	w.buf.Reset()
}

// BufferedBytes 返回当前缓冲字节数（测试/诊断用）。
func (w *FirstPacketBufferWriter) BufferedBytes() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Len()
}

// Committed 报告是否已进入直通模式。
func (w *FirstPacketBufferWriter) Committed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.committed
}

func (w *FirstPacketBufferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		return w.ResponseWriter.Write(p)
	}
	// 超限：放行模式（丢弃缓冲、直接写穿），防止异常响应无限占内存。
	if w.buf.Len()+len(p) > maxFirstPacketBufferBytes {
		w.dropped = true
		w.committed = true
		return w.ResponseWriter.Write(p)
	}
	return w.buf.Write(p)
}

func (w *FirstPacketBufferWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// WriteHeaderNow 缓冲期内 no-op：防止提前上线响应头。
func (w *FirstPacketBufferWriter) WriteHeaderNow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		w.ResponseWriter.WriteHeaderNow()
	}
}

// Flush 缓冲期内 no-op（gin Flush 会先 WriteHeaderNow 提交响应头）。
func (w *FirstPacketBufferWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.committed {
		w.ResponseWriter.Flush()
	}
}

// SetWriteDeadline 代理到底层连接，兼容 ExtendWriteDeadline 的
// http.NewResponseController 穿透。
func (w *FirstPacketBufferWriter) SetWriteDeadline(t time.Time) error {
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(t)
}
