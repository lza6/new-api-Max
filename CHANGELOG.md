# Changelog

All notable changes to this project are documented here.

## [v1.3.192] - 2026-10-23

### Added
- **Database log retention is configurable from the admin UI.** `LogRetentionDays` had
  no setting on the Log Maintenance page — it could only be changed by writing the
  `options` row directly, so the feature was invisible to operators. The section now has
  a "Database log retention (days)" field (0 = keep forever, the default).
- Fixed the same form's save handler: it returned early whenever the "record consumption"
  switch was unchanged, which would have silently dropped a retention-only edit.

### Changed
- `/api/log/report` gains a 60-second Redis cache keyed by `(group_by, start, end)`.
  Its six groupings — especially channel/user/group/token — fall outside the `logs`
  covering index, so every switch of `group_by` re-scans the whole window. The payload is
  display-only and cached with concrete types (not `gin.H`) to avoid int64 → float64 drift.

## [v1.3.191] - 2026-10-23

### Added
- **Database log retention.** The admin page only ever offered a **one-shot** cleanup of
  the *log files* under `/app/logs`; the `logs` table itself had **no automatic cleanup at
  all**, so production had accumulated 1,861,886 rows / 3.5GB over 42 days — exactly what
  fed the 2026-10-23 OOM. New `LogRetentionDays` option (0 = keep forever, the default)
  is consumed by `StartLogRetentionTask`, which on the master enqueues the existing
  **batched** `log_cleanup` system task every 6 hours. Batched on purpose: a single
  DELETE over 1.8M rows would blow up WAL and lock the table for a long time.
- First run after deploy removed 1,221,924 rows (09-11 → 10-16, i.e. exactly 7 days
  retained). Historical statistics stay intact because `log_daily_stats` keeps its own
  43-day aggregate (406 rows / 160KB).

## [v1.3.190] - 2026-10-23

### Fixed
- **`log_daily_stats` was never created in production.** The startup log printed
  `database migration started`, yet the table did not exist and the refresh task died on
  its first tick with `42P01 relation does not exist`. The same deploy path also left the
  obsolete `idx_logs_traffic` in place. `AutoMigrate(&LogDailyStat{})` now runs at the
  point of use (`RefreshLogDailyStats`), so a missing table fails loudly instead of
  silently degrading the stats endpoints back to full table scans. The root cause of
  `migrateLOGDB()` not applying these schema changes in this deployment shape is still open.

## [v1.3.189] - 2026-10-23

Daily log statistics are pre-aggregated, so the stats endpoints stop scanning the whole
`logs` table.

### Added
- **`log_daily_stats` summary table + `StartLogDailyStatsRefresh`.** `/api/log/traffic`,
  both bandwidth leaderboards and `/api/site/stats` used to aggregate the entire requested
  window straight off `logs` (1.84M rows / 3.5GB; ~1.8s for 30 days with the covering
  index, degrading linearly — a year out it would be back to 4-5s). They now read a
  per-day pre-aggregated table (≤ days × 2 rows, milliseconds). Refreshed every 10 minutes
  on the master node; the first run backfills history (capped at 400 days). Queries fall
  back to the raw scan when the summary does not cover the requested window, so a fresh
  deployment, an un-finished backfill, or a ClickHouse log DB behaves exactly as before.
  Dimension coverage is `total` and `by-model` — string dimensions only, because numeric
  dimensions would need dialect-specific `CAST`. `/api/log/report`'s six dimensions keep
  the original path.

## [v1.3.188] - 2026-10-23

Production Postgres OOM chain closed; four audit P0/P1 defects fixed.

### Fixed
- **Model rate limit failed closed on Redis errors.** Both the success-slot
  reservation and the total-request check returned `500 rate_limit_check_failed`
  when Redis errored. With `ModelRequestRateLimitEnabled=true`, one Redis blip
  would turn every `/v1/*` relay call into a 500 while `/api/status` kept
  returning 200 — probes keep steering traffic, replaying the 2026-09-29 outage
  shape. Both paths now degrade to the in-process limiter, matching what IP/user
  rate limiting has done since that incident.
- **Tiered settlement could pay the user for a cache hit.** The save-time smoke
  test only varies P/C/Len (CR/CC/Img/AI stay 0), so an expression whose discount
  term exceeds the principal (e.g. `p*3 + c*15 - cr*6`) passed validation and
  turned negative at runtime; `SettleBilling` then routed it through
  `IncreaseUserQuota`, crediting the user on every cache hit. Negative cost is now
  clamped to 0 with a logged audit record.
- **Negative `p` / `page_size` produced a negative OFFSET.** `GetPageQuery` passed
  negatives through, so `GetStartIdx()` returned a negative offset and
  PostgreSQL/MySQL rejected the statement — `/api/log/` and `/api/log/self` 500ed.
- **Day key used `/ 86400`, which breaks on MySQL.** MySQL's `/` is decimal
  division, so the result failed to scan into int64. Switched to the integer form
  `x - (x % 86400)` already documented in `model/usage_report.go`.
- **Any 500 navigated the user to `/500`, even from a background poller.**
  `queryCache.onError` called `onInternalServerError()` unconditionally on 500, so a
  transient failure of a dashboard/ops polling query kicked the user out of an open
  drawer or form and discarded unsaved input. Polling queries (any observer with
  `refetchInterval`) now only toast; page-level queries still navigate.

### Tooling
- `web/package.json` gained `test:shards`. The planning guide referenced
  `bun run test:shards` in seven places but the script was never registered.

## [v1.3.187] - 2026-10-23

Frontend suite goes fully green (1582/1582) and the dead-code gate is honest again.

### Fixed
- **Plugin icon integrity was fail-open.** `fetchPluginIconDataUri` only compared the
  digest when `crypto.subtle` existed (`actual !== null && actual !== expected`), so in a
  non-secure context (http intranet, older browser) a **pinned** icon was shown without
  verifying it. A pinned `sha256` now fails closed: no hash → no icon.
- **Policy Centre was invisible in the settings nav.** `security/section-registry.tsx`
  renders `policy-center`, but the lightweight metadata list in
  `security/sections.ts` never registered it, so it produced no nav item.
  `section-metadata.test.ts` caught the drift.
- **Two navigation CTAs lost their test contract**: the tests asserted
  `role="link"` while Base UI renders `<a role="button">` for
  `<Button render={<Link/>}>`. Assertions now describe the real contract.

### Changed
- **Test isolation: `vmThreads` → `threads`.** `vmThreads` shares one VM context per
  worker, so one file's `vi.mock('sonner')` was reused by the next file and assertions
  landed on the wrong `vi.fn` (`单跑绿、合跑红`). Reproduced on `delete-plan-dialog`,
  `confirm-dialog`, `model-cards` and `transparency-panel`; `threads` + `maxWorkers: 1`
  gives real per-file isolation at the same speed.
- **`test-setup` now provides WebCrypto**, so digest-dependent tests execute the real
  hashing path instead of silently taking the "cannot hash" branch.
- **`handleServerError` exposes `resetServerErrorDedupe()`** and `delete-plan-dialog`
  clears it per test: the "report each error once" table is module-level state and made
  that test order-dependent.
- **`config-default-language.test.ts` spies the `localStorage` instance** instead of
  `Storage.prototype`, which is not shared across realms under the `threads` pool.

### Removed (dead code, no behaviour change)
- 40 exports that were only ever used inside their own file (constants, section tables,
  barrel re-exports), `OpsDaySeriesItem` + `getOpsDailySeries` (unused API wrapper),
  `site-subscription-stats-card.tsx` (never imported), and the
  `i18next-browser-languagedetector` dependency (nothing imports it since the default
  language became zhCN).

### Gates
- `web/scripts/knip-gate.mjs` no longer counts `e2e-local/` and `scripts/` (local probes
  and CLI tools are entry points, not dead modules) and ignores `playwright`, which only
  those probes use. Gate is green: 586 legacy issues, **0 new**.
- Design-token snapshot regenerated for the `ink` preset (117 drift entries accepted —
  the change is the V13 preset, nothing else).
- Full suite: **1582/1582 passed, 0 failed** (5 shards, 567s). `typecheck` clean,
  `bun run build` clean, `go build ./...` + `go vet ./controller ./common` clean.

## [v1.3.186] - 2026-10-22

Visual identity batch 3 — motion rule, `ink` structural preset, visual baseline, preset contrast fix (视觉体系 V8/V12/V13).

### Added
- **`.transition-quiet`** utility + project-wide replacement of `transition-all` (41 sites, 25
  files). The new class animates only paint/composite properties (color, background, border,
  fill, stroke, opacity, box-shadow, transform, translate, scale) with `--motion-fast` and
  `--ease-quiet`; layout properties can no longer be animated by accident. The progress bar
  keeps an explicit `transition-[width]` because a smooth progress fill is intended behaviour.
- **`ink` preset** (D4): graphite neutrals + a single signal colour, and the first *structural*
  preset — `PRESET_DEFAULT_RADIUS` / `PRESET_DEFAULT_SCALE` tighten radius and density while
  `resolveThemeRadius` / `resolveThemeScale` guarantee an explicit user choice always wins.
- **`web/e2e-local/visual-baseline.cjs`** (V12): one command captures pages × breakpoints ×
  presets × light/dark and writes `results.json` plus per-page axe results, so the visual layer
  has a repeatable gate instead of ad-hoc screenshots.

### Fixed
- **`anthropic` preset failed WCAG AA**: white text on the original clay primary measured
  **2.9:1**. The primary is now `oklch(0.57 0.128 38)` (same hue, darker) — 0 contrast
  violations in light and dark, verified with axe on `/keys` and `/dashboard/overview`.

### Verified
- `bun run typecheck` PASS · `bun run build` PASS · `vitest` on lib/components/dashboard
  **163/163 PASS** across 24 files (incl. new theme-customization resolver tests, 6 cases).
- Preset contrast sweep (axe `color-contrast`, `/keys` + `/dashboard/overview` × 11 presets):
  **all 0 violations** after the anthropic fix (before: anthropic 1 node on both pages).
- Visual baseline run: 18 screenshots (2 pages × 3 presets × 2 widths + dark), axe 0 violations
  on `default` and `ink`, 1 benign console 401 before login.

### Not done (tracked in test_ledger.md)
- V4b numeric column alignment, V5 remaining `bg-*-50` backlog, V6b `--chart-N` token unification,
  V9–V11 page-by-page coverage.
- The frontend suite still has the pre-existing red files documented in `test_ledger.md`
  (proven to fail on `6f284de93` as well).

## [v1.3.185] - 2026-10-22

Visual identity batch 2 — table micro-labels, tokenised status colours, single-hue chart ramp (视觉体系 V4/V5/V6).

### Changed
- **Tables (V4)**: `TableHead` is now a micro-label (11px, muted, letter-spaced) and the
  table root no longer forces `[&_th]:text-sm`, which used to win over the header class.
  Data-table frames and mobile card lists moved to `border-rule + bg-surface-1 + rounded-xl`,
  so dense tables read as one instrument surface instead of a bordered grey box.
- **Status colours (V5)**: `Alert` gained token-based `success` / `warning` / `info` variants
  (`--success-text` / `--warning-text` keep WCAG AA). The system-task panel and the setup
  database step now use tokens instead of `bg-amber-50` / `bg-emerald-50` / `bg-sky-50`.

### Added
- **Chart ramp (V6)**: `lib/chart-palette.ts` builds a canvas-safe single-hue series ramp from
  `--primary` / `--background` (normalises any CSS colour through a 2d context, falls back to a
  fixed ramp without one). Dashboard charts use it instead of VChart's blue/cyan/yellow/orange/purple
  default scheme; ordinal `USER_COLORS` follows the active preset and theme.

### Verified
- `bun run typecheck` PASS · oxlint on touched files clean · `bun run build` PASS.
- `vitest run src/features/dashboard src/components/ui src/components/__tests__` 81/81 PASS;
  `lib/__tests__/chart-palette.test.ts` 4/4 PASS.
- Browser probe: `--primary` resolves to `#146fa8` and survives `ctx.fillStyle` normalisation,
  so the ramp works on canvas; `/keys` header computes to 11px with 0.275px tracking.

### Not done (tracked in test_ledger.md)
- V4b: numeric column right-alignment needs a column metadata contract.
- V5: the remaining `bg-{emerald,amber,sky,…}-50` backlog is swept per batch, not in one shot.
- V6b: charts that reference `--chart-1..5` directly still use the old hue set, because those tokens
  double as StatusBadge text colours and cannot be re-hued without a contrast audit.

## [v1.3.184] - 2026-10-23

### Fixed
- Policy-centre textareas (per-model request budget, semantic cache models) now
  cap input at 20 000 characters before writing to options (review finding: an
  oversized paste could bloat the stored option).

## [v1.3.183] - 2026-10-23

Gate hardening + policy centre + semantic response cache (参考的结果计划指南.md 批次 B-1/B-2/B-3/B-4/B-5/B-6).

### Fixed
- **i18n coverage gate was false-green** (`web/scripts/check-missing-i18n.cjs` only
  checked "keys used in source exist in en.json"; it never compared locales against
  en.json). fr/ja/ru/vi were **95 keys short** and zh-TW **66** — all of them on the
  training-data export panel — so those users saw English via `fallbackLng: 'en'`.
  Added `web/scripts/check-i18n-coverage.cjs` (per-locale coverage **and**
  `{{placeholder}}` parity), filled the 446 missing translations, and wired both
  i18n checks plus the new token-drift gate into `.github/workflows/ci.yml`.
- **Interpolation placeholders lost in ja/vi**: `Failed to parse JSON file: {{name}}`,
  `Model pull failed: {{msg}}`, `Parsed {{count}} service account file(s)` rendered a
  literal `__ PH_0 __` to Japanese users, and the Vietnamese
  `Minimum LinuxDO trust level required` invented a `{{LinuxDO}}` placeholder.

### Added
- **Design-token snapshot + drift gate** (`web/scripts/ds-tokens-{lib,export,check}.mjs`,
  `web/.ai/ds/ds-tokens.json`): 34 blocks / 659 tokens extracted deterministically from
  `theme.css` + `theme-presets.css`. `bun run ds:tokens` regenerates; `bun run
  ds:tokens:check` exits 1 on drift, so the parallel visual work (V1–V13) can no longer
  change a token without a reviewable snapshot diff.
- **Built-in billing reachability guard** (`setting/billing_setting/builtin_billing_test.go`):
  `InitRatioSettings()` seeds the *default* legacy tables into the same maps
  `GetBillingMode` consults, so a model present in both tables is silently billed by the
  legacy number and its built-in expression becomes dead code. The new test compiles
  every built-in expression and fails if a model is shadowed by the legacy defaults.
- **Per-model request budget** (`relay.policy_model_request_limits`): a request whose
  estimated cost exceeds its model's limit is blocked with `403 policy_blocked`
  (`rule=model_request_limit`) when the policy engine is in `enforce` mode. Pure
  input-driven (no counters), so it is fully unit-tested and has zero state.
- **Unified policy visibility**: sensitive-word blocks (which already existed and already
  blocked) are now counted in `policy_decision_total{decision="guardrail:block"}`, closing
  the gap where the policy centre could not see the guardrail that actually fires most.
- **Semantic response cache** (`service/semantic_cache.go`, `SEMANTIC_RESPONSE_CACHE`):
  when an exact cache miss occurs, a prompt close enough to a cached one reuses that
  response instead of calling the upstream. Three hard boundaries: **off by default**,
  **explicit model allowlist required**, **threshold 0.97 by default**. Per-user /
  per-model / per-format buckets, bounded memory (64 per bucket, 2048 total), hit
  counters exposed as `semantic_cache_{hits_total,misses_total,entries}` in `/metrics`.
  The built-in embedder is a deterministic bag-of-words hash (robust to case, whitespace
  and punctuation — **not** a semantic model); real embeddings would require an external
  paid service, which this change deliberately does not call.
- **Admin UI** (`request-limits/policy-center-section.tsx` + registry entry): the four
  values an administrator must be able to change (prompt character limit, per-model
  request budget, semantic allowlist, similarity threshold) are editable in
  System settings → Request limits, with `model=quota` / one-model-per-line text editors,
  inline validation and 8-language strings.

### Changed
- `PolicyInput` gained `ModelRequestLimit`; `evalBudget` reports `model_request_limit`
  alongside the existing `insufficient_quota` rule.
- `ResponseCacheServeHit` falls through to the semantic layer only after an exact miss, so
  exact-match behaviour is unchanged when the new switch is off.

### Verification
- Real end-to-end run against the built binary + a mock OpenAI-compatible upstream
  (15/15 cases): baseline 200 through the real relay chain, `403 policy_blocked` on the
  per-model budget, recovery after clearing it, sensitive-word block + policy counter
  increment, semantic cache off → 2 upstream calls, exact-cache baseline → 0 calls,
  semantic hit → 0 calls, unrelated prompt → 1 call (no wrong match), switch off →
  immediate rollback to upstream.
- `go vet ./service ./setting/... ./controller ./common` clean; `go build ./...` clean;
  `go test ./service ./setting/...` green; `bun run typecheck` clean;
  `check-i18n-coverage` / `check-missing-i18n` / `ds-tokens-check` / `check-terminology` green.

### Gates & error hygiene (independent batch, 2026-10-23)

- **Version-consistency gate** (`scripts/check-version-consistency.mjs`): compares `VERSION`,
  the first CHANGELOG heading, the newest git tag, `workflow_status.md` §0 and the
  `计划书/README.md` claim. It caught a real drift while being written (README still said
  v1.3.180 after v1.3.181 had shipped). `--self-test` covers 9 fabricated fact sets;
  `--require-tag` is the strict release mode. Wired into CI.
- **Terminology gate was false-green**: its exempt regex `/rate|limit|concurren/` matched the
  `limit` inside `Unlimited`, so `Unlimited Quota → 无限配额` was skipped and the gate printed
  PASS with 4 live violations. Now uses word boundaries; zh + zh-TW fixed to 「额度」.
- **39 raw `err.Error()` responses closed** (N15 follow-up): `ApiErrorMsg(c, err.Error())`
  bypassed the sanitizer in `common.ApiError`. Bare cases now call `ApiError`; prefixed cases use
  the new `ApiErrorMsgLogged` (user sees plain copy, details go to the server log under the same
  request id). 2 new tests in `common/gin_error_test.go`.
- **`window.confirm` gone from real code paths** (N4): the Waffo/Pancake section uses the shared
  `ConfirmDialog`; the 0-call `use-form-dirty-guard` hook was deleted.
- **Frontend suite runner** (`web/scripts/test-shards.mjs`): splits the 199 test files into
  balanced shards, prints one summary plus failure names. It also exposed a **test hang**
  (`plan-comparison.test.tsx`): the i18n mock returned a new `t` on every render, so a
  `useCallback([t, …])` + `useEffect([loadData])` pair looped forever — >6 min became 2.9 s.
  Two failures previously masked by that hang are now visible and still open.
- **Spec files generated, not hand-written**: `db_structure.md` (55 tables / 637 columns, from
  `scripts/gen-db-structure.mjs` against a real AutoMigrate run), `project_specs.md`, and
  `.agent/memory/*` with explicit authority declarations.

## [v1.3.182] - 2026-10-22

Visual identity batch 1 — "quiet instrument" token layer + dashboard pilot
(计划书/视觉识别体系-反AI通用美学界面改造指南.md, V0/V1/V2/V3).

### Added
- **Design tokens** (`web/src/styles/theme.css`): layered surfaces
  (`--surface-1/2/3`), hairline rules (`--rule`, `--rule-strong`), empty-state
  grid (`--grid-line`, `--grid-step`), quiet accent (`--accent-quiet*`), motion
  durations (`--motion-fast/base/slow`) and one high-value easing
  (`--ease-quiet`). Every value derives from `--foreground` / `--background` /
  `--primary` via `color-mix(in oklch, …)`, so all 10 colour presets and both
  themes get correct surfaces without per-preset overrides.
- **Numeric face**: `@fontsource-variable/jetbrains-mono` (self-hosted, no CDN)
  exposed as `--font-numeric` → `font-numeric`. Data surfaces (额度/TOKEN/金额/ID)
  render tabular monospace; CJK body text is untouched (still Public Sans).
- **`Card` tones** (`components/ui/card.tsx`): `tone?: 'default' | 'panel' |
  'plain' | 'inset'`. `default` is byte-identical to before, so all 42 existing
  call sites are unaffected. The new tones express hierarchy with surface
  lightness + hairline rules instead of shadow stacking.
- **`Metric` primitives** (`components/metric.tsx`): `Metric`, `MetricLabel`,
  `MetricValue` — one implementation for readouts (11px muted micro-label,
  monospace tabular value, separated unit, single `—` placeholder for empty
  values). Covered by `components/__tests__/metric.test.tsx` (5 cases).

### Changed
- `--radius` 1rem → 0.625rem (visual blueprint D2): controls land at ~10px,
  containers at ~14px, pills stay `rounded-full` — the uniform "template" radius
  is gone. The 7-step radius ladder and the appearance drawer's radius axis still
  work unchanged.
- Dashboard pilot (`features/dashboard`): `StatCard` gained `variant='strip'`
  (no pastel icon chip, single-colour sparkline), `PanelWrapper` and the overview
  containers moved from `rounded-2xl + shadow-xs + bg-card` to
  `rounded-xl + border-rule + bg-surface-1/2`, the credit-remaining panel's
  three-stop rainbow gradient became `bg-surface-2`, and the models stat strip
  now uses `Metric`/`MetricValue`. `rounded-2xl` count on the dashboard pages is
  now **0** (was 6).

### Removed
- `web/src/config/fonts.ts` and `web/src/context/font-provider.tsx` (D5). The
  provider only added a `font-{inter,manrope,system}` class to `<html>` that no
  stylesheet defined and that `body` overrode anyway; the real font axis lives in
  `lib/theme-customization.ts` (`default|sans|serif`). Dead config with two
  competing sources of truth removed; `main.tsx` unwrapped accordingly.

### Verified
- `bun run typecheck` PASS · `bunx oxlint` on all touched files clean ·
  `bun run build` PASS (font adds 40.4 kB latin woff2, 84 kB all subsets).
- `vitest run src/features/dashboard src/components/ui` 54/54 PASS;
  `vitest run src/components/__tests__/metric.test.tsx` 5/5 PASS.
- Full suite: 1 failure — `usage-logs/.../task-event-stream.test.tsx`
  (documented known noise in `test_ledger.md:286`, passes 6/6 in isolation).
- Real browser E2E (Playwright, local backend + dev server, admin login):
  10 screenshots (3 pages × 1440/768/320 + dark), axe WCAG 2.1 A/AA
  **0 violations**, 1 benign console 401 before login.
- In-browser token probe: `--radius` = `.625rem`, `--surface-1`/`--rule` resolve,
  metric values render `JetBrains Mono Variable` + `tabular-nums`,
  `rounded-2xl` = 0 on dashboard, body font still `Public Sans`.
- `check-missing-i18n.cjs` 0 missing · `check-terminology.mjs` PASS ·
  `format-with-protected-headers.mjs --check` clean.
- Knip gate: 117 "new" dead-code entries are pre-existing baseline drift
  (none reference the files in this change); documented, not masked.

## [v1.3.181] - 2026-10-21

### Fixed (N15, first slice)
- `common.ApiError` no longer returns raw driver/runtime text to the client. When
  the error message looks like an implementation detail (`sql:`, `gorm:`,
  `record not found`, `UNIQUE constraint failed`, `json: cannot unmarshal`,
  `context deadline exceeded`, …) the response becomes:

  ```json
  { "success": false, "code": "internal_error", "request_id": "<id>",
    "message": "The request could not be completed. … request ID: <id>" }
  ```

  The original error goes to the server log with the same request id.
  User-facing messages ("额度不足，请充值") pass through unchanged.
- The frontend maps `code: "internal_error"` to a localised retry message, so a
  Chinese UI never shows a SQL string.

### Verified
- `go build ./...` PASS, `go vet ./common ./controller` clean.
- `go test ./common/` PASS (incl. new sanitizer tests: case-insensitive hints,
  sanitised path, pass-through path).
- `go test ./controller/ -run 'TestRegister|TestLogin|TestOAuth'` PASS.
- Frontend: typecheck/build PASS, `server-error-code.test.ts` 2/2 PASS,
  i18n check 0 missing.
- Production v1.3.181 healthy, 0 5xx, public smoke 8/8 PASS.
- Limitation: the sanitised branch only fires on an actual internal failure, so it
  was not observed against production traffic this round; it is covered by the Go
  unit tests and the pre-existing call sites are unchanged.

## [v1.3.180] - 2026-10-21

### Added
- `ConfirmDialog` accepts `requireTypedText`. When set it renders an input and
  keeps the confirm button disabled until the user types the exact string — the
  L3 level of the destructive-action contract in `web/AGENTS.md §3.17`.
- Pricing settings use it: saving after changing `QuotaPerUnit`, `USDExchangeRate`,
  the display currency, or the custom exchange rate now opens
  "Change pricing conversion?" and requires typing `QUOTA`. Those fields change
  what every user sees as their balance and price, so a single mis-click is not
  enough anymore.
- Regression test `src/components/__tests__/confirm-dialog.test.tsx` locks both the
  typed-confirmation behaviour and the unchanged behaviour when the prop is absent.

### Verified
- typecheck / build PASS; system-settings 57 cases PASS; ConfirmDialog 2/2 PASS.
- Production: `v1.3.180` healthy, 0 5xx.
- Limitation: the pricing screen on the live instance does not render
  "Quota Per Unit" (display currency is CNY), so the end-to-end dialog was not
  captured in the browser this round — the contract is covered by the component
  test above.

## [v1.3.178] - 2026-10-21

### Accessibility (N11 — axe WCAG 2.1 A/AA on production)
- Axe audit across 8 production routes (/, /pricing, /status, /sign-in,
  /dashboard/models, /system-settings/site/system-info, /wallet, /keys) now reports
  **zero critical/serious violations**. Four categories were fixed:
  - contrast (10 → 0): social-proof labels, the status description, pricing metric
    values and the pricing-type badge sat between 2.2:1 and 3.9:1. Text now uses
    full-opacity muted colour, emerald/cyan-700 instead of 600, and three new
    text-only tokens (`--success-text`, `--chart-4-text`, `--warning-text`).
  - definition-list (6 → 0): the pricing performance badge put `role="img"` on a
    `<dd>`; the role moved to an inner `<span>`.
  - label (1 → 0): the wallet referral-link input had no accessible name.
  - button-name (1 → 0): the table page-size select had no accessible name.
- Added `web/e2e-local/prod-axe-audit.cjs` so the same audit can be re-run after any
  UI change (runs against production through the 443 tunnel).

## [v1.3.174] - 2026-10-21

### Fixed
- **The channel editor drawer no longer discards edits silently.** It is a
  5000-line form; pressing Esc or clicking Cancel used to close it and reset the
  form with no warning. It now checks `form.formState.isDirty` and asks
  "未保存的更改 / 取消 / 离开" through the shared `ConfirmDialog` before closing.
  Verified in production with a real admin: open the drawer, edit the channel
  name, press Cancel → confirm dialog appears and the drawer stays open.

### Notes
- N7 (empty-state actions) and N3 (typed confirmation for billing changes) are
  still open; both need per-page callbacks / new copy and are tracked in the
  Nielsen backlog rather than half-implemented here.

## [v1.3.173] - 2026-10-21

### Added
- Command palette now has a **Quick actions** group (API keys / Wallet / Usage logs /
  Status) next to navigation and theme, so the most common destinations are one
  Ctrl/⌘+K away. Verified in production: palette opens from the keyboard and the
  group contains all four actions.
- `web/AGENTS.md` gains **§3.17 Usability contract** — the enforceable subset of
  Nielsen's heuristics: four-state matrix, submit contract, three levels of
  destructive actions, three-layer error copy, terminology, consistency and
  accessibility rules.

### Fixed
- 24 Zod validation messages (e.g. "Username is required") were rendered in
  English inside a Chinese UI. `FormMessage` already routes messages through
  `t()`, so the keys simply did not exist; they are now translated for zh /
  zh-TW and registered in en.json.

### Verified
- `bun run i18n:terminology` PASS, `check-missing-i18n` 0 missing.
- 20 component/user test files, 99 cases PASS.
- Production: version header `v1.3.173`, 0 5xx in the last 400 log lines.

## [v1.3.172] - 2026-10-21

### Changed
- All error screens (401 / 403 / 404 / 503) lead with a human sentence instead of
  a 7rem HTTP code; the code is now a small chip and no longer the main visual.
- The generic error screen (500 / 429) shows a copyable **Request ID** plus a
  diagnostic payload (status, request id, URL, timestamp, message) so users can
  report a failure without guessing which request broke.

### Fixed
- `Reset all check-in records` and `Delete Channels` no longer use
  `window.confirm` / a hand-rolled dialog footer. Both go through the project
  `ConfirmDialog` (destructive styling, loading state, disabled while pending).

### i18n
- Every `quota` string in zh / zh-TW now says 额度 / 額度 instead of mixing
  配额 / 容量 / 配額. New gate: `bun run i18n:terminology`.

### Decisions recorded (no code change)
- `计划书/ops/DECISION-claude-billing.md` — Claude cache-token billing stays as
  is until a production impact query and a shadow comparison exist.
- Test gate: CI is the authoritative full test gate. The local equivalent was a
  hand-run shard matrix (≈880 cases measured in 5 batches); it has since been
  superseded by `cd web && bun run test:shards` (T-A1), and the matrix itself is
  archived under `docs/archive/plans/20261023/`.

## [v1.3.170] - 2026-10-21

### Added
- **Unsaved-changes protection now covers every settings page.** The dirty state
  of each `useSettingsForm` instance is registered with `SettingsPageProvider`,
  which renders a single `FormNavigationGuard`. Previously only 4 of the 42
  settings sections remembered to mount the guard, so switching sections after
  editing silently discarded the changes.
  Verified against production: an untouched page navigates straight through; an
  edited page shows "未保存的更改 / 留下来 / 离开" and stays on the page.

### Changed
- Removed the four per-section `FormNavigationGuard` instances so there is only
  one router blocker on the page (two would fight over the same navigation).

### Fixed (test infrastructure)
- `prod-smoke` treated `#root` having children as "rendered", but the boot
  loader also lives inside `#root`; it now waits for the loader to detach and
  for an `h1` to have text. The old check could report a false "blank page" on
  a cold load.

## [v1.3.169] - 2026-10-21

### Fixed
- Four long-standing flaky frontend tests: jsdom has no `ReadableStream`
  (now polyfilled from `node:stream/web`) and one spec switched the shared
  i18next instance to zh without restoring it (`afterEach` now resets to `en`).
  Both previously failing files pass twice in a row (15/15).
- `TabsList` no longer overlaps at 320px: the OAuth section uses
  `grid-cols-3 sm:grid-cols-6`, the ratio card adds `max-sm:grid-cols-3`.
  Verified in production at 320×640 (`tabsOverflow = 0`, no page overflow).
- Uptime status dots expose `role="img"` with an accessible label instead of
  encoding state with colour alone.
- `theme-color` now comes from a `--theme-color` design token and updates every
  meta tag; the old code wrote a hard-coded `#020817` into the first tag only.

### Changed
- Dropped the `vendor-react` cache group from `rsbuild.config.ts`; it never
  applied (Rsbuild's built-in `lib-react` group wins), so the config now matches
  the actual output.

### Known issues (pre-existing)
- Settings sections use 42 separate `SettingsPageFormActions` call sites and only
  a handful register `FormNavigationGuard`; unifying that is a framework-level
  change tracked separately.

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
