# Python Patch Tool v6.11.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| AI COLLECT request ZIP-only delivery | COMPLETE / documented |
| Overlay-native exact-file `pack` action | COMPLETE / v6.11.0 |
| Historical v5 COLLECT action list | OBSOLETE as universal schema; only `pack` is overlay-guaranteed |
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
| Exact private collector action-schema replacement | PARTIAL: `pack` guaranteed by overlay; all other actions remain private-core specific |
| Exact LAST_RUN/private-core audit synthesis | PRIVATE-CORE DEPENDENT |
| Real large-project COLLECT validation | PARTIAL COMPLETE; real M3 COLLECT PASS observed |
| Phase-inference refinement | DEFERRED until real evidence shows a concrete deficiency |

## v6.11.0 completion

- Adds a compatibility collector layer that natively handles pack-only requests without reconstructing the private collector core.
- `pack.paths` collects exact project-relative regular files, rejects path traversal/symlinks/directories/missing sources, records size + SHA-256, and emits a canonical `COLLECTION_MANIFEST.json` result ZIP.
- Non-pack requests are delegated intact to the installed private collector, preserving existing collector behavior.
- Retains one-COLLECT-only selection and no project/process queue lock.
- Does not claim support for `overview` or any other private action without exact current collector evidence.
