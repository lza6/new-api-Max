package oauth

import "errors"

// ErrLegacyMigrationUnverified 历史绑定迁移缺少可采信的账号证据。
//
// 触发场景：GitHub 登录名匹配到某个历史账号，但该 GitHub 账号的
// **已验证邮箱**与目标账号邮箱不一致（或拿不到）。此时**拒绝迁移** ——
// 因为登录名可以被改名/抢注，不构成账号所有权证据，放行等于账号接管。
//
// 用户侧表现：这次 OAuth 登录失败；他仍可用其它已绑定的方式登录，
// 登录后在设置里显式绑定这个 GitHub 账号。
var ErrLegacyMigrationUnverified = errors.New(
	"oauth: legacy account migration refused: the provider account has no verified email matching this account")
