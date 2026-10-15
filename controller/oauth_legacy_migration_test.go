package controller

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/lza6/new-api-Max/model"
	"github.com/lza6/new-api-Max/oauth"
)

// 历史 OAuth 绑定迁移的**账号证据**判据测试。
//
// ## 这是安全边界，不是功能测试
//
// 旧实现只要「GitHub 登录名 == 库里存的值」就迁移并直接登录。
// GitHub 允许改名、旧名可被他人抢注 —— 抢到就能登进别人的账号。
// 因此现在**必须**有「已验证邮箱一致」这类不可伪造的证据才迁移，
// 且拿不到证据时**拒绝**（fail-closed），而不是放行。
//
// 下面每一条都是在钉住「哪种情况必须拒绝」。

func evidenceContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	return c
}

func TestLegacyMigrationRefusesWithoutVerifiedEmails(t *testing.T) {
	target := &model.User{Email: "victim@example.com"}

	cases := []struct {
		name string
		user *oauth.OAuthUser
	}{
		{"Extra 里没有该键", &oauth.OAuthUser{Extra: map[string]any{}}},
		{"Extra 为 nil", &oauth.OAuthUser{}},
		{"键存在但为 nil", &oauth.OAuthUser{Extra: map[string]any{"verified_emails": nil}}},
		{"空列表", &oauth.OAuthUser{Extra: map[string]any{"verified_emails": []string{}}}},
		{"类型不对（string 而非 []string）", &oauth.OAuthUser{Extra: map[string]any{"verified_emails": "victim@example.com"}}},
		{"类型不对（[]any）", &oauth.OAuthUser{Extra: map[string]any{"verified_emails": []any{"victim@example.com"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, legacyMigrationHasAccountEvidence(evidenceContext(), tc.user, target),
				"拿不到已验证邮箱时必须**拒绝迁移**（fail-closed），"+
					"否则任何抢注登录名的人都能接管账号")
		})
	}
}

func TestLegacyMigrationRefusesWhenEmailDiffers(t *testing.T) {
	target := &model.User{Email: "victim@example.com"}
	// 攻击者：抢到了受害者的旧 GitHub 登录名，但邮箱是他自己的。
	attacker := &oauth.OAuthUser{Extra: map[string]any{
		"verified_emails": []string{"attacker@evil.example"},
	}}

	assert.False(t, legacyMigrationHasAccountEvidence(evidenceContext(), attacker, target),
		"邮箱不一致时必须拒绝 —— 这正是账号接管的典型形态")
}

func TestLegacyMigrationAcceptsWhenVerifiedEmailMatches(t *testing.T) {
	target := &model.User{Email: "Victim@Example.com"}
	// 本人：GitHub 上有一个已验证邮箱与账号邮箱一致（大小写不同也算）。
	owner := &oauth.OAuthUser{Extra: map[string]any{
		"verified_emails": []string{"other@example.com", "victim@example.com"},
	}}

	assert.True(t, legacyMigrationHasAccountEvidence(evidenceContext(), owner, target),
		"有匹配的已验证邮箱时应当允许迁移（否则老用户永远无法平滑迁移）")
}

func TestLegacyMigrationRefusesWhenTargetHasNoEmail(t *testing.T) {
	noEmail := &model.User{Email: ""}
	provider := &oauth.OAuthUser{Extra: map[string]any{
		"verified_emails": []string{"whatever@example.com"},
	}}

	assert.False(t, legacyMigrationHasAccountEvidence(evidenceContext(), provider, noEmail),
		"目标账号没有邮箱时无从比对 → 不给证据（不能因为对方有邮箱就放行）")
}

func TestIsAllDigitsExcludesNumericLegacyIDs(t *testing.T) {
	// 纯数字的 legacy 值不可能是一个 GitHub 登录名 ——
	// 若参与登录名比较，会出现「数字 id 恰好等于某人登录名」的碰撞。
	assert.True(t, isAllDigits("12345"))
	assert.False(t, isAllDigits("user123"))
	assert.False(t, isAllDigits(""))
	assert.False(t, isAllDigits("12a45"))
	// 带符号/空格都不是纯数字。
	assert.False(t, isAllDigits("-1"))
	assert.False(t, isAllDigits(" 12"))
}
