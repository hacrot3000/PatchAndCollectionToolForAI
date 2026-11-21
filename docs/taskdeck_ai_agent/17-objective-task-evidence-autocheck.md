# HANDOFF 17 — Objective acceptance evidence and exact-call Auto-check

**Date:** 2026-11-21  
**Repository:** `hacrot3000/PatchAndCollectionToolForAI`  
**Pull request:** [#162](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/162)  
**Base:** `main` at `828bb0f944e5877dffd0b4a520f6494d23e45ebb`.  
**Previous:** [Handoff 16](16-durable-task-plan-orchestration.md).

## Purpose

Continue TaskDeck's multi-day Agent design. Fix false-positive milestone acceptance and reduce human intervention for **objective** results already verified by the native Patch Tool or read-only Collection. A model answer is NEVER itself authority to mark a user-owned task done.

## Existing defect identified and fixed

Before this change, `VerifyTaskPlanEvidence` accepted an acknowledged Native Patch with `Status=="error"` or `Status=="rejected"`, and it treated a matched Collection function-call/output pair as successful even when the returned JSON contained read/action errors. Thus the Task plan API could incorrectly mark a milestone `done` or complete a plan using failed operations.

PR #162 introduces `internal/agentchat/task_evidence_validate.go` and updates manual and archived evidence checks. A Patch used as success proof now requires `call.Status=="done"`, `FeedbackStatus=="acknowledged"`, a structured JSON result with `status=="PASS"`, zero RC when present, no timeout/nonzero command RC/failing command statuses, and a matching native run ID when present. Collection proof requires a parseable nonempty `results` array without top-level, action, or file errors. It remains correct to inspect unsuccessful operations as diagnostic evidence; they must **not** satisfy a success acceptance gate.

## Exact-call user opt-in: Auto-check

- A per-step `auto_check` rule is an explicit **user-owned** object stored in the canonical session journal; it cannot be created by model output, tool text, or source code.
- `set_auto_check` action on `POST /api/agent/sessions/{id}/plan` requires revision CAS, the session owner, step ID, and `confirm_auto_accept: true`.
- `patch_pass`: requires one exact Native Patch call ID, an acknowledged `PASS` native result with matching immutable run ID, approved package SHA-256 and native session receipt. It confirms **the Patch wrapper succeeded**, not that all app tests passed.
- `full_file_read`: requires one exact Collection call ID and project-relative path. The canonical request must name that file and the actual result must contain the same file with successful, untruncated content and no more pages.
- `Task plan → Enable exact-proof auto-check` presents a separate confirmation. The owner may later remove a rule before the step is accepted. Rules cannot silently grant Auto Patch or provider network permission.
- The scheduler checks eligible rules after normal native Patch reconciliation / verified feedback, then before any next AI continuation. It records successful proofs **once** using the normal session CAS, without sending another provider request or executing a Patch. It **does not** automatically set the overall Task plan to completed.
- Rules may be configured after evidence was already moved to a private integrity-checked Collection/Patch archive; in that case API verifies the exact old receipt immediately. No archived source content is sent to the model as part of verification.
- Each rule stores its last-inspected Collection/Patch archive generation. If a long provider turn archives the exact tool receipt before the scheduler polls, the changed archive counter wakes one recheck. Absent evidence does not trigger an unbounded periodic archive scan.

## API examples

Exact Patch result:

```json
{
  "action":"set_auto_check",
  "expected_revision":22,
  "step_id":"implement",
  "enabled":true,
  "confirm_auto_accept":true,
  "auto_kind":"patch_pass",
  "auto_call_id":"actual-patch-call-id"
}
```

Exact complete file inspection:

```json
{
  "action":"set_auto_check",
  "expected_revision":23,
  "step_id":"research",
  "enabled":true,
  "confirm_auto_accept":true,
  "auto_kind":"full_file_read",
  "auto_call_id":"actual-collection-call-id",
  "auto_path":"src/main.go"
}
```

Disable:

```json
{"action":"set_auto_check","expected_revision":24,"step_id":"research","enabled":false}
```

The user must bind **the actual exact call ID**; a name, file path or the model's claimed success alone is insufficient. If the receipt is missing, failed, ambiguous or truncated, the milestone remains open.

## Testing

New unit tests reject `FAIL`, `PREFLIGHT_FAIL`, `INCOMPLETE`, nonzero RC, nested command failure/timeout, unparseable Patch result, mismatched native run ID, missing/failed/empty Collection responses, partially paged file reads, incorrect paths and unsafe rules. Server integration tests cover owner consent, durable pending rules, automatic acceptance of an exact successful native Patch/Collection result, and once-only transition with the whole plan left open. Another test generates **28 Patch and 160 Collection calls**, forces both archive types, then validates the old exact receipts without replay.

**CI:** record final actual commit and workflow outcomes below before merge.

## Remaining risks and next work

- **P0 — Semantic acceptance:** Patch wrapper `PASS` is not proof of app correctness, migrations, full test suites, performance, or all acceptance criteria. Future deterministic test-run verification must use an authenticated, structured test result with exact run ID/exit status and independent executed command evidence; never mine raw stdout or trust an AI claim.
- **P0 — Ambiguous provider acknowledgement:** `SENDING`/`UNKNOWN` egress is not auto-replayable. Provider-specific idempotency/receipt reconciliation remains necessary.
- **P1 — Configuring future exact call IDs:** because the user must know the actual immutable call ID before enabling a rule, this first safe Auto-check implementation does not yet autonomously map every model-produced milestone. Introduce a separate model-suggested proof claim that can be reviewed and bound to a user-authored objective rule without granting model-owned completion authority.
- **P1 — Plan completeness:** this patch does not lift the 128-step active plan size, 4 MiB journal ceiling, provider request cost or worker fairness limitations. Continue paginated planning/long-run health monitoring.
- **P1 — Mechanical vs semantic test evidence:** the verification of a full file read proves the file was returned untruncated; it does not establish that its behavior is correct. UI and handoff must display this distinction.

## Transfer instructions

1. Fetch latest `main`, read handoffs 13–17 and PR #162.
2. Check actual final CI results. Do not merge failing code.
3. Continue with structured external test receipts and optional reviewed evidence proposal protocol.
4. Preserve exact provider/Native Patch egress boundary and full canonical history; never replay a completed or ambiguous Patch.
