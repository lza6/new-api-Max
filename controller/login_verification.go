package controller

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/lza6/new-api-Max/common"
	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/service"
	passkeysvc "github.com/lza6/new-api-Max/service/passkey"
)

func VerifyLogin(c *gin.Context) {
	var request struct {
		FlowToken string `json:"flow_token"`
		Method    string `json:"method"`
		Code      string `json:"code"`
	}
	if common.DecodeJson(c.Request.Body, &request) != nil || request.FlowToken == "" || request.Code == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if request.Method == "" {
		request.Method = service.VerificationMethodTwoFA
	}
	if request.Method != service.VerificationMethodTwoFA {
		writeSecurityOperationError(c, service.ErrProofMethod)
		return
	}
	bundle, err := service.VerifyLoginCode(request.FlowToken, request.Code, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		// 账号级失败台账：登录流程的第二步（2FA/备用码）失败同样计入，
		// 否则攻击者可先用正确密码通过第一步，再无上限猜 2FA 验证码。
		// 这里只记账、不改变响应——锁定判定仍在密码登录入口，
		// 且 2FA 自身已有 locked_until（model/twofa.go）双重约束。
		recordAccountLockoutFailureForLoginFlow(request.FlowToken)
		writeSecurityOperationError(c, err)
		return
	}
	completeVerifiedLoginResponse(c, bundle, service.VerificationMethodTwoFA)
}

// recordAccountLockoutFailureForLoginFlow 按一次性登录流程令牌定位账号并记一次失败。
// 令牌无法解析/已过期时静默跳过（不引入新的失败面）。
func recordAccountLockoutFailureForLoginFlow(flowToken string) {
	user, err := service.ResolveLoginFlowUser(flowToken)
	if err != nil {
		return
	}
	recordAccountLockoutFailure(user)
}

func LoginPasskeyBegin(c *gin.Context) {
	var request struct {
		FlowToken string `json:"flow_token"`
	}
	if common.DecodeJson(c.Request.Body, &request) != nil || request.FlowToken == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	verification, err := service.RequireLoginVerification(request.FlowToken, service.VerificationMethodPasskey)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	credential, err := model.GetPasskeyByUserID(verification.State.UserID)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	wa, err := passkeysvc.BuildWebAuthn(c.Request)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	user := &model.User{Id: verification.State.UserID}
	options, sessionData, err := wa.BeginLogin(passkeysvc.NewWebAuthnUser(user, credential), webauthnlib.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	token, expiresAt, err := passkeysvc.CreateSessionDataFlow(model.AuthFlowPurposeLoginPasskey, passkeysvc.FlowSecurity{
		AuthSessionIdentity: model.AuthSessionIdentity{UserID: user.Id, UserAuthVersion: verification.State.AuthVersion},
		LoginFlowID:         verification.Flow.Id, LoginExpiresAt: verification.Flow.ExpiresAt.Unix(),
	}, sessionData)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"flow_token": token, "expires_at": expiresAt, "options": options})
}

func LoginPasskeyFinish(c *gin.Context) {
	var request struct {
		FlowToken        string          `json:"flow_token"`
		PasskeyFlowToken string          `json:"passkey_flow_token"`
		Credential       json.RawMessage `json:"credential"`
	}
	if common.DecodeJson(c.Request.Body, &request) != nil || request.FlowToken == "" || request.PasskeyFlowToken == "" || len(request.Credential) == 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	verification, err := service.RequireLoginVerification(request.FlowToken, service.VerificationMethodPasskey)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(request.Credential)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	identity := model.AuthSessionIdentity{UserID: verification.State.UserID, UserAuthVersion: verification.State.AuthVersion}
	sessionData, security, err := passkeysvc.PopSessionDataFlow(request.PasskeyFlowToken, model.AuthFlowPurposeLoginPasskey, identity)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	if security.LoginFlowID != verification.Flow.Id || sessionData.UserVerification != protocol.VerificationRequired {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return
	}
	credential, err := model.GetPasskeyByUserID(identity.UserID)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	wa, err := passkeysvc.BuildWebAuthn(c.Request)
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	validated, err := wa.ValidateLogin(passkeysvc.NewWebAuthnUser(&model.User{Id: identity.UserID}, credential), *sessionData, parsed)
	if err != nil {
		// passkey 断言校验失败：属"凭证持有物"校验失败，计入账号级失败台账。
		// 其余分支是流程/状态错误（不可用令牌、过期、账号状态变更），不记账。
		recordAccountLockoutFailureForUserID(identity.UserID)
		writeSecurityOperationError(c, err)
		return
	}
	if err := model.UpdatePasskeyAssertionState(identity.UserID, validated, time.Now()); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	bundle, err := service.CompleteLoginVerification(request.FlowToken, verification, service.VerificationMethodPasskey, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	completeVerifiedLoginResponse(c, bundle, service.VerificationMethodPasskey)
}

// recordAccountLockoutFailureForUserID 按用户 ID 记一次凭证校验失败
// （passkey 流程持有已解析的 identity，无需再依赖流程令牌）。
func recordAccountLockoutFailureForUserID(userID int) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return
	}
	recordAccountLockoutFailure(user)
}

func completeVerifiedLoginResponse(c *gin.Context, bundle *service.AuthBundle, method string) {
	identity, err := service.ParseAccessToken(bundle.AccessToken)
	if err != nil {
		writeAuthSessionError(c, err)
		return
	}
	user, err := model.GetSelfUserById(identity.UserID)
	if err != nil {
		writeAuthSessionError(c, err)
		return
	}
	// 所有登录方式（含 2FA 与 passkey 这些不走 controller.Login 的路径）在
	// 会话签发成功时清零账号级失败台账——成功认证必须立刻解除锁定。
	clearAccountLockout(user)
	c.Set("login_verification_method", method)
	writeLoginResponse(c, user, bundle)
}
