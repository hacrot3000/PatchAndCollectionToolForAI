# Python Patch Tool v6.7.12 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE; COLLECT is internally routed and public manual COLLECT subcommands are rejected |
| AI COLLECT request delivery: ZIP containing exactly one request JSON | DOCUMENTED / ENFORCED BY QUEUE |
| Loose COLLECT request JSON | REJECTED |
| Filename-independent v5 PATCH discovery by package structure | COMPLETE |
| Legacy-v4 positive patch recognition | COMPLETE |
| Non-patch HANDOFF/report/tool archive rejection | COMPLETE; support identity precedes COLLECT, with wrapped/content and generated PTV filename signatures |
| Queue TOCTOU / symlink rejection | COMPLETE; selected item identity metadata is snapshotted and revalidated immediately before execution |
| Single-item default selection | COMPLETE |
| TTY selector | COMPLETE for documented controls |
| Line selector numbers/lists/ranges/all/none/quit/delete | COMPLETE |
| Config-driven zero-argument prompt/all/first/newest selection | COMPLETE |
| Stop on first selected-item failure | COMPLETE |
| TTY one-line COLLECT progress / resize handling | COMPLETE |
| Invalid UTF-8/control-character robustness | COMPLETE |
| COLLECT result ZIP path with quotes/spaces | FIXED in v6.7.12 |
| COLLECT rc=0 without a usable result ZIP | FAIL-CLOSED as rc=2; result/request completion paths are retained independently and survive beyond bounded failure tail |
| COLLECT child-process cleanup on SIGINT/SIGTERM/SIGHUP/SIGQUIT | FIXED / hardened in v6.7.12 |
| COLLECT spawn-vs-signal orphan race | FIXED in v6.7.12 |
| COLLECT PASS result ZIP highlighted once / duplicate path suppression | COMPLETE |
| Transaction worktree execution | REMOVED — PATCH routes forced in-place |
| Transaction/SANDBOX flags without a recognized PATCH route | FAIL-CLOSED; legacy `-a/-y/--zip-failed/--keep-failed-zip/--move` execution routes also force in-place |
| Readonly COLLECT | COMPLETE |
| Package checksum self-test on a real project containing private/stale tools | FIXED in v6.7.12; verifies managed files without claiming project-owned extras |
| Exact replacement of private installed core modules | NOT CLAIMED; exact current private-core source is not present in this release input |
| Unified pre-selector identity/history/duplicate filtering | PRIVATE-CORE DEPENDENT; not reconstructed from incomplete source |
| Exact LAST_RUN audit synthesis for dispatcher-only delete/cancel/not-selected events | PRIVATE-CORE DEPENDENT; not reconstructed from incomplete source |
| Real large-project COLLECT validation | PENDING USER RUNTIME EVIDENCE |
| Phase-inference refinement | DEFERRED unless real COLLECT output demonstrates missing/poor markers |

No new feature group is started by v6.7.12. This release is bug/regression
repair only.
