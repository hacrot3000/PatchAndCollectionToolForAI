# Python Patch Tool v6.17.0 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| POSIX `.sh` launcher | COMPLETE |
| Windows `.bat` + PowerShell launcher | COMPLETE |
| Windows Python 3.10+ discovery | COMPLETE |
| Windows internal PATCH/COLLECT routing without Bash | **COMPLETE v6.17.0** |
| POSIX fullscreen selector | COMPLETE |
| Windows fullscreen selector (`msvcrt` + VT) + line fallback | **COMPLETE v6.17.0** |
| PATCH priority / inspect / validate / health controls | COMPLETE |
| Exactly one COLLECT / no PATCH mix / no process lock | COMPLETE |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Exact PATCH package schema + compatibility | COMPLETE |
| Multi-error manifest lint + migration hints | **COMPLETE v6.17.0** |
| Read-only `validate --patch` result classification | **COMPLETE v6.17.0** |
| Aggregate source SHA/existence/anchor diagnostics | **COMPLETE v6.17.0** |
| Sequential data-only OPS dry-run before payload | **COMPLETE v6.17.0** |
| Partial-modification detection | COMPLETE |
| Metadata-driven exact-target rollback | COMPLETE |
| Windows process-tree timeout/Ctrl+C containment | **COMPLETE v6.17.0** |
| Windows reparse/junction-aware project safety | **COMPLETE v6.17.0** |
| Generic rollback without recovery metadata / Git-policy rollback | FAIL-CLOSED BY DESIGN |
| LAST_RUN / bounded history / resume / inspect | COMPLETE |
| Per-run batch summary + aggregate/detail logs | **COMPLETE v6.17.0** |
| Interactive/reopenable `report` browser + history navigation | **COMPLETE v6.17.0** |
| Structured diagnosis / FAIL_HANDOFF / narrowed source-drift COLLECT recovery | COMPLETE |
| Exact COLLECT schema + readonly actions | COMPLETE |
| Tool Health self-audit (`h`; compact on IDLE) | COMPLETE |
| Windows launchers included in Tool Health/SHA256 coverage | COMPLETE |
| Machine-readable `PATCH_PACKAGE_CHECKLIST.json` | **COMPLETE v6.17.0** |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## Windows portability notes

The dispatcher routes PATCH/COLLECT/inspect/validate/report directly through the packaged Python runtime, so native Windows zero-argument use does not require Bash. A native console gets fullscreen arrow/Space/priority controls when `msvcrt` input and VT output are available; non-TTY/unsupported consoles fall back to the stable line selector. External post-patch executables remain OS-dependent.

Batch execution defaults to fail-fast but v6.17.0 also supports explicit `continue_independent`. Continued execution is allowed only after a failure that leaves project state proven clean or successfully rolled back; unsafe partial/tool/rollback failures safety-stop. Dependency failures render successors as `BLOCKED` unless `run_anyway` is explicitly declared.

## Stop condition

v6.17.0 completes persistent batch reporting, aggregate/detail log browsing, diagnostics, Windows robustness/fullscreen parity and final audit scope. No new capability is started automatically after this release.
