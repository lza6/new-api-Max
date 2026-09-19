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
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeApiEquivalentUsd(t *testing.T) {
	// gpt-4o-mini: $0.15 / $0.60 per 1M
	// 1000 input + 100 output → (1000*0.15 + 100*0.60)/1e6*1e6 = 210 微美元
	usd, ok := ComputeApiEquivalentUsd("gpt-4o-mini", 1000, 100)
	require.True(t, ok)
	assert.Equal(t, int64(210), usd)

	// deepseek-chat: $0.27 / $1.10；1M input + 1M output → 1,370,000 微美元
	usd2, ok2 := ComputeApiEquivalentUsd("deepseek-chat", 1_000_000, 1_000_000)
	require.True(t, ok2)
	assert.Equal(t, int64(1_370_000), usd2)
}

func TestComputeApiEquivalentUsdUnknownModel(t *testing.T) {
	// 未收录模型：不估算、不编造，返回 (0, false)
	usd, ok := ComputeApiEquivalentUsd("deepseek-v4-flash", 1000, 100)
	assert.False(t, ok)
	assert.Equal(t, int64(0), usd)
}

func TestComputeApiEquivalentUsdSaturates(t *testing.T) {
	// 超大 token 数 → 饱和到上界（防溢出）
	usd, ok := ComputeApiEquivalentUsd("gpt-4o", 1<<60, 1<<60)
	require.True(t, ok)
	assert.Equal(t, int64(maxShadowUsdMicros), usd)
}

func TestComputeApiEquivalentUsdZeroTokens(t *testing.T) {
	usd, ok := ComputeApiEquivalentUsd("gpt-4o-mini", 0, 0)
	require.True(t, ok)
	assert.Equal(t, int64(0), usd)

	// 负 token 按 0 处理
	usdNeg, okNeg := ComputeApiEquivalentUsd("gpt-4o-mini", -100, -50)
	require.True(t, okNeg)
	assert.Equal(t, int64(0), usdNeg)
}