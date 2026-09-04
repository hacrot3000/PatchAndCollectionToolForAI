# Python Patch Tool v6.10.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| AI COLLECT request ZIP-only delivery | COMPLETE / documented |
| Legacy v5 COLLECT action list | OBSOLETE as universal schema; current collector/template is authoritative |
| Exactly one COLLECT per invocation | COMPLETE / enforced |
| COLLECT + PATCH mixed selection | REJECTED |
| Multiple COLLECT selection | REJECTED |
| PATCH priority `0..9` | COMPLETE |
| TTY/line selector width-height safety | COMPLETE |
| Project/process queue lock | REMOVED BY REQUIREMENT — concurrent terminals are operator-controlled |
| Local-only duplicate PATCH filtering | COMPLETE; exact SHA-256 against local `patchs/patched/` only |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Readonly COLLECT progress/result validation | COMPLETE |
| Collection-result archive non-runnable classification | COMPLETE |
| Exact private collector action-schema replacement | NOT CLAIMED; exact installed private collector is not shipped in this overlay |
| Exact LAST_RUN/private-core audit synthesis | PRIVATE-CORE DEPENDENT |
| Real large-project COLLECT validation | PARTIAL COMPLETE; real M3 COLLECT PASS observed |
| Phase-inference refinement | DEFERRED until real evidence shows a concrete deficiency |

## v6.10.0 completion

- Replaces the stale `CODE_COLLECTION_GUIDE.md` path so direct portable extraction removes the misleading v5 action table from the active tool documentation.
- Enforces one-COLLECT-only selection at TTY, line mode and execution boundary.
- Does not use a process/project lock; separate terminals may run concurrently by explicit operator choice.
- Does not invent new collector action names or reconstruct missing private collector code.
