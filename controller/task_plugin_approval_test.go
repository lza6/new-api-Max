package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/pkg/jsplugin"
	"github.com/lza6/new-api-Max/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approveTaskPluginViaAPI(t *testing.T, key string, body map[string]any) gin.H {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/"+key+"/approve", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "key", Value: key}}
	ApproveTaskPlugin(ctx)
	var response gin.H
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	return response
}

func withApprovalRequired(t *testing.T) {
	t.Helper()
	require.NoError(t, setting.SetTaskPluginApprovalRequiredOption(true))
	t.Cleanup(func() { _ = setting.SetTaskPluginApprovalRequiredOption(false) })
}

// TestApproveTaskPluginActivatesAndGates 验收：审批→激活并进入内容寻址闸门；
// 拒绝→停用且闸门标记 rejected。
func TestApproveTaskPluginActivatesAndGates(t *testing.T) {
	setupTaskPluginControllerTest(t)
	withApprovalRequired(t)

	source := taskPluginControllerTestSource("approval-flow", "1.0.0")
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
	plugin := &model.TaskPlugin{
		Key: "approval-flow", APIVersion: 1, Version: "1.0.0",
		Source: source, SourceHash: hash, Enabled: false, Active: false,
	}
	require.NoError(t, model.SaveTaskPlugin(plugin))
	require.NoError(t, model.SetTaskPluginApprovalStatus(plugin.Key, plugin.Version, "pending", false))

	require.Equal(t, jsplugin.ApprovalPending, jsplugin.DefaultGate().Check("approval-flow", hash).State)

	response := approveTaskPluginViaAPI(t, "approval-flow", map[string]any{"version": "1.0.0", "approve": true})
	require.True(t, response["success"].(bool))
	require.Equal(t, jsplugin.ApprovalApproved, jsplugin.DefaultGate().Check("approval-flow", hash).State)
	row, err := model.GetTaskPluginVersion("approval-flow", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "approved", row.ApprovalStatus)
	assert.True(t, row.Active)
	assert.True(t, row.Enabled)

	response = approveTaskPluginViaAPI(t, "approval-flow", map[string]any{"version": "1.0.0", "approve": false})
	require.True(t, response["success"].(bool))
	require.Equal(t, jsplugin.ApprovalRejected, jsplugin.DefaultGate().Check("approval-flow", hash).State)
	row, err = model.GetTaskPluginVersion("approval-flow", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "rejected", row.ApprovalStatus)
	assert.False(t, row.Active)
	assert.False(t, row.Enabled)
}

// TestApproveTaskPluginRejectKeepsDifferentHashPending 验收：审批绑定 SourceHash，
// 另一哈希（改行）仍保持待审批 —— 杜绝「审 A 跑 B」。
func TestApproveTaskPluginRejectKeepsDifferentHashPending(t *testing.T) {
	setupTaskPluginControllerTest(t)
	withApprovalRequired(t)

	sourceA := taskPluginControllerTestSource("approval-hash", "1.0.0")
	hashA := fmt.Sprintf("%x", sha256.Sum256([]byte(sourceA)))
	hashB := fmt.Sprintf("%x", sha256.Sum256([]byte(sourceA+"// changed")))

	plugin := &model.TaskPlugin{
		Key: "approval-hash", APIVersion: 1, Version: "1.0.0",
		Source: sourceA, SourceHash: hashA, Enabled: false, Active: false,
	}
	require.NoError(t, model.SaveTaskPlugin(plugin))
	require.NoError(t, model.SetTaskPluginApprovalStatus(plugin.Key, plugin.Version, "pending", false))

	response := approveTaskPluginViaAPI(t, "approval-hash", map[string]any{"version": "1.0.0", "approve": true})
	require.True(t, response["success"].(bool))
	require.Equal(t, jsplugin.ApprovalApproved, jsplugin.DefaultGate().Check("approval-hash", hashA).State)
	require.Equal(t, jsplugin.ApprovalPending, jsplugin.DefaultGate().Check("approval-hash", hashB).State)
}

func TestApproveTaskPluginVersionNotFound(t *testing.T) {
	setupTaskPluginControllerTest(t)
	response := approveTaskPluginViaAPI(t, "missing-plugin", map[string]any{"version": "1.0.0", "approve": true})
	assert.False(t, response["success"].(bool))
}
