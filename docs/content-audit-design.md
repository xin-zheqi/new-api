# Content audit and usage export

## Scope and privacy boundary

Content audit is an opt-in, per-user inspection performed after authentication and before billing or upstream forwarding. The site-wide switch defaults off. It cannot prevent the incoming request from reaching the gateway; users must redact secrets locally before transmission. Application authorization isolates rules and hits by the authenticated user. Server operators and database/backups are outside this application-level isolation guarantee. No remote regex service, telemetry, or third-party content analysis is used.

## Interaction

Both frontends provide Rules and Hit records tabs. Rules have a name, enabled state, RE2 pattern, case/multiline options, action (warn, replace, block), and replacement template. Warning and blocking stop after the first match in each text field; replacement rules may choose first or all matches. Rules run in listed order against original text; blocking takes precedence over replacement. A test panel uses synthetic text and explicitly warns against entering real secrets. Presets are suggestions, not a guarantee of detecting every secret.

Hit lists expose timestamp, request ID, model, token name, endpoint, action and hit count. Expanded details show rule names, text locations, matched snippets and replacement snippets. Long records are bounded and visibly marked truncated; truncation of display data must never truncate enforcement. Users can delete their own records. Retention uses hard deletion, including while the feature is disabled; database free pages and administrator backups require separate operational retention.

## Runtime constraints

Inspect only protocol-defined text leaves; never match serialized JSON or modify structural keys, model identifiers, credentials, tool schemas, URLs or binary payloads. Replacement serializes changed string values correctly and must survive passthrough and channel retries. Audit runs once per HTTP request and once per Responses WebSocket create event before charging. Limits on rule size/count, inspected text, expanded output and record size bound per-request resource use; record retention bounds storage lifetime without an early count-based eviction. Go RE2 avoids catastrophic backtracking. Disabled mode returns immediately without policy queries or regex work.

| Bound | Value |
| --- | --- |
| Rules per user | 20; compiled policies cached for at most 64 users/policy versions |
| Pattern / replacement | 512 bytes each, at most 32 capture groups and 1,024 compiled regex instructions |
| Request / inspected text / expanded text | 8 MiB / 1 MiB / 2 MiB |
| JSON structural tokens / traversal depth | 65,536 / 32, including encoded tool arguments |
| Matches / retained details | 10,000 / first 48; each original/replacement snippet limited to 160 bytes |
| Records | No count-based eviction; retention cleanup is the only lifetime policy |
| Retention | 1–90 days (default 7), expiration hidden immediately, physical DELETE every 10 minutes |
| Preview | 8,192 bytes of synthetic text, never persisted as a hit |

All rules inspect original text. Block dominates; overlapping replacements use the first configured rule. “First match” means once per text field, not once per request. Unknown protocol extensions, images, audio, URLs and file bytes are deliberately not scanned. Multipart inspection is limited to the documented `input` and `prompt` text parts and preserves file contents. Realtime audio cannot be audited: an active user policy rejects it before upgrade; disabling the user policy restores existing behavior. Responses WebSocket is supported per normalized turn.

Audit failures are fail-closed only for users with active rules: block returns 403 before billing; resource limits return 413; unavailable private storage returns 503 rather than forwarding unrecorded sensitive content. None of these errors retries another channel. HTTP warnings expose only action/count in `X-Content-Audit-Action` and `X-Content-Audit-Matches`; successful provider response bodies are unchanged. WebSocket warnings are visible in the private audit records, not by injecting nonstandard provider events.

Rules and hit details use AES-256-GCM with owner/purpose-bound authenticated data. Configure a persistent `CRYPTO_SECRET` or `SESSION_SECRET` in the server environment before enabling the feature; the key is never accepted from or returned to the browser and is not stored in the options table. Keep the effective key consistent across replicas and restarts. Rotating it without migrating ciphertext makes saved policies unreadable. Searchable hit metadata remains in the main database, while each encrypted match-detail payload is stored as a `0600` file under `CONTENT_AUDIT_DIR/records/<user-id>/<record-id>.enc` (the default directory is `./content-audit`, which is `/data/content-audit` in the Docker image). List/detail/delete operations always use the authenticated owner, without an administrator override. Clearing records hard-deletes the database index and unlinks the detail file; the file's disk blocks are released immediately, while database free pages, WAL/binlogs and backups remain an operator retention concern. Mount `CONTENT_AUDIT_DIR` on persistent shared storage for multi-instance deployments.

Legacy rows that still contain an encrypted detail column are read in place and are migrated to files at startup in bounded, retryable batches. Slot replacement explicitly updates `created_at`; GORM's default conflict-update behavior would otherwise retain an expired slot's old timestamp. Both cases are covered by real-database regressions.

Both frontends hide navigation when the site status disables audit; backend audit APIs independently return 404. Already-open browser views follow the existing site-status refresh lifecycle. Private React Query entries have zero garbage-collection retention after unmount and are not written to browser storage.

## Other changes

- DeepSeek per-channel opt-in: Responses to Anthropic Messages, preserving native forwarding when disabled.
- Group status: display success rate without the successful-request count.
- Usage export: XLSX grouped by token ID and model, merged centered token-name cells, current time/name filters, numeric usage/cost columns, no key material and no server-side temporary workbook.

Export uses the configured consumption-log database, including a separately configured log database. It limits exports to 10,000 keys/rows, two concurrent exports and a 30-second database context. Unknown names are escaped inline strings, never workbook formulas or external links. Timestamps are labeled UTC; amounts are USD with eight decimal places. Counts follow the existing consumption-log token accounting convention. Deleted keys are excluded consistently with the usage page; keys without usage remain visible as zero-usage rows. Browser object URLs are revoked after download.

The DeepSeek switch is off by default, applies to type-43 client Responses requests and uses `/anthropic/v1/messages`. It preserves the native `/responses` path when disabled, body passthrough and existing Chat/Claude-to-Responses policy bridges. Streaming responses and cached-token usage use the existing Messages conversion/accounting pipeline.

## Verification

Validation date: 2026-09-30. This is a release candidate, not a zero-defect or production-release guarantee.

- Passed the related Go package suite: `go test -p 2 ./common ./pkg/contentaudit ./service ./model ./relay/... ./controller ./middleware ./router -count=1`.
- After the final engine changes, passed `go test -p 2 ./pkg/contentaudit ./service ./controller -run 'ContentAudit|Audit|TokenUsage' -count=1`. This includes regression tests first reproduced failing for JSON-like plain tool output and encoded JSON bypassing structural limits.
- Real WebSocket integration `TestContentAuditResponsesWebSocketPerTurnBeforeBilling`: first-turn replacement reaches upstream; changing the next turn to block returns 403, never forwards it and never adds a second charge.
- Real database matrix `go test -v ./model -run TestContentAuditDatabaseMatrix -count=1`, with both `TEST_MYSQL_DSN` and `TEST_POSTGRES_DSN` configured: SQLite **3.50.4**, MySQL **8.4.11**, PostgreSQL **16.15**. Fresh audit tables, repeated migrations, preserved legacy token, owner isolation, concurrent inserts, expiry/hard deletion and monotonic uncapped slots passed. Exports covered separate SQLite log DBs and same-engine log DBs. Minimum supported MySQL/PostgreSQL versions were not separately exercised; the new queries do not introduce version-specific features.
- Actual old-image upgrades and repeated restarts passed on all three engines. Baseline was the locally available published `thefoolishfox1/newapi:latest`, image `sha256:0ae7cd9e0169af281d70ec3e918fd56be037b35f93115128d8295ff119aeca5d`; this does not assert that it is the newest remote tag. `scripts/test-content-audit-runtime.mjs` seeds legacy user/token/quota/options, then checks preservation, encrypted rules, preview, non-persistence of preview hits, XLSX download and disabled 404 behavior. It targets only explicitly named disposable test containers.
- Passed `cd relaykit && GOWORK=off go build ./...` independently.
- New frontend typecheck and all nine new Vitest cases passed: `bun run typecheck` and `bun run test src/features/content-audit/__tests__ src/features/channels/lib/__tests__/deepseek-conversion.test.ts src/lib/__tests__/download-workbook.test.ts`. Both frontends passed scoped lint; newly introduced literal translation keys were checked in all seven languages. Classic also includes its `zh-CN` locale.
- Workbook tests parse every OOXML part and verify merges, filters, time range, monetary precision, untrusted strings and integer overflow. Actual Excel/LibreOffice rendering has not been verified.

- Final Docker candidate `newapi-content-audit:validation`, image `sha256:33bb75aa85b3e25f08f6d21a66666b0ef1131d4c110df34662f0f383beb4c085`, built with `docker build --ulimit core=0:0 --progress plain -t newapi-content-audit:validation .`. The candidate passed the runtime script on all three upgraded databases, then passed again after `docker restart`. Earlier rebuilds hit a Tailwind/Bun native extractor panic; reinstallation of the build dependencies and a rebuild succeeded. No application dependency versions or Dockerfile were changed to conceal this error.

- Final full-package rerun: `pkg/contentaudit`, `service` and `model` passed. The controller run concurrent with Docker builds failed; its voluminous output was truncated before the exact failing assertion was retained. An isolated complete rerun, `go test -json -failfast ./controller -count=1`, passed in **117.968 s**. The initial failure's root cause is not established; do not describe all runs as failure-free.

Outstanding release gate: visual browser acceptance for both themes (the isolated account's first-login identity selection needs explicit user approval after a safety-tool rejection). DeepSeek tests use deterministic protocol/SSE fixtures, not a live provider key. No production image has been pushed, and no Git commit/merge has been made for this change set. Temporary translation/check scripts were removed; the intentionally reusable runtime integration script remains.
