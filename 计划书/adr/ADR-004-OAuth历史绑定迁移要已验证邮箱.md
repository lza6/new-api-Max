# ADR-004：OAuth 历史绑定迁移必须要求「已验证邮箱」（fail-closed）

> 目录：`计划书/adr/`（编号在本目录内独立）。本次决策批次：2026-10-16 安全修复。
> 最后核对：2026-10-16（`VERSION` = `v1.3.147`）。

## 背景

网关支持把 OAuth 账号从旧的 provider id（`legacy_id`）迁移到当前的 provider user id。
原实现只要 `legacy_id` 在 provider 侧存在、且能 `FillUserByProviderID` 找到账号，就执行迁移。

这是一个**账号接管**漏洞：攻击者若能构造一个带受害者 `legacy_id` 的 OAuth 账号，
就可能把受害者的账号迁移/绑定到自己的 provider 身份下。

## 决策

迁移前必须有**可采信的账号证据**：该 OAuth 账号拥有一个**已验证**邮箱，
且与目标账号邮箱（归一化后）**一致**。判据实现在 `legacyMigrationHasAccountEvidence`
（`controller/oauth.go:654-685`）。**拿不到已验证邮箱列表 → 拒绝迁移（fail-closed）**
（`controller/oauth.go:443-449,458-467`），并记审计事件
`user.oauth.legacy_migration_refused`（`:462-467`）。

**拒绝迁移 ≠ 拒绝登录**：用户仍可用其它已绑定方式登录（`:446-448`）。

所需能力通过新增接口 `VerifiedEmailProvider`（`oauth/provider.go:56-59`）暴露；
GitHub 实现 `GetVerifiedEmails`（`oauth/github.go:204-234`），接口失败时明确记日志并让迁移被拒
（`oauth/github.go:160-163`）。

## 被拒绝的替代方案

1. **只要 `legacy_id` 匹配就迁移**（原始做法）：即账号接管漏洞本身。
2. **用未验证邮箱做证据**：攻击者可在 provider 侧填任意未验证邮箱 → 证据无意义。
   （判据必须限定「**已验证**」邮箱，见 `oauth/github.go:204` 注释。）
3. **迁移失败就整体登录失败**：过度惩罚——拒绝迁移只是保守，用户仍应能登录（`controller/oauth.go:446-448`）。

## 后果（含代价）

- ✅ 消除了「无证据迁移」这一接管入口；证据缺失一律拒绝。
- ⚠️ 若 provider 未实现 `VerifiedEmailProvider` 或接口调用失败，历史迁移**不再发生**
  （用户需用别的方式登录）——这是刻意选「拒绝」而非「放行」的取舍。
- ⚠️ 迁移成功与拒绝都写审计，便于事后调查（`:462-477`）。
- ⚠️ 本决策**不应被回滚**：回滚 = 重新打开接管漏洞（见 `SOP-回滚.md` §7）。

## 证据

- `controller/oauth.go:443-477,654-685`
- `oauth/provider.go:56-59`（`VerifiedEmailProvider.GetVerifiedEmails`）
- `oauth/github.go:160-163,204-234`
- 攻击者场景用例（5 条）：`controller/oauth_legacy_migration_test.go:30,53,64,75,85`
  （`TestLegacyMigrationRefusesWithoutVerifiedEmails` / `RefusesWhenEmailDiffers` /
  `AcceptsWhenVerifiedEmailMatches` / `RefusesWhenTargetHasNoEmail` / `TestIsAllDigitsExcludesNumericLegacyIDs`）
