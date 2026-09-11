package relay

import (
	"os"
	"testing"

	"github.com/lza6/new-api-Max/service"
)

// TestMain relay 包单测全部使用 httptest 环回服务器作为上游，
// B1-2 SSRF 守卫必须整体跳过（守卫自身的行为在 service/url_guard_test.go 覆盖）。
func TestMain(m *testing.M) {
	service.SetSSRFGuardDisabled(true)
	os.Exit(m.Run())
}
