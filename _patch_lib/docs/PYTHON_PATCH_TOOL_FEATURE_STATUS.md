# Python Patch Tool v6.14.2 feature status

| Capability | Status |
|---|---|
| Public zero-argument PATCH/COLLECT queue | COMPLETE |
| POSIX `.sh` launcher | COMPLETE |
| Windows `.bat` + PowerShell launcher | **COMPLETE v6.14.2** |
| Windows Python 3.10+ discovery | **COMPLETE v6.14.2** |
| Windows/non-TTY line selector | **COMPLETE v6.14.2** |
| Current-session/local-history duplicate handling | COMPLETE |
| PATCH priority / terminal-safe selector | COMPLETE; priority fullscreen UI is POSIX TTY only |
| Exactly one COLLECT / no PATCH mix / no process lock | COMPLETE |
| Permanent PATCH in-place / SANDBOX removal | COMPLETE |
| Full self-contained runtime | COMPLETE |
| Exact PATCH package schema + preflight + compatibility | COMPLETE |
| Partial-modification detection | COMPLETE |
| Metadata-driven exact-target rollback for payload/post-patch failure | COMPLETE v6.14.1 |
| Generic rollback without recovery metadata / Git-policy rollback | FAIL-CLOSED BY DESIGN |
| LAST_RUN / bounded history / resume / inspect | COMPLETE |
| Structured diagnosis / FAIL_HANDOFF / source-drift COLLECT recovery | COMPLETE |
| Exact COLLECT schema + readonly actions | COMPLETE |
| COLLECT result validation / quality summary | COMPLETE |
| Tool Health self-audit (`h`; compact on IDLE) | COMPLETE |
| Windows launchers included in Tool Health/SHA256 coverage | **COMPLETE v6.14.2** |
| Historical private-core parity outside current contract | FAIL-CLOSED / NOT GUARANTEED |

## v6.14.2 Windows portability notes

All public launchers resolve the same project root and call the same Python dispatcher/runner/collector. Windows PowerShell launcher probes `py -3`, `python`, then `python3` and requires Python 3.10+. The `.bat` wrapper is the recommended Windows entry point when local PowerShell script execution policy would otherwise block a direct `.ps1` launch.

Windows uses the line selector because the fullscreen selector depends on POSIX `termios`. This is a UI difference, not a different queue/schema. External post-patch executables remain OS-dependent and must exist on the machine running the PATCH.

## Stop condition

v6.14.2 completes the requested Windows launcher/usage support. No new capability is started automatically after this release.
