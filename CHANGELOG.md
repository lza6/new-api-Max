# Changelog

All notable changes to this project are documented here.

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
