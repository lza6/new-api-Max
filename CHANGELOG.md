# Changelog

All notable changes to this project are documented here.

## [v1.3.168] - 2026-10-21

### Performance
- **First-paint locale is now a 12KB subset instead of the full 172KB bundle.**
  `i18nReady` only waits for `locales/<lang>.common.json` (445–494 keys covering
  every string the shell and public pages render); the full bundle keeps loading
  in the background and re-renders once, so the UI never flashes English.
  Local measurement: LCP 3980ms → 3448ms, React mount 1263ms → 1121ms.
- Locale chunks are named (`i18n-common-<lang>` / `i18n-full-<lang>`) so the
  first-paint window can be exercised in tests.

### Added
- `web/scripts/scan-i18n-common.mjs`, `build-i18n-common.mjs`,
  `fix-i18n-common.mjs`, `merge-prod-i18n-missing.mjs` — the key-subset pipeline.
- `i18next` `missingKeyHandler` records keys that fall back to their English
  source, which is what the automated "no English flash" checks consume.
- `web/e2e-local/i18n-common-audit.cjs` (local), `prod-i18n-no-flash.cjs`
  (production, via the 443 tunnel) — both assert zero untranslated keys with the
  full bundle blocked. Production run covers 12 public routes.
- `web/e2e-local/prod-admin-verify.cjs` — real admin login against production,
  walks dashboard / settings / wallet / keys, then the temporary account is
  removed from the database.

### Fixed
- Added the 49 keys that production actually renders but a local mock does not
  (OAuth buttons, model marketplace, rankings, docs).

### Known issues (pre-existing, not introduced here)
- Nav labels such as 主页 / 控制台 are passed to `t()` as Chinese source strings
  coming from the server-side nav configuration; they have no entry in any locale
  file, so an English UI shows them in Chinese.

## [v1.3.166] - 2026-10-21

### Performance
- Split system-settings section metadata (ids / titles / nav) into lightweight
  `sections.ts` modules. Routes and the sidebar config no longer import the
  component registries, so the entry bundle drops from **653KB to 332KB gzip**
  (the `index` chunk alone: 233.2KB → 34.5KB).
- The WebGL hero no longer mounts below 640px, on `saveData`, or on 2g/3g:
  mobile stops downloading the 234KB three.js chunk and running an off-screen
  60fps loop.
- KaTeX stylesheet now loads together with the KaTeX JS instead of sitting in
  the entry CSS (59 @font-face rules removed from first paint).
- Service Worker version is injected at build time (`bun scripts/stamp-sw.mjs`);
  a failed `cache.put` no longer turns a successful network response into an error.

### Fixed
- Dialogs had no max-height and no internal scroll: on phones the footer buttons
  were pushed out of the viewport and unreachable.
- `100vh` → `100dvh` in the shared dialog wrapper; drawer footers now respect
  `env(safe-area-inset-bottom)`.
- Attachment delete button was invisible but clickable on touch screens, and its
  44px hit area covered the filename (tapping the name deleted the file).
- Non-admin users could open an empty "Usage Report" tab; the dashboard now
  filters admin-only sections from one source and renders a permission notice on
  direct URL access.
- Wallet showed a **0 balance** when `/api/user/self` failed; it now shows an
  error state with retry.
- An empty redemption code made the Redeem button do nothing; the validation
  toast is reachable again.
- System settings rendered code-default values (and allowed saving them) when
  `/api/option/` failed; it now shows an error state instead.
- CSV export swallowed failures and could download a JSON error envelope as
  `.csv`; content-type is now validated.
- Channel bulk actions (Test All / Update All Balances) could be clicked
  repeatedly and multiply upstream requests; they are now guarded while running.
- CC Switch rendered a failed model request as "No models found".
- `home_page_content` / `notice` returning a non-string crashed the whole page
  (route error boundary); both are validated at the boundary.
- `StatusBadge` copy was mouse-only; it is now a focusable button (Enter/Space).
- Mobile inputs use `max(16px, 1rem)` so a user-raised root font size is no
  longer overridden.
- Card hover effects are gated behind `(hover: hover)` to stop sticky hover on
  touch; login inputs received `autoComplete`.

## [v1.3.165] - 2026-10-21

### Performance
- Added a finance covering index on `logs`:
  `idx_log_type_created_finance (type, created_at, quota, cost_quota, cost_rule_id)`.
  The ops overview aggregate (1.4M+ rows, `type = 2`) now runs as a PostgreSQL
  index-only scan: **12.2s → 1.67s** on production. The index was created on the
  live database with `CREATE INDEX CONCURRENTLY` before deploying, so startup
  migration only verifies it; new installations create it via AutoMigrate.

### Security
- Login timing no longer distinguishes "account does not exist" / "account has
  no password" from a wrong password: both branches run one bcrypt comparison
  against a fixed dummy hash (OWASP account-enumeration timing side channel).
- Registration pre-check no longer queries the email when email verification is
  enabled, so an already-registered email cannot be detected at the
  `MsgUserExists` step; the email conflict is handled by the existing generic
  response plus asynchronous notice.

### Tests
- Three-database conformance asserts the finance index exists on
  SQLite/MySQL/PostgreSQL.
- Unit tests prove the dummy-hash verification runs for missing and
  passwordless accounts (via an injectable validator, no flaky timing asserts).

## [v1.3.164] - 2026-10-21

### Fixed
- **Graph transport now forces certificate verification** even when
  `TLS_INSECURE_SKIP_VERIFY=1` has already mutated `http.DefaultTransport`
  (`common.Init` rewrites it in place; the lazily built sink cloned the polluted
  transport, so training-data uploads would silently skip verification).
- **Model success limiter ordering restored**: the success slot is reserved
  before the total-request token bucket and refunded when the total check
  rejects, so a success-limited request no longer consumes total-request quota.
  A panic in the handler now refunds via `defer` instead of leaking the slot
  until the window expires.

### Tests
- Handler-level assertion that the success counter is a `v2` ZSET (the previous
  test called the helper directly and could not catch a routing-layer key
  regression), panic-refund test, and a TLS-pollution regression test that
  mutates `http.DefaultTransport` before constructing the sink.

## [v1.3.163] - 2026-10-21

### Fixed
- **Model success-rate limit is now atomic.** Success slots are reserved with a
  single Lua `ZSET` script before the request and refunded when the request
  fails. The previous check-then-record flow (`LLEN` before, `LPUSH` after) let
  concurrent requests pass the "under the limit" check together, so the
  per-group success limit could be exceeded; failed requests also burned quota
  through a separate `_check` counter in the in-memory path. The key version is
  bumped to `MRRLS:v2` so legacy LIST keys cannot produce `WRONGTYPE`.
- **Training-capture OneDrive honours `Retry-After`.** 429 responses now carry
  the server-provided delay (seconds or HTTP-date, capped at the worker's max
  backoff); the worker waits at least that long instead of only using
  exponential backoff.
- **Graph calls no longer use the default transport.** The OneDrive sink builds
  a dedicated cloned transport (`MaxIdleConnsPerHost=32`) and keeps certificate
  verification on, so training-data uploads never inherit
  `TLS_INSECURE_SKIP_VERIFY` from the shared outbound pool.

### Tests
- Concurrency test proves exactly `max` reservations under 60 goroutines; refund
  and sliding-window expiry tests; handler-level test that a failed request does
  not consume a success slot.
- New legacy-drift `AutoMigrate` convergence test on real MySQL and PostgreSQL:
  a narrowed column is repaired once and **not** rewritten on the next start.
- OneDrive `Retry-After` parsing/status/backoff/transport tests.
- Three-database conformance (SQLite/MySQL/PostgreSQL) PASS, including the new
  drift-convergence case.

### Docs
- Audit recheck (`计划书/audit/scan-auth.md`, `scan-billing.md`): both auth P1s
  were already fixed in earlier batches; the two Stripe "P1"s were false
  positives — `StripeUnitPrice` is local-currency-per-USD, not a credit ratio.
- Production UI acceptance recorded: ops profit/cost panel, cost-rule table,
  320px layout and privacy copy verified in a real browser (16/16), with
  screenshots under `计划书/e2e-evidence/v1.3.162-ui/`.

## [v1.3.162] - 2026-10-20

### Fixed
- Redis limiter no longer caches a script SHA with `sync.Once`. `redis.Script.Run`
  falls back to EVAL on NOSCRIPT, so a Redis restart / failover / SCRIPT FLUSH no longer
  turns every model-rate-limit check into a 500 (the limiter is currently disabled in
  production, but the bug was latent).
- Cost coverage is now measurable: `logs.cost_rule_id` (0 = no rule) and
  `today/month_cost_coverage_percent` (revenue-weighted). The profit card warns when
  today's revenue is not fully covered instead of silently counting unconfigured models
  as zero cost (which overstated margins).
- ClickHouse create/ensure column lists include `cost_rule_id`.

### Docs
- Plan-document audit completed: **zero deletion candidates** under the strict rules;
  manifest at `计划书/archive/2026-10-20-doc-audit/MANIFEST.md`.
- Fixed stale versions/contradictions in `workflow_status.md`, `test_ledger.md`,
  `policy.md`, `计划书/README.md`, and the ops SOP/ADR references to the deleted
  `计划书/workflow_status.md`.
- Backfilled CHANGELOG entries for v1.3.150/v1.3.151.

## [v1.3.161] - 2026-10-20

### Fixed
- The `/static` 404 guard now uses the decoded URL path, so encoded variants such as
  `/static%2F...` can no longer skip the page rate limit and fall through to SPA rendering.
- Ops overview now has a process-local 5-minute cache fallback: without Redis, every panel
  load would otherwise run the 1.46M-row aggregate (~9s).

### Tests
- ClickHouse CREATE-table / ALTER-column consistency test.
- Token-cost overflow test now asserts the exact saturated value (a mere >0 assertion could
  not detect removal of the guard).

### Docs
- Corrected task-billing and IP-recording comments to match behavior; documented that
  token-recalculation task paths do not snapshot token costs yet.

## [v1.3.160] - 2026-10-20

### Fixed (second independent review)
- **ClickHouse log backend**: `cost_quota` was missing from the CREATE TABLE and the
  idempotent ADD COLUMN migration. GORM would have inserted an unknown column and every
  consume log write would fail. Added the column to both paths.
- Responses error events: `response.error` (used by the conversion layer) now counts as
  an explicit failure like `error`, so a failed stream is never billed estimated prompt.
- Subscription Waffo checkout now requires `StoreID`, matching the top-up guard and the
  webhook fail-closed rule (otherwise payment succeeds but the subscription never activates).
- Token cost multiplication now saturates on int64 overflow instead of wrapping to a
  bogus (possibly negative) cost snapshot.
- Unknown `/static/*` paths now return 404 instead of falling through to unlimited SPA
  rendering; real static assets are served before this guard.
- System monitor logs the metric source per resource (`cpu=host|cgroup memory=host|cgroup`)
  instead of only warning about memory.
- The IP-recording setting text now matches behavior: usage logs always record the caller
  IP; the switch only controls error logs.
- Added a status-code regression test: oversized task-protocol body → 413, malformed
  small JSON → 400.
- Notification-limit memory test cleans its key so `-count=2` is stable.

## [v1.3.159] - 2026-10-20

### Performance
- Ops overview now uses one full-table aggregate with conditional SUMs instead of five
  separate scans (1.46M logs: ~9s cold, cached for 5 minutes instead of 60s).

## [v1.3.158] - 2026-10-20

### Added — 运营财务（营收/成本/利润）
- **上游成本配置（管理员专属）**：按「渠道 × 模型」录入人民币成本，支持
  **按次**与**按 token（输入/输出每百万）**两种模式；模型留空表示渠道默认。
- **成本历史 append-only**：每次保存追加一条带 `effective_from` 的规则，
  旧规则永不修改；每个请求结算时把当时生效的成本**快照**写入 `logs.cost_quota`，
  因此调价不会改写历史利润，日报/月报/年报可复现。
- **运营面板财务卡**：今日/本月/本年的营收、上游成本、毛利、毛利率；
  预计本月/本年利润（按已过天数外推）；建议每日上游充值（日均成本 × 1.5 缓冲）；
  未配置成本规则时显式提示「利润按成本 0 计算」。
- **逐日财务序列** `GET /api/admin/ops/daily`：便于观察调价前后的波动。
- **成本规则接口** `GET/POST /api/admin/ops/model-costs`：只追加，不提供修改/删除。
- 成本金额以微元（1e-6 元）存整数；额度换算与前端展示营收使用同一口径。
- **使用日志始终记录调用者 IP**（不再受个人设置开关影响），便于排查滥用。

### Fixed (independent review of v1.3.156/157)
- 任务请求体超限现在真正返回 **413**：`getModelFromRequest` 保留错误链，
  route request 构建阶段也识别 `ErrRequestBodyTooLarge`。
- Responses 流的上游 `type:error` 事件不再估算 prompt（避免为失败请求计费）；
  `response.incomplete/cancelled` 携带的权威 usage 会被保留。
- Responses 路径也映射 `output_tokens_details`（img_o/audio_o/reasoning）。
- Waffo Pancake：`StoreID` 纳入支付/webhook 启用条件，避免「下单成功但回调被拒」。
- 静态资源限流豁免改为**精确白名单**（`/static/`、`/assets/` 与已知根文件），
  不再按扩展名放行任意路径。
- 内存通知限流加互斥（复合读-改-写原子）；cgroup 不可用时打印一次明确日志。
- 补充 Claude tool_use 计费、Responses 截断流、成本快照与 IP 的回归测试。

## [v1.3.157] - 2026-10-19

### Fixed
- Claude streams cut before terminal usage now include `tool_use` blocks in the
  completion estimate (~40 tokens per block). `ResponseText` only contains prose,
  so tool-only turns previously settled with an undercount (or zero).
- Task-plugin protocol pre-checks preserve the real failure reason: oversized
  bodies return **413** with the configured limit instead of a generic
  `400 Invalid task protocol request`; other read/parse errors include their cause.

## [v1.3.156] - 2026-10-19

Upstream-alignment hardening (security, billing correctness, container awareness).

### Security
- Waffo Pancake webhooks now verify the event's `StoreID` against the configured
  store and fail closed when it is unset. Signatures only prove *a* store sent the
  event, not that it was ours.
- `CreateUser` rejects non-standard role integers (for example `5`, `-1`, `99`)
  instead of only comparing against the operator's role.
- All outbound `http.Transport`s now use `InsecureTLSConfig.Clone()`; sharing one
  `*tls.Config` pointer across transports is unsafe after first use.
- Gemini `:countTokens` is rejected as an unknown route instead of being forwarded
  upstream as `generateContent` (unnecessary calls and billing).
- Bumped `gorilla/websocket` 1.5.0 → 1.5.3.

### Billing correctness
- OpenAI image responses are billed by **image payload**: object-shaped `data`
  counts once when it has `url`/`b64_json`, arrays bill `max(url entries,
  b64_json entries)`, and entries with only `revised_prompt` bill nothing.
- `output_tokens_details` is mapped onto `completion_tokens_details`, so `img_o`,
  `audio_o` and reasoning fields are no longer billed as zero.
- Responses streams cut before `response.completed` now estimate prompt tokens
  when any event was observed (unless upstream reported `response.failed`), and
  tool-argument/reasoning/refusal deltas feed the completion estimate.
- Ollama streams forward `tool_calls` carried by the final `done` frame.
- Task submission accepts any 2xx (201/202 are valid "accepted" responses).

### Reliability
- Notification limits use a fixed window (no hour suffix in the key), count
  atomically with TTL repair, and fall back to a 10-minute window when
  misconfigured.
- Added a `(created_at, event_id)` composite index on `audit_logs`.
- Task plugin `source` uses a 1 MiB-capable column (MySQL `mediumtext`); the old
  bare `text` capped at 64 KiB.
- Overload detection now uses cgroup CPU/memory figures, requires 3 consecutive
  samples (~15 s), and ignores stale samples (>30 s).
- Frontend build assets no longer consume the per-IP page rate limit (cold starts
  fetch dozens of files and were producing false 429s for the next user).
- Model request rate-limit counters are isolated per group, so one group's usage
  no longer consumes another group's quota.

## [v1.3.155] - 2026-10-19

### Fixed
- Failed capture signals are now retried **before** newer queued signals instead of being
  appended to the queue tail. A newer signal for the same conversation could otherwise
  advance the watermark first, forcing the older retry to re-upload the whole prefix
  (duplicate data). Pending retries are re-queued best-effort on shutdown; the retry
  limit (3) and `retried` counter are unchanged.
- Admin export list is paginated (20 per page). Previously it fetched only the first 100
  rows and discarded `total`, leaving older exports unreachable.

## [v1.3.154] - 2026-10-19

### Fixed
- Training capture ownership is now fail-closed whenever **any** storage account cannot be
  listed: the unreadable account may be the real writer, so guessing would write new parts
  into an older branch and split the directory irreversibly.
- Read fallbacks no longer change write ownership (a single file found on another account
  does not prove that account owns the directory).
- Delete now runs on **every** account; a real error (429/500/timeout) on any account is
  reported instead of claiming the file was deleted.
- OneDrive `DeleteFile` maps 404 to `ErrFileNotFound`; idempotence is handled by the
  controller / multi-account sink, so a delete on the wrong account can no longer report
  success while the file remains elsewhere.
- File listing keeps the backend's truncation flag (partial subdirectory failure/depth cap
  is no longer displayed as a complete list); `Stat` marks `truncated` when an account fails.
- Admin overview shows `retried`; user consent cards poll status every 30s and refresh the
  policy version after a rejected grant; daily history no longer shows "no data" alongside
  a read error; analyze/delete actions only appear for part files.

### Documentation
- README record format now documents the `kind` field (`full|suffix|message`); deployment and
  troubleshooting SOPs cover ownership, delete semantics, per-user leases, the production probe,
  and the current `NODE_TYPE` / metrics status.

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

## [v1.3.151] - 2026-10-18

### Fixed
- Training capture multi-account ownership is fail-closed when an account cannot be
  listed; read fallbacks no longer change write ownership; OneDrive 404 maps to
  `ErrFileNotFound` so a delete on the wrong account cannot report success; failed
  capture signals are retried (bounded) instead of being dropped.

## [v1.3.150] - 2026-10-18

### Added
- Training data capture multi-account load balancing (`TRAINING_ONEDRIVE_2_*`):
  directories are pinned to one account; existing data wins, new directories hash;
  reads merge accounts. Production E2E verified both accounts receive data.

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
