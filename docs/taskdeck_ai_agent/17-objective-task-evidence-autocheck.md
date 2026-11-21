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

## Completion record — 2026-11-21

- **Merged:** [PR #162](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/162).
- **Merge commit:** `90a5ca769857004e23db0433ad3103251e7c56a9`.
- **Final functional+docs head validated:** `fad665f2f311f3f239079c50dc718ce40503fe11`.
- **Agent Chat protocol core CI:** **PASS**, including tests for failed native Patch/Collection proof rejection, exact-call Auto-check, owner consent, once-only status update, cross-archive receipt verification (28 Patch / 160 Collection records) and existing Agent regressions.
- **Remote Agent (Go) / Remote Portal (PHP):** Still running / pending on final CI head at the time of this record. Do not claim either passed without final evidence.
- **Safety:** This feature performs no extra provider egress and no native Patch replay. Automatic milestone acceptance is restricted to explicitly user-configured objective rules, and a machine-verified Patch wrapper PASS is not semantic end-to-end acceptance. Provider SENDING/UNKNOWN remains non-replayable.
- **Next recommended focus:** Provide structured, independently verifiable test receipts for functional acceptance, then investigate provider-aware acknowledgement reconciliation where supported. Keep a per-response handoff current, and never infer task completion from a model answer.


## Continuation — 2026-11-21 · exact full-file evidence hardening

Scope remains the original objective Auto-check; this does **not** introduce new task kinds, grant extra provider egress, or claim functional tests are verified.

- **Issue confirmed in implementation:** a read starting at a later line could finish without a `next_line` and report `truncated=false`. The previous verifier checked truncation but not the start line or required total-lines metadata, so a tail-only Collection read could be auto-accepted as a **full-file** read.
- **Correction:** `full_file_read` now requires the actual native result to explicitly provide `start_line=1`, a parseable nonnegative `total_lines`, and `truncated=false` with no continuation/error. The paired canonical request must start at line 1 (or omit `start_line`, which defaults to 1). An incomplete final page, missing metadata or a mismatched request is not proof. The ordinary, manually inspected Collection evidence path is unchanged.
- **Regression coverage:** included a real tail-only page of a 43-line file, a false first-line response to a tail-only request, missing metadata, a truncated page and a valid default first-line read; existing live and archived Auto-check fixtures now use the native Collection pagination metadata.
- **Branch:** `feature/agent-exact-full-read-evidence-20261121` based on `2b0bbe8e22b4c7b255e611007558daf0294a48a9`. Commit and CI validation are recorded in the later finalization entry. The remaining external structured test receipt and provider acknowledgment work listed above remain **unimplemented** and must not be described as completed by this hardening.
