# Parsed configuration cache and prompt-cache forwarding — 2026-10-24

This follow-up to `relay-performance-20261024` changes three production files.
It does not change database queries/schema/drivers, Redis behavior, authorization,
billing calculations, retry decisions, or history-repair policy. Both embedded
frontends are reused unchanged from the previous verified release.

## Changes

1. `relay/helper/model_mapped.go`: retain successfully parsed model mappings in a
   bounded, process-local LRU using the existing `samber/hot` dependency. Its key
   is the exact configuration string, not channel ID; edits and retries therefore
   cannot select stale contents after receiving a different configuration. Keep
   the existing per-request cache. Store at most 128 configurations, each no more
   than 64 KiB of source JSON; larger configurations still use the original parse
   path. Shared maps are read-only. No request/history content enters this cache.
2. `relaykit/relayconvert/internal/oai_chat/to_oai_responses_req.go`: preserve the
   client's `prompt_cache_retention` when converting Chat to Responses. Previously
   this field was omitted while `prompt_cache_key` survived. Absent values remain
   absent; explicit values are forwarded unchanged. The regression test failed
   before the fix for both `in_memory` and `24h`.
3. `relay/channel/openai/chat_via_responses.go`: keep the original client model
   when upstream Responses metadata updates the streaming conversion state.
   Container comparison exposed an existing omission: Chat streams returned the
   upstream model on answer chunks and the client model on the final usage chunk.
   Normalize the already-decoded metadata before conversion, without another JSON
   parse or per-delta JSON rewrite. The scanner retains the upstream model for
   admin diagnostics first. Routing, token estimation, answer text and usage keep
   their original sources. Chat and Claude regression cases failed before this
   fix; the fixed test also covers the Gemini conversion output and cached usage.

## Upstream prompt-cache findings

The omitted retention field is a confirmed forwarding defect, not proof of the
production cache-hit decline. It matters only when Chat is converted to Responses
and the client supplied that field. Native `/v1/responses` retains
`prompt_cache_key`, `prompt_cache_retention`, `prompt_cache_options`, raw `input`
and raw `tools` in its request DTO. A downstream Sub2API conversion performed
before New API receives the request is outside this fix's scope.

The official [prompt caching guide](https://developers.openai.com/api/docs/guides/prompt-caching)
documents model-dependent caching policies. Omitting a requested retention value
leaves the provider's default policy in effect; it does not establish a universal
shorter retention period. This release does not inject retention or change cache
policy for requests that did not ask for it.

Other possible causes that require production evidence:

- Changing actual upstream/provider/model, or a provider-defined cache namespace,
  can change reuse opportunities. Identical public aliases are insufficient to
  establish identical provider cache context.
- Changing the system prompt, tools, earlier conversation text, or compacting
  history changes the reusable prefix. Protocol conversion is not guaranteed to
  produce the same provider prompt as a native request.
- Configured parameter overrides and audit replacements can change the forwarded
  prompt or cache fields. Disabled/empty audit rules do not rewrite content.
- Enabled reasoning/tool repairs can add missing history or rename duplicate tool
  identities. Existing valid history is preserved, and duplicate-call repair IDs
  are deterministic for identical history. Still, changing repair settings or
  switching channels with different history requirements can change the prefix.
  Reasoning repair disabled returns before parsing. When enabled, it may add
  `summary: []` to a bare encrypted reasoning item, without modifying the ciphertext.
- Response-side model normalization runs after upstream generation and cannot
  retroactively change that request's upstream cache lookup.
- A stream ending before usage arrives may lack reported cached-token counts.
  A low/zero cached-token log in that situation does not prove a real cache miss.

No generic native-Responses history truncation was found in the inspected path.
Synthetic repeated native requests produced byte-identical upstream bodies.
This is not a real-provider cache benchmark or an OpenAI decryption test. The
previous encrypted-history findings remain applicable; no encryption handling
was changed here.

## Configuration and database hot-path review

Global options, pricing ratios, retry policy and channel snapshots already have
memory caches; user/token metadata has Redis caching with database fallback.
Subscription plan metadata also uses a hybrid cache. Therefore, adding another
generic configuration cache is not automatically a useful improvement.

The following remaining candidates were inspected but deliberately left unchanged
to keep this release small and preserve accounting/security semantics:

| Existing work | Possible improvement | Required care |
| --- | --- | --- |
| Content audit reads user policy when globally enabled, even if that user's policy has no active rules | Versioned policy cache; compiled-rule cache currently holds 64 entries | Distributed invalidation and failure behavior must not delay enforcement |
| `subscription_first` checks active subscriptions with SQL COUNT per request | Existence query; later consider invalidated metadata cache | A stale negative result must not divert a subscription charge to the wallet |
| User cache reads user hash, then auth version/fence via another Redis command; later paths may read again | Reduce round trips | Preserve restrictive-update fences and fresh quota; do not reuse stale balances |
| Channel settings/overrides repeatedly parse JSON despite cached channel rows | Immutable parsed snapshots | Nested maps/pointers must not leak mutations across requests/retries |
| Sensitive-word matcher is cached but dictionary normalization/sorting/hash is repeated | Prepare dictionary snapshot when settings change | Updates must take effect correctly across instances |
| Periodic option refresh reapplies unchanged options under the option mutex | Change-aware refresh | Preserve side effects of settings updates |

Audit matches also perform durable encrypted-file/database recording. Making that
asynchronous would change audit guarantees. Redis pool and SQL pool settings need
production wait/RTT evidence before tuning; code defaults do not describe the
user's deployed configuration.

## Measurements and verification

Function benchmark, Linux/amd64, Go 1.26.10, same isolated container, distinct Gin
contexts, three samples. The medians below describe mapping-cache hits only:

| Mapping entries | Before | After | Bytes / allocations per operation |
| --- | ---: | ---: | --- |
| 1 | 1.136 µs | 0.591 µs | 2528 B / 17 → 1920 B / 8 |
| 200 | 60.862 µs | 0.674 µs | 40352 B / 625 → 1920 B / 8 |

These are not end-to-end latency improvements; upstream generation/network time
can dominate the request. Invalid mapping, correction, cycles, changes/removal,
large valid mappings, independent parallel aliases and retry-context behavior
are protected by deterministic tests.

Passed commands:

```text
go test ./relay/... ./service ./setting -count=1
CGO_ENABLED=1 go test -race ./relay/helper -run ModelMapping -count=1
go test ./relay/channel/openai -count=1
cd relaykit && GOWORK=off go build ./...
cd relaykit && GOWORK=off go test ./relayconvert/... -count=1
```

The complete relay/service/setting run covers the mapping/retention changes; the
OpenAI package was rerun after the additional converted-stream metadata fix.
The independent relaykit build does not rely on the root workspace. A first race
command accidentally matched no tests; the recorded `mapping-race.log` comes from
the corrected `ModelMapping` command. A build attempt encountered an internal Go
compiler EOF while using inherited cache artifacts. Building with a fresh GOCACHE
succeeded; no compiler error was suppressed.

All **108 final-runtime-container checks** passed, bound to the same image ID:

- 16 encrypted-history checks, including replay, compact, repair on/off, mapping,
  overrides and upstream hydration failures.
- 26 tool-history checks, including multiple rounds, duplicate IDs, protected
  history and original usage.
- 27 model-normalization/logging checks across native and converted protocols.
- 10 search/protocol checks.
- 29 cache/accounting checks, exercising 25 successful requests against each of
  the previous and final images. Native Chat, native Responses, converted Chat,
  streaming/nonstreaming, absent/explicit retention, native cache options, repeated
  bodies, and mapping edits/removal/restoration were covered.

For all 25 accounting requests, the synthetic upstream reported 4096 input tokens,
3072 cached input tokens and 32 output tokens. Each image recorded those exact
counts and one charge per request. With explicit fixture ratios, every charge was
1363 quota units and the total wallet debit was **34075**, identical between
releases and equal to the sum of consume logs. Cached usage was also returned to
the client correctly. This verifies the tested accounting contracts, not every
possible pricing configuration or real upstream's accounting.

Integration instances used isolated SQLite databases and synthetic upstreams.
There are no production DB changes in this follow-up, and it makes no new
SQLite/MySQL/PostgreSQL compatibility-certification claim. No real provider keys
or conversation history were used. Evidence: `.local-tests/config-cache-20261024/`.

## Image

Release tag: `thefoolishfox1/new-api:config-cache-20261024`, linux/amd64, Go 1.26.10.
Tested local image ID:
`sha256:a5e1687dd01236ef83e6cf26e60586dc6b0c102871d4db2b85277ce6cbf50074`.

Rollback image: `thefoolishfox1/new-api:relay-performance-20261024`, published
digest `sha256:43cc3ee92c6fe11efb530d72cd19526a06434b80aee70421d5e2679472562952`.
Published and verified: both `config-cache-20261024` and `latest` resolve to
`sha256:d2a9e9a1892164b97b369c68fc4156a663144d3403917510d74dd34fd0acb74f`.
The remote config digest matches the tested image's saved config:
`sha256:750c4279abb7d5056d2c18abb341b9c05481ade7b6fa9c20fcfcd0c9f4223754`.
Publication records and remote verification are stored in
`.local-tests/config-cache-20261024/release.json`.
