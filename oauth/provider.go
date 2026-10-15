package oauth

import (
	"context"

	"github.com/lza6/new-api-Max/model"
	"github.com/gin-gonic/gin"
)

// Provider defines the interface for OAuth providers
type Provider interface {
	// GetName returns the display name of the provider (e.g., "GitHub", "Discord")
	GetName() string

	// IsEnabled returns whether this OAuth provider is enabled
	IsEnabled() bool

	// ExchangeToken exchanges the authorization code for an access token
	// The gin.Context is passed for providers that need request info (e.g., for redirect_uri)
	ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error)

	// GetUserInfo retrieves user information using the access token
	GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error)

	// IsUserIDTaken checks if the provider user ID is already associated with an account
	IsUserIDTaken(providerUserID string) bool

	// FillUserByProviderID fills the user model by provider user ID
	FillUserByProviderID(user *model.User, providerUserID string) error

	// SetProviderUserID sets the provider user ID on the user model
	SetProviderUserID(user *model.User, providerUserID string)

	// GetProviderPrefix returns the prefix for auto-generated usernames (e.g., "github_")
	GetProviderPrefix() string

	// ProviderUserIDColumn returns the users-table column that stores this provider's
	// user ID, used by bind flows to update only the binding column instead of
	// writing back a full user snapshot. Providers that persist bindings elsewhere
	// (e.g. the user_oauth_bindings table) return an empty string.
	ProviderUserIDColumn() string
}

// VerifiedEmailProvider 是**可选**能力：返回该 OAuth 账号**已被提供商验证**的邮箱列表。
//
// ## 为什么需要它（安全边界，不是锦上添花）
//
// 历史绑定迁移（把用户表里按「登录名」存的 provider id 迁到「数字 id」）时，
// **登录名是用户可以改的**。GitHub 允许改名，且释放出的旧登录名会被他人抢注。
// 因此「登录名 == 我库里的旧值」**不构成任何账号所有权的证据** ——
// 只凭它就迁移并登录，等于把受害者账号交给任何抢到该登录名的人。
//
// 正确的证据是「同一个已验证邮箱」或「已通过二次因子」。
// 本接口提供前者；不具备该能力的 provider 断言失败，迁移路径会**拒绝迁移**
// （fail-closed：宁可让老用户手工重新绑定，也不冒账号被接管的风险）。
type VerifiedEmailProvider interface {
	// GetVerifiedEmails 返回已验证邮箱（已归一化由调用方处理）。
	// 实现应只返回 provider 明确标记为 verified 的邮箱。
	GetVerifiedEmails(ctx context.Context, token *OAuthToken) ([]string, error)
}
