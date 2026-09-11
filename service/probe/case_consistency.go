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

// ConsistencyCase 用例 4 重复一致性：温度=0 时同一请求两次结果应一致
// （或高度相似）；随机套壳（每次换模型）会出现明显分歧。
type ConsistencyCase struct{}

func (ConsistencyCase) Name() string    { return "consistency" }
func (ConsistencyCase) Weight() float64 { return 15 }

func (c ConsistencyCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	question := "用一句话概括：什么是万有引力。"
	first, raw1, err := chatCall(ctx, ch, "你是一个严谨的回答者。", question, 60, map[string]any{"temperature": 0})
	if err != nil {
		result.Error = "first call: " + err.Error()
		result.Evidence = truncateEvidence(raw1)
		return result
	}
	second, raw2, err := chatCall(ctx, ch, "你是一个严谨的回答者。", question, 60, map[string]any{"temperature": 0})
	if err != nil {
		result.Error = "second call: " + err.Error()
		result.Evidence = truncateEvidence(raw2)
		return result
	}

	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	if first == "" || second == "" {
		result.Error = "empty response in consistency check"
		result.Evidence = truncateEvidence("first=" + first + "\nsecond=" + second)
		return result
	}
	// 一致性判定：完全一致，或关键词重叠（前 100 字符内共现 >=2 个 3 字词片段）。
	similar := first == second || commonSubstringCount(first, second) >= 2
	result.Evidence = truncateEvidence("first=" + first + "\nsecond=" + second)
	if similar {
		result.Passed = true
		result.Score = c.Weight()
	} else {
		result.Error = "temperature=0 answers diverge significantly between two calls"
	}
	return result
}

// commonSubstringCount 统计 a 中长度 >=4 的片段在 b 中出现的数量（粗略相似度）。
func commonSubstringCount(a, b string) int {
	const n = 4
	count := 0
	runesA := []rune(a)
	seen := map[string]bool{}
	for i := 0; i+n <= len(runesA); i++ {
		frag := string(runesA[i : i+n])
		if seen[frag] {
			continue
		}
		seen[frag] = true
		if strings.Contains(b, frag) {
			count++
		}
		if count >= 2 {
			return count
		}
	}
	return count
}
