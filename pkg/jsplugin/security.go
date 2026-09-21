package jsplugin

// P2-3 安全模型升级：ExecutionGate 审批 + 分层权限 + Ed25519 签名 + 沙箱降级链。
//
// 目标（next-step 指南 P2-3）：任何未经审批/未声明权限/未满足沙箱策略的插件
// 执行必须被拒绝（fail-closed），杜绝「审 A 跑 B」与静默降级到无防护。
//
// 设计约定：
//   - 全部为纯内存/可插拔实现，不依赖数据库与三库方言，可独立单测；
//   - 引擎执行路径（Engine.Call*）是唯一收口点：策略启用后，每个钩子在真正
//     执行前先过审批闸门；
//   - 默认（未配置策略）保持既有行为，策略为显式加固选项。

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// 分层权限
// ---------------------------------------------------------------------------

// PermissionKind 插件可声明的能力。无声明/未列出 = 默认拒绝。
type PermissionKind string

const (
	PermissionNetwork PermissionKind = "network" // 外联（协议请求/上游调用）
	PermissionFile    PermissionKind = "file"    // 文件读写
	PermissionProcess PermissionKind = "process" // 进程执行
	PermissionSecret  PermissionKind = "secret"  // 密钥/凭据访问
	PermissionRun     PermissionKind = "run"     // 纯计算钩子（parse/render/error 等）
)

// Permissions 插件声明的权限集合。零值 = 全禁（deny-by-default）。
type Permissions struct {
	Network bool `json:"network"`
	File    bool `json:"file"`
	Process bool `json:"process"`
	Secret  bool `json:"secret"`
	Run     bool `json:"run"`
}

var (
	// ErrPluginPermissionDenied 插件未声明调用所需权限。
	ErrPluginPermissionDenied = errors.New("plugin permission denied")
	// ErrPluginUnknownPermission 声明了不存在的权限名。
	ErrPluginUnknownPermission = errors.New("plugin declares unknown permission")
)

// ParsePermissions 从插件 meta 的 permissions 字段解析权限集合。
// 非布尔/未知名称返回错误；缺省全部为 false。
func ParsePermissions(raw any) (Permissions, error) {
	var p Permissions
	switch v := raw.(type) {
	case nil:
		return p, nil
	case []any:
		for _, item := range v {
			name, ok := item.(string)
			if !ok {
				return p, ErrPluginUnknownPermission
			}
			if err := p.enable(name); err != nil {
				return p, err
			}
		}
	case []string:
		for _, name := range v {
			if err := p.enable(name); err != nil {
				return p, err
			}
		}
	default:
		return p, ErrPluginUnknownPermission
	}
	return p, nil
}

func (p *Permissions) enable(name string) error {
	switch PermissionKind(name) {
	case PermissionNetwork:
		p.Network = true
	case PermissionFile:
		p.File = true
	case PermissionProcess:
		p.Process = true
	case PermissionSecret:
		p.Secret = true
	case PermissionRun:
		p.Run = true
	default:
		return fmt.Errorf("%w: %q", ErrPluginUnknownPermission, name)
	}
	return nil
}

// Allows 判断是否声明了指定权限。
func (p Permissions) Allows(kind PermissionKind) bool {
	switch kind {
	case PermissionNetwork:
		return p.Network
	case PermissionFile:
		return p.File
	case PermissionProcess:
		return p.Process
	case PermissionSecret:
		return p.Secret
	case PermissionRun:
		return p.Run
	default:
		return false
	}
}

// RequirePermission 校验插件已声明调用所需权限；未声明返回 ErrPluginPermissionDenied。
func (p Permissions) RequirePermission(kind PermissionKind) error {
	if !p.Allows(kind) {
		return fmt.Errorf("%w: plugin lacks %q permission", ErrPluginPermissionDenied, kind)
	}
	return nil
}

// SecurityPolicy 引擎执行安全策略。零值/空策略 = 维持既有行为（不强制）。
// 启用后 Engine.Call* 在真正执行前先过审批闸门与权限校验（fail-closed）。
type SecurityPolicy struct {
	// Gate 审批闸门；nil 表示不启用审批。
	Gate *ExecutionGate
	// RequireApproval 为 true 时，每次调用都校验 (pluginKey, sourceHash) 已获批。
	RequireApproval bool
	// HookPermissions 指定钩子所需权限：hook -> PermissionKind。
	// 未列出 = 不额外要求权限（纯计算）。声明了但缺失 → 拒绝执行。
	HookPermissions map[string]PermissionKind
	// Declared 插件声明的权限集合（注册时由插件 meta 注入）。
	Declared Permissions
}

// SetSecurityPolicy 为引擎设置执行安全策略（并发安全，原子换指针）。
func (e *Engine) SetSecurityPolicy(p *SecurityPolicy) {
	if p == nil {
		e.policy.Store(&SecurityPolicy{})
		return
	}
	e.policy.Store(p)
}

// SourceHash 返回该引擎绑定源码的内容寻址（SHA-256 hex）。
func (e *Engine) SourceHash() string {
	return e.sourceHash
}

// callHookKey 生成钩子标示（exportName 或 exportName.member.path）。
func callHookKey(exportName string, members []string) string {
	if len(members) == 0 {
		return exportName
	}
	return exportName + "." + strings.Join(members, ".")
}

// enforceCallSecurity 在钩子执行前做策略校验：审批闸门 + 分层权限。
func (e *Engine) enforceCallSecurity(_ context.Context, hook string) error {
	p := e.policy.Load()
	if p == nil || p.Gate == nil {
		return nil
	}
	if p.RequireApproval {
		d := p.Gate.Check(e.key, e.sourceHash)
		switch d.State {
		case ApprovalApproved:
			// 通过
		case ApprovalRejected:
			return fmt.Errorf("%w: %s", ErrPluginRejected, d.Reason)
		default:
			return fmt.Errorf("%w: %s", ErrPluginNotApproved, d.Reason)
		}
	}
	if len(p.HookPermissions) > 0 {
		if kind, ok := p.HookPermissions[hook]; ok {
			if err := p.Declared.RequirePermission(kind); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ExecutionGate：内容寻址一次性审批
// ---------------------------------------------------------------------------

// ApprovalState 审批状态机：pending（未审）→ approved（已批）| rejected（拒绝）。
type ApprovalState string

const (
	ApprovalPending  ApprovalState = "pending"
	ApprovalApproved ApprovalState = "approved"
	ApprovalRejected ApprovalState = "rejected"
)

// ApprovalDecision Check 的结果。
type ApprovalDecision struct {
	State  ApprovalState
	Reason string
}

var (
	// ErrPluginNotApproved 插件（或其内容哈希）尚未获批，拒绝执行。
	ErrPluginNotApproved = errors.New("plugin not approved")
	// ErrPluginRejected 插件被显式拒绝。
	ErrPluginRejected = errors.New("plugin rejected")
	// ErrApprovalTokenUsed 一次性审批 token 已被消费。
	ErrApprovalTokenUsed = errors.New("approval token already used")
)

// ContentHash 计算插件源码内容寻址（SHA-256 hex）。
func ContentHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// ExecutionGate 内容寻址审批闸门。以 (pluginKey, sourceHash) 为审批单元：
// 审批通过只对该哈希生效，源码改动 → 哈希变化 → 必须重新审批（防「审 A 跑 B」）。
// 并发安全；内存态，可替换为持久化存储（见下）。
type ExecutionGate struct {
	mu        sync.Mutex
	approvals map[string]ApprovalState
	tokens    map[string]string // token -> "pluginKey\nsourceHash"
	used      map[string]bool   // 已消费 token
}

// NewExecutionGate 创建空审批闸门。
func NewExecutionGate() *ExecutionGate {
	return &ExecutionGate{
		approvals: make(map[string]ApprovalState),
		tokens:    make(map[string]string),
		used:      make(map[string]bool),
	}
}

func gateKey(pluginKey, sourceHash string) string {
	return pluginKey + "\n" + sourceHash
}

// Check 查询某插件具体内容是否获批。
func (g *ExecutionGate) Check(pluginKey, sourceHash string) ApprovalDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch g.approvals[gateKey(pluginKey, sourceHash)] {
	case ApprovalApproved:
		return ApprovalDecision{State: ApprovalApproved, Reason: "approved for this exact content hash"}
	case ApprovalRejected:
		return ApprovalDecision{State: ApprovalRejected, Reason: "explicitly rejected"}
	default:
		return ApprovalDecision{State: ApprovalPending, Reason: "no approval record for this content hash"}
	}
}

// Approve 审批通过指定内容，返回一次性 token（绑定 pluginKey+sourceHash）。
// token 仅可消费一次；作用：审批记录不可被复制用于其它哈希。
func (g *ExecutionGate) Approve(pluginKey, sourceHash string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	key := gateKey(pluginKey, sourceHash)
	g.approvals[key] = ApprovalApproved
	token := ContentHash(pluginKey + "\x00" + sourceHash + "\x00" + fmt.Sprintf("%d", len(g.tokens)+1))
	g.tokens[token] = key
	return token, nil
}

// ConsumeToken 消费一次性审批 token。成功且仅第一次成功；绑定内容哈希。
func (g *ExecutionGate) ConsumeToken(pluginKey, sourceHash, token string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.used[token] {
		return ErrApprovalTokenUsed
	}
	key, ok := g.tokens[token]
	if !ok {
		return ErrPluginNotApproved
	}
	if key != gateKey(pluginKey, sourceHash) {
		return fmt.Errorf("%w: token bound to different content hash", ErrPluginNotApproved)
	}
	g.used[token] = true
	g.approvals[key] = ApprovalApproved
	return nil
}

// Reject 拒绝指定内容（覆盖已批状态；后续 Check 返回 rejected）。
func (g *ExecutionGate) Reject(pluginKey, sourceHash string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approvals[gateKey(pluginKey, sourceHash)] = ApprovalRejected
}

// Revoke 撤销某插件的全部审批（含其它内容哈希）。
func (g *ExecutionGate) Revoke(pluginKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for k := range g.approvals {
		if strings.HasPrefix(k, pluginKey+"\n") {
			delete(g.approvals, k)
		}
	}
}

// ---------------------------------------------------------------------------
// Ed25519 签名
// ---------------------------------------------------------------------------

var (
	// ErrPluginSignatureInvalid 签名校验失败（被篡改或公钥不匹配）。
	ErrPluginSignatureInvalid = errors.New("plugin signature invalid")
)

// VerifyPluginSignature 校验插件源码的 Ed25519 签名（公钥为 32 字节种子）。
func VerifyPluginSignature(source []byte, signature []byte, publicKey []byte) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: bad public key length", ErrPluginSignatureInvalid)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: bad signature length", ErrPluginSignatureInvalid)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), source, signature) {
		return ErrPluginSignatureInvalid
	}
	return nil
}

// ---------------------------------------------------------------------------
// 沙箱降级链
// ---------------------------------------------------------------------------

// SandboxMode 隔离等级。
type SandboxMode string

const (
	SandboxModeDocker      SandboxMode = "docker"
	SandboxModeBubblewrap  SandboxMode = "bubblewrap"
	SandboxModeWorkspace   SandboxMode = "workspace"
	SandboxModeUnavailable SandboxMode = "unavailable"
	SandboxModeNone        SandboxMode = "none"
)

// SandboxRequirement 沙箱策略：Required=无沙箱拒绝高风险（fail-closed）；
// Lenient=降级但仍明示；Disabled=不要求。
type SandboxRequirement string

const (
	SandboxRequired SandboxRequirement = "required"
	SandboxLenient  SandboxRequirement = "lenient"
	SandboxDisabled SandboxRequirement = "disabled"
)

var (
	// ErrSandboxUnavailable 策略要求沙箱但当前无可用隔离，拒绝执行（fail-closed）。
	ErrSandboxUnavailable = errors.New("required sandbox unavailable; refusing to run unsandboxed")
)

// ResolveSandboxPolicy 解析沙箱策略：
//   - 检测到 docker/bubblewrap → 直接使用对应隔离；
//   - 未检测到且策略要求 → 返回 unavailable（调用方应拒绝高风险操作）；
//   - 未检测到且策略宽松 → 降级 workspace 并置 degraded=true（调用方明示降级日志）；
//   - 策略禁用 → none。
//
// 传入 require 的降级目标参数由调用方决定（如 workspace 目录）。
func ResolveSandboxPolicy(require SandboxRequirement, detected []SandboxMode) (mode SandboxMode, degraded bool, err error) {
	switch require {
	case SandboxDisabled:
		return SandboxModeNone, false, nil
	case SandboxRequired:
		if m, ok := strongestDetected(detected); ok {
			return m, false, nil
		}
		return SandboxModeUnavailable, false, ErrSandboxUnavailable
	case SandboxLenient:
		if m, ok := strongestDetected(detected); ok {
			return m, false, nil
		}
		return SandboxModeWorkspace, true, nil
	default:
		return SandboxModeNone, false, nil
	}
}

func strongestDetected(detected []SandboxMode) (SandboxMode, bool) {
	for _, m := range []SandboxMode{SandboxModeDocker, SandboxModeBubblewrap, SandboxModeWorkspace} {
		for _, d := range detected {
			if d == m {
				return m, true
			}
		}
	}
	return "", false
}
