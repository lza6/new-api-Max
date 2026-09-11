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

// B1-1 密钥日志指纹化：日志与错误消息中的上游 API key 统一替换为指纹。
// 日志是密钥泄露的头号通道（N1/N16 侦察证据），本模块在 logger 出口
// 单点接入，避免逐个调用点遗漏。

package common

import (
	"strings"
)

// secretKeyPrefixes 已知上游 API key 前缀集合（以各渠道 adaptor 真实实现为准）。
// OpenAI 系 sk-、Anthropic sk-ant-、Google AIza；Authorization: Bearer 后的
// 任意长 token 由 token 扫描兜底。
var secretKeyPrefixes = []string{
	"sk-ant-",
	"sk-",
	"AIza",
}

// MaskKey 返回 key 的指纹：前 4 后 4 中间 ****；长度不足 12 只保留末 4 位。
// 空串返回空串。仅供日志/错误文案展示，不用于任何匹配逻辑。
func MaskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 12 {
		return "****" + key[len(key)-4:]
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// isSecretTokenChar 判断字符是否属于连续 token 的一部分。
// token 以空白、引号、逗号、分号、尖括号、反引号等终止。
func isSecretTokenChar(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '"', '\'', ',', ';', ')', '>', '`', ']', '}':
		return false
	}
	return true
}

// MaskMessage 扫描消息中疑似 API key 的片段（以已知前缀开头、总长 >=20 的
// 连续 token）并替换为指纹。一次 strings.Builder 线性扫描，无正则回溯。
func MaskMessage(msg string) string {
	if msg == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(msg))
	for i := 0; i < len(msg); {
		matched := false
		for _, prefix := range secretKeyPrefixes {
			if !strings.HasPrefix(msg[i:], prefix) {
				continue
			}
			// 前缀命中：向后取完整 token（前缀 + 连续 token 字符）。
			end := i + len(prefix)
			for end < len(msg) && isSecretTokenChar(msg[end]) {
				end++
			}
			token := msg[i:end]
			if len(token) >= 20 {
				b.WriteString(MaskKey(token))
			} else {
				b.WriteString(token)
			}
			i = end
			matched = true
			break
		}
		if !matched {
			b.WriteByte(msg[i])
			i++
		}
	}
	return b.String()
}
