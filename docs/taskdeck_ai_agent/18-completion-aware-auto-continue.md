# HANDOFF 18 — Completion-aware Auto Continue

Date: 2026-11-23
Repository: hacrot3000/PatchAndCollectionToolForAI
Baseline main: 20eb828aecc783ceb50da0db80b9611c441cb342
Scope: TaskDeck Agent Chat; builds upon Handoffs 16 and 17.

## User problem

The provider frequently finishes one response with a report such as
"Audit chưa hoàn tất ... Bước tiếp theo: kiểm tra M03–M06, sau đó M07–M10".
A provider turn finishing does NOT imply the 37-case audit was accepted.
TaskDeck should continue automatically when the owner explicitly opted in,
without requiring manual "continue" after every intermediate report.

## Implemented design

- Added internal/agentchat/turn_disposition.go with a strict, bounded
  taskdeck-progress JSON fence parser and conservative bilingual fallback.
  Dispositions: IN_PROGRESS, NEEDS_CONTINUATION, WAITING_TOOL, BLOCKED,
  CLAIMED_COMPLETE and UNKNOWN. All model-derived values are UNTRUSTED.
- Appended server-owned scheduling state to the canonical Session journal:
  response SHA-256, normalized action signature, repeated text count, native
  verified work signature, no-verified-work count and durable admission delay.
  The text parser cannot override these machine counters.
- Existing owner-owned Task plan continues using acceptance and exact native
  receipts, not a model claim. Next prompt names the next unmet owner milestone.
  A model-proposed next action is only an explicitly unverified hint. After
  two no-work turns, the next instruction prioritizes a narrow read-only
  Collection; after five, Auto continue pauses rather than spending forever
  on text-only loops.
- Added an explicit LOCAL owner setting for continuing a conversation with
  NO Task plan. API: POST /api/agent/sessions/{id}/automation with
  option=continue, enabled=true, confirm_external=true and revision CAS.
  The new Automation menu item is "Auto continue (no plan)".
  It is OFF by default and cannot replace the plan's separate authorization.
- No-plan continuation reuses the existing revision-fenced message API,
  provider identity and native approval gates. It is eligible only when the
  actual session is completed, no tool work is unresolved, an explicit
  incomplete/progress advisory exists, and the latest provider run is not
  PREPARED/SENDING/UNKNOWN. Admission errors are backed off durably, but
  ambiguous provider requests and Patch runs are never replayed.
- Updated Task plan UI to show the last disposition, proposed next action and
  "Continuing", "Waiting for tool", "Needs review", "Stalled" or
  "Task completed". The handoff records only bounded provenance/advisory
  metadata, not the claimed contents of a checkpoint file.

## Proof boundaries

- A reported checkpoint path is NOT proof that a checkpoint exists. The
  continuation prompt requests native read-only Collection verification.
- A changed or rewritten progress message cannot reset the no-work guard.
- A native Patch PASS proves only its authorized wrapper result, never
  unexecuted application tests.
- No model metadata can mark a task step done, create an Auto-check rule,
  authorize Patch execution, or override an unknown provider acknowledgement.
- This feature does not guarantee completion without enough valid tools,
  resources or user permissions, and does not offer an unbounded text loop.

## Regression coverage added

- Vietnamese M03–M06 audit example; concrete next M07–M10 request.
- Valid and malformed taskdeck-progress metadata and unsafe checkpoint path.
- Missing consent, owner opt-in, unanswered model turns, text repetition,
  stall and no provider request after the stall.
- Pending Native Patch and ambiguous provider fences.
- Existing Task plan three-turn brake regression updated for the new bounded
  five-turn recovery policy.

## Validation / handoff requirements

Run the repository's real Agent Chat Go regression suite and JavaScript
bootstrap tests on the exact final commit. Verify CI results rather than
claiming a test run from code inspection. Do not complete this handoff or
declare production behavior proven without those checks. Review the
repository's existing CIs for other platforms separately.

Remaining risks: semantic completion remains under the owner-owned Task plan;
a plain no-plan chat only has a conservative continuation signal and never
automatically establishes verified acceptance. Existing authoritative provider
SENDING/UNKNOWN reconciliation limitations remain unchanged.
