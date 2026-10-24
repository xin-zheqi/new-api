# Relay performance and encrypted-history verification — 2026-10-24

This release reduces redundant work in the relay without changing protocol,
retry, history-repair or billing policy. No frontend, database schema, driver,
database query or Redis behavior is changed by this optimization.

## Changes

| Path | Change | Preserved behavior |
| --- | --- | --- |
| Empty sensitive-word dictionary | Skip prompt extraction used only for a dictionary that cannot match anything | Token counting still extracts required metadata when enabled; configured words still apply immediately |
| Request billing metadata | Preserve Gemini output budget on the lightweight metadata path | Chat, Responses, Claude and Gemini reserve the same quota with an empty/populated dictionary |
| Content audit miss | Skip the second full JSON decode used only for match-record metadata | Rule inspection, block/replace/warn actions, audit records and policy lookup remain unchanged |
| Response model normalization | Avoid copying the entire event just to read known model paths; skip path lookups for simple deltas with no model key | Validate JSON; preserve escaped keys, upstream model diagnostics, ciphertext, tools and usage |
| Disabled request fields | Probe only disabled fields and stop at the first match | Existing field-removal behavior remains unchanged |
| Parameter override reasoning metadata | Read the original body only when the final reasoning field is absent | Explicit values, deletion and derived reasoning effort retain existing semantics |

## Findings about the remaining chain

- Model mapping and an empty parameter-override configuration already have cheap
  paths. Native OpenAI Responses requests do not go through a Chat conversion.
- Reasoning and tool-history repair remain opt-in. Disabled request repair returns
  before parsing; disabled streaming reasoning repair uses a nil collector. Enabled
  repair scans are bounded. Request copies needed to isolate retry attempts remain.
- Empty/disabled content-audit rules already skip body inspection. When global
  auditing is enabled, the user policy lookup still occurs. Removing that lookup
  without distributed invalidation could serve stale security policy, so it was
  not changed. Compiled regex rules are cached already.
- Token counting, active audit inspection, actual protocol conversion and large
  parameter overrides still cost CPU in proportion to request size. This release
  does not silently disable them.
- Empty-response checking intentionally buffers initial events until it can
  distinguish useful output from a retryable early failure. Removing that buffer
  would compromise retry safety. Upstream waiting, network transport and downstream
  backpressure remain separate from local processing cost.
- The existing relay timing starts after initial channel selection. It is not a
  complete measurement of authentication, rate limiting and routing latency.

## Local benchmarks

Linux/amd64 Docker, Go 1.26.10, i7-13700K. Fixed fixtures, three samples; rounded
medians. Audit before/after was measured sequentially in the same container with
`GIN_MODE=release`. These are local function measurements, not production latency
or overall request speedup. Allocation figures refer to the named function only.

| Fixture | Before | After | Allocation change |
| --- | ---: | ---: | --- |
| Small Responses delta without model metadata | 533 ns | 61 ns | 80 B → 0 B |
| 45 KB escaped tool delta, model normalization | 98.4 µs | 88.6 µs | 49,152 B → 0 B |
| 82 KB response with already-matching model | 117.6 µs | 104.6 µs | 90,112 B → 16 B |
| 278 KB request, disabled fields absent | 225.7 µs | 183.0 µs | 480 B → 0 B |
| 278 KB request, all controlled fields allowed | 222.2 µs | 7.9 ns | 480 B → 0 B |
| 278 KB request, active audit rule misses | 6.345 ms | 5.608 ms | No material allocation claim |

The largest reduction is unnecessary scanning/allocation, especially across many
stream events. Saving local microseconds or milliseconds does not eliminate
seconds spent waiting for an upstream model.

## Encrypted-content investigation

Reported error: `The encrypted content for item amsg_... could not be verified.
Reason: encrypted content hydration failed: Encrypted content could not be
decrypted or parsed.` This is an opaque-history validation/decryption error, not
the DeepSeek plaintext `reasoning_text` requirement.

Code inspection and local regression checks did not find ciphertext modification
in the native OpenAI Responses path with reasoning repair disabled:

- `RepairReasoningRequest` returns before parsing when disabled; channel metadata
  initialization also clears state between attempts.
- Native request `input` is retained as `json.RawMessage`. The OpenAI adaptor
  normalizes request-level reasoning settings, not encrypted input items.
- Response model normalization patches known envelope fields, not output items.
- Content audit skips reasoning/compaction bodies and does not inspect the
  `encrypted_content` field as plaintext.
- Tests exercise mapping, parameter overrides, disabled-field removal, repair
  enabled/disabled, retry preparation, passthrough, streaming, non-streaming and
  compaction followed by history replay. Ciphertext values remain byte-identical;
  output item IDs and other item fields are retained.
- A synthetic upstream hydration error is returned as a failure, without deleting
  encrypted history or introducing extra upstream attempts/consume records.

This does **not** prove actual OpenAI decryption succeeds for the production
history. The container uses synthetic ciphertext and a synthetic upstream. No
real failing payload or provider response was available. Upstream provenance,
actual model/routing changes, intermediary conversions, stale or incomplete
history and provider-side behavior remain possibilities, not confirmed causes.
No inference about the provider's exact key/account/model cryptographic binding
is made here. No encryption-handling production change was made in this release.

OpenAI's [reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)
describes retaining output reasoning items and replaying relevant history intact.
For a future incident, correlate the failing request with its preceding successful
turn: request ID, actual channel/upstream/model, item ID, ciphertext byte length and
SHA-256 at each boundary. Compare these without logging ciphertext or dialogue.
Matching hashes rule out byte changes at those boundaries, but cannot establish
provider-side validity.

## Verification

Passed:

```text
go test ./relay/... ./service ./setting -count=1
go test ./relay ./relay/channel/openai -count=1
go test ./pkg/contentaudit -count=1
go test ./controller -run 'Test(ShouldRetry|IsClientCanceledRelayError|WriteRelayErrorResponse|ResponsesStreamOutcomesPreserveAccounting|ResponsesInterruptedStreamHealth|ContentAuditResponsesWebSocketPerTurnBeforeBilling|ResponsesHTTPHealthCountsFinalResult|ResponsesWebSocketReusesConnectionAndSettlesEachRequest)' -count=1 -timeout=150s
```

The second relay run includes the newly added encrypted-history tests. The
controller run used `TMPDIR=/dev/shm` and `GOTMPDIR=/tmp`; it passed in 65.834 s.
An earlier full controller run was manually interrupted while inspecting slow
SQLite migration work; it is not reported as a passing full-controller suite.
This release does not change production database behavior and does not claim a
new SQLite/MySQL/PostgreSQL compatibility certification.

All 79 release-container checks passed (16 encrypted-history, 26 tool-history,
27 model-normalization/logging and 10 search/protocol checks). They use an isolated SQLite instance and local
synthetic upstreams. They cover original usage/consume logs, failed-request logs,
model normalization across native/converted protocols, search forwarding,
multi-round duplicate tool IDs and encrypted history. Both embedded frontend
builds are reused from the verified homepage-footer release; this release changes
no UI. Evidence is under `.local-tests/performance-20261024/`.

## Image

Release tag: `thefoolishfox1/new-api:relay-performance-20261024` (linux/amd64).
Published and verified: both the release tag and `thefoolishfox1/new-api:latest`
resolve to `sha256:43cc3ee92c6fe11efb530d72cd19526a06434b80aee70421d5e2679472562952`.
Remote config digest matches the tested image's saved config:
`sha256:f55b3618fe2cb7e9cc046ae7a479b1d44c8e1d0044312fe0446e494c923d4e86`.

Rollback: `thefoolishfox1/new-api:home-footer-20261023`, previously published as
`sha256:a59ace98c8305d437b036c4c2add5839581f446aab1e52570df93310361eb569`.
Publication records are in `.local-tests/performance-20261024/release.json`.
