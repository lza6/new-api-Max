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

package probe

import (
	"context"
	"strings"
)

// StreamIntegrityCase 用例5 流完整性：请求 stream=true 时，
// 渠道应返回多个增量 data 块（而非一次性塞满整个响应），证明真流式而非静默降级。
type StreamIntegrityCase struct{}

func (StreamIntegrityCase) Name() string    { return "stream_integrity" }
func (StreamIntegrityCase) Weight() float64 { return 10 }

func (c StreamIntegrityCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	chunks, raw, err := streamChat(ctx, ch, "你是一个严谨的助手，请分点作答。", "请用三句话介绍你自己。", 80)
	if err != nil {
		result.Error = "stream call: " + err.Error()
		result.Evidence = truncateEvidence(raw)
		return result
	}
	if len(chunks) < 2 {
		result.Error = "expected >=2 SSE data chunks, got " + itoa(len(chunks))
		result.Evidence = truncateEvidence("chunks=" + itoa(len(chunks)) + " raw=" + raw)
		return result
	}
	result.Passed = true
	result.Score = c.Weight()
	result.Evidence = truncateEvidence("chunks=" + itoa(len(chunks)) + " first=" + firstN(chunks[0], 80))
	return result
}

// firstN 返回字符串前 n 字符。
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// itoa 简易整数转字符串（避免引入 strconv 依赖的样板）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b strings.Builder
	for v > 0 {
		b.WriteByte(byte('0' + v%10))
		v /= 10
	}
	s := b.String()
	var out strings.Builder
	for i := len(s) - 1; i >= 0; i-- {
		out.WriteByte(s[i])
	}
	if neg {
		return "-" + out.String()
	}
	return out.String()
}
