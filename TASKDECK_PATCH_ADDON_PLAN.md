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
Status: **IN PROGRESS**

- [x] Installer creates `~/.local/lib/taskdeck/releases/<revision>/taskdeck`.
- [x] Installer places the Patch Tool runtime beside it under `patchtool/`.
- [x] Installer atomically switches `~/.local/lib/taskdeck/current`.
- [x] Installer keeps `~/.local/bin/taskdeck` as the stable symlink entrypoint.
- [ ] Update self-update/handoff to install and switch versioned releases.
- [x] Installer never removes old release directories, so in-flight processes keep their runtime.

### Phase 1D — Dedicated Patch panel
Status: **TODO**

- [ ] Add Patch icon/view to the TaskDeck Activity Bar.
- [ ] Launch Patch Tool without a `.vscode/tasks.json` entry.
- [ ] Phase 1 UI may use an embedded PTY terminal.
- [ ] Detect/hide duplicate legacy `run_python_patches.sh` task where safe.

### Phase 1E — Legacy project runtime migration
Status: **TODO**

- [ ] Detect project-local official Patch Tool runtime.
- [ ] Verify managed files/checksums before cleanup.
- [ ] Never remove locally modified/unknown files.
- [ ] Never cleanup the PatchAndCollectionToolForAI source repository.
- [ ] Keep a compatibility path for old launchers.

### Phase 1.5 — Protocol v1
Status: **TODO**

- [ ] Add optional event FD/pipe to Python entrypoint.
- [ ] Add optional command FD/pipe.
- [ ] Emit queue/status/progress/artifact events while terminal UI remains authoritative.
- [ ] Add protocol schema/version tests.
- [ ] TaskDeck consumes events for native summary while PTY remains visible.

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
| Phase 1C.1 versioned installer | DONE | this commit | Installer stages TaskDeck + Patch runtime as one immutable release and atomically switches `current` and the PATH symlink. |

## Next action

Continue **Phase 1C.2**: teach in-app self-update to build/install the same versioned release bundle and atomically switch the global entrypoint.
