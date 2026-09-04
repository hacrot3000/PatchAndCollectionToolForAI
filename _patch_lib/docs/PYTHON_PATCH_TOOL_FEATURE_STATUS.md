# Python Patch Tool v6.9.0 feature status

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
| TTY selector | COMPLETE; Space `[x]` plus explicit priority `0..9` |
| Line selector numbers/lists/ranges/all/none/quit/delete | COMPLETE |
| Config-driven zero-argument prompt/all/first/newest selection | COMPLETE |
| Stop on first selected-item failure | COMPLETE |
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
| COLLECT child-process cleanup on SIGINT/SIGTERM | COMPLETE |
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
| Real large-project COLLECT validation | PENDING USER RUNTIME EVIDENCE |
| Phase-inference refinement | DEFERRED unless real COLLECT output demonstrates missing/poor markers |

v6.9.0 adds only the explicitly requested priority ordering to the existing TTY
selector. It does not start an unrelated feature group. Duplicate-local behavior
remains machine-local: a PATCH that ran on one computer is still runnable on
another unless that second project root contains the same package bytes in its
own `patchs/patched/`.

## Retained duplicate-local hardening

- `patchs/` and `patchs/patched/` must be real project-local directories for
  duplicate suppression. A symlinked/shared history never suppresses a PATCH;
  the tool warns and leaves the PATCH runnable.
- Selected PATCHes are rechecked immediately before launch. If an earlier
  identical PATCH in the same batch has just been archived locally, the later
  copy becomes `[SKIPPED:DUPLICATE_LOCAL]` instead of executing again.
- These changes only harden the existing local duplicate feature; no global,
  PROJECT KEY, network, or cross-machine history was added.

## v6.9.0 selector priority completion

- On the TTY selector, `0`..`9` assigns an execution priority to the current
  selected row. Lower numbers execute first. Equal numbers preserve the exact
  natural queue ordering already displayed.
- Plain Space selections remain `[x]`; if mixed with numbered selections they
  execute afterwards in the displayed queue order.
- Example priorities `[0],[1],[3],[0],[2]` on rows 1..5 execute as
  `1 -> 4 -> 2 -> 5 -> 3`.
- This extends the existing selector only; the non-TTY line selector retains
  its historical numeric item-index grammar to avoid a breaking ambiguity.
