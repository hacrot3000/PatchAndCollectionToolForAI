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


## Completion addendum — 2026-11-21 · PR #163

- **Merged follow-up:** [PR #163](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/163).
- **Merged commit on main:** `168d43e0e077713419311b4f1901381ee2bcda21`.
- **Validated feature HEAD:** `3ca8307a6102fcff898c1680fff948a9852c04c6`.
- **CI for exact feature HEAD:** two Agent Chat protocol core checks **PASS**; Remote Portal (PHP) **PASS**; Remote Agent (Go) **still in progress at merge**, not yet verified successful.
- **Closed objective-proof gap:** exact `full_file_read` evidence must include native `start_line=1`, parseable `total_lines`, `truncated=false`, and no continuation/error; its canonical request must start at line 1 or use the native first-line default. A tail-only final read can never satisfy a full-file milestone. Regression tests cover invalid last-page/missing-metadata claims and preserve verified live/archive Auto-check behavior.
- **Scope boundary:** This closes the **objective full-file Auto-check hardening** described above. Independent functional-test receipts, model-suggested reviewed proof, provider-aware SENDING/UNKNOWN reconciliation and long-run scheduler fairness remain separate **open work** from the existing risks section. They are not silently considered complete, and this change does not authorize additional provider/network calls or repeat Native Patch side effects.


## Continuation — 2026-11-21 · independent native Task-run exit receipt

This follow-up remains within the Handoff 17 **structured external test receipt** priority; it does not enable arbitrary shell commands, provider egress, Native Patch replays or automatic plan completion.

- **Mechanism:** TaskDeck's existing authenticated SessionService runs a Task using the normal user-controlled Tasks UI. The session manager now records a SHA-256 of the **actual launched command, argv and cwd** (not just the display preview and not secret environment values). Its terminal metadata provides the exact session ID, Task ID, project/owner scope, start/end timestamps, status and exit code.
- **User action:** `Task plan → Mark done → Verified Task run (exit 0)` requires **the exact Task session ID and numeric Task ID**, a separate confirmation, session revision CAS and the normal step acceptance action. The server looks up the exact native session; logs/browser text/model claims are never considered authoritative success.
- **Objective gate:** accept only a finished, non-stopped Task with exit code 0, a machine-created command digest, fresh post-plan timestamps, and a cwd under the active project. Explicitly reject failed/still-running/stopped runs, missing session, wrong Task ID, mismatched project/owner, older-than-plan or ambiguous command identity.
- **Persistence:** the accepted immutable digest and exact Task session reference are recorded in the canonical Agent journal and handoff. The Task session must still be retrievable from the native SessionService **at the time of proof adoption**; merely having a user-supplied log or historical Task-run JSON is insufficient.
- **Meaning:** this is machine proof that a particular Task **exited 0**; it is not semantic evidence of full application correctness, and the user must review whether the Task actually implements the milestone's acceptance criteria. This feature intentionally does not auto-accept test results or alter `set_auto_check` scope.
- **Regression plan:** API positive Task run with real command; reject wrong ID, failed exit, invalid status, unrelated workspace and missing/stale identity; verify journal survives reopening. Record actual CI and merge SHA below.


## Completion addendum — 2026-11-21 · PR #164 (native Task-run evidence)

- **Merged PR:** [#164](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/164).
- **Merge commit on main:** `1c4f258ba47c411b0228540845da274ea2b8a9e8`.
- **Validated feature HEAD:** `8c4a338a3c50e1d9cc3c6a51becdfea4ea9e1cc3`.
- **CI on the final feature HEAD:** Agent Chat protocol core **PASS twice**; read-only Collection/Patch regression **PASS**; Remote Portal PHP **PASS**. Remote Agent Go **still running at merge**, so no claim of Go CI success.
- **Delivered:** machine-derived SHA-256 of actual Task `command + argv + cwd` in native SessionService metadata; revision-fenced `set_step` support for `evidence_kind:"task_run"`, an exact real Task session ID and explicit Task ID. Server verifies terminal `exited/exit_code=0`, owner/project identity (including current workspace identity), post-plan run timing, expected Task ID, command digest and cwd containment. The UI asks for the exact IDs and confirms acceptance. Accepted evidence is included in the durable plan/handoff even when the native session later disappears.
- **Tests:** actual POSIX Task process PASS and FAIL, fake session ID, wrong Task ID, stopped/nonzero/missing exit, missing/malformed command identity, stale/inconsistent times, session kind mismatch, workspace escape, shared owner/project mismatch and durable journal reopen.
- **API example (explicit user review only):**

```json
{
  "action": "set_step",
  "expected_revision": 35,
  "step_id": "test",
  "status": "done",
  "evidence_kind": "task_run",
  "evidence_ref": "actual-native-task-session-id",
  "evidence_task_id": 42,
  "evidence_note": "Reviewed that this Task runs the intended acceptance tests"
}
```

- **Narrow, important limitation:** this verifies **one TaskDeck native Task ran and exited zero**, not that arbitrary app semantics or every acceptance criterion passed. Its originating native session must still be reachable **when first recording the proof**; a user-uploaded log or historical unsourced Task-run JSON is not an authoritative substitute. This does **not** automatically execute or select a test, grant shell/tool permissions, replay a Patch, invoke a provider or auto-complete the whole plan.
- **Unresolved next work inside the original handoff:** provider-specific authoritative reconciliation of SENDING/UNKNOWN; optional reviewed model-suggested proof claims bound to user-owned rules; richer external test suite identity/coverage and retained verifiable receipts after native session eviction; long-run fairness/pagination limits. None of these are represented as completed by PR #164.


## Continuation — 2026-11-21 · reviewed AI evidence suggestions

This iteration addresses the exact Handoff 17 P1 item: user-reviewed binding of a model-suggested proof pointer to the existing **user-owned objective Auto-check** rule. It does not introduce a new provider request, new executable action, approval bypass, semantic validator or automatic whole-plan completion.

- A model may optionally include one fenced `taskdeck-evidence-suggestion` JSON object after a real tool operation: `step_id`, `kind=patch_pass|full_file_read`, exact `call_id`, and `path` only for full-file reads.
- TaskDeck parses only the **actual canonical provider response** (not user text, Collection results or attached files), rejects extra keys and unsafe paths, checks the matching uncompleted user plan milestone, and retains at most eight model-proposed hints with provider-turn identity, source SHA-256 and time in the durable Task plan. They remain explicitly **UNTRUSTED**.
- Task plan displays these hints next to the unchanged native proof rule editor. Clicking `Review suggested proof` only fills the local editor controls. The existing `Enable exact-proof auto-check` action still requires a separate click, explicit owner confirmation, revision CAS and the owner-scoped real receipt check. No suggested hint can directly create an Auto-check rule or mark `done`.
- Historical/reopened journals retain the hints while recent ones can roll off at the defined cap. The plan can still finish only when actual verified evidence or separately user-attested acceptance satisfies every milestone. Suggestion messages must never count as verified work for the no-progress guard.
- Regression tests cover valid provider-sourced suggestion, invalid or invented call ID, missing consent, unknown step, unsupported kind, malformed/extra fields, duplicate and bounded history, and persistence after reopening. Record exact merged commit and CI outcome in the completion addendum.


## Completion addendum — 2026-11-21 · PR #165 (reviewed AI evidence pointers)

- **Merged PR:** [#165](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/165).
- **Merge commit on main:** `7f70e785d624675d7d61df3e9d7dc0da93b48459`.
- **Final validated PR HEAD:** `773d53c12bbfbea2cd4fe7e2a72f8f5d638ecb52`.
- **CI on exact PR HEAD:** Agent Chat protocol core **PASS twice**, Remote Portal (PHP) **PASS**; Remote Agent (Go) still **in progress at merge**, not claimed successful.
- **Implemented:** when the provider itself sends a bounded `taskdeck-evidence-suggestion` fenced JSON block, TaskDeck captures its exact `step_id`, `kind`, `call_id`, optional `path`, actual model turn ID, source SHA-256 and time. Only `patch_pass` or `full_file_read` hints are allowed. Maximum eight untrusted suggestions retained in the canonical Task plan. Malformed, duplicate, unknown-step and unsafe-path suggestions are rejected, and suggestions survive journal restart and appear in the per-revision handoff.
- **User control:** `Task plan → Review suggested proof` fills the existing editor but **does not** change the canonical rule. A separate `Enable exact-proof auto-check` confirmation still requires owner authorization/revision CAS; only an actual verified native result can mark the step `done`. No automatic provider calls, Patch execution or complete-plan acceptance were introduced.
- **Regressions:** true provider-result origin, spoofed user text ignored, no-consent rejection, absent receipt stays pending, subsequent canonical Collection proof succeeds once, no automatic whole-plan completion, malformed/extra fields, invalid paths, duplicates/bounds and durable handoff inclusion.
- **Not completed by #165:** independent semantic test-suite/coverage validation, retained executable proof after native Task session eviction, authoritative provider SENDING/UNKNOWN reconciliation, cross-daemon fairness and long-running queue/resource limits. Do not infer any of those outcomes from these advisory model pointers.


## Continuation — 2026-11-21 · retained native Task receipt after session eviction

- **Scope:** narrow follow-up to Handoff 17's durable, independently verifiable external Task-run proof; no new task execution, browser permission, provider call, or change to Auto-check kinds.
- **Existing problem:** `task_run` proof worked only while a finished native SessionService metadata record remained resident. Existing workspace Task History stores a browser-provided log and cannot safely replace missing native execution metadata.
- **Solution:** on an existing user-requested `POST /api/task-runs` for a completed successful native Task, the server alone snapshots real SessionService metadata and writes a bounded HMAC-SHA256-authenticated receipt under TaskDeck's owner-only **host-private** config directory. The random signing key stays alongside the private receipts with mode 0600; the receipt binds project identity, exact Task session and ID, native command+argv digest, status/exit code and timestamps. No browser log or client-supplied claimed status is signed.
- **Verification:** `set_step` still prefers current authoritative native metadata. Only if the session has been evicted, the owner may reference the exact Task ID and Task session ID; the server reads the retained receipt, validates private scope, identity, HMAC, and the **same** existing Task evidence conditions, including post-plan timing and correct owner/workspace. A missing, forged, altered, stopped or failed receipt never proves success.
- **Compatibility:** older Task History rows, logs and runs not recorded through the native finished-session path are NOT backfilled or elevated to proof. The normal Task History workflow continues even if private receipt creation fails; in that event evidence remains unavailable after eviction.
- **Limit:** this verifies a real native Task exited zero; it does not establish that the selected Task test suite covers all acceptance criteria or that tests have appropriate semantic validity. Retained receipts are per recorded Task session, not a global archive of every process started by TaskDeck. Log text remains untrusted.
- **Required tests:** native successful Task -> history POST -> evict native session -> exact Task plan evidence accepted after restart; negative failed/stopped runs, nonexistent/legacy logs, tampered receipt/MAC and wrong project/Task/owner. CI and merge evidence are recorded below after validation.


## Completion addendum — 2026-11-21 · PR #166 (durable native Task receipts)

- **Merged PR:** [#166](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/166).
- **Merge commit on main:** `0dbade299a13b8d34029afd29e5f574c4df8e76f`.
- **Validated feature HEAD:** `f43fad0727716d9146437bf2fe8ab1dd4fd46651`.
- **CI on exact feature HEAD:** Agent Chat protocol core **PASS twice**; Remote Portal PHP **PASS**; Remote Agent Go **still in progress** at final merge check, not claimed passed.
- **Closed scoped gap:** `task_run` milestones can now use a host-private, HMAC-SHA256-verified structured native Task receipt when the original Task session has been evicted. Native Task History POST creates this only from authoritative, already-finished SessionService metadata and records `verified_receipt` on the corresponding history row if successful. The row itself, client logs, older unsigned history, model text, or an absent native receipt are not an authority to mark completion.
- **Retention/atomicity:** receipts are written with private key material and exclusive final-name creation; a repeated writer must not silently overwrite earlier authentic proof. Verification re-checks project ID, exact Task/session ID, execution command digest, timestamps, status, exit code, user/owner and workspace bounds. The same evidence checks are enforced whether native metadata is live or recovered. Receipt lookup can function when the native broker has no session.
- **Regression coverage:** actual native Task execution PASS+FAIL; forged browser PASS log; no signed receipt for failed tasks; session removal followed by successful exact evidence adoption in the plan API; rejected wrong Task ID, changed receipt JSON with original MAC, wrong project, invalid key and shared user ownership; plan evidence survives store reopen and still does **not** automatically complete the entire plan.
- **Limit:** receipts are emitted by the existing `POST /api/task-runs` after a finished Task while the native session still exists. They are *not* retroactively produced for already-evicted runs or automatically produced for every background Task. A host-private receipt's exit code is mechanical evidence only, not proof that a test suite covers user-defined semantics. The Task History flow continues if receipt persistence fails, but those old runs cannot be used as recovered proof.
- **Remaining earlier Handoff 17 risks:** independent test-suite semantic/coverage verification, provider-specific authoritative `SENDING/UNKNOWN` egress reconciliation, long-run worker fairness and plan capacity/pagination. None are silently counted as done by PR #166; do not widen the scope without separate evidence and approval.
