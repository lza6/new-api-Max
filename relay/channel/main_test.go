package channel

import (
	"os"
	"testing"

	"github.com/lza6/new-api-Max/service"
)

// TestMain relay/channel 包单测使用 httptest 环回/本地上游，
// B1-2 SSRF 守卫必须整体跳过（守卫自身行为在 service/url_guard_test.go 覆盖）。
func TestMain(m *testing.M) {
	service.SetSSRFGuardDisabled(true)
	os.Exit(m.Run())
}
