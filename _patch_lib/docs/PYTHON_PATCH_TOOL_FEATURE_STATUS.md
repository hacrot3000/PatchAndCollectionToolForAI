# Python Patch Tool v6.12.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| AI COLLECT request ZIP-only delivery | COMPLETE |
| Current-session exact duplicate collapse/removal | COMPLETE / v6.12.0 |
| Local-history duplicate filtering | COMPLETE |
| PATCH priority `0..9` | COMPLETE |
| TTY/line selector width-height safety | COMPLETE |
| Exactly one COLLECT per invocation / no PATCH mix | COMPLETE |
| Project/process queue lock | REMOVED BY REQUIREMENT |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Self-contained Python PATCH runner | COMPLETE / v6.12.0 |
| Self-contained `PATCH_TOOL_OPS.json` runner | COMPLETE / v6.12.0 |
| Self-contained patch utility compatibility helpers | COMPLETE / v6.12.0 contract |
| Readonly COLLECT progress/result validation | COMPLETE |
| `pack` action | COMPLETE |
| `overview` action | COMPLETE / v6.12.0 |
| `find` action | COMPLETE / v6.12.0 |
| `search` action | COMPLETE / v6.12.0 |
| `git` readonly action | COMPLETE / v6.12.0 |
| Exact machine-readable COLLECT schema | COMPLETE / v6.12.0 |
| COLLECT schema preflight in queue | COMPLETE / v6.12.0 |
| Full self-contained package for documented v6.12.0 contract | COMPLETE |
| `tools/implementing.md` live task tracker | COMPLETE |
| Vietnamese feature matrix | COMPLETE |
| Vietnamese HTML guide + VI/EN/RU AI prompts | COMPLETE |
| Historical private-core parity outside current contract | PARTIAL / FAIL-CLOSED BY DESIGN |
| Advanced historical COLLECT actions outside schema | NOT IMPLEMENTED in v6.12.0 |
| Exact historical LAST_RUN/private report parity | DEFERRED |
| Phase-inference refinement | DEFERRED pending concrete runtime evidence |

## v6.12.0 development stop condition

The explicitly requested v6.12.0 scope is complete. Do not automatically begin another feature. Ask the user which task should be next.
