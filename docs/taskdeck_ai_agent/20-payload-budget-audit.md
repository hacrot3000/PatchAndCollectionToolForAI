# HANDOFF 20 — Final payload budget audit (CLP-05.01.02)

> Static inspection audit. Produced by native read-only COLLECT + source PATCH only.
> No Go toolchain, no provider, no real request was executed. Findings reference
> SYMBOL and TEST names (stable) rather than line numbers (which drift).

## Scope
- Package: `vscode_tasks_menu_go/internal/agentchat/contextcompact` (`compact.go`, `compact_test.go`, `cache_epoch*.go`).
- Task: CLP-05.01.02 — audit History/Next duplicate, schema overhead, initial pack, legacy fallback grouping.
- Related: CLP-05.01.01 (per-attempt byte/token instrumentation, DONE), CLP-05.01.03/.04 (final-wire budget validation, VERIFY_PENDING).

## 1. History/Next duplicate tool pairs
- `groups(history, synthetic, next)` builds an `awaiting` map from every `next` item whose type is `function_call_output`. A `function_call` in History whose CallID is in `awaiting` is emitted as a `pending` group with `keep:true` and does NOT consume a following History output (there is none); a completed History call/output pair is emitted as one group. This structure prevents a single logical call from being emitted twice across the History/Next boundary.
- `Build` reserves `nextTokens := groupTokens(p.Next)` in `overhead` before selecting retained groups, and the caller passes `Next` separately to `agentchat.BuildResponsesRequest` (see `TestChangingEvidenceCatalogDoesNotSplitStablePrefixOrCurrentReceipt`, `TestNewPatchReceiptOnlyOnceThenSummary`).
- `appendBeforePending(history, note, next)` inserts catalog/archive notes before the first pending call (or before the last item when a text-observation fallback pair spans History/Next), so a note never splits a live call from its incoming result.
- `FullHistory` tracks `outputs`/`pending`/`callCount`/`outputCount` and only summarizes a call/output pair that is corroborated by an exact ledger digest; a pending or ambiguous pair is preserved verbatim (`TestFullHistoryAcknowledgedFailedPatchSummaryIsLedgerBound`).
- Finding: no duplicate-pair defect found in the inspected code. Coverage is by existing regression tests; runtime re-verification belongs to V-BATCH-01.

## 2. Schema / instructions overhead
- `Build` computes `ceiling := p.MaxInputTokens * percent / 100` (safe percent 70, or an explicit lower `TargetInputPercent`), then `overhead := Tokens(p.Instructions) + nextTokens`, and `budget := ceiling - overhead` (floored at 0). `Tokens` is a deliberately conservative bytes/2+4 heuristic, not a provider tokenizer.
- `Policy.Minimal` drops the optional catalog entirely on technical overflow; `Instructions` is always counted, so a large embedded AI Pack cannot silently crowd out the mandatory receipt.
- Finding: overhead is accounted before group selection. Estimator error bounds and real 304->942 KB behaviour are runtime items (CLP-05.01.04 / V-BATCH-01).

## 3. Initial pack / small-history path
- With a small history, every `plain` user group is kept (first user goal plus up to 8 recent user instructions), `RecentFullCollections` retains the latest completed collections within `collectionBudget`, and the catalog is only built when there is something to summarize. A lone small collection stays a full tool pair (verified during this session while authoring CLP-05.02.01: a direct `collectionSummary` assertion is required rather than expecting a catalog entry from `Build`).
- Finding: initial-pack behaviour is conservative and lossless for small histories. No source change required.

## 4. Legacy fallback grouping
- `groups` detects `agentchat.CollectionTextObservation(history, i)` pairs and emits a `fallback` group with `plainSummary`; an unpaired legacy prefix item is handled conservatively. `epochAppendable`/`epochTextReceipt` in `cache_epoch.go` refuse to freeze fallback/envelope prefixes into a reusable cache epoch, so a legacy text fallback cannot be silently replayed as a current receipt.
- Coverage: `TestCurrentTextFallbackReceiptsDoNotConsumeHumanInstructionBudget`, `TestRecentCatalogRetainedAcrossRecompaction`, `TestFullHistoryEpochUsesActualItemLimit`, `TestHistoricalOrphanPatchIsNotReplayedButAuditRemains`.
- Finding: legacy fallback grouping is classified correctly and bounded; no implementation gap found.

## 5. Conclusion and limitations
- No implementation gap was found for CLP-05.01.02 by static inspection of `Build`, `groups`, `FullHistory`, `appendBeforePending`, `collectionSummary`, `plainSummary`, and their tests. This subtask therefore records an AUDIT (this document) plus the already-present regression source, and is marked VERIFY_PENDING — NOT DONE — because acceptance also requires runtime evidence.
- Runtime evidence still owed by the operator (see `20-external-verification-queue.md`, V-BATCH-01): synthetic 304 KB -> 942 KB overflow handling, no duplicate tool pair on the wire, mandatory receipt intact, and Responses/Chat Completions + stateful rebase matrices. Operator-supplied CI evidence applies only to source commit `88a909b4d564b9a1f1e3f35959bc2de55d85647d` and was not re-verified here.
