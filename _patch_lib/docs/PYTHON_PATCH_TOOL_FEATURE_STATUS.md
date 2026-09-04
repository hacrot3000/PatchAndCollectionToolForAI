# Python Patch Tool v6.13.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| Current-session exact duplicate collapse/removal | COMPLETE |
| Local-history duplicate filtering | COMPLETE |
| PATCH priority `0..9` | COMPLETE |
| TTY/line selector width-height safety | COMPLETE |
| Exactly one COLLECT / no PATCH mix | COMPLETE |
| Project/process lock | REMOVED BY REQUIREMENT |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Full self-contained runtime | COMPLETE |
| Exact PATCH package schema | **COMPLETE v6.13.0** |
| PATCH schema/resource/source/post-command preflight | **COMPLETE v6.13.0** |
| PATCH tool-version negotiation | **COMPLETE v6.13.0** |
| Partial-modification detection | **COMPLETE v6.13.0** |
| Structured current `LAST_RUN.json` | **COMPLETE v6.13.0** |
| Bounded local run history | **COMPLETE v6.13.0** |
| Resume hint after fail-fast | **COMPLETE v6.13.0** |
| Structured automatic diagnosis | **COMPLETE v6.13.0** |
| PATCH FAIL handoff ZIP | **COMPLETE v6.13.0** |
| Source-drift/anchor recovery COLLECT request | **COMPLETE v6.13.0** |
| PATCH inspect/dry-run | **COMPLETE v6.13.0** |
| Exact COLLECT action schema + preflight | COMPLETE |
| `pack`/`overview`/`find`/`search`/`git` readonly | COMPLETE |
| COLLECT result validation/banner | COMPLETE |
| COLLECT quality/truncation summary | **COMPLETE v6.13.0** |
| Generic automatic rollback | NOT IMPLEMENTED BY DESIGN — detect/diagnose rather than guess rollback |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## Stop condition

The user-approved Phase A → B → C scope is complete. Do not automatically begin another capability; ask the user what should be next.
