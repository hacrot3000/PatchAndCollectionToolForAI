# HANDOFF 20 — Legacy / Unified pipeline trace map (CLP-00.01.02/.03/.04)

> **Current local workflow (2026-11-27):** [20-coder-execution-contract.md](20-coder-execution-contract.md) supersedes historical process/stop instructions and completion claims below. Reference evidence only; use this audit to guide concrete source changes, not as a mandatory repeated audit or reason to stop. Local coder writes source/test fixes, operator runs CI/Git. Continue independent local work without external-only BLOCKED; no acceptance or safety relaxation.

> Static trace from read-only native COLLECT. Symbols marked "verified" were read in
> the current workspace; symbols marked "pinned" come from the master checklist's
> pinned-source trace (recorded at commits a7cc2a1711… / fe4cc344f1…). No provider,
> native tool, test or build was executed. This closes the documentation trace gap,
> not runtime validation.

## 1. Legacy pipeline — entrypoint → collect → journal → provider → patch → continuation
| Stage | Source / symbol (verified or pinned) | Durability / fence |
|-------|----------------------------------------|--------------------|
| Message admission | `agent_chat_messages.go:162,356–448,512–586` (pinned) | mutable Session with revision CAS |
| Request projection | same file (pinned) | exact current receipt retained |
| Collection flow | `agent_chat_collection_flow.go:113–359` (pinned): validate native request (113), save provider-response checkpoint (181–184), save native start (218–223), execute native (225), repair (279–301), build exact next output/request (285–349), save before egress (355–359) | journal before native start and before provider egress; inner loop has no fixed round cap (139) |
| Feedback | `agent_chat_tool_feedback.go:204–425` (pinned): duplicate canonical output check, sibling output pair, save then continue; 406–413 separates Collection continuation failure from Patch feedback delivery | canonical call/output pairing |
| Auto Continue | `agent_chat_continuation.go:20` (pinned) outer loop | owner opt-in + no-progress guard |
| Prepared recovery | `agent_chat_provider_resume.go:15` (pinned) | PREPARED-only replay |
| Provider retry | `agent_chat_retry.go:241` (pinned) | retry counted separately from native observations |

## 2. Unified (Agent Run) pipeline — RunSpec → resolver → scheduler → receipt → TaskPlan
| Stage | Source / symbol | Evidence |
|-------|-----------------|----------|
| API entry / owner binding | `agent_run_api.go:49,84` (pinned) | immutable RunSpec + owner-bound snapshot |
| Provider executor | `agent_run_provider_executor.go:27,72,119,138` (pinned) | pins owner principal, `RunSpec.OwnerWorkspaceID`, provider/model, config version; credentials resolved only at invocation (08-future-planner.md:22, pinned) |
| Waiting-tool resolver | `agent_run_collection.go:77–155` (pinned) | exact run provenance, execution identity, turn lock, native-only collector, exact result, snapshot `CreateWithPolicy` |
| Native execution count | `agent_run_collection.go:21,45` (pinned) | one authoritative native execution per logical observation |
| Collection continuation | `agent_run_continuation.go:61,103` (pinned) | verified progress signature + completed-turn decision |
| Final continuation/plan | `agent_run_continuation.go:103–187` (pinned) | owner-pinned mechanical proofs, CAS, verified work signature, consent, no-progress guard |
| Global slot / lease | `manager.go:42` `GlobalSlotHeld`, `:47` `AcquireGlobalSlotContext` (VERIFIED, agent_collect_a053b91bc6f…) | live, revocable lease carried in context; nested admission checks `GlobalSlotHeld` to avoid double acquisition; plain/expired contexts cannot claim |
| DAG scheduler | `dag.go:60` `NewDAGScheduler`, `:103` `(*DAGScheduler).ClaimReady`, `:243` `Complete`, `:358` `Retry` (VERIFIED) | bounded concurrency, task state transitions, fence-guarded completion, explicit retry |
| Run fence | `run_fencing.go:33` `RunFenceAdmission`, `run_fencing_unix.go:39` and `run_fencing_other.go:8` `AcquireRunFence` (VERIFIED) | cross-process OS lease; fail-closed when platform lacks verified lease (`run_fencing_other.go:5`) |
| Manager retry | `manager.go:341` `(*Manager).Retry` → `retry(runID,false)` (VERIFIED) | manager-owned retry path |
| Durable save | `store.go:129` `saveLocked` (pinned) | CAS journal persistence |
| Patch / ACK | `agent_run_patch.go:144–255`, `agent_run_feedback_ack.go:13` (pinned) | sync native result, refuse existing feedback intent/unknown, persist prepared Session before manager continuation |
| Manager history fence | `manager.go:788–819` (pinned) | validates run-bound history before queue; 256-item run history capacity (`:415/797`) is a storage limit, not a read quota |

## 3. Patch gateway (shared) — verified in 01-architecture.md
`agent_chat_tools.go:31` `parseAgentPatchArguments` (single JSON object, name <=80, Python <=480 KiB, DisallowUnknownFields, trailing/NUL/invalid-UTF8 rejected); `:79` `agentChatToolDecisionAPI` (owner/revision/project-id/patch.run/approval TTL + Git snapshot, drift → unknown); `patch_paste.go:103` `pastedPythonPackage` (both `TASKDECK_PROJECT_KEY:` and `TASKDECK_PROJECT_ID:` headers, ZIP = manifest + entrypoint.py, paste only queues); `agent_chat_native.go` (`agentStartNativePatch`, `agentExactPatchSelection` with `taskdeck.patch` v1/`queue_selection`, `agentPatchResultFromProtocol` requires run_id + matching index); `agent_chat_tool_feedback.go:19` `agentChatToolContinueAPI` (persist FeedbackSent before egress).

## 4. Guard inventory and gaps
- Guards present (both pipelines): immutable provider/owner binding, canonical receipt exactness, one logical observation per native execution, UNKNOWN egress fence, owner consent at auto-continue, bounded no-progress hysteresis, worktree/pinned-snapshot resolution.
- Distinguishing gaps (NOT erased by this map):
  - Legacy inner collection loop has no durable cross-round progress guard (pinned, line 139).
  - Unified collection matching counts consecutive prior requests only; guard must not treat attempt IDs as objective IDs (pinned).
  - Native selective mutation proof still requires an authoritative receipt changed-files adapter proof (external).
  - Combined crash-window tests (W1–W7) need a test-only Store commit-barrier hook (20-crash-recovery-matrix.md §4).
- No semantic change was made to any scheduler, permission, journal, fence or PATCH recovery path by this documentation.

## 5. Handoff 13–19 reconciliation (CLP-00.01.04)
Pinned at checklist line 594: 13/14 canonical compact/checkpoints; 15 PREPARED-only replay; 16/17 owner acceptance and authoritative native receipt; 18 native-only and completion-aware continuation; 19 immutable run-bound continuation. Handoff 17's active single-engine owner directive is respected: one reusable engine-neutral guard, no independent execution path. Full Git/test-permission and CI reconciliation is operator-only.

## 6. Status
This static map closes the documented trace gap for CLP-00.01.02/.03 and records the handoff reconciliation for .04. It does NOT constitute runtime validation; those subtasks remain VERIFY_PENDING until checkout/CI receipts. No regression gate or rollup changed by this document.
