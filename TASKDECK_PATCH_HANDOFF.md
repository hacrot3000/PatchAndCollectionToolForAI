# TaskDeck Patch Native UI — Handoff / Recovery State

Last updated: 2026-09-25  
Repository: `hacrot3000/PatchAndCollectionToolForAI`  
Branch: `main`

## Critical status

**Integration is NOT complete. Phase 2 is IN PROGRESS.**

A previous closeout incorrectly treated structured protocol/rendering parity as proof that Patch Tool
had fully cut over to TaskDeck native UI. That conclusion was wrong because built-in Patch actions
still created ordinary terminal task tabs.

The historical Phase 2H.7 PASS and Phase 2I closeout are invalidated as product-level acceptance
results. Keep them only as implementation history.

## Source-of-truth rule

Use these documents together:

1. `TASKDECK_PATCH_HANDOFF.md` — concise recovery state and next action.
2. `TASKDECK_PATCH_ADDON_PLAN.md` — detailed roadmap/checkpoints.
3. `TASKDECK_PATCH_NATIVE_PARITY_AUDIT.md` — parity evidence plus the invalidation/corrected gate.

Do **not** declare Phase 2 complete from protocol/rendering tests alone.

## What Phase 1 actually delivered

Phase 1 and Phase 1.5 provided:

- bundled/global Patch Tool runtime;
- `taskdeck patch ...` CLI integration;
- Patch Activity Bar/menu entry;
- built-in Patch sessions;
- versioned event/command protocol;
- native renderers/controls for Queue, Resume, History, Plan, Health, progress and artifacts.

Those pieces were necessary scaffolding, but they did not by themselves make Patch a true native
TaskDeck workspace.

## Corrected Phase 2 status

### Phase 2A — Headless Patch backing sessions — IMPLEMENTED + CI VERIFIED

Commit: `355676a3`  
Test-fix checkpoint: `bc978abe`

- Built-in Patch sessions keep the Python/PTTY engine for compatibility/evidence.
- Reserved `task_id=-1` sessions are excluded from normal terminal-tab synchronization.
- Queue/Resume/History/Plan/Health start without calling the normal terminal attach path.
- Explicit terminal evidence can materialize the backing session on demand.

### Phase 2B — Native Patch workspace — IMPLEMENTED + CI VERIFIED

Commit: `634b1a2f`

- Patch activates an external TaskDeck view with `activateExternalView('patch')`.
- Patch occupies the primary workspace instead of the old 310 px sidebar overlay.
- Underlying terminal workspace is hidden while Patch is active.
- The previous view is restored on close.

A later audit found and fixed a return-view bug: the panel must read `app.active`, not the
nonexistent `app.activeSessionId`.

### Phase 2C — Native navigation/lifecycle — IMPLEMENTED + CI VERIFIED

Commit: `e0b41a4a`

- Resume → History no longer opens terminal history.
- TaskDeck starts the dedicated native History route.
- The Python native Resume path exits before entering `_history_browser()`.

### Phase 2D — Explicit-only terminal fallback — IMPLEMENTED + CI VERIFIED

Commit: `e0b41a4a`

- Protocol loss/timeouts remain visible in native UI.
- They do not automatically switch to a terminal.
- Terminal materialization is reserved for explicit evidence/fallback controls.

### Phase 2E.1 — Automated product acceptance gate — IMPLEMENTED + CI VERIFIED

Commit: `eec38265`  
GitHub Actions run: `36084258908` — PASS.

Additional runtime hardening: `89d3a458` — CI run `36086689002` PASS. Patch start now fails closed if backend metadata is not reserved `task_id=-1` or if the new session is unexpectedly already materialized as a terminal tab.

Recovery-document consistency guard: `7d199813` — CI run `36086873370` PASS.

The gate now checks product-level invariants rather than merely renderer/protocol presence:

- no implicit terminal tab on Patch start;
- headless Patch sessions remain headless during periodic/reload synchronization;
- full native workspace activation;
- native Resume → History;
- exactly one implementation path may call `materializeSession(...)`, inside explicit evidence;
- explicit terminal evidence buttons are the only normal UI callers of that path.

### Phase 2E.2 — Installed-runtime browser smoke — PENDING

**This is the current blocking acceptance task.**

Minimum native-UI code checkpoint remains `15711d4d`, but revision `a0b774c8` cannot be installed through self-update because the repository-document guard was mistakenly placed inside `go test ./...`. **For self-update/browser smoke, install `f9b4737b` or a later descendant on `main`.**

Checkpoint `15711d4d` passed GitHub Actions run `36087008801`. The self-update staging regression is fixed by `5b9a37e2` + `572368a1` + `3dbf06fb`, with a non-regression guard added in `afbb82be`.

After TaskDeck is self-updated/restarted to a build containing the corrected Phase 2 commits,
verify in the real browser UI:

1. Open Patch from Activity Bar.
2. Queue opens in the Patch workspace and does not create a task/terminal tab.
3. Resume opens natively and does not create a task/terminal tab.
4. Resume → History stays in native Patch UI.
5. History opens natively and does not create a task/terminal tab.
6. Plan opens natively and does not create a task/terminal tab.
7. Health opens natively and does not create a task/terminal tab.
8. Browser reload/session polling does not cause hidden Patch backing sessions to appear as tabs.
9. Explicit **Terminal / Open terminal evidence** creates a terminal tab and switches to it.
10. Closing/reopening Patch restores/navigates workspace correctly.

Do not mark Phase 2E DONE until this real-runtime smoke passes.

### Phase 2F — Cutover closeout — PENDING

Only after Phase 2E.2 passes:

- update user-facing docs to describe Patch as the native primary UI;
- mark the corrected Phase 2 acceptance criteria complete;
- mark Phase 2 / roadmap complete.

## Recent recovery commits

| Commit | Purpose |
| --- | --- |
| `8f00d995` | Reopen native Patch Phase 2; invalidate premature closeout |
| `355676a3` | Keep Patch backing sessions headless |
| `bc978abe` | Correct stale headless-session UI tests |
| `634b1a2f` | Promote Patch to native workspace |
| `e0b41a4a` | Keep Resume/History navigation native; explicit-only fallback |
| `eec38265` | Add product acceptance gate; fix previous-view state |
| `592c4dc9` | Add accurate recovery handoff |
| `082a8b72` | Correct README native-completion claims |
| `89d3a458` | Fail closed if Patch loses headless session invariant |
| `14a259fb` | Record verified Phase 2E.1 recovery checkpoint |
| `7d199813` | Add CI guard against premature handoff/roadmap completion |
| `9096299f` | Point implementing.md to the TaskDeck recovery handoff |
| `8c9f1882` | Remove self-invalidating “current head” field from handoff |
| `15711d4d` | Clarify explicit-only terminal fallback wording — CI 36087008801 PASS |
| `5b9a37e2` | Remove repo-doc guard from self-update Go tests |
| `572368a1` | Move Patch handoff guard to repository-level test |
| `3dbf06fb` | Run handoff guard only in repository CI and trigger it on docs changes |
| `afbb82be` | Guard self-update Go tests from repository-only document dependencies |
| `19c2ebc3` | Add CI gate that runs Go tests from the exact self-update staged source layout — CI 36087734095 PASS |
| `0ef148e8` | Defer appearance/project-state workspace access until `taskmenu:tasks`; removes `taskData=null` bootstrap errors |
| `1cb1fc0b` / `f5fe1382` | Add legacy-broker compatibility service and routing tests |
| `973af90b` | Wire daemon through compatibility service so stale broker terminals are preserved while native Patch uses a protocol-capable local manager |
| `f22cf21e` / `84900727` | Add and correct end-to-end FD3 Health protocol fallback test |
| `114a9222` | Correct project-state bootstrap contract test; CI 36088980153 PASS on Go 1.19/1.23 |
| `ae9acfcf` | Render native Patch inside `#panes` with a real switchable `Patch Tool` tab; fix file Open visibility and add full-path Copy |
| `b917de9f` / `fb0b7fc5` | Add backend `native` / `terminal` Patch session mode; terminal mode disables FD3/FD4 and native Python flags |
| `94715c30` / `b3915538` | Add Settings → PATCH TOOL → Interface: Native UI / Terminal (legacy) |
| `459c5f06` / `9a491503` / `9f35e6ee` | Wire interface setting into Patch launcher and update native/legacy acceptance gates |
| `4c7ddeaa` / `30d1b069` | Correct test fixtures; CI 36090834860 PASS on Go 1.19/1.23 |

## Resolved self-update failure

Revision `a0b774c8` failed self-update during `go test ./...` because
`vscode_tasks_menu_go/internal/server/patch_handoff_state_test.go` tried to read
`../../../TASKDECK_PATCH_HANDOFF.md`. Self-update intentionally stages only the Go module plus a
bounded set of runtime-support root files, so repository documentation is absent there.

Resolution:

- `5b9a37e2`: remove the repository-doc assertion from Go runtime tests;
- `572368a1`: recreate it as a repository-level Python test;
- `3dbf06fb`: run that guard in GitHub Actions from repository root and trigger CI on the relevant docs;
- `afbb82be`: add a self-update regression test preventing Go tests from depending on repository-only Patch docs.

This failure is considered fixed on `19c2ebc3` or a later descendant. GitHub Actions run `36087734095` PASSed both Go 1.19 and 1.23, including the dedicated `Self-update staged Go tests` step that reproduces the updater's docs-free staged source layout.

## Resolved stale broker Patch protocol failure

Installed-runtime smoke exposed a second issue after the self-update staging fix: History remained at
`Loading…` and Health reported `Native Health protocol state unavailable`.

Root cause:

- the long-lived session broker intentionally survives daemon/self-update handoff to preserve
  terminal processes, IDs and scrollback;
- older broker processes can still use broker `protocol_version=1` but predate
  `patch_protocol_events` / `patch_protocol_commands` capabilities;
- `EnsureClient()` therefore reused a healthy-but-legacy broker;
- `broker.Client.Start()` correctly stripped unsupported protocol flags, which left native Patch
  sessions without FD3/FD4 state even though the newly updated web daemon supported them.

The fix does **not** kill/restart that legacy broker, because doing so would lose user terminal
sessions. Instead, `broker.CompatibilityService` keeps ordinary terminal/task sessions on the
existing broker and routes only executions requiring missing Patch protocol capabilities to a
daemon-local `session.Manager`. The wrapper also routes metadata, PTY streaming, protocol state
and protocol commands by session ID, so explicit terminal evidence continues to work.

Related bootstrap noise was fixed independently: appearance and legacy project-state migration now
wait for `taskmenu:tasks` before reading the workspace, avoiding
`app.taskData.workspace` dereferences while `taskData` is still null.

Verification:

- `0ef148e8`: bootstrap guard;
- `973af90b`: daemon compatibility-service wiring;
- `f22cf21e` + `84900727`: real FD3 Health event integration test through a legacy-broker fallback;
- `114a9222`: final test correction;
- GitHub Actions run `36088980153`: PASS on Go 1.19 and 1.23, including
  `Self-update staged Go tests`, full `go test ./...`, vet and build.

## Native tab, file actions, and selectable terminal UI

Installed-runtime feedback changed the intended desktop interaction without changing Python policy:

- Native Patch is now a first-class `Patch Tool` tab in the normal TaskDeck tab strip.
  Switching to a terminal or editor hides the Patch pane but leaves its tab available for switching back.
- Patch no longer uses a fixed overlay that obscures the tab strip.
- History/artifact **Open** now opens through `TaskMenuEditor.openFile()`; activating the editor
  deactivates the Patch pane, so the file is actually visible.
- History/artifact rows now include **Copy path**, which copies the full absolute path rooted at the
  current workspace.
- Settings now has **PATCH TOOL → Interface** with:
  - **Native UI** (default): protocol-backed native tab, backing PTY remains headless;
  - **Terminal (legacy)**: launches the historical terminal UI. The server disables protocol event/
    command channels and does not set `TASKDECK_PATCH_NATIVE_*`, so Python falls back to its
    original terminal selector/history behavior.
- Changing to Terminal mode closes/hides the native Patch tab; opening Patch in Terminal mode
  launches the Queue/selector directly as a normal terminal tab.

Verification: GitHub Actions run `36090834860` PASS on Go 1.19 and Go 1.23, including
`Self-update staged Go tests`, full tests, vet and build.

## Resolved grouped-menu bootstrap crash

After adding the Patch Tool interface selector, `menus.js` created its workspace-scoped localStorage
key during module evaluation. Because `app.js` uses top-level await while loading `/api/tasks`,
sibling feature modules may execute before `app.taskData` is populated. That produced:

`TypeError: can't access property "workspace", app.taskData is null`

Fix `945f9514` makes `installHeaderMenus()` fail closed until
`app.taskData?.workspace` exists, then retries exactly once on `taskmenu:tasks`. The module no
longer contains a direct `app.taskData.workspace` dereference. A repository search also found no
remaining direct `taskData.workspace` access under `web/featuremods/`.

GitHub Actions run `36091412442` PASS on Go 1.19 and 1.23, including staged self-update tests,
full tests, vet and build.

## Phase 2E.2 UX smoke fixes: queue clarity, run liveness, tab persistence

Installed smoke found three native-UI defects and they are now source/CI fixed:

1. **Duplicate Queue presentation**
   - The same runnable item was visible once in Queue summary (including a destructive Delete action)
     and again in the authoritative `queue_selection` prompt.
   - While a queue prompt is active, the summary now keeps only overview counts/warnings; the
     actionable item list exists only in **Choose PATCH/COLLECT work**.
   - This removes the misleading second Delete surface.

2. **Running COLLECT looked frozen**
   - The Python COLLECT supervisor already emits typed `progress` heartbeats (default ~0.8s)
     containing phase, elapsed time, output-line count and detail.
   - Native Running now also renders a per-poll liveness line with elapsed time, protocol event
     count and last-activity age. If typed progress is present it adds phase/output counts.
   - After 5s without a new protocol event it explicitly reports the quiet interval; after 10s it
     highlights the liveness line as stale. It does not infer terminal text or claim that the process
     is hung.
   - `b3bd40e0` adds an end-to-end Unix integration test through the real nested path:
     TaskDeck entry writer → child relay → dispatcher `_run_foreground_child` → real COLLECT
     progress supervisor → FD3 → outer relay. The test requires multiple RUNNING progress events,
     elapsed time, search/zip phase evidence, final PASS and contiguous re-sequencing.

3. **Patch tab disappeared when switching to another tab**
   - Root cause was Activity Bar code retaining old sidebar semantics and calling
     `TaskMenuPatchPanel.close()` whenever another view became active.
   - Activity Bar now calls `deactivate()`, which hides only the Patch pane. The `Patch Tool`
     tab remains available for switching back.
   - Only the explicit Patch `×` close action (or switching to Terminal legacy mode) hides the tab.

Verification: GitHub Actions run `36092673478` PASS on Go 1.19 and Go 1.23, including JavaScript
syntax, repository handoff guard, exact self-update staged-source tests, full tests, vet and build.

## Phase 2E.2 UX: per-line warnings, selector delete, paired artifacts, parallel COLLECT

Installed feedback added four native UX requirements without changing Python authority:

1. **Queue warnings are line-oriented**
   - Queue warnings are no longer concatenated with ` | `.
   - Each skipped/non-runnable queue warning is rendered on its own line under the warning count.

2. **Delete is available on the authoritative selector row**
   - Every item in **Choose PATCH/COLLECT work** gets a right-aligned **Delete** button when Python
     advertises the existing `queue_actions: ["delete"]` capability.
   - The button uses the same prompt-bound `queue-delete` command and refresh contract as the
     earlier Queue summary action; no browser-side filesystem delete was introduced.

3. **ZIP/TXT variants are grouped as one logical result**
   - Live protocol artifacts group `collect_result_zip/text`, `fail_handoff_zip/text`, and
     `ai_sync_zip/text` by logical family + item.
   - History artifacts group matching `.zip` / `.txt` stems the same way.
   - Each format keeps separate, explicit actions: **ZIP Download**, **ZIP Copy path**,
     **TXT Download**, **TXT Copy path**. **Open TXT** appears only on TXT.

4. **Multiple COLLECT requests can run concurrently in native UI**
   - Python's existing safety invariant remains unchanged: one dispatcher invocation accepts at most
     one COLLECT and cannot mix COLLECT with PATCH.
   - Python now advertises `parallel_collect_processes = {strategy: independent_processes, max: 16}`.
   - When 2–16 COLLECT items are selected, TaskDeck cancels only the selector prompt and launches
     one independent reserved `task_id=-1` COLLECT worker per request.
   - Each worker has its own typed progress, item lifecycle and artifact events. The native dashboard
     shows status, elapsed time, phase, output-line count, protocol activity, explicit Terminal
     evidence, and its own grouped result ZIP/TXT.
   - Direct COLLECT workers emit structured artifact events from the existing COLLECT result
     metadata; browser code never parses terminal output to discover results.

Checkpoints:
- `1c3ec879` — per-line warnings + per-item selector Delete.
- `8d7c0819` — pair ZIP/TXT variants in live Artifacts and History.
- `21eef976` — prompt-bound server/Python independent COLLECT worker contract and direct artifact events.
- `d16ee44d` — multi-COLLECT selection + native per-worker dashboard.
- `bc2137bd` / `f9b4737b` — refresh product/source gates for grouped artifacts and generic explicit evidence path.

Verification: GitHub Actions run `36095759264` PASS on Go 1.19 and Go 1.23, including
Patch entry routing/direct-COLLECT protocol tests, JavaScript syntax, exact self-update staged-source
tests, full tests, vet and build.

## Phase 2E.2 UX: reopen Queue while runs remain active

Installed feedback exposed a lifecycle gap in the first parallel-COLLECT UI: selecting several COLLECTs
at once worked, but once any PATCH/COLLECT was already running there was no path back to a fresh
Queue without abandoning the run view.

The native UI now treats execution and queue selection as independent surfaces:

- **Back to Queue / Add more** is available immediately while a run is active; it does **not** stop
  that session.
- The foreground session is moved into the persistent in-page **Active runs** dashboard and gets its
  own protocol polling, status, progress, artifacts and explicit terminal-evidence action.
- TaskDeck starts a fresh headless Queue session, so newly arrived files under `patchs/` can be
  discovered while earlier work continues.
- **Refresh Queue** stops/replaces only the waiting Queue selector session. Active PATCH/COLLECT
  sessions are untouched.
- While at least one run is still active, the fresh Queue enters conservative **add-mode**:
  - PATCH checkboxes are locked;
  - an item already running is locked and cannot be Run/Delete selected again;
  - additional COLLECT requests remain selectable.
- A single additional COLLECT is allowed through the same prompt-bound independent-worker endpoint
  used for multi-COLLECT fan-out. The server accepts 1..16 advertised COLLECT indexes, while Python's
  one-COLLECT-per-process invariant remains unchanged.
- When no active run remains, a refreshed Queue restores the normal PATCH/COLLECT selection rules.

Safety rationale:
- PATCH mutation remains serialized by the existing Python project mutation lock.
- The browser does not start a second PATCH in add-mode.
- COLLECT stays read-only and is launched only from a current Python-advertised, prompt-bound Queue
  item.
- Returning to Queue never sends `/stop` to the running execution session.

Checkpoints:
- `e2b67fe9` — allow one prompt-bound independent COLLECT worker.
- `0e7e38c0` — preserve foreground/background active runs, add fresh Queue and add-mode locking.
- `a2251226` / `1beb18a0` — refresh source/product gates for the new lifecycle.

Verification: GitHub Actions run `36096780483` PASS on Go 1.19 and Go 1.23, including
launcher/entry routing, JavaScript syntax, handoff guard, exact self-update staged-source tests,
full tests, vet and build.

## Phase 2E.2 UX correction: paired artifacts are one visual row

Installed History smoke showed that the prior ZIP/TXT grouping was only logical: the renderer still
created one nested variant row for TXT and one nested variant row for ZIP. This still looked like
two results.

The renderer now enforces one visual row per paired result/handoff:

- Exact pairing is by artifact stem, not only by family/item.
- A TXT+ZIP pair displays one combined path, e.g. `.../RESULT_NAME.{txt|zip}`.
- The single row exposes format-specific actions in a clear order:
  **TXT Download**, **ZIP Download**, **TXT Copy path**, **ZIP Copy path**, **Open TXT**.
- **Open TXT** is emitted once and always targets the TXT member.
- The same combined-row renderer contract applies to live protocol Artifacts and native History.
- Single-file artifacts (run summary, batch log, request archive, support ZIP, etc.) remain individual
  rows.

Checkpoints:
- `41e2eb39` — replace per-variant subrows with a combined artifact/history row and exact-stem pairing.
- `9c91cc84` / `2377149e` — refresh source contracts for combined download/copy/Open TXT helpers.

Verification: GitHub Actions run `36104471672` PASS on Go 1.19 and Go 1.23, including
JavaScript syntax, staged self-update tests, full tests, vet and build.

## Phase 2E.2 UX correction: History Terminal opens real terminal browser

Installed smoke showed that the native History **Terminal** button materialized the backing native
History PTY. That session intentionally runs the structured native `report` route, so its PTY may
contain only the TaskDeck launch header instead of the interactive terminal History browser.

The explicit fallback is now separated correctly:

- Native History continues to use `report` with FD3/FD4 and
  `TASKDECK_PATCH_NATIVE_HISTORY=1`.
- Python exposes a bounded read-only `history` dispatcher command that calls
  `_history_browser(root)` directly.
- Server maps `patch_mode=history, patch_ui=terminal` to `history`, not `report`.
- Native **History → Terminal** creates a new explicit legacy terminal History session and
  materializes that session.
- It does not materialize the native History backing `activeSessionId`.
- Other explicit terminal-evidence buttons retain their existing backing-session evidence behavior.

Expected installed command after clicking History → Terminal:

`python_patch_entry.py --project-root <workspace> -- history`

not:

`python_patch_entry.py --project-root <workspace> -- report`

Checkpoints:
- `3f25556a` — add explicit Python `history` route, terminal-only server mapping, and separate UI launcher.
- `1134bdb2` — entry/server/UI regression tests.
- `e934b3cd` — update product acceptance/navigation gates for the explicit History legacy session.

Verification: GitHub Actions run `36105569250` PASS on Go 1.19 and Go 1.23, including
entry routing, JavaScript syntax, exact staged self-update tests, full tests, vet and build.

## Non-regression invariants

- Python remains authoritative for Patch Tool policy/business logic.
- Do not parse console text to drive native behavior.
- Direct `taskdeck patch ...` terminal usage remains supported.
- PTY/backing session may remain available for compatibility/evidence.
- In **Native UI** mode, Patch actions must not create ordinary terminal tabs by default.
- Switching from the active Patch pane to a terminal/editor/Activity Bar view must deactivate the pane but preserve the `Patch Tool` tab until explicit close.
- In Native UI mode, a terminal tab may be materialized only by an explicit evidence/fallback action.
- **History → Terminal** is an explicit legacy History browser session (`history` command), not a materialized native History backing PTY (`report`).
- Parallel COLLECT workers must remain independent `task_id=-1` sessions; do not weaken Python's one-COLLECT-per-invocation safety contract.
- **Back to Queue / Add more** must never stop an active execution session; only a waiting Queue selector may be replaced by Refresh Queue.
- While active executions exist, Queue add-mode must not permit launching another PATCH or re-running/deleting an already-running item.
- Native parallel COLLECT state/results must come from typed protocol events, never console-text parsing.
- In **Terminal (legacy)** mode, materializing a normal terminal tab is intentional and required; that session must not opt into native FD3/FD4/Python routes.
- Patch start must fail closed if backend metadata is not reserved `task_id=-1` or if the new session is already materialized in `app.views`.
- Never infer Phase 2 completion from the existence of native renderers or protocol endpoints.

## Commit discipline

For the remaining Phase 2 work, commit each independently testable slice directly to `main`.
Do not accumulate multiple recovery-critical changes before committing. Update this handoff and
`TASKDECK_PATCH_ADDON_PLAN.md` whenever a phase/checkpoint state changes.

## Next action

Phase 2E.1b runtime invariant hardening is committed. Continue Phase 2E.2 installed-runtime browser smoke. If smoke exposes a UI/runtime defect, fix it in
a small commit, update this handoff, rerun CI, and repeat the smoke. Only after the real browser
behavior passes all Phase 2E.2 checks should Phase 2F closeout begin.
