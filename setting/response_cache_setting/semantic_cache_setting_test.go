package response_cache_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 语义缓存的阈值回退与生效条件是最容易「配错却没人发现」的两处：
// 一个越界阈值必须回退默认值（否则 0 会让任何请求都命中，>1 会让它永不命中），
// 一个空白名单必须让语义缓存整体不生效（否则会变成对全部模型开放）。
func TestSemanticThresholdFallsBackWhenOutOfRange(t *testing.T) {
	saved := setting.SemanticSimilarity
	t.Cleanup(func() { setting.SemanticSimilarity = saved })

	for _, tc := range []struct {
		name string
		set  float64
		want float64
	}{
		{"unset", 0, DefaultSemanticSimilarity},
		{"negative", -0.5, DefaultSemanticSimilarity},
		{"above one", 1.5, DefaultSemanticSimilarity},
		{"valid", 0.82, 0.82},
		{"exactly one", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.SemanticSimilarity = tc.set
			assert.Equal(t, tc.want, SemanticThreshold())
		})
	}
}

func TestSemanticCacheInactiveWithoutAllowlist(t *testing.T) {
	saved := setting.SemanticAllowModels
	t.Cleanup(func() { setting.SemanticAllowModels = saved })

	setting.SemanticAllowModels = nil
	assert.False(t, SemanticCacheActive(), "empty allowlist must keep semantic cache off")
	assert.Empty(t, SemanticModels())
}
