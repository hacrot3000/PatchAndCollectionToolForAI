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

Minimum code checkpoint for this smoke: the installed TaskDeck revision must contain `15711d4d` (or be a later descendant on `main`). Older installed revisions are not valid evidence for Phase 2E.2.

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
| `15711d4d` | Clarify explicit-only terminal fallback wording |

## Non-regression invariants

- Python remains authoritative for Patch Tool policy/business logic.
- Do not parse console text to drive native behavior.
- Direct `taskdeck patch ...` terminal usage remains supported.
- PTY/backing session may remain available for compatibility/evidence.
- Native Patch actions must not create ordinary terminal tabs by default.
- A terminal tab may be materialized only by an explicit user evidence/fallback action.
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
