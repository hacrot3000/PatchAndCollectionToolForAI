# Python Patch Tool v6.14.1 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| Current-session/local-history duplicate handling | COMPLETE |
| PATCH priority / terminal-safe selector | COMPLETE |
| Exactly one COLLECT / no PATCH mix / no process lock | COMPLETE |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Full self-contained runtime | COMPLETE |
| Exact PATCH package schema + preflight + compatibility | COMPLETE |
| Partial-modification detection | COMPLETE |
| Metadata-driven exact-target rollback for payload/post-patch failure | **COMPLETE v6.14.1** |
| Rollback whole-project verification on Git / PARTIAL on out-of-scope delta | **COMPLETE v6.14.1** |
| Generic rollback without recovery metadata / Git-policy rollback | FAIL-CLOSED BY DESIGN |
| LAST_RUN / bounded history / resume / inspect | COMPLETE |
| Structured diagnosis / FAIL_HANDOFF / source-drift COLLECT recovery | COMPLETE |
| Exact COLLECT schema + readonly actions | COMPLETE |
| COLLECT result validation / quality summary | COMPLETE |
| Tool Health self-audit (`h`; compact on IDLE) | **COMPLETE v6.14.1** |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## v6.14.1 robustness audit

| Capability | Status |
|---|---|
| Rollback ancestor/path symlink fail-closed | COMPLETE |
| Rollback missing-parent false-PASS prevention | COMPLETE |
| Rollback snapshot TOCTOU baseline re-check | COMPLETE |
| Dispatcher → runner SIGINT/SIGTERM process-group forwarding | COMPLETE |
| Payload/post-command descendant termination before rollback | COMPLETE |
| Exact PATCH input snapshot/archive lifecycle | COMPLETE |
| Exact COLLECT request snapshot/archive lifecycle | COMPLETE |
| FAIL_HANDOFF exact executed-package identity | COMPLETE |
| Tool Health required-checksum coverage | COMPLETE |
| Tool Health unsafe symlink-ancestor rejection | COMPLETE |
| Symlinked `patchs/` queue-root rejection | COMPLETE |
| Current-session duplicate safe-removal race hardening | COMPLETE |

## Stop condition

v6.14.1 is a robustness-only patch release. No new capability is started automatically after this audit.
