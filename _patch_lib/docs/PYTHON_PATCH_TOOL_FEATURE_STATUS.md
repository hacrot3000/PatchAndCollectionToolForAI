# Python Patch Tool v6.14.0 feature status

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
| Metadata-driven exact-target rollback for payload/post-patch failure | **COMPLETE v6.14.0** |
| Rollback whole-project verification on Git / PARTIAL on out-of-scope delta | **COMPLETE v6.14.0** |
| Generic rollback without recovery metadata / Git-policy rollback | FAIL-CLOSED BY DESIGN |
| LAST_RUN / bounded history / resume / inspect | COMPLETE |
| Structured diagnosis / FAIL_HANDOFF / source-drift COLLECT recovery | COMPLETE |
| Exact COLLECT schema + readonly actions | COMPLETE |
| COLLECT result validation / quality summary | COMPLETE |
| Tool Health self-audit (`h`; compact on IDLE) | **COMPLETE v6.14.0** |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## Stop condition

The remaining user-approved items from the original ordered proposal are complete. Do not automatically start another capability; ask the user what should be next.
