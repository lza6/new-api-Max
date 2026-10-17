package service

import (
	"errors"
	"strings"
	"time"

	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
)

// G5-3 动作级授权（deny-by-default）。
//
// 与「插件级内容寻址审批」不同：这里授权的是**动作本身**（例如
// `mcp.tool.create_image`），而不是某个插件产物。默认拒绝，白名单只包含
// 明确的只读动作；有副作用/会产生费用的动作必须由管理员逐条批准，
// 且**一次批准只授权一次调用**（ConsumeApprovedActionApproval 的 CAS）。
const (
	// ActionMcpPaidTool 是会产生上游费用的 MCP tool 调用。
	ActionMcpPaidTool = "mcp.tool.paid"
	// ActionMcpResourceRead 是只读资源读取（静态白名单放行）。
	ActionMcpResourceRead = "mcp.resource.read"
)

// actionApprovalTTL 是待审批/已批准记录的有效期。
//
// 待审批过期后需要重新发起；已批准但未消费的记录过期后自动失效，
// 避免管理员很久以前批的一次动作被无限期复用。
var actionApprovalTTL = 15 * time.Minute

// SetActionApprovalTTLForTest 仅测试使用。
func SetActionApprovalTTLForTest(d time.Duration) func() {
	previous := actionApprovalTTL
	actionApprovalTTL = d
	return func() { actionApprovalTTL = previous }
}

// ActionIsReadOnly 是 deny-by-default 的唯一白名单。
//
// 只读动作不改变任何上游状态、不产生费用，因此不需要审批。
// 任何新增动作默认**不在**白名单里，必须显式加入并说明原因。
func ActionIsReadOnly(action string) bool {
	switch action {
	case ActionMcpResourceRead:
		return true
	default:
		return false
	}
}

// ActionDecision 是一次动作授权的结果。
type ActionDecision struct {
	Allowed    bool
	ApprovalId int64
	Reason     string
}

// AuthorizeAction 判断动作是否可执行。
//
//   - 只读动作：直接放行。
//   - 付费/有副作用动作：消费一条 approved 且未过期的记录；没有则确保存在
//     一条 pending 记录并返回 denial（含 approval_id，调用方展示给用户）。
//
// 这里不做 HTTP 状态码/文案，保持服务层可复用（MCP、未来的写操作都能用）。
func AuthorizeAction(userId int, action, resource string) (ActionDecision, error) {
	if userId <= 0 {
		return ActionDecision{Reason: "invalid user"}, errors.New("invalid user")
	}
	if ActionIsReadOnly(action) {
		return ActionDecision{Allowed: true, Reason: "read_only"}, nil
	}

	now := common.GetTimestamp()
	consumed, err := model.ConsumeApprovedActionApproval(userId, action, resource, now)
	if err != nil {
		return ActionDecision{}, err
	}
	if consumed {
		return ActionDecision{Allowed: true, Reason: "approved"}, nil
	}

	pending, err := model.HasPendingActionApproval(userId, action, resource)
	if err != nil {
		return ActionDecision{}, err
	}
	if pending == nil {
		pending = &model.ActionApproval{
			UserId:    userId,
			Action:    action,
			Resource:  strings.TrimSpace(resource),
			Status:    model.ActionApprovalPending,
			ExpiresAt: now + int64(actionApprovalTTL.Seconds()),
		}
		if err := model.CreateActionApproval(pending); err != nil {
			return ActionDecision{}, err
		}
	}
	return ActionDecision{
		Allowed:    false,
		ApprovalId: pending.Id,
		Reason:     "approval_required",
	}, nil
}

// ApproveAction 由管理员批准一条待审批动作。
func ApproveAction(id int64, adminId int, reason string) error {
	if id <= 0 || adminId <= 0 {
		return errors.New("invalid approval or admin id")
	}
	expiresAt := common.GetTimestamp() + int64(actionApprovalTTL.Seconds())
	return model.ApproveActionApproval(id, adminId, expiresAt, reason)
}

// DenyAction 由管理员拒绝一条待审批动作。
func DenyAction(id int64, adminId int, reason string) error {
	if id <= 0 || adminId <= 0 {
		return errors.New("invalid approval or admin id")
	}
	return model.DenyActionApproval(id, adminId, reason)
}

// CleanupActionApprovals 标记过期记录，返回处理条数。
func CleanupActionApprovals() (int64, error) {
	return model.CleanupExpiredActionApprovals(common.GetTimestamp(), 500)
}
