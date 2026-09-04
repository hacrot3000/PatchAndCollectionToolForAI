# Python Patch Tool v6.7.9 feature status

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
| TTY one-line COLLECT progress / resize handling | COMPLETE |
| Invalid UTF-8/control-character robustness | COMPLETE |
| COLLECT child-process cleanup on SIGINT/SIGTERM | FIXED in v6.7.9 |
| COLLECT PASS result ZIP highlighted once / duplicate path suppression | FIXED in v6.7.9 |
| Transaction worktree execution | REMOVED — PATCH routes forced in-place |
| Transaction/SANDBOX-only launcher invocation | FAIL-CLOSED in v6.7.9 |
| Readonly COLLECT | COMPLETE |
| Exact replacement of private installed core modules | NOT CLAIMED; exact current private-core source is not present in this release input |
| Unified pre-selector identity/history/duplicate filtering | PRIVATE-CORE DEPENDENT; not reconstructed from incomplete source |
| Exact LAST_RUN audit synthesis for dispatcher-only delete/cancel/not-selected events | PRIVATE-CORE DEPENDENT; not reconstructed from incomplete source |
| Real large-project COLLECT validation | PENDING USER RUNTIME EVIDENCE |
| Phase-inference refinement | DEFERRED unless real COLLECT output demonstrates missing/poor markers |

No new feature group is started by v6.7.9. This release fixes regressions and
clarifies the already-established COLLECT ZIP-only public contract.
