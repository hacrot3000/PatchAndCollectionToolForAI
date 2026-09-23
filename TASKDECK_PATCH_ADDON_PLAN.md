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
Status: **IN PROGRESS**

- [x] Add optional POSIX event FD/pipe to Python entrypoint via `TASKDECK_PATCH_EVENT_FD`; no FD keeps the historical `execv` path.
- [x] Define optional POSIX command FD contract via `TASKDECK_PATCH_COMMAND_FD`; it requires a separate event FD.
- [x] Session manager has disabled-by-default FD4 command plumbing with bounded/versioned envelope validation; built-in Patch sessions do not enable it yet.
- [x] Define versioned JSONL envelope and initial `hello/run_started/run_finished/error` lifecycle events.
- [x] Emit Python-owned `queue_snapshot` with stable item/warning/count fields before dispatcher-backed runs.
- [ ] Emit item/progress/artifact events while terminal UI remains authoritative.
- [x] Add protocol envelope/lifecycle/version tests.
- [x] Session manager supports an opt-in FD 3 event pipe and bounded protocol state while PTY remains unchanged.
- [x] Broker advertises `patch_protocol_events`, carries the opt-in flag and exposes protocol state; old brokers fall back to PTY-only.
- [x] Patch panel polls bounded protocol state and renders a native queue summary while PTY remains the authoritative interactive surface.

### Phase 2 — Native Patch web UI
Status: **TODO**

- [ ] Native Queue and Failed views.
- [ ] Native Inspect / Preview / Validate.
- [ ] Native Run selection and confirmation prompts.
- [ ] Native Running/progress view with console as secondary evidence.
- [ ] Native Resume/recovery.
- [ ] Native History/report browser.
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
| Phase 1.5D.2a session command pipe | DONE | this commit | Added opt-in FD4 command pipe, bounded envelope validation and session write primitive; Patch sessions still leave it disabled. |

## Next action

Continue **Phase 1.5D.2b**: carry the disabled-by-default command channel through broker capability/wire/internal API with old-broker fallback.
