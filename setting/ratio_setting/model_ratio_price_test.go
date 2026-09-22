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
package ratio_setting

import (
	"testing"

	"github.com/lza6/new-api-Max/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 计费安全不变量：负 model_price 写入必须被拒绝（预扣侧有兜底，结算侧对负值无防御）。
func TestUpdateModelPriceByJSONStringRejectsNegative(t *testing.T) {
	payload := `{"model-a":-0.01,"model-b":0.2}`
	err := UpdateModelPriceByJSONString(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不能为负数")
	// 负值被拒绝时不应写入任何价格。
	_, ok := GetModelPrice("model-a", false)
	assert.False(t, ok)
	_, ok2 := GetModelPrice("model-b", false)
	assert.False(t, ok2)
}

func TestUpdateModelPriceByJSONStringAcceptsZeroAndPositive(t *testing.T) {
	payload, err := common.Marshal(map[string]float64{"model-price-test": 0})
	require.NoError(t, err)
	require.NoError(t, UpdateModelPriceByJSONString(string(payload)))
	price, ok := GetModelPrice("model-price-test", false)
	assert.True(t, ok)
	assert.Equal(t, float64(0), price)
}
