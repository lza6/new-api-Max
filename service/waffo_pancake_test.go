package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Waffo Pancake 回调的 StoreID 校验是**跨店铺伪造**的唯一防线：
// 签名只证明「某个店铺发的」，不证明「我们的店铺发的」。
func TestWaffoPancakeStoreMatches(t *testing.T) {
	cases := []struct {
		name       string
		event      string
		configured string
		want       bool
	}{
		{"完全一致", "store_abc", "store_abc", true},
		{"大小写不同", "STORE_ABC", "store_abc", true},
		{"空白被忽略", " store_abc ", "store_abc", true},
		{"另一店铺", "store_other", "store_abc", false},
		{"事件为空", "", "store_abc", false},
		{"未配置（fail-closed）", "store_abc", "", false},
		{"未配置且事件为空", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, waffoPancakeStoreMatches(tc.event, tc.configured))
		})
	}
}
