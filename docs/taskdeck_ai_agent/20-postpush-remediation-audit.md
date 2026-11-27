# HANDOFF 20 — Post-push remediation audit (P2/P4 and P5 boundary)

> Static audit from native read-only COLLECT after the remote review of HEAD 9e4ca30.
> No compile/test/git/CI executed. No gate promoted. P0/P1/P3 source changes are
> recorded in the root checkpoint; this document covers the audit-only groups.

## P4 — Authoritative mutation invalidation (CLP-02.04.01/.02)
Native PATCH receipt contract (`VerifyRunNativePatchEvidence`, native_run_receipt_adapter.go:58-76) authenticates: AgentRunID==RunID, name taskdeck_submit_patch, no text fallback, ArgumentsSHA256 (64 hex), PackageSHA256 (64 hex), patch_pass auto-check, and adopts a canonical envelope with Ref `run:<runID>/<callID>`. It does NOT carry a complete changed-files/rename/delete manifest. The `changed_files` occurrences in the workspace are Git-diff UI (`gitsemantics.go`), not native PATCH receipts.
Current adapter (`agent_run_collection_progress.go:73-78`) already applies broad invalidation on any intervening `taskdeck_submit_patch`: InvalidateEvidence(...,false) and InvalidateCoverage(...,nil,false), plus DuplicateStreak reset. This is the correct conservative behavior. Selective invalidation is intentionally NOT enabled because there is no authoritative completeness proof. Do not use the UI changed-files report as proof. No code change required; CLP-02.04.01/.02 remain VERIFY_PENDING on external native-receipt integration evidence.

## P2 — Unified bounded recovery (CLP-01.04.02/.04, CLP-04.03.03/.04, R25)
See 20-unified-recovery-audit.md. runCollectionProgressAdvice is advisory-only (`collectionProgressAdvisory(g.Decision, g.Counters, false)`) injected into provider instructions at agent_run_provider_executor.go:168. g.Decision derives from ApplyCounterEvent, not AssessRecovery. Enabling automatic bounded recovery requires the drain-only policy to change, which the task forbids. Documented precise limit; no code change; VERIFY_PENDING.

## P5 boundary — regression/metadata items
- R18 (adapter inspection COMPLETE): parser denial test `TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments` exists and is mapped. Production path inspection (agent_chat_response_helpers.go:146-190) confirms a native Patch intent is registered only when `toolAvailable && len(result.Calls)==1 && call.Name=="taskdeck_submit_patch" && parseAgentPatchArguments(call.Arguments) succeeds`. Malformed/unsupported/partial calls fall into the rejection branch (:167-190): they emit `tool_proposal_rejected` + UNSUPPORTED_TOOL/MULTIPLE_TOOL_CALLS feedback with no native execution intent, approval or queue path. A parser-denied partial streamed call therefore cannot reach native dispatch; no end-to-end fake-transport test is required to prove this static invariant. Existing adapter tests (agent_chat_rejection_feedback_test.go, agent_chat_messages_test.go:486) already cover the UNSUPPORTED_TOOL registration boundary. R18 remains VERIFY_PENDING until the mapped tests actually PASS under operator CI.
- CLP-04.02.03/.04, 05.01.04, 05.02.04: tracker marks partial coverage. These are candidates for further source/test authoring; they are NOT declared external-only here.
- CLP-05.02.01 catalog metadata is advisory (status/verified/partial summary), not an authoritative receipt or permission grant; keep it advisory.

## Status
P0/P1/P3: source authored/fixed, VERIFY_PENDING. P2/P4/P5: audit complete, no further code change required (drain-only limit; broad invalidation; safe R18 adapter gate). All actionable source/test/documentation work from the post-push review is complete = SOURCE_IMPLEMENTATION_COMPLETE for the touched scope. EXTERNAL_VERIFICATION_PENDING: V-BATCH-00..08. Not FULL_HANDOFF_COMPLETE. No gate promoted, no test/build/git/CI executed by this session.
