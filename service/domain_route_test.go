package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func domainRouteCtx(t *testing.T, header string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if header != "" {
		req.Header.Set("X-Route-Tag", header)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

// §4.8.1 默认关闭：不介入。
func TestDomainRouteDisabledByDefault(t *testing.T) {
	resetDomainRouteForTest()
	c := domainRouteCtx(t, "medical")
	_, ok := ResolveDomainRoute(c, "default")
	assert.False(t, ok, "default-off must not override routing")
}

// 命中白名单且用户在可用分组内 → 覆盖。
func TestDomainRouteAppliesWhenAllowed(t *testing.T) {
	SetDomainRouteForTest(true, map[string]string{"medical": "medical-group"})
	t.Cleanup(resetDomainRouteForTest)

	prev := setting.GetUserUsableGroupsCopy()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认","medical-group":"医疗"}`))
	t.Cleanup(func() {
		b, _ := json.Marshal(prev)
		_ = setting.UpdateUserUsableGroupsByJSONString(string(b))
	})

	c := domainRouteCtx(t, "medical")
	target, ok := ResolveDomainRoute(c, "default")
	assert.True(t, ok)
	assert.Equal(t, "medical-group", target)
}

// 不越权：目标分组不在用户可用分组内 → 不覆盖。
func TestDomainRouteNoPrivilegeEscalation(t *testing.T) {
	SetDomainRouteForTest(true, map[string]string{"medical": "vip-only"})
	t.Cleanup(resetDomainRouteForTest)

	prev := setting.GetUserUsableGroupsCopy()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认"}`)) // 不含 vip-only
	t.Cleanup(func() {
		b, _ := json.Marshal(prev)
		_ = setting.UpdateUserUsableGroupsByJSONString(string(b))
	})

	c := domainRouteCtx(t, "medical")
	_, ok := ResolveDomainRoute(c, "default")
	assert.False(t, ok, "must not route to a group the user cannot use")
}

// 未配置的标签 → 不介入。
func TestDomainRouteUnknownTagIgnored(t *testing.T) {
	SetDomainRouteForTest(true, map[string]string{"medical": "medical-group"})
	t.Cleanup(resetDomainRouteForTest)
	c := domainRouteCtx(t, "unknown-tag")
	_, ok := ResolveDomainRoute(c, "default")
	assert.False(t, ok)
}
