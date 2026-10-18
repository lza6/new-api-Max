# Changelog

All notable changes to this project are documented here.

## [v1.3.153] - 2026-10-19

### Fixed
- First-purchase discount could be claimed twice by concurrent balance purchases: the prior-purchase check now runs after the user row lock and uses a locking read; wallet deduction is a conditional update (`quota >= cost`).
- 100% first-purchase discount was treated as "no discount" by the UI: `discounted_price` no longer uses `omitempty`, and the UI decides eligibility from `first_purchase_discount_on && is_first_purchase && percent > 0`.
- External payment providers (Stripe/Creem/Waffo Pancake/Epay) no longer show the balance-only discounted price; "Amount Due" no longer prepends a literal `$` (CNY previously rendered as `$¥`).
- Non-CNY amounts lost their currency symbol.
- Savings no longer reports the full pay-as-you-go value as "saved" when a subscription has no price snapshot (legacy rows or after a quota-period reset).

### Verification
- `go build ./...` PASS; targeted model tests PASS.
- `bun run typecheck` PASS; subscription component tests PASS.
- `go test ./model/ -run 'EffectiveSubscriptionPrice|BuildSubscriptionSummary|PurchaseSubscription|TrainingCaptureLease'` PASS.

## [v1.3.152] - 2026-10-18

### Added
- Subscription first-purchase discount: per-plan percentage (0-100), applied only to balance-paid subscriptions for users who have never purchased; gated by `SUBSCRIPTION_FIRST_PURCHASE_DISCOUNT` (default off). External fixed-price providers are unaffected.
- Subscription savings card: per-subscription `consumed_quota`, `remaining_quota`, `plan_price`, `pay_as_you_go_value`, `saved_amount` using the existing quota/price conversion (no fabricated model-price estimates).
- Multi-instance training capture coordination: per-user DB lease (`system_task_locks`) so only one instance uploads a user's conversation; `training_capture_skipped_lease_total` metric.
- Admin plan form field for first-purchase discount; plan comparison and purchase dialog show the discounted price.

### Changed
- SQLite `subscription_plans` manual migration now includes `first_purchase_discount_percent`.
- `user_subscriptions` stores price snapshots (`price_amount`, `period_price`, `currency`); `period_price` resets with `amount_used`.

### Verification
- Subscription first-purchase E2E: 5/5 (plan creation, discounted list price, UI display, balance charge = 80 × QuotaPerUnit, cleanup).
- Three-DB conformance: SQLite / MySQL 5.7 / PostgreSQL 15 PASS.
- Training capture package tests PASS.

## [v1.3.149] - 2026-10-18

### Fixed
- Approval resource hash placed first so 191-byte truncation cannot drop parameter binding.
- Ops list responses fail closed on malformed JSON (empty `data` instead of passing through).
- Ops list/create responses over 8MB return 502 instead of truncated JSON.

### Verification
- Independent review re-verdict: Go Approve, frontend Approve, security Approve.
- Production deployed in blue-green `3000 → 3001`.

## [v1.3.148] - 2026-10-18

### Added
- MCP Gateway (Streamable HTTP / JSON-RPC 2.0), DB sessions, tools/resources, SSE progress, paid-tool approvals.
- Action-level deny-by-default approvals with one-shot CAS and admin UI.
- `/v1/files`, `/v1/fine-tunes`, `/v1/batches` real relay with per-object ownership registry.
- G4 complexity routing enforce with token/subscription guards and price fallback.
- hreflang locale prefixes, auth noindex, Organization/BreadcrumbList, policy-page meta.
- Key creation curl snippet; subscription renewal reminders at 3 days and 1 day.

### Fixed
- Claude cache-only usage billing and stream usage overwrite.
- Percent-encoded ops paths bypassing fine-tune/batch billing.
- Open redirect in `/zh/...` handling.
- Ops request body memory amplification.
- Channel key encryption now refuses migration without an explicit `CRYPTO_SECRET`, and refuses disabling while ciphertext exists.
- `$` expansion in SEO regex replacements.

### Verification
- Real browser E2E 7/7; local stress 50 concurrency 0 errors; production deployment and smoke tests.
