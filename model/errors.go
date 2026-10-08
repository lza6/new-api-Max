package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

// User auth errors
var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUserEmptyCredentials = errors.New("empty credentials")
	// ErrUserNoPassword：账号存在但未设置密码（通常由第三方登录注册，如 NodeLoc）。
	// 与 ErrInvalidCredentials 区分，便于登录失败时给出「请用第三方登录」的精确提示，
	// 而不是笼统的「用户名或密码错误，或用户已被封禁」。
	ErrUserNoPassword    = errors.New("user has no password")
	ErrEmailAlreadyTaken = errors.New("email already taken")
	ErrEmailNotFound     = errors.New("email not found")
	ErrEmailAmbiguous    = errors.New("email matches multiple users")
)

// Token auth errors
var (
	ErrTokenNotProvided = errors.New("token not provided")
	ErrTokenInvalid     = errors.New("token invalid")
	// ErrTokenQuotaExhausted 令牌存在但额度已用尽（与无效令牌区分，便于给出明确提示）。
	ErrTokenQuotaExhausted = errors.New("token quota exhausted")
)

// Redemption errors
var ErrRedeemFailed = errors.New("redeem.failed")

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")
var ErrTwoFAAlreadyEnabled = errors.New("2fa already enabled")
