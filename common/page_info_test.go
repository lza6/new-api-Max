package common

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 负数页码会让 GetStartIdx() 返回负 OFFSET，PostgreSQL 报
// "OFFSET must not be negative"、MySQL 报语法错误，/api/log/ 与
// /api/log/self 直接 500。负的 page_size 同理会产成负 LIMIT。
// 本测试锁死「任何输入下 OFFSET/LIMIT 都不为负」这一契约。
func TestGetPageQueryNeverProducesNegativeOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		query     string
		wantPage  int
		wantSize  int
		wantStart int
	}{
		{"p=-5&page_size=10", 1, 10, 0},
		{"p=0&page_size=10", 1, 10, 0},
		{"p=1&page_size=10", 1, 10, 0},
		{"p=3&page_size=10", 3, 10, 20},
		{"p=2&page_size=-7", 2, ItemsPerPage, ItemsPerPage},
		{"p=1&page_size=1000", 1, 100, 0},
		{"p=-1&page_size=-1", 1, ItemsPerPage, 0},
	}

	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/?"+tc.query, nil)

		p := GetPageQuery(c)

		assert.Equal(t, tc.wantPage, p.Page, "page for %q", tc.query)
		assert.Equal(t, tc.wantSize, p.PageSize, "page_size for %q", tc.query)
		assert.Equal(t, tc.wantStart, p.GetStartIdx(), "startIdx for %q", tc.query)
		assert.GreaterOrEqual(t, p.GetStartIdx(), 0, "OFFSET must never be negative for %q", tc.query)
		assert.Greater(t, p.PageSize, 0, "LIMIT must never be non-positive for %q", tc.query)
	}
}

// GetStartIdx 必须有独立兜底：调用方可能自行构造 PageInfo（不走 GetPageQuery），
// 例如内部调用直接 `&PageInfo{Page: -1, PageSize: 20}`。
func TestGetStartIdxGuardsDirectlyConstructedPageInfo(t *testing.T) {
	assert.Equal(t, 0, (&PageInfo{Page: -1, PageSize: 20}).GetStartIdx())
	assert.Equal(t, 0, (&PageInfo{Page: 5, PageSize: -20}).GetStartIdx())
	assert.Equal(t, 0, (&PageInfo{Page: 0, PageSize: 0}).GetStartIdx())
	assert.Equal(t, 20, (&PageInfo{Page: 3, PageSize: 10}).GetStartIdx())
}
