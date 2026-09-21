package jsplugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentHashDeterministicAndSensitive(t *testing.T) {
	sourceA := "export function run() { return 1; }"
	sourceB := "export function run() { return 2; }"
	assert.Equal(t, ContentHash(sourceA), ContentHash(sourceA), "同一源码哈希必须稳定")
	assert.NotEqual(t, ContentHash(sourceA), ContentHash(sourceB), "改一行源码哈希必须变化")
	assert.Len(t, ContentHash(sourceA), 64, "SHA-256 hex 长度")
}

// ---------------------------------------------------------------------------
// 分层权限
// ---------------------------------------------------------------------------

func TestParsePermissionsDenyByDefault(t *testing.T) {
	p, err := ParsePermissions(nil)
	require.NoError(t, err)
	assert.False(t, p.Network)
	assert.False(t, p.File)
	assert.False(t, p.Process)
	assert.False(t, p.Secret)

	declared, err := ParsePermissions([]any{"network", "run"})
	require.NoError(t, err)
	assert.True(t, declared.Network)
	assert.True(t, declared.Run)
	assert.False(t, declared.File)
	assert.False(t, declared.Process)
	assert.False(t, declared.Secret)

	_, err = ParsePermissions([]any{"shell"})
	require.ErrorIs(t, err, ErrPluginUnknownPermission)
	_, err = ParsePermissions(map[string]any{"network": true})
	require.ErrorIs(t, err, ErrPluginUnknownPermission)
}

func TestRequirePermissionEnforcesDeclaration(t *testing.T) {
	p, err := ParsePermissions([]any{"network"})
	require.NoError(t, err)
	require.NoError(t, p.RequirePermission(PermissionNetwork))
	require.ErrorIs(t, p.RequirePermission(PermissionFile), ErrPluginPermissionDenied)
	require.ErrorIs(t, p.RequirePermission(PermissionProcess), ErrPluginPermissionDenied)
	require.ErrorIs(t, p.RequirePermission(PermissionSecret), ErrPluginPermissionDenied)
	require.ErrorIs(t, p.RequirePermission("unknown"), ErrPluginPermissionDenied)

	empty, err := ParsePermissions(nil)
	require.NoError(t, err)
	require.ErrorIs(t, empty.RequirePermission(PermissionNetwork), ErrPluginPermissionDenied, "无声明默认全禁")
}

// ---------------------------------------------------------------------------
// ExecutionGate：内容寻址一次性审批
// ---------------------------------------------------------------------------

func TestExecutionGateContentAddressedApproval(t *testing.T) {
	gate := NewExecutionGate()
	hashA := ContentHash(`export function run() { return "A"; }`)
	hashB := ContentHash(`export function run() { return "B"; }`)

	// 未审批 → pending（拒绝执行）
	d := gate.Check("p1", hashA)
	require.Equal(t, ApprovalPending, d.State)

	// 审批 hashA 后仅 A 获批；改动一行（hashB）仍需重新审批 → 杜绝「审 A 跑 B」。
	token, err := gate.Approve("p1", hashA)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, ApprovalApproved, gate.Check("p1", hashA).State)
	require.Equal(t, ApprovalPending, gate.Check("p1", hashB).State, "不同内容哈希必须重新审批")
	require.Equal(t, ApprovalPending, gate.Check("p2", hashA).State, "审批按插件隔离")

	// token 绑定哈希：用 A 的 token 换 B 被拒（且不消耗 token）。
	require.ErrorIs(t, gate.ConsumeToken("p1", hashB, token), ErrPluginNotApproved)
	// 未知 token 被拒。
	require.ErrorIs(t, gate.ConsumeToken("p1", hashA, "forged"), ErrPluginNotApproved)
	// 一次性 token：仅首次有效。
	require.NoError(t, gate.ConsumeToken("p1", hashA, token))
	require.ErrorIs(t, gate.ConsumeToken("p1", hashA, token), ErrApprovalTokenUsed)
}

func TestExecutionGateRejectAndRevoke(t *testing.T) {
	gate := NewExecutionGate()
	hash := ContentHash("source")
	_, approveErr := gate.Approve("p1", hash)
	require.NoError(t, approveErr)
	require.Equal(t, ApprovalApproved, gate.Check("p1", hash).State)

	gate.Reject("p1", hash)
	require.Equal(t, ApprovalRejected, gate.Check("p1", hash).State)

	_, approveErr = gate.Approve("p1", hash)
	require.NoError(t, approveErr)
	gate.Revoke("p1")
	require.Equal(t, ApprovalPending, gate.Check("p1", hash).State, "撤销后回到待审批")
}

// ---------------------------------------------------------------------------
// Ed25519 签名
// ---------------------------------------------------------------------------

func TestVerifyPluginSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	source := []byte(`export function run() { return "signed"; }`)
	signature := ed25519.Sign(privateKey, source)

	require.NoError(t, VerifyPluginSignature(source, signature, publicKey))
	require.ErrorIs(t, VerifyPluginSignature([]byte(`export function run() { return "tampered"; }`), signature, publicKey), ErrPluginSignatureInvalid)
	require.ErrorIs(t, VerifyPluginSignature(source, signature, ed25519.PublicKey(make([]byte, 32))), ErrPluginSignatureInvalid)
	require.ErrorIs(t, VerifyPluginSignature(source, []byte("short"), publicKey), ErrPluginSignatureInvalid)
	require.ErrorIs(t, VerifyPluginSignature(source, signature, []byte("bad-key")), ErrPluginSignatureInvalid)
}

// ---------------------------------------------------------------------------
// 沙箱降级链
// ---------------------------------------------------------------------------

func TestResolveSandboxPolicyFailClosed(t *testing.T) {
	// 策略要求沙箱：有 docker → 用 docker，无降级。
	mode, degraded, err := ResolveSandboxPolicy(SandboxRequired, []SandboxMode{SandboxModeDocker})
	require.NoError(t, err)
	assert.Equal(t, SandboxModeDocker, mode)
	assert.False(t, degraded)

	// 策略要求沙箱但无任何隔离 → fail-closed 拒绝，绝不静默降级。
	mode, degraded, err = ResolveSandboxPolicy(SandboxRequired, nil)
	require.ErrorIs(t, err, ErrSandboxUnavailable)
	assert.Equal(t, SandboxModeUnavailable, mode)
	assert.False(t, degraded)

	// 宽松策略：无沙箱 → 降级 workspace 并明示 degraded（调用方须记降级日志）。
	mode, degraded, err = ResolveSandboxPolicy(SandboxLenient, nil)
	require.NoError(t, err)
	assert.Equal(t, SandboxModeWorkspace, mode)
	assert.True(t, degraded)

	// bubblewrap 优先级高于 workspace。
	mode, _, err = ResolveSandboxPolicy(SandboxRequired, []SandboxMode{SandboxModeWorkspace, SandboxModeBubblewrap})
	require.NoError(t, err)
	assert.Equal(t, SandboxModeBubblewrap, mode)

	// 禁用策略 → none。
	mode, degraded, err = ResolveSandboxPolicy(SandboxDisabled, nil)
	require.NoError(t, err)
	assert.Equal(t, SandboxModeNone, mode)
	assert.False(t, degraded)
}

// ---------------------------------------------------------------------------
// 沙箱运行时探测
// ---------------------------------------------------------------------------

func TestDetectSandboxModesProbesBinaries(t *testing.T) {
	original := probeExecutable
	t.Cleanup(func() { probeExecutable = original })

	// docker + bwrap 均可用。
	probeExecutable = func(name string) (string, error) {
		if name == "docker" || name == "bwrap" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	modes := DetectSandboxModes()
	assert.Contains(t, modes, SandboxModeDocker)
	assert.Contains(t, modes, SandboxModeBubblewrap)
	assert.Contains(t, modes, SandboxModeWorkspace)

	// 无 docker/bwrap → 仅 workspace。
	probeExecutable = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	modes = DetectSandboxModes()
	assert.Len(t, modes, 1)
	assert.Equal(t, SandboxModeWorkspace, modes[0])
}

func TestResolveSandboxPolicyForHostFailClosed(t *testing.T) {
	original := probeExecutable
	t.Cleanup(func() { probeExecutable = original })
	probeExecutable = func(_ string) (string, error) {
		return "", errors.New("not found")
	}

	// 策略要求沙箱但主机无 docker/bwrap → fail-closed 拒绝。
	mode, degraded, err := ResolveSandboxPolicyForHost(SandboxRequired)
	require.ErrorIs(t, err, ErrSandboxUnavailable)
	assert.Equal(t, SandboxModeUnavailable, mode)
	assert.False(t, degraded)

	// 宽松策略 → 降级 workspace 并明示 degraded。
	mode, degraded, err = ResolveSandboxPolicyForHost(SandboxLenient)
	require.NoError(t, err)
	assert.Equal(t, SandboxModeWorkspace, mode)
	assert.True(t, degraded)
}

// ---------------------------------------------------------------------------
// 引擎层执行收口
// ---------------------------------------------------------------------------

const securityPluginSource = `export function run(label) { return label; }
export function submit(input) { return { url: input.url || "https://example.com" }; }`

func TestEngineSecurityGateBlocksUntilApproved(t *testing.T) {
	engine, err := Compile(securityPluginSource, Options{Key: "secure", Version: "1.0.0"})
	require.NoError(t, err)

	gate := NewExecutionGate()
	engine.SetSecurityPolicy(&SecurityPolicy{Gate: gate, RequireApproval: true})

	// 未审批 → 拒绝执行。
	_, err = engine.Call(context.Background(), "run", "hello")
	require.ErrorIs(t, err, ErrPluginNotApproved)

	// 审批当前内容哈希 → 执行成功。
	_, approveErr := gate.Approve("secure", engine.SourceHash())
	require.NoError(t, approveErr)
	result, err := engine.Call(context.Background(), "run", "hello")
	require.NoError(t, err)
	require.Equal(t, "hello", result)
}

func TestEngineSecurityGateReapprovalOnHashChange(t *testing.T) {
	engineA, err := Compile(`export function run() { return "A"; }`, Options{Key: "secure2"})
	require.NoError(t, err)
	engineB, err := Compile(`export function run() { return "B"; }`, Options{Key: "secure2"})
	require.NoError(t, err)

	gate := NewExecutionGate()
	engineA.SetSecurityPolicy(&SecurityPolicy{Gate: gate, RequireApproval: true})
	engineB.SetSecurityPolicy(&SecurityPolicy{Gate: gate, RequireApproval: true})

	_, err = gate.Approve("secure2", engineA.SourceHash())
	require.NoError(t, err)
	require.NotEqual(t, engineA.SourceHash(), engineB.SourceHash(), "改一行 → 哈希变化")

	_, err = engineA.Call(context.Background(), "run")
	require.NoError(t, err)
	_, err = engineB.Call(context.Background(), "run")
	require.ErrorIs(t, err, ErrPluginNotApproved, "新内容必须重新审批（审 A 跑 B 被拒绝）")
}

func TestEngineSecurityPermissionBoundHook(t *testing.T) {
	engine, err := Compile(securityPluginSource, Options{Key: "perms"})
	require.NoError(t, err)
	gate := NewExecutionGate()
	_, err = gate.Approve("perms", engine.SourceHash())
	require.NoError(t, err)

	// 未声明 network 权限 → submit 钩子（外联）被拒；run（纯计算）不受限。
	engine.SetSecurityPolicy(&SecurityPolicy{
		Gate:            gate,
		RequireApproval: true,
		Declared:        Permissions{},
		HookPermissions: map[string]PermissionKind{"submit": PermissionNetwork},
	})
	_, err = engine.Call(context.Background(), "run", "x")
	require.NoError(t, err)
	_, err = engine.Call(context.Background(), "submit", map[string]any{"url": "https://example.com"})
	require.ErrorIs(t, err, ErrPluginPermissionDenied)

	// 声明 network 后 submit 放行。
	declared, err := ParsePermissions([]any{"network"})
	require.NoError(t, err)
	engine.SetSecurityPolicy(&SecurityPolicy{
		Gate:            gate,
		RequireApproval: true,
		Declared:        declared,
		HookPermissions: map[string]PermissionKind{"submit": PermissionNetwork},
	})
	_, err = engine.Call(context.Background(), "submit", map[string]any{"url": "https://example.com"})
	require.NoError(t, err)
}

func TestEngineWithoutPolicyKeepsBackwardBehavior(t *testing.T) {
	engine, err := Compile(`export function run() { return "ok"; }`, Options{Key: "legacy"})
	require.NoError(t, err)
	result, err := engine.Call(context.Background(), "run")
	require.NoError(t, err)
	require.Equal(t, "ok", result)
}
