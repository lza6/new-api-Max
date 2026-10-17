package service

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/lza6/new-api-Max/model"
)

func seedActionApprovalDB(t *testing.T) {
	t.Helper()
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.ActionApproval{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
}

func TestActionRegistryDenyByDefault(t *testing.T) {
	seedActionApprovalDB(t)
	restore := SetActionApprovalTTLForTest(time.Minute)
	t.Cleanup(restore)

	// 只读动作直接放行。
	decision, err := AuthorizeAction(1, ActionMcpResourceRead, "newapi://me/quota")
	require.NoError(t, err)
	assert.True(t, decision.Allowed)

	// 未登记/付费动作：第一次拒绝并创建 pending。
	decision, err = AuthorizeAction(1, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, "approval_required", decision.Reason)
	require.NotZero(t, decision.ApprovalId)

	// 重复请求复用同一条 pending，不刷单。
	second, err := AuthorizeAction(1, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.Equal(t, decision.ApprovalId, second.ApprovalId)
}

func TestActionRegistryApproveConsumeOnce(t *testing.T) {
	seedActionApprovalDB(t)
	restore := SetActionApprovalTTLForTest(time.Minute)
	t.Cleanup(restore)

	decision, err := AuthorizeAction(2, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	require.False(t, decision.Allowed)

	require.NoError(t, ApproveAction(decision.ApprovalId, 99, "ok"))

	allowed, err := AuthorizeAction(2, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.True(t, allowed.Allowed, "批准后第一次调用应放行")

	again, err := AuthorizeAction(2, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.False(t, again.Allowed, "一次批准只能授权一次调用")
	assert.NotZero(t, again.ApprovalId, "消费后应生成新的待审批记录")
}

func TestActionRegistryDeniedAndExpired(t *testing.T) {
	seedActionApprovalDB(t)
	restore := SetActionApprovalTTLForTest(time.Minute)
	t.Cleanup(restore)

	decision, err := AuthorizeAction(3, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	require.False(t, decision.Allowed)

	// 批准后把过期时间改到过去 → 不得再放行。
	require.NoError(t, ApproveAction(decision.ApprovalId, 99, "ok"))
	require.NoError(t, model.DB.Model(&model.ActionApproval{}).
		Where("id = ?", decision.ApprovalId).
		Update("expires_at", time.Now().Add(-time.Hour).Unix()).Error)

	expired, err := AuthorizeAction(3, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.False(t, expired.Allowed)

	require.NoError(t, DenyAction(expired.ApprovalId, 99, "no"))
	denied, err := AuthorizeAction(3, ActionMcpPaidTool, "create_image")
	require.NoError(t, err)
	assert.False(t, denied.Allowed)
}
