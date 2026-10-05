package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §审查建议：CSV 公式注入防护。
func TestCsvSafeCell(t *testing.T) {
	assert.Equal(t, "'=cmd|'/c calc'!A1", csvSafeCell("=cmd|'/c calc'!A1"))
	assert.Equal(t, "'+1+1", csvSafeCell("+1+1"))
	assert.Equal(t, "'-2+3", csvSafeCell("-2+3"))
	assert.Equal(t, "'@SUM(A1)", csvSafeCell("@SUM(A1)"))
	assert.Equal(t, "'\tX", csvSafeCell("\tX"))
	assert.Equal(t, "'\rX", csvSafeCell("\rX"))
	// 正常值不变。
	assert.Equal(t, "gpt-4o", csvSafeCell("gpt-4o"))
	assert.Equal(t, "deepseek-v4-flash", csvSafeCell("deepseek-v4-flash"))
	assert.Equal(t, "", csvSafeCell(""))
}
