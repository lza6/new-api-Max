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
	"fmt"
	"strings"
)

// ModelIDCase 用例 1 模型身份：确定性问答验证是否真为目标模型。
// 上游换模型（套壳/降级）时该用例最先暴露分歧。
type ModelIDCase struct{}

func (ModelIDCase) Name() string    { return "model_identity" }
func (ModelIDCase) Weight() float64 { return 25 }

// identityQuestions 确定性问答对（答案唯一且不随时间变化）。
var identityQuestions = []struct {
	question   string
	wantSubstr string
}{
	{question: "《滕王阁序》的作者是谁？只回答人名。", wantSubstr: "王勃"},
	{question: "水的化学式是什么？只回答化学式。", wantSubstr: "H2O"},
}

func (c ModelIDCase) Run(ctx context.Context, ch *ProbeTarget) CaseResult {
	result := CaseResult{Name: c.Name(), Weight: c.Weight()}
	var evidence []string
	for i, qa := range identityQuestions {
		content, raw, err := chatCall(ctx, ch, "你是身份校验助手，必须给出准确答案。", qa.question, 20, nil)
		if err != nil {
			result.Error = fmt.Sprintf("question %d: %v", i+1, err)
			result.Evidence = truncateEvidence(raw)
			return result
		}
		pass := strings.Contains(content, qa.wantSubstr)
		evidence = append(evidence, fmt.Sprintf("Q%d => %q (expect contains %q, pass=%v)", i+1, content, qa.wantSubstr, pass))
		if !pass {
			result.Error = fmt.Sprintf("identity mismatch on question %d", i+1)
			result.Evidence = truncateEvidence(strings.Join(evidence, "\n"))
			return result
		}
	}
	result.Passed = true
	result.Score = c.Weight()
	result.Evidence = truncateEvidence(strings.Join(evidence, "\n"))
	return result
}
