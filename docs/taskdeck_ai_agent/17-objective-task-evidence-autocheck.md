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


## Continuation — 2026-11-21 · owner-bound native test command identity

This iteration implements only the **expected command identity** portion of the Handoff 17 external functional-test proof gap. It does **not** claim full semantic test-suite or coverage verification.

- **Mechanism:** optional per-milestone `TaskCommandGuard` with expected native Task ID, lowercase SHA-256 of executed `command+argv+cwd` and exact user-confirmed bind timestamp. The Task Run History detail shows this fingerprint from native metadata. It is a hint for choosing an expected command, **not proof by itself**.
- **Explicit workflow:** an owner clicks `Lock expected test command`, enters Task ID and the fingerprint and confirms via the existing revision-fenced task plan API. Then they run the Task **again**. Existing accepted milestones cannot be silently repinned; modifying or removing a guard on an unaccepted step requires explicit confirmation.
- **Machine proof:** when the user later marks this milestone `done` using `task_run`, the server requires actual native metadata or PR #166's host-private signed receipt to have the same Task ID and executed-command SHA-256 **and a start timestamp later than guard binding**. Same-second native starts are conservatively rejected because SessionService timestamps have second precision. Failed/stopped Tasks still fail the previous exit-code guard. No provider output or user-supplied history field can substitute for native proof.
- **Compatibility:** entirely optional, so old/unguarded plans retain their prior exact Task ID + exit-0 verification flow. The command lock and any resulting evidence survive journal reopen and appear in the per-session handoff.
- **Non-goals:** the hash is only mechanical command identity. It does not establish that the command has meaningful assertions or which source files/tests it covers. There is no new shell execution, provider transmission, Patch replay, or silent plan/milestone completion.
- **Regression requirements:** reject forged/invalid fingerprint, missing user confirmation, stale revisions, wrong Task ID, edited command args, old pre-binding run, same-second ambiguity, cross-session identity mismatch; accept only a new native run with the expected digest, preserving handoff and immutable evidence. Record validated commit and CI in a completion addendum.


## Completion addendum — 2026-11-21 · PR #167 (owner-bound test command)

- **Merged PR:** [#167](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/167).
- **Merge commit on main:** `c077702b080a88662705149ce0beaf954d248532`.
- **Validated final feature HEAD:** `710b29add948a45102d603c38a049c536370b46e`.
- **CI on exact final feature HEAD:** Agent Chat protocol core **PASS**; Remote Portal (PHP) **PASS**; Remote Agent Go **still in progress at merge** and is not claimed successful.
- **Delivered:** optional, explicit, user-owned per-step test command fingerprint guard (Task ID, native `command + argv + cwd` SHA-256 and binding timestamp). Task Run History now surfaces the native executed-command fingerprint. A bound milestone only accepts a **later** Task with that exact native command digest and Task ID, still subject to existing exit-code 0, workspace, owner/project, plan age and signed-receipt checks. A pre-bind run or a same-second ambiguous Task start is rejected; an altered command/arguments cannot satisfy the bound milestone.
- **Controls:** `Lock expected test command` and removal require explicit owner confirmation and the existing session-revision CAS, and accepted steps cannot silently change their lock. Binding does not launch a Task or automatically mark a step done. Existing unbound task plans remain compatible. All accepted evidence and the owner-bound expected command survive journal reopen and appear in the per-session handoff.
- **Tests:** direct native metadata negative cases (wrong Task ID, changed executed command, stale/pre-binding or ambiguous same-second start, nonzero exit, malformed/case-invalid guard); real Task execution before/after user binding; missing consent, invalid hash, stale revision, wrong Task ID; completed-step immutability, persisted plan guard and handoff; Task Run History reproduces the actual native `CommandSHA256`, not the browser log.
- **Precise limit:** this closes only **test command identity checking**, not independent semantic test coverage. A command can exit zero without exercising all acceptance criteria, and user manual acceptance remains explicitly available. Provider-aware `SENDING/UNKNOWN` reconciliation, suite semantic/coverage attestation and long-run scheduler fairness/pagination remain open; no broader implementation is implied by PR #167.


## Continuation — 2026-11-21 · verified named Go test result from complete native output

This follow-up narrows the remaining external **test semantics** gap to exactly one machine-observed named Go test outcome, without claiming application-wide coverage.

- A new optional evidence kind `go_test_case` lets a user name a Go package import path and exact `Test...` case for an unfinished milestone, along with the exact Task session ID and Task ID. It is **not** a new shell execution or Auto-check kind. The user still explicitly marks the milestone and confirms the interpretation.
- The milestone must already have a user-confirmed expected command guard from PR #167. Native metadata must prove the *actual process* is `go test -json`, its command digest/Task ID match the bound expectation, its start was later than binding, and the Task exited 0 with the original owner/project/workspace checks.
- Only complete, live native SessionService output may be inspected: the native manager exposes total PTY bytes and retained scrollback bytes as metadata. A truncated output, missing counters from a stale broker, unavailable/evicted native session, oversized stream or missing exact Go test `run`, `pass`, or package-level `pass` event fails closed. Browser Task History log, provider/model narration, manually attached text and a signed **metadata-only** receipt are NOT structured test results.
- The exact case's machine-observed events and complete output digest are bound into the accepted plan evidence, which is retained in the private journal/handoff. The test itself may have incomplete assertions or inadequate coverage; this proves only that the specified case ran and passed inside a successful Go test package run.
- Existing `task_run`, manual and other evidence kinds remain unchanged. No provider egress, new Task launch, Patch retries, automatic acceptance or claimed semantic coverage is added.
- **Validation targets:** real native Go test -json execution (positive exact named test); nonexistent case, wrong package, skipped/failed test, fake shell-printed JSON, truncated/evicted output, changed command, pre-binding run, missing user guard and journal reopen. Record exact CI and merge information in the completion section.


## Completion addendum — 2026-11-21 · PR #168 (named native Go test evidence)

- **Merged PR:** [#168](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/168).
- **Merge commit on main:** `e9736b28345d54b4771cb4a296a60bb42ef07a5d`.
- **Final validated feature HEAD:** `785bdc87a74adce6facd8b8250453ac77f60942c`.
- **CI on exact feature HEAD:** Agent Chat protocol core **PASS twice**; Collection/Patch regression **PASS**; Remote Portal PHP validation **PASS**. Remote Agent Go **still in progress at merge**, not claimed successful.
- **Delivered:** opt-in `go_test_case` evidence accepts an exact Go package import path, named `Test...`, native Task session ID and numeric Task ID, only when the unfinished milestone already has a user-bound expected command identity. The server confirms the native executed program/arguments are `go test -json`, the Task started after the user binding, and the verified native Task ended with exit code 0. It then reads only the *complete* SessionService PTY output and requires test `RUN`, test `PASS` and package `PASS` events in sequence for that exact test. The output SHA-256 is bound into durable evidence, without storing test logs in the Agent journal.
- **Fail-closed coverage:** metadata now exposes `output_bytes` and `retained_bytes` from the native SessionService. Incomplete/truncated or oversized output, unknown broker counters, a session evicted before review, missing/skipped/failed named tests, wrong package, malformed event output, spoofed shell-printed Go JSON, altered/pre-bound commands or absent user guard are never accepted. Client-uploaded Task History logs and HMAC-protected **metadata-only** receipts cannot replace actual named-test output. Existing `task_run` exit-only evidence remains independently available.
- **Regression checks:** real native `go test -json` positive and negative cases, wrong package/test, pre-binding Task, forged output from `/bin/sh`, truncated native PTY scrollback, parser missing `RUN`/`PASS`/package `PASS`, malformed JSON, result survival after Agent store reopen and no implicit whole-plan completion. An initial CI failure caused by a malformed generated parser file was fixed before merge; only the final HEAD's passing checks are recorded here.
- **Scope/remaining work:** this confirms **one named Go test** ran and passed within a successful package run. It does *not* establish meaningful assertions, full requirement coverage, cross-language suite semantics, or overall plan acceptance. Provider-specific authoritative `SENDING/UNKNOWN` reconciliation and long-running scheduler fairness/limits remain unresolved; no new provider calls, Task execution or Native Patch replay were added.


## Continuation — 2026-11-21 · owner-predeclared required Go test checklist

This iteration closes only the **predeclared finite named-test checklist** portion of Handoff 17 test coverage verification. It does not infer broad semantic acceptance from a generic exit code, or add a new test runner.

- **User-owned requirement:** an unfinished Task milestone may explicitly lock one Go package and **1–16 unique exact named Go tests** after its expected native Task command has already been locked (PR #167). This is a separate user-confirmed, revision-CAS operation (`set_go_test_requirements`); the project, selected tests, and server-side binding timestamp persist with the step. A missing command guard, malformed/duplicate/oversize list or missing consent is rejected. Changes require a separate explicit user confirmation and a **new** Task run.
- **Objective acceptance:** a new `go_test_suite` evidence kind reads the **stored owner-bound list**. The caller cannot replace it with a shorter list at the moment of acceptance. The existing verified named Go test mechanism (PR #168) independently checks live, complete, bounded native `go test -json` PTY output, post-command-binding launch time, exact Task ID and executed command SHA-256, owner/project/workspace, and process exit 0. The suite verifier additionally requires the run to have **started after the required-test list was bound**, and every listed case to show exact RUN/PASS plus package PASS inside that same native Task session.
- **No weaker silent shortcut:** while a required list is locked for a milestone, `task_run`, an opportunistically selected single `go_test_case`, other tool evidence and manual signoff cannot silently substitute for `go_test_suite`. An owner may explicitly remove the required test list while a milestone is unfinished, with another confirmation; a command guard cannot be modified under an active list. Accepted milestones cannot change the list without reopening.
- **Fail-closed limits:** signed metadata-only Task receipts, browser logs, model narration and expired/evicted/truncated native output remain insufficient as structured test proof. Only that exact suite in one complete Task execution qualifies, and a passing named test does **not** prove the test contains meaningful assertions or that unlisted requirements are covered.
- **Regression targets:** store/API validation, duplicate and invalid names, 16-name bound, missing consent/command guard, pre-binding and wrong/missing case, replacement requiring another new native run, rejection of weaker proof kinds, immutable accepted step, durable reopened journal and per-revision handoff, no implicit whole-plan acceptance. Actual final PR/CI and commit references follow after validation.


## Completion addendum — 2026-11-21 · PR #169 (owner-bound required Go test checklist)

- **Merged PR:** [#169](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/169).
- **Merge commit on main:** `ba183817a29f3dccfb53c22a8b34b5a631c0ecdd`.
- **Validated exact feature HEAD:** `242ba12e261cc5202d7841cb27b79220054964a9`.
- **CI on final feature HEAD:** Agent Chat protocol core **PASS**; Remote Portal PHP validation **PASS**. Remote Agent Go **still in progress at merge**, not claimed successful.
- **Delivered:** user-owned, explicit, per-milestone Go package plus 1–16 unique named tests, bound after the user locks an exact native Task command and before the qualifying execution. `go_test_suite` evidence is assembled from one successful, complete native `go test -json` run and requires exact `RUN/PASS` for **every** owner-declared test plus package `PASS`, with matching Task ID/command fingerprint, owner/project/workspace, post-binding start and exit code 0. Test names at evidence submission cannot replace or shorten the stored checklist.
- **Explicit permission and fail-closed rules:** separate confirmation and session-revision CAS for binding/editing/removing expected tests. Missing/duplicate/malformed/unbounded test names, missing command lock, stale Task, absent required case, or a running/evicted/truncated Task cannot count as structured proof. While the checklist is active, weaker `task_run`, opportunistic `go_test_case`, manual or unrelated evidence does not silently satisfy the milestone. No new Task launch, provider call, Patch replay or automatic whole-plan acceptance.
- **Regressions:** bounded/duplicate/invalid test list validation, command guard dependency, missing owner consent, real native Go Task suite with one missing required case and a later corrected list requiring another new run, rejection of old and weaker proof, immutable accepted milestone, durable plan/handoff after store reopen and no silent whole-plan completion.
- **Remaining within original Handoff 17:** a passing predeclared named-test checklist does not prove semantic assertions or coverage for **unlisted requirements**. Provider-specific authoritative `SENDING/UNKNOWN` reconciliation, broader semantic/coverage evidence and long-running scheduler fairness/capacity/pagination remain open; none are claimed completed by PR #169.


## Continuation — 2026-11-21 · fair admission for multi-day Agent sessions

- **Scope:** P1 process-local worker scheduling fairness; no changes to provider `SENDING/UNKNOWN` reconciliation, network retries, native Patch approvals, global worker cap, or user authorization.
- **Verified problem:** `Store.List("local")` sorts by most recently updated session. The background scheduler scanned that list from the beginning on every poll and reserved up to four process-global workers. With five or more always-ready long-running chats, high-activity chats could repeatedly regain a slot while older, quiet chats never advanced.
- **Fix:** a lock-protected round-robin *admission cursor per local workspace scope*. Candidate IDs are sorted deterministically; admission starts after the last successfully admitted ID and skips already active session workers. Worker reservation, duplicate prevention, cursor update and global capacity (still four) happen atomically. A full pool does not advance the cursor. Stale cursors are handled by sorted upper-bound lookup. Cursor scopes are bounded to 128 with oldest-scope eviction.
- **Consent and safety:** only the existing `agentAutoFeedbackNeedsWork` predicate can supply a candidate; every admitted task is processed using the same existing owner/project/turn-lock/Native Patch/provider egress checks. A cancelled context releases any reserved-but-not-started worker. SENDING/UNKNOWN remains blocked, and an expired heartbeat never permits replay.
- **Regression targets:** deterministic rotation independent of most-recent update order, fairness after saturation/release, no duplicate active worker, no oversubscription under concurrency, bounded memory for many workspace scopes, cancelled dispatch handling and unchanged prepared-only recovery. No new provider calls or automatic whole-plan completion.
- **Limit:** the cursor is process-memory and is reset after daemon restart. This improves fairness **among active conversations within a workspace** but does not provide distributed scheduling, inter-workspace fairness guarantees, or a durable cross-daemon work queue. Provider-aware authoritative acknowledgement reconciliation remains a separate P0 issue.


## Completion addendum — 2026-11-21 · PR #170 (fair Agent worker admission)

- **Merged PR:** [#170](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/170).
- **Merge commit on main:** `6602e4b9d10a46c739bd29ffce2a9e5bd3049707`.
- **Validated exact final feature HEAD:** `793c99e0458b5e856e100caefe104161d88cdfcd`.
- **CI on exact final feature HEAD:** Agent Chat core **PASS twice**; Remote Portal PHP validation **PASS**; Remote Agent Go **still in progress when merged** and is not claimed successful.
- **Closed scoped gap:** the process-local scheduler no longer takes four workers in `Store.List` newest-update order on each poll. It collects only the existing eligible `local` sessions, then allocates workers in deterministic, rotating session-ID order within a workspace. The last actually admitted session acts as a cursor; already active sessions are skipped, and saturation does not move the cursor. Reservation, per-session duplicate prevention and the unchanged global four-worker limit are protected by the same mutex.
- **Memory and cancellation:** workspace-local cursors have a bounded 128-scope FIFO retention limit; contexts cancelled between reservation and goroutine launch release the unused slot. A daemon restart resets the admission cursor; it is not a distributed execution lease or proof of provider acknowledgement.
- **New regression tests:** cyclic fairness with seven candidates and only four worker slots; saturation and re-admission after release; older/deleted candidates and changing `Store.List` order; independent cursor scopes; concurrent scheduler ticks cannot oversubscribe the global cap or start duplicate session workers; cursors never exceed 128 tracked workspace scopes.
- **Strict non-goals:** no new provider API calls, patch execution, feedback replay, retry behavior, user authorization, automatic plan completion or new tool approval. Existing provider `SENDING/UNKNOWN` remains fail-closed. This is **within-workspace process-local fairness**, not cross-workspace global scheduling fairness, durable cross-daemon queue ownership or authoritative provider acknowledgement reconciliation.
- **Remaining Handoff 17 work:** provider-specific authoritative `SENDING/UNKNOWN` reconciliation, source/test-suite semantic coverage for unlisted user requirements, long-run resource/capacity and durable cross-daemon coordination. None are declared done by this scheduler iteration.


## Continuation — 2026-11-21 · bounded process-local fairness across workspaces

- **Verified gap after PR #170:** the four-worker pool was global, but `acquireFairBatch` only rotated sessions *within one workspace*. Independent workspace polls could arrive in an unfavorable order; a noisy workspace could take newly freed global slots before a quieter workspace's poll.
- **Scope:** process-local **cross-workspace admission fairness** only. The global worker cap remains four, existing session fairness continues, and no provider/Native Patch retry, request replay, approval, Auto Continue or execution semantics change.
- **Mechanism:** the shared mutex tracks workspaces with eligible, not-currently-active conversations; a completed reservation advances a global workspace cursor. Each workspace poll registers or clears its own pending state, and a free slot is reserved only for the workspace whose turn is next. When multiple scopes wait, a scope receives at most one slot per admission pass so another waiting scope is not crowded out. The session cursor within each workspace remains deterministic.
- **Bounded scheduling state:** waiting scopes refresh on the existing 3-second poll and expire after six missed poll intervals (18 seconds). Pending scope registry has an upper bound of 128 entries; overflow discards the oldest waiter, which can re-register on a future poll. A stale/closed workspace therefore cannot indefinitely block admissions; active worker reservations still must be released on termination/cancellation. Neither cursor nor waiter state is durable across daemon restarts.
- **Safety:** a waiting workspace registration is **only scheduling metadata**, never authorization for network egress. Existing owner/project checks and prepared-only provider recovery remain authoritative. An ambiguous `SENDING/UNKNOWN` provider run still cannot be automatically replayed.
- **Regression scope:** A occupies all four slots; B registers while full; A cannot steal B's turn when a slot frees; B gets a slot on the next poll; A/B then alternate without duplicates or overbooking. Also stale waiters, many workspace scopes (bounded memory) and concurrent scheduler ticks.
- **Limits:** fairness is eventual while a workspace continues polling and a worker eventually frees. No forced preemption of long-running providers, multi-daemon fairness guarantees, provider acknowledgement reconciliation or durable cross-process queue is claimed.


## Completion addendum — 2026-11-21 · PR #171 (cross-workspace fair admission)

- **Merged PR:** [#171](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/171).
- **Merge commit on main:** `6c5e29d47ef3a5da896885bac428685a2584f108`.
- **Validated exact final feature HEAD:** `95a64c60b4642cc89c7a866c879a87db940c2d59`.
- **CI on final feature HEAD:** Agent Chat core **PASS twice**; Remote Portal PHP validation **PASS**; Remote Agent Go **still in progress at merge**, not claimed passed.
- **Delivered:** process-global, mutex-protected round-robin turn across local workspaces, layered over PR #170's deterministic per-conversation rotation. Every poll registers only workspace sessions with work that is not already occupying an active worker. If more than one workspace is pending, at most one worker is admitted per turn, with the next scope chosen after the last successful allocation. When B queued while A held all four workers, A cannot reclaim B's turn merely by polling first after a slot frees. Existing four-worker global limit and no-duplicate session checks remain unchanged.
- **Liveness and memory:** pending scopes have an 18-second missed-poll TTL (six normal scheduler intervals); registration memory is bounded to 128 workspace entries. Stale/abandoned scopes cannot indefinitely reserve the next turn. This is a process-memory scheduling hint only; on restart the cursor resets, and worker preemption or durable cross-daemon coordination is not provided.
- **Regression coverage:** A/B global competition after four-slot saturation, A blocked until B is admitted, alternating workspace turns, stale pending expiration, explicit empty-candidate clearing, bounded registry after more than 128 competing workspaces and concurrent allocation remaining under four active workers. Existing per-session fairness and provider recovery checks are unchanged.
- **Safety boundary:** no changes to explicit user consent, provider PREPARED-only safe recovery, SENDING/UNKNOWN no-replay behavior, native Patch approvals, provider retry controls or whole-plan completion. A waiting scope never grants network egress authority.
- **Remaining Handoff 17 gaps:** trustworthy provider-specific acknowledgement/idempotency reconciliation for ambiguous SENDING/UNKNOWN requests; independently demonstrated semantic test coverage of unlisted requirements; durable cross-daemon scheduler ownership and multi-day capacity/pagination. Cross-workspace process-local admission fairness is complete for this iteration only.


## Continuation — 2026-11-21 · opt-in bounded plan pagination

- **Observed issue:** the Task-plan panel always fetched and rendered the complete canonical plan. As evidence-bearing milestones accumulated, each open/refresh built all action controls at once even though the journal already caps plans at 128 steps.
- **GET contract:** `/api/agent/sessions/{id}/plan?offset=N&limit=M` returns only requested `plan.steps`, with `M` capped at 32 (default 24 when omitted). Whole-plan `done`, `total`, `completed`, owner-scoped `revision` and authoritative `all_done` remain computed on the full persisted plan. The `page` object includes `offset`, `limit`, `total_steps`, `has_next`. A missing plan yields an empty page without creating one.
- **Compatibility and mutation fence:** existing GET with no pagination parameters and all POST operations retain complete-plan response semantics. Pagination is a transient copied response, never a journal mutation; every POST still requires the full session revision CAS. Unknown/malformed, negative, duplicate, empty, over-limit or out-of-range pagination values are refused (HTTP 400). The valid `offset=total_steps` empty tail is supported.
- **UI:** the task-plan panel fetches 24 steps per page with Previous/Next buttons, refreshes only the selected page after an action, and resets the page on conversation change. The whole-plan “Complete verified plan” action is presented **only** if the server-computed `all_done` is true, not if a displayed page alone has all steps done.
- **Tests:** a real persisted 128-step task plan with evidence on exactly the first 24 demonstrates correct pagination/progress and that all-done remains false. Full pages, tail/empty-tail, default limit, bad parameters, canonical persistence, stale POST CAS and unpaginated POST compatibility are exercised. After all 128 steps gain valid evidence, the same page must report whole-plan `all_done=true`.
- **Explicit limits:** this is an HTTP/UI read-optimization for the existing 128-step model, not virtualized unbounded task storage, durable inter-daemon queueing, provider acknowledgement reconciliation or a relaxation of any native approval/test-evidence gate. Large-scale multi-day plan storage and semantic test coverage remain open.


## Completion addendum — 2026-11-21 · PR #172 (bounded Task plan reads)

- **Merged PR:** [#172](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/172).
- **Merge commit on main:** `0c86735aeb98ef56c0bd7b997342f2ca6d0ec09b`.
- **Validated final feature HEAD:** `4570d4f30161f47b11824605aee97035e0d1edd5`.
- **CI on final HEAD:** Agent Chat core **PASS** (includes 128-step read-pagination and completion-invariant regression); Remote Portal PHP validation **PASS**; Remote Agent Go **still in progress at merge**, not claimed successful.
- **Delivered:** optional read-only query `GET /api/agent/sessions/{id}/plan?offset=0&limit=24` (max 32), returning copied plan-step pages with unchanged plan metadata plus whole-plan progress, immutable session `revision`, authoritative `all_done` and bounded page metadata. Browser panel navigates with Previous/Next, and only the selected page is rendered after updates. Client without paging arguments receives the original full plan; POST operations still return the complete plan for backward compatibility. No change to the 128-step canonical journal cap.
- **Key safety and correctness gate:** only server-computed **all-plan** `all_done` can offer completion; a full visible page of completed steps does not suffice. Session-owner checks, original revision CAS, source evidence verification, Go test-suite binding, provider egress rules, Native Patch approvals and Auto Continue are unchanged.
- **Regression coverage:** full 128-step stored plan, first/middle/last/empty terminal page, default 24 and maximum 32, wrong/duplicate/oversized/negative offsets and limits fail 400, evidence retained on page, canonical plan not truncated by read, stale CAS still rejected after pagination, unpaginated POST still full, and `all_done` becomes true only when all 128 milestones have evidence.
- **Remaining Handoff 17:** this is **read/UI pagination** only, not unbounded write-side task storage, 1,000+ task hierarchy, journal compaction, distributed scheduling, provider-specific authoritative `SENDING/UNKNOWN` reconciliation or semantic coverage proof for unlisted acceptance requirements.
