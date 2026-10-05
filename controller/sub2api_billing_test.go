package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/constant"
	"github.com/lza6/new-api-Max/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §端点适配：/v1/sub2api/billing 返回分组倍率与计费口径（421 契约形状）。
func TestGetSub2ApiBillingReturnsGroupMultiplier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/v1/sub2api/billing", nil)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")

	GetSub2ApiBilling(c)

	require.Equal(t, 200, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "sub2api.key_billing", body["object"])
	assert.Equal(t, "token", body["billing_scope"])
	assert.Equal(t, "default", body["group"])
	assert.EqualValues(t, ratio_setting.GetGroupRatio("default"), body["effective_rate_multiplier"])
	_, hasObserved := body["observed_at"]
	assert.True(t, hasObserved)
}
