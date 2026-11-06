# TaskDeck Remote — mainline development

## Merge record (2026-11-06)

- Source: `feature/taskdeck-remote-access-architecture` at
  `ff9b31fa486f3aac31d7bd04fb0a6124aa063c8c`.
- Target before merge: `main` at
  `2de1825038ffa4a2c7f9c2c385006f9409f57d6d`.
- GitHub PR #146 was merged with method `merge`, preserving full commit
  history, at merge commit
  `5569c99e4c0a6e379996968ba084a2fc8004addf`.
- Source branch was 759 commits ahead / 0 behind, and its last code
  revision `6769022abc26efa125a36fa2ede028b46a92eb85`
  passed both TaskDeck Remote Agent (Go) and Remote Portal (PHP) CI.
  The source HEAD after that code commit only added documentation.

## Authoritative working branch

**From this merge onward, development of TaskDeck Remote is done directly
on `main`.** Do not open new feature branches for this workflow unless
the repository owner explicitly changes the decision. Do not continue
committing to `feature/taskdeck-remote-access-architecture`.

Remote Agent and Remote Portal push CI now target `main`; normal
`pull_request` triggers remain for externally submitted changes.
No force-push or history rewriting is authorized by this policy.

Before a direct `main` commit: read current HEAD, use a compare-and-swap
expected SHA on ref updates, keep E2EE/RBAC/audit boundaries, add bounded
regression tests, then inspect the CI for the committed revision. Never
silently mix incompatible pre-E2EE protocols from the historical `main`
implementation.

## Remaining code work (priority order)

1. Tasks — investigate and restore genuinely missing dependent-workflow
   reports and managed-task behavior; no blind protocol copying.
2. Git — larger paged reviews, remaining read-only interactive reviews and
   narrowly authorized mutations.
3. Files — safe deeper navigation, file-content reads within explicitly
   allowed subfolders and reviewed E2EE binary transfer contracts.
4. Editor/GUI — compare-and-swap guarded edits, conflict handling, and
   comprehensive file browser/editor interface.
5. Security — filesystem races, session revocation, quotas, concurrency,
   comprehensive permission and release-gate regression coverage.

Features whose code and automated checks are complete but only need
real-host testing are considered **code complete**.

## 2026-11-06 — Read-only, principal-scoped task outcome report

- Added `tasks.report` as an argument-free, explicitly whitelisted E2EE
  action, separately gated by the current TaskDeck user's project-scoped
  `tasks.view` permission and the LIVE authenticated Portal/client-key
  grant. The trusted host must enable BOTH
  `TASKDECK_REMOTE_TASKS_PREVIEW=1` and
  `TASKDECK_REMOTE_TASKS_REPORT=1`; report reads fail closed by default.
- The Agent reads only the local TaskDeck `remote.action.tasks_run_finished`
  audit rows filtered by the exact authenticated `user_id` AND
  `project_id`. It enforces known schema and `transport=e2ee` and returns
  at most three recent entries (task ID, completed/failed/cancelled/timed_out,
  exit code, duration and UTC timestamp), never raw audit text, command,
  argv, cwd, task input, output, environment, credentials or other users'
  records. Invalid/malicious legacy audit entries are skipped.
- A fully typed strict JSON parser and the existing 512-byte AEAD plaintext
  ceiling bind every E2EE response. The browser cannot supply task ID,
  username, project, filters or pagination offsets. The native GUI shows a
  names-free outcome summary via `textContent` and scrubs it on logout.
- The server appends a TaskDeck-local audit event for each authorized
  `tasks.report` action. Focused Go race/Node tests cover principal
  isolation, host opt-in, malformed/forged JSON, quotas, native no-argument
  requests and display cleanup.
- This adds **historical run results**, not dependency execution, queued
  workflow orchestration, persisted output or arbitrary report export.
  These are separate unfinished functionality and require new explicit
  E2EE contracts; do not use a generic remote audit/file API.

## 2026-11-06 — Read-only dependency graph for issued Task IDs

- Added the **read-only** `tasks.dependencies` E2EE action. The request
  carries exactly one stable numeric Task.ID **already issued by the same
  authenticated session's `tasks.preview` pages**. Client/PHP/Relay cannot
  select a tasks.json path, workspace, command, arguments, edges or executor.
- This check requires the existing `tasks.view` RBAC and
  `TASKDECK_REMOTE_TASKS_PREVIEW=1`. Enable dependency graph content with
  `TASKDECK_REMOTE_TASKS_DEPS=1` on the host; absent this opt-in the
  authenticated response is **content-free** `available=false`, not a
  failed login or a session-killing permission downgrade.
- Agent reads only the canonical host workspace's
  `.vscode/tasks.json`, verifies the task-source SHA-256 bound to the
  session's issued preview, rejects changed source/inode, and delegates
  graph correctness (missing dependencies, duplicate labels, cycles and
  topological ordering) to the existing TaskDeck
  `tasks.BuildWorkflowGraph`.
- The response returns only a bounded topological array of Task IDs and
  prerequisite edges (at most 8 nodes and 8 edges, inside the existing
  **512-byte AEAD plaintext**). No labels, commands, secrets, input values,
  workflow scripts or raw task configurations are sent to the Relay.
  The local GUI displays known labels from the previously authenticated
  preview using text-only rendering, otherwise displays just `Task #ID`.
- Every action is checked again for local identity, project permission,
  live Portal grant and E2EE; TaskDeck audits authorized reads. Graph
  inspection **never** starts a process, never schedules dependencies and
  does not grant `tasks.run`. The old task execution preflight still
  rejects `dependsOn` and other unapproved workflows.
- Tests cover malformed/duplicate/wrong-task E2EE replies, topological
  validation, unissued ID attempts, source changes, missing/cyclic
  dependencies, host opt-out, unchanged `tasks.run` block, and native GUI
  logout cleanup. The next workflow milestone is a separately reviewed
  host-owned, cancelable DAG executor with per-step rights and audit; it
  must not reuse this preview action as an execution capability.

## 2026-11-06 — Opt-in linear Task workflow execution

- `tasks.workflow.start` is a **separate**, authenticated E2EE mutation,
  not an extension of the read-only `tasks.dependencies` privilege.
  Native client sends only an already-issued root Task.ID. Every start
  requires a live TaskDeck `tasks.run` permission, current approved
  client-key proof, password, project grant and uncached Portal
  authorization, plus explicit host opt-ins:

  ```shell
  TASKDECK_REMOTE_TASKS_PREVIEW=1
  TASKDECK_REMOTE_TASKS_DEPS=1
  TASKDECK_REMOTE_TASKS_RUN=1
  TASKDECK_REMOTE_TASKS_LONG=1
  TASKDECK_REMOTE_TASKS_WORKFLOW_RUN=1
  ```

- This first executor is intentionally conservative: exactly **2–4**
  sequential `process` tasks in one linear dependency chain. It rejects
  branching/parallel graphs, scripts/shell interpreters, hidden inputs,
  secret environment values, arbitrary command templates, host workflow
  retry/output/policy metadata and workflows containing extra nodes.
  All `tasks.run` and `tasks.start` single-task paths **still reject
  dependsOn**. A DAG preview alone never authorizes mutation.
- The host builds and validates the exact pinned `.vscode/tasks.json`
  graph and each Task.ID before accepting the start. Every step
  independently re-checks the approved client credentials/RBAC, live
  Portal grant and the original source hash; changes or revocation
  fail closed before launching the next process.
- The Agent retains the exclusive long-task/terminal execution slot for
  the whole workflow; a single cancelable 90-second host job executes
  the steps in order. The **existing** `tasks.poll`, `tasks.cancel`
  and `tasks.clear` actions work with the root Task.ID. Standard
  process-group cleanup terminates children when canceled, disconnected
  or expired. Execution stops on the first nonzero exit code.
- Durable TaskDeck-local audit records a root workflow attempt before
  any execution plus **attempted and completed/failed/cancelled/timed_out**
  outcome events for each step, followed by the aggregate root outcome.
  Audits carry only numeric IDs, step count, exit code and status;
  never commands, paths, inputs, credentials or output.
- Native trusted GUI activates **Chạy workflow đã kiểm tra** only after
  authenticated DAG review and task-long/run capabilities, and always
  requires a confirmation. No user-entered shell command or arguments
  are accepted; ordinary long-task progress and cancellation are reused.
  Tests exercise E2EE-only start, topology/bounds, source drift, opt-out,
  execution ordering, fail-fast, process cancellation, audits and UI
  confirmation and logout.
- Not yet supported: branching DAG, parallelism, workflow secret inputs,
  retry/condition policies, per-node output exports, workflow resume,
  persistent step-by-step result pagination and Windows execution.
  Those require separate reviewed contracts rather than silently relaxing
  this conservative host gate.
