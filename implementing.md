# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.15.0**  
Trạng thái: **DIAGNOSTICS + WINDOWS ROBUSTNESS + WINDOWS FULLSCREEN SELECTOR — COMPLETE / STOP**

## Baseline

Baseline kỹ thuật: **v6.14.2**. Release này giữ nguyên PATCH/COLLECT contract fail-closed, in-place execution và zero-argument workflow; không đưa SANDBOX/worktree trở lại.

## Acceptance / task status

| Phase | Hạng mục | Trạng thái |
|---|---|---|
| 1 | Multi-error PATCH manifest lint + migration hints | **COMPLETE** |
| 2 | Project-aware inspect/preflight: `PATCH_INVALID` / `SOURCE_DRIFT` / `READY_TO_APPLY` / `TOOL_ERROR` | **COMPLETE** |
| 3 | Source-drift diagnostics expected/actual SHA, anchors, affected-path-only recovery COLLECT | **COMPLETE** |
| 4 | AI PATCH contract + machine-readable checklist | **COMPLETE** |
| 5 | `validate --patch` read-only validator; same gate re-runs before payload | **COMPLETE** |
| 6 | Windows internal routing/process containment/reparse robustness | **COMPLETE** |
| 7 | Native Windows fullscreen selector with arrows/Space/priority/inspect/validate/health | **COMPLETE** with safe line-selector fallback |
| 8 | Full regression, package integrity, docs/version synchronization | **COMPLETE** |

## Key behavior

- Invalid manifests report multiple independent schema errors in one pass. Known bad legacy/custom `source_baseline` is explicitly mapped to `preflight.files`; invalid timeout values are reported in the same result.
- Source preflight aggregates mismatches across files instead of stopping at the first SHA/anchor mismatch. Structured evidence records expected/actual values and all affected paths.
- Data-only `PATCH_TOOL_OPS.json` is simulated against a private temporary mirror before real payload execution. Sequential OPS semantics are preserved while the project remains unchanged. Anchor/source mismatch therefore fails during preflight.
- `inspect` and `validate` are read-only. Result classes are `READY_TO_APPLY`, `PATCH_INVALID`, `SOURCE_DRIFT`, or `TOOL_ERROR`.
- Normal execution always performs the same preflight again immediately before payload; a previous validate result is never trusted as an execution bypass.
- Recovery COLLECT requests use only structured `affected_paths` that are safe readable current-source files.

## Public commands

Normal use remains zero-argument:

```bash
./tools/run_python_patches.sh
```

```bat
tools\run_python_patches.bat
```

Direct validation for diagnostics/AI/debugging:

```bash
./tools/run_python_patches.sh validate --patch patchs/example.zip
```

```powershell
.\tools\run_python_patches.ps1 validate --patch patchs/example.zip
```

## Windows parity / robustness

- Dispatcher no longer calls the POSIX `.sh` launcher internally. PATCH, COLLECT, inspect and validate route directly through the packaged Python runtime with `sys.executable`, so zero-argument Windows use does not require Bash/Git-Bash.
- Payload/post-command subprocesses use `CREATE_NEW_PROCESS_GROUP`; timeout/Ctrl+C containment escalates to Windows `taskkill /T /F` before rollback/result publication.
- Windows project safety rejects reparse-point paths where a symlink/junction-like redirection could escape the validated project path.
- Native Windows console selector uses `msvcrt` and VT output when available: ↑/↓, Space, 0–9 priority, `a`, `n`, `d`, `i`, `v`, `h`, Enter, q/Esc. If a console cannot support the safe fullscreen path, tool automatically falls back to the stable line selector.

## Release stop condition

**COMPLETE. DỪNG. Không tự mở task/tính năng mới sau release này.**
