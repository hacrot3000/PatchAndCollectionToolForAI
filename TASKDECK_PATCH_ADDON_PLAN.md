# TaskDeck Patch Add-on Integration Plan

Status: **IN PROGRESS**

This file is the recovery/source-of-truth document for integrating Python Patch Tool into TaskDeck.
Every implementation commit for this work must update this file so development can resume safely after an interruption.

## Goals

1. Keep Python Patch Tool as the existing Python engine. Do **not** rewrite its business logic in Go.
2. Make Patch Tool a first-class TaskDeck add-on/feature instead of a project-local VS Code task.
3. Install TaskDeck and the Patch Tool runtime together as one versioned global release.
4. Keep all project-specific Patch Tool data in the project:
   - `patchs/`
   - `artifacts/patch_tool/`
   - `artifacts/ptv_to_ai/`
   - project-local Patch Tool configuration such as `.python_patch_tool.json`
5. Preserve terminal usage with `taskdeck patch ...`.
6. Add a dedicated Patch panel to TaskDeck.
7. Introduce a versioned machine protocol so the web UI can become fully native without duplicating Patch Tool policy/business logic in Go.
8. Keep legacy `tools/run_python_patches.sh` compatibility during migration.

## Architectural boundary

### Global application/runtime

Target layout:

```text
~/.local/bin/taskdeck
    -> ~/.local/lib/taskdeck/current/taskdeck

~/.local/lib/taskdeck/
├── current -> releases/<revision>/
└── releases/
    └── <revision>/
        ├── taskdeck
        └── patchtool/
            ├── python_patch_entry.py
            ├── run_python_patches.sh
            └── _patch_lib/
```

TaskDeck binary and Patch Tool runtime must come from the same release revision.

### Per-project data

```text
<project>/
├── .vscode/
│   ├── tasks.json
│   └── vscode_tasks_menu.ini
├── patchs/
├── artifacts/
│   ├── patch_tool/
│   └── ptv_to_ai/
└── .python_patch_tool.json
```

No Patch Tool runtime is required under `<project>/tools` after migration.

## Runtime principles

- Python remains authoritative for queue discovery, dependency/failure policy, validation, execution, rollback, COLLECT, history, recovery and artifact semantics.
- Go/TaskDeck is the process supervisor, web renderer, user-input frontend and artifact viewer.
- Terminal frontend remains supported permanently.
- Web frontend must not infer Patch Tool policy from console text.
- Structured machine data must use a separate channel from stdout/stderr.

## Protocol direction

Planned Protocol v1:

- human channel: stdin/stdout/stderr, unchanged;
- machine event channel: JSONL events from Python to TaskDeck;
- machine command channel: JSONL commands/responses from TaskDeck to Python;
- protocol is additive: terminal mode works without machine channels.

Initial event families:

```text
hello
queue_snapshot
prompt
item_started
progress
artifact
item_finished
run_finished
error
```

TaskDeck must not parse internal `history/*.json` schemas as its long-term API; Python exposes stable protocol/bridge views.

## Phases

### Phase 0 — Recovery plan and invariants
Status: **DONE**

- [x] Create this roadmap.
- [x] Record target architecture and non-regression invariants.

### Phase 1A — Canonical Python entrypoint
Status: **DONE**

- [x] Add `python_patch_entry.py` as the canonical router.
- [x] Make POSIX and PowerShell launchers thin compatibility wrappers.
- [x] Preserve zero-arg, report/run/resume/plan, collect, automation and direct runner semantics.
- [x] Add routing-contract tests and CI syntax/compile checks.

### Phase 1B — TaskDeck CLI integration
Status: **DONE**

- [x] Add `taskdeck patch [args...]`.
- [x] Resolve bundled Patch Tool runtime first, with project-entry/legacy-launcher fallback during migration.
- [x] Pass the current TaskDeck workspace as `--project-root`.
- [x] Preserve interactive terminal behavior by inheriting stdin/stdout/stderr and child exit code.

### Phase 1C — Versioned global release layout
Status: **DONE**

- [x] Installer creates `~/.local/lib/taskdeck/releases/<revision>/taskdeck`.
- [x] Installer places the Patch Tool runtime beside it under `patchtool/`.
- [x] Installer atomically switches `~/.local/lib/taskdeck/current`.
- [x] Installer keeps `~/.local/bin/taskdeck` as the stable symlink entrypoint.
- [x] Self-update installs/switches versioned releases and daemon handoff resolves the stable global symlink.
- [x] Installer never removes old release directories, so in-flight processes keep their runtime.

### Phase 1D — Dedicated Patch panel
Status: **DONE**

- [x] Add Patch icon/view to the TaskDeck Activity Bar.
- [x] Backend launches Patch Tool without a `.vscode/tasks.json` entry through bounded built-in session modes.
- [x] Phase 1 panel exposes Queue/Resume/History/Plan and renders the launched process through the existing PTY/session view.
- [x] Hide only the definite zero-argument legacy `tools/run_python_patches.sh` duplicate; parameterized/special tasks remain visible.

### Phase 1E — Legacy project runtime migration
Status: **DONE**

- [x] Detect project-local Patch Tool runtime under `tools/`.
- [x] Compare SHA-256 of candidate managed files against the bundled runtime from the active TaskDeck release.
- [x] Any modified, symlinked or unknown file inside the legacy runtime blocks cleanup.
- [x] Cleanup flow checks and skips the PatchAndCollectionToolForAI source repository before runtime inspection.
- [x] Migration primitive replaces verified legacy launchers with tiny `taskdeck patch` compatibility shims and removes only revalidated runtime payload.
- [x] Interactive `--cleanup-legacy` and normal global-startup cleanup can migrate only a verified-safe runtime; unsafe runtime is retained.

### Phase 1.5 — Protocol v1
Status: **DONE**

- [x] Add optional POSIX event FD/pipe to Python entrypoint via `TASKDECK_PATCH_EVENT_FD`; no FD keeps the historical `execv` path.
- [x] Define optional POSIX command FD contract via `TASKDECK_PATCH_COMMAND_FD`; it requires a separate event FD.
- [x] Session manager has disabled-by-default FD4 command plumbing with bounded/versioned envelope validation.
- [x] Broker carries command capability/wire/internal writes with old-broker fallback.
- [x] TaskDeck retains the active Python prompt, exposes only a bounded prompt-response API, and enables commands for built-in Patch sessions.
- [x] Define versioned JSONL envelope and initial `hello/run_started/run_finished/error` lifecycle events.
- [x] Emit Python-owned `queue_snapshot` with stable item/warning/count fields before dispatcher-backed runs.
- [x] Emit Python-owned item_started/item_finished events only around actual payload execution.
- [x] Emit Python-owned artifact events only for verified user-facing files under project artifacts/.
- [x] Emit bounded Python-owned COLLECT progress events from the existing progress supervisor while terminal UI remains authoritative.
- [x] Child Python processes emit into an internal pipe; the entrypoint relays them onto the TaskDeck event channel with one global sequence.
- [x] Add protocol envelope/lifecycle/version tests.
- [x] Session manager supports an opt-in FD 3 event pipe and bounded protocol state while PTY remains unchanged.
- [x] TaskDeck retains up to 4096 item lifecycle entries and Patch panel shows native running/completed status with polling stopped when hidden.
- [x] TaskDeck retains up to 256 validated artifact entries and Patch panel exposes native Download/Open actions from structured artifact events.
- [x] TaskDeck retains the latest validated progress event and Patch panel renders native phase/status/elapsed/output/detail without parsing console text.
- [x] Broker advertises `patch_protocol_events`, carries the opt-in flag and exposes protocol state; old brokers fall back to PTY-only.
- [x] Patch panel polls bounded protocol state and renders a native queue summary while PTY remains the authoritative interactive surface.
- [x] Patch panel renders Python-owned queue_selection prompts and submits only bounded select/cancel responses; PTY remains available as fallback.

### Phase 2 — Native Patch web UI
Status: **IN PROGRESS**

- [x] Native Queue and Failed views.
  - [x] Phase 2A.1 Python-owned stable Queue/Failed protocol projection.
  - [x] Phase 2A.2 Web Queue/Failed views from queue_snapshot.
- [x] Native Inspect / Preview / Validate.
  - [x] Phase 2B.1 Python-owned item_action/action_result protocol contract over the active Queue prompt.
  - [x] Phase 2B.2 TaskDeck backend endpoint/state for bounded native item actions.
    - [x] Phase 2B.2a session action_result state and stale-prompt write gate.
    - [x] Phase 2B.2b narrow public item-action endpoint.
  - [x] Phase 2B.3 Patch panel Inspect/Preview/Validate controls and result viewer.
- [x] Native Run selection and confirmation prompts.
  - [x] Phase 2C.1 audit confirms the existing native queue_selection select/cancel response is the normal-run confirmation boundary; there is no second terminal confirmation after selection.
  - [x] Existing non-interactive selection remains separately gated by non_interactive_confirmed and PATCH-only queue rules.
- [x] Native Running/progress view with console as secondary evidence.
  - [x] Phase 2D.1 Queue execution switches to native lifecycle/progress/artifact view; PTY is retained and explicitly available as secondary evidence.

#### Phase 2C.1 interaction audit

- **Normal Queue run:** native already. The Python-owned `queue_selection` prompt + `select/cancel` response is the explicit run-selection/confirmation boundary. After a valid selection, Python performs planner/resource/preflight/transaction safety gates and reaches `execute_items()` without another stdin/key confirmation.
- **Configured non-interactive run:** intentionally not a web prompt. It executes only when project config has `non_interactive_confirmed=true`, and automatic selection remains PATCH-only; otherwise Python falls back to the normal prompt.
- **Queue item delete:** terminal-only mutation/confirmation today; classify as Queue management rather than Run confirmation. Native support must preserve Python-owned delete safety semantics.
- **Smart Resume choices / failed-row multi-select / collect_failed / delete_failed:** terminal-only today and belong to the dedicated Native Resume/recovery phase.
- **History/report pagination and menus:** terminal-only today and belong to Native History/report browser.

- [ ] Native Resume/recovery.
  - [x] Phase 2E.1 Python-owned Resume snapshot/action contract + TaskDeck retained state; terminal Resume remains authoritative.
  - [x] Phase 2E.2 narrow Resume action endpoint and protocol prompt execution.
    - [x] Phase 2E.2a Python opt-in resume_action prompt/command path.
    - [x] Phase 2E.2b TaskDeck endpoint, final-write gate and built-in Resume opt-in.
  - [x] Phase 2E.3 native Resume/recovery web controls.
- [ ] Native History/report browser.
  - [x] Phase 2F.1 Python-owned bounded History list/report projection + additive history_snapshot event.
  - [ ] Phase 2F.2 native History detail command/state/endpoint.
  - [ ] Phase 2F.3 native History/report web browser.
- [ ] Native artifact actions.
- [ ] Remove default dependence on terminal rendering only after feature parity is proven.

## Non-regression gates

Every phase must preserve:

- existing terminal Patch Tool workflow;
- PATCH/COLLECT queue semantics;
- failed/unresolved state;
- History and report behavior;
- Smart Resume/recovery;
- fail handoff and AI-facing artifacts;
- search coverage/incomplete semantics;
- database SELECT safety boundary;
- Git-safe behavior;
- current project data paths;
- Linux CI for supported Go versions;
- existing Python Patch Tool semantic/self-tests relevant to changed surfaces.

## Progress log

| Step | Status | Commit | Notes |
| --- | --- | --- | --- |
| Phase 0 roadmap | DONE | 0ac231f2 | Architecture/recovery plan created before runtime changes. |
| Phase 1A canonical entrypoint | DONE | 0e871ed1 | Routing moved from shell/PowerShell to one Python entrypoint; wrappers remain compatible. |
| Phase 1A CI path correction | DONE | 14adf537 | Fix CI working-directory path for the new Python routing contract; no runtime behavior change. |
| Phase 1B TaskDeck CLI | DONE | 02b72b94 | Added shared Patch runtime resolver and `taskdeck patch`; bundled runtime wins, legacy project launcher remains a migration fallback. |
| Phase 1C.1 versioned installer | DONE | 2e909a70 | Installer stages TaskDeck + Patch runtime as one immutable release and atomically switches `current` and the PATH symlink. |
| Phase 1C.1 shell interpolation correction | DONE | 02a80f37 | Removed accidental literal backslashes before Bash `${...}` expansions introduced by commit orchestration; no design change. |
| Phase 1C.2a self-update release primitives | DONE | 0e2fc6a0 | Added versioned release stage/install helpers, atomic symlink switching, Patch runtime source staging, and old-release preservation tests; main self-update flow not switched yet. |
| Phase 1C.2a support-file mode test correction | DONE | 52af3a60 | Test now distinguishes executable launchers from non-executable Python test/support files; runtime behavior unchanged. |
| Phase 1C.2b self-update wiring | DONE | 23ce9d21 | Self-update now requires a complete versioned TaskDeck + Patch release, installs it atomically, and hands the daemon off through the stable global entrypoint. |
| Phase 1D.1 built-in Patch session backend | DONE | bc8e5b4a | Added bounded `kind:patch` session modes backed by the shared runtime resolver; Patch sessions use reserved task id -1 and do not depend on tasks.json. |
| Phase 1D.2 Patch Activity Bar panel | DONE | 0d32d411 | Added Patch icon/panel with Queue/Resume/History/Plan actions; sessions use the existing TaskDeck PTY renderer. |
| Phase 1D.3 legacy task de-dup | DONE | ae49e6ca | Task list hides only a zero-argument, exact legacy tools/run_python_patches.sh duplicate; customized/parameterized tasks remain visible. |
| Phase 1E.1 migration safety plan | DONE | e9506386 | Added fail-closed SHA-256 comparison of legacy tools runtime against the active bundled release; modified/symlinked/unknown files block cleanup. |
| Phase 1E.2a guarded migration apply | DONE | 6a7cbf1c | Revalidates hashes immediately before cleanup, installs compatibility launchers first, removes only verified runtime files, and keeps unrelated tools files. |
| Phase 1E.2a migrated-shim recognition | DONE | 00072c0d | Planner treats TaskDeck-generated compatibility launchers as already migrated instead of locally modified legacy runtime. |
| Phase 1E.2b cleanup wiring | DONE | 19f19167 | Wired verified Patch runtime migration into interactive/global legacy cleanup, kept unsafe runtimes, and skipped the source repository. |
| Phase 1.5A protocol event channel | DONE | 150f26b8 | Added opt-in protocol v1 JSONL event FD with lifecycle events; no-protocol terminal path remains execv-compatible. |
| Phase 1.5B Python queue snapshot | DONE | 840ee91d | Protocol bridge calls authoritative dispatcher queue discovery directly and emits stable queue_snapshot data before terminal execution. |
| Phase 1.5C.1a session protocol state | DONE | 3ae480a1 | Added opt-in FD 3 transport in the session manager and bounded latest-event/queue-snapshot state without changing PTY output. |
| Phase 1.5C.1b broker protocol capability | DONE | 5fd3a3d6 | Added capability-gated broker wire/API for protocol events with old-broker PTY-only fallback. |
| Phase 1.5C.1c public Patch protocol API | DONE | 07b11200 | Built-in Patch sessions now request protocol events and TaskDeck exposes optional per-session protocol state with legacy fallback. |
| Phase 1.5C.2 native queue summary | DONE | e24e619e | Patch panel renders bounded Python-owned queue_snapshot data, falls back to PTY-only, and never parses terminal text. |
| Phase 1.5D.1 command channel contract | DONE | 70eb2dc7 | Added bounded versioned JSONL CommandReader and optional command FD pass-through contract; TaskDeck does not enable it yet. |
| Phase 1.5D.2a session command pipe | DONE | 1a6aa5f9 | Added opt-in FD4 command pipe, bounded envelope validation and session write primitive; Patch sessions still leave it disabled. |
| Phase 1.5D.2b broker command plumbing | DONE | 74279462 | Added capability-gated command flag and internal broker command endpoint; built-in Patch sessions remain command-disabled. |
| Phase 1.5E.1 child event relay | DONE | 6b7ab13b | Child protocol events now use an internal pipe and are re-sequenced by the entrypoint before reaching TaskDeck. |
| Phase 1.5E.1 protocol test discovery correction | DONE | 13c2848a | Moved unittest.main() after ProtocolContractTests so CI executes the complete protocol contract suite instead of routing tests only. |
| Phase 1.5E.1 lifecycle test scope correction | DONE | 7c146ccf | Updated the lifecycle-only protocol test to use a non-queue route; queue_snapshot behavior remains covered by its dedicated test. |
| Phase 1.5E.2 queue selection prompt contract | DONE | c81046a2 | Added Python-owned queue_selection prompt/prompt_response handling with strict prompt-id/index/COLLECT validation and terminal fallback on protocol failure. |
| Phase 1.5E.3 public prompt response backend | DONE | 3b047a68 | Stores active prompt state, enables built-in command channels, exposes only bounded prompt responses, and rejects stale/double responses before FD4 writes. |
| Phase 1.5E.3 test import correction | DONE | 6d571371 | Added the missing os import required by the public prompt-response contract test; no runtime behavior change. |
| Phase 1.5E.4 native queue selection UI | DONE | 3160762d | Patch panel renders queue_selection directly from Python prompt data and submits bounded select/cancel responses, with PTY fallback preserved. |
| Phase 1.5F.1 item lifecycle events | DONE | dff4ab05 | Dispatcher emits item_started/item_finished only for actual PATCH/COLLECT payload execution; preflight/blocked/duplicate skips are not misreported as started. |
| Phase 1.5F.2 native item lifecycle state | DONE | 87efcaff | TaskDeck retains bounded item lifecycle state and Patch panel displays running/completed item status while PTY remains available. |
| Phase 1.5F.2 UI contract test correction | DONE | 8b087ceb | Updated the stale fixed-40-iteration assertion to the new bounded lifecycle polling contract; no runtime behavior change. |
| Phase 1.5F.3a Python artifact events | DONE | 858f2293 | Emits fail handoff, AI sync and COLLECT result artifacts only after validating a real non-link file under project artifacts/; internal/history files are not exposed. |
| Phase 1.5F.3b native artifact state/actions | DONE | 033af150 | TaskDeck retains a bounded validated artifact list; Patch panel offers existing safe download plus text-editor open actions without parsing terminal/history output. |
| Phase 1.5G.1 Python COLLECT progress events | DONE | e5a55760 | COLLECT progress supervisor emits bounded phase/status/elapsed/output/detail events over the inherited protocol FD; Go never parses console text. |
| Phase 1.5G.2 native progress state/UI | DONE | 851611a4 | TaskDeck validates and retains only the latest bounded progress event; Patch panel renders native progress while PTY remains unchanged. |
| Phase 2A.1 Queue/Failed protocol projection | DONE | 44ea3cce | Python dispatcher projects new/failed grouping and bounded failure summaries from the same unresolved policy used by the terminal selector; Go does not read history schemas. |
| Phase 2A.2 native Queue/Failed web views | DONE | dd41c7b0 | Patch panel separates Queue/Failed strictly by Python snapshot group and renders bounded failure summaries; legacy snapshots remain Queue-only without inferred failure policy. |
| Phase 2B.1 native item action protocol | DONE | c9fe4dfb | Active queue prompts accept bounded inspect/preview/validate item_action commands, execute the existing read-only Python runner path, emit correlated action_result events, then continue waiting for selection. |
| Phase 2B.2a action result state/gate | DONE | 01e0936f | Session state validates/retains latest action_result, clears it on a new prompt/run, and binds item_action to the still-active prompt at the final FD4 write gate. |
| Phase 2B.2b narrow item-action endpoint | DONE | 3bb89fe6 | Public API accepts only prompt-bound inspect/preview/validate + index, validates PATCH-only prompt items, generates action_id server-side and never exposes raw protocol command writes. |
| Phase 2B.3 native item action UI | DONE | 39f59d45 | Queue/Failed PATCH rows expose only Python-advertised Inspect/Preview/Validate actions; result polling is correlated by action_id and rendered from structured action_result state. |
| Phase 2C.1 normal-run interaction audit | DONE | 6d7accbf | Verified queue_selection select/cancel is the only interactive normal-run confirmation boundary; added a regression contract preventing a second terminal-input gate before execute_items. Resume/delete/history interactions are explicitly deferred to their owning phases. |
| Phase 2D.1 native Running primary view | DONE | 12a9dcdc | Queue PTY is attached without activation; after native selection the Patch panel prioritizes lifecycle/progress/artifacts, with explicit terminal-evidence fallback and post-finish return to Queue. |
| Phase 2E.1 Resume projection/contract | DONE | 195ebd3a | Python emits a stable resume_snapshot from the exact Smart Resume boundary, shares action definitions with terminal Resume, defines resume_action contract, and TaskDeck retains the snapshot without changing terminal interaction. |
| Phase 2E.2a native Resume Python command path | DONE | fee4eafa | Python handles prompt-bound resume_action only when TASKDECK_PATCH_NATIVE_RESUME=1 and both protocol channels exist; malformed/unavailable native input falls back to the historical terminal Resume path. |
| Phase 2E.2a test fixture correction | DONE | 9f8883bb | Resume protocol tests now use an existing temp project and explicit recovery-row binding mocks; runtime behavior is unchanged. |
| Phase 2E.2b TaskDeck Resume endpoint/gate | DONE | 36d9cb75 | Built-in Resume explicitly opts into native mode; public API accepts only active-prompt advertised Resume actions, and session final-write gate consumes the prompt to reject stale/double submissions. |
| Phase 2E.3 native Smart Resume UI | DONE | 2c36121d | Patch panel renders Python-owned Resume snapshot/options/failed-item capabilities, submits only through /resume-action, confirms destructive delete, and preserves PTY fallback/history handoff. |
| Phase 2E.3 UI contract test correction | DONE | 1b3811df | Updated the legacy Running-view PTY activation assertion for native Resume; runtime behavior is unchanged. |
| Phase 2F.1 History/report projection | DONE | 235847fb | Python exposes bounded meaningful-run summaries and one-run report detail views with verified project-relative files; report route emits history_snapshot additively and Go never sees raw history JSON. |
| Phase 2F.1 read-only pin loader correction | DONE | this commit | History projections no longer materialize artifacts/patch_tool merely to read PINNED_RUNS; Pin/Unpin writes keep the existing mutating path. |

## Next action

Continue **Phase 2F.2**: add an opt-in native History detail command that requests one run_id, emits history_report, and TaskDeck retains it behind a narrow prompt-bound/read-only endpoint while terminal History remains unchanged.
