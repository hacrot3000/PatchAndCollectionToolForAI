# Python Patch Tool v6.9.6 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE for structural unified routing |
| AI COLLECT request delivery: ZIP containing exactly one request JSON | DOCUMENTED / ENFORCED BY QUEUE |
| Loose COLLECT request JSON | REJECTED |
| Filename-independent v5 PATCH discovery by package structure | COMPLETE |
| Legacy-v4 positive patch recognition | COMPLETE |
| Non-patch HANDOFF/report/tool archive rejection | COMPLETE |
| Symlink queue entry rejection | COMPLETE |
| Single-item default selection | COMPLETE |
| TTY selector | COMPLETE; Space `[x]` plus explicit priority `0..9`; width/height-safe viewport |
| Line selector numbers/lists/ranges/all/none/quit/delete | COMPLETE |
| Config-driven zero-argument prompt/all/first/newest selection | COMPLETE |
| Stop on first selected-item failure | COMPLETE |
| Project-local zero-argument concurrency lock | COMPLETE; fail-closed symlink/hardlink-safe `flock` |
| Local-only duplicate PATCH filtering against `patchs/patched/` | COMPLETE / retained from v6.8.x — SHA-256 content identity, skip-only |
| Renamed identical PATCH detection | COMPLETE / retained |
| Same-name/different-content PATCH remains runnable | COMPLETE / retained |
| Duplicate history scope | LOCAL PROJECT ONLY — no PROJECT KEY/network/shared history |
| Duplicate queue file lifecycle | SKIP ONLY — file remains untouched in `patchs/` |
| Duplicate summary reason | `[SKIPPED:DUPLICATE_LOCAL]` with matching local history path |
| PATCH child signal exit-code normalization (SIGINT/SIGTERM -> 130/143) | COMPLETE / retained |
| Line selector Ctrl+C clean cancellation / no traceback | COMPLETE / retained |
| TTY one-line COLLECT progress / resize handling | COMPLETE |
| Invalid UTF-8/control-character robustness | COMPLETE |
| COLLECT child-process cleanup on SIGINT/SIGTERM | COMPLETE; includes post-parent-exit drain window |
| COLLECT PASS result ZIP highlighted once / duplicate path suppression | COMPLETE |
| COLLECT upload ZIP existence/location/CRC verification | COMPLETE |
| COLLECT result-candidate fallback when earlier reported ZIP is stale | COMPLETE |
| COLLECT PASS request archive postcondition (`patchs/` -> `patchs/patched/`) | ENFORCED |
| Release checksum self-test on installed overlays with private core/older files | COMPLETE |
| Transaction worktree execution | REMOVED — PATCH routes forced in-place |
| Transaction/SANDBOX-only launcher invocation | FAIL-CLOSED |
| Readonly COLLECT | COMPLETE |
| Exact replacement of private installed core modules | NOT CLAIMED; exact current private-core source is not present in this release input |
| Exact LAST_RUN audit synthesis for dispatcher-only delete/cancel/not-selected events | PRIVATE-CORE DEPENDENT; not reconstructed from incomplete source |
| Real large-project COLLECT validation | PARTIAL COMPLETE — real M3 COLLECT PASS observed; phase-quality evidence still limited |
| Phase-inference refinement | DEFERRED unless real COLLECT output demonstrates missing/poor markers |

v6.9.6 is a regression-only PATCH over the v6.9.0 selector-priority baseline.
It does not start an unrelated feature group. Duplicate-local behavior remains
project-local: a PATCH that ran on one computer is still runnable on another
unless that second project root contains the same package bytes in its own
`patchs/patched/`.

## Retained duplicate-local hardening

- `patchs/` and `patchs/patched/` must be real project-local directories for
  duplicate suppression. A symlinked/shared history never suppresses a PATCH;
  the tool warns and leaves the PATCH runnable.
- Selected PATCHes are rechecked immediately before launch. If an earlier
  identical PATCH in the same batch has just been archived locally, the later
  copy becomes `[SKIPPED:DUPLICATE_LOCAL]` instead of executing again.
- These changes only harden the existing local duplicate feature; no global,
  PROJECT KEY, network, or cross-machine history was added.

## Retained v6.9.0 selector priority completion

- On the TTY selector, `0`..`9` assigns an execution priority to the current
  selected row. Lower numbers execute first. Equal numbers preserve the exact
  natural queue ordering already displayed.
- Plain Space selections remain `[x]`; if mixed with numbered selections they
  execute afterwards in the displayed queue order.
- Example priorities `[0],[1],[3],[0],[2]` on rows 1..5 execute as
  `1 -> 4 -> 2 -> 5 -> 3`.
- This extends the existing selector only; the non-TTY line selector retains
  its historical numeric item-index grammar to avoid a breaking ambiguity.

## v6.9.6 regression repair

- PASS summary counts executed items separately from `[SKIPPED:DUPLICATE_LOCAL]` items.
- HANDOFF/tool-distribution ZIP identity is resolved before COLLECT request
  discovery, so preserved request JSON inside support evidence cannot rerun.
- COLLECT drains buffered stdout for a bounded post-exit grace period so a
  final result ZIP line is not lost merely because the reader thread lags the
  already-exited collector process.
- Result/request metadata is tracked independently of the 120-line failure tail,
  so long trailing logs cannot erase an already-reported valid upload ZIP.
- No new capability was added.


V6.9.6 in-place boundary hardening:
  Historical/short PATCH execution flags such as `-a -y --move` are treated as
  execution-capable and receive `--transaction off`. Only documented read-only
  utility routes (`paths`, help, version) bypass the execution-only argument.

## v6.9.6 regression repair

- Fullscreen selector rows are clipped by live terminal **cell width** with a
  two-cell safety margin. Long OTA/NFC filenames and CJK/full-width text can no
  longer wrap into extra physical rows and corrupt cursor-up redraw accounting.
- Zero-argument queue discovery/execution is protected by a project-local
  advisory lock. Two simultaneous tool sessions in the same project cannot
  both scan and execute the same PATCH before local history is updated.
- Lock contention is fail-closed as `BUSY` / temporary failure; the lock is
  project-local only, so another project or another machine remains independent.
- Selector priority `0..9`, local SHA-256 duplicate semantics, COLLECT routing,
  and all v6.9.1 in-place/SANDBOX guards are otherwise unchanged.

- Release packaging preserves executable mode on `tools/run_python_patches.sh`; clean extraction is tested before release.

## v6.9.6 regression repair

- Fullscreen selector rendering is bounded by live terminal height as well as
  width. A long queue uses a stable cursor-centered viewport, preventing frame
  scrolling from corrupting cursor-up redraw accounting.
- The zero-argument project lock no longer follows a symlinked lock path and
  refuses multi-hardlink lock inodes. The lock inode is not truncated or used
  as PID storage; kernel `flock` state is authoritative.
- Lock contention (`BUSY`) is distinguished from unsafe/unavailable lock state
  (`QUEUE LOCK` error).
- COLLECT keeps its signal-forwarding handlers installed through post-exit
  stdout drain/descendant cleanup, so supervisor-only SIGINT/SIGTERM cannot
  orphan a child after the collector parent has already exited.
- No new feature family is started; selector priority, duplicate-local,
  PATCH/COLLECT routing and permanent in-place/SANDBOX removal are unchanged.


## v6.9.6 UI regression repair
- Fullscreen selector rows are clipped to terminal cell width before ANSI styling; the current row is bold/reverse highlighted and the header always shows `CON TRỎ i/N`, preventing loss of position with very long filenames.
- Successful COLLECT uses a high-contrast `ACTION REQUIRED` upload banner. The verified result ZIP path is printed exactly once; the archived request remains informational.

## v6.9.6 regression repair

- Canonical readonly collection-result archives (`COLLECTION_MANIFEST.json` at
  archive root, or under one complete wrapper directory) are classified as
  `collection_result_archive` and are never executable queue PATCHes. This
  check runs before legacy `patch_*.py`/`PATCH_NAME` fallback because collected
  source may legitimately contain patch-looking Python evidence.
- An ambiguous ZIP with both root `COLLECTION_MANIFEST.json` and
  `PATCH_TOOL_MANIFEST.json` fails closed as a collection result. A normal v5
  PATCH with only `PATCH_TOOL_MANIFEST.json` still routes as PATCH; nested
  `CODE_COLLECTION_REQUEST*.json` resources do not change that behavior.
- Fullscreen selector headers put `CON TRỎ i/N` at the left edge so narrow
  terminal clipping cannot remove the operator's position indicator.
- COLLECT success banner/rule rows no longer force a 24-cell decorative
  minimum wider than the live terminal. The complete result path remains
  unmodified/copyable and may naturally wrap after collection is finished.
- No new feature family is started; these changes only close classifier/UI
  regressions in existing PATCH/COLLECT/selector behavior.


## v6.9.6 regression repair

- Re-zipped readonly COLLECT results remain non-runnable even when macOS/desktop
  metadata such as `__MACOSX/` or `.DS_Store` sits outside the collection wrapper.
  Those metadata files are ignored only for structural result identity; collected
  `patch_*.py` evidence can therefore never fall through to legacy PATCH routing.
- The master release regression runner starts each child self-test with `python -S`
  so machine-local `sitecustomize`/user-site hooks cannot make a clean tool release
  print PASS and then remain alive or exceed the release timeout.
- No new capability is started.
