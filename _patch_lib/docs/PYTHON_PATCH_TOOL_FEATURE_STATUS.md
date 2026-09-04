# Python Patch Tool v6.8.0 feature status

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
| TTY selector | COMPLETE for documented controls |
| Line selector numbers/lists/ranges/all/none/quit/delete | COMPLETE |
| Config-driven zero-argument prompt/all/first/newest selection | COMPLETE |
| Stop on first selected-item failure | COMPLETE |
| Local-only duplicate PATCH filtering against `patchs/patched/` | COMPLETE in v6.8.0 — SHA-256 content identity, skip-only |
| Renamed identical PATCH detection | COMPLETE in v6.8.0 |
| Same-name/different-content PATCH remains runnable | COMPLETE in v6.8.0 |
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

v6.8.0 completes the already-listed duplicate-filtering work. It does not start
an unrelated feature group. The duplicate decision is intentionally machine-local:
a PATCH that has run on one computer is still runnable on another computer unless
that second project root has the same package content in its own `patchs/patched/`.
