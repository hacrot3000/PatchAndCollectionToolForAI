# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.15.1**  
Trạng thái: **OUTPUT CLARITY + SKIP-ONCE IGNORE LIFECYCLE — COMPLETE / STOP**

## Baseline

Baseline kỹ thuật: **v6.15.0**. Release này chỉ tinh gọn lifecycle/output sau chạy; giữ nguyên PATCH/COLLECT contract fail-closed, in-place execution, diagnostics, Windows parity và zero-argument workflow.

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

## v6.15.1 output clarity acceptance

- `SKIPPED:DUPLICATE_LOCAL` được báo đúng một record trong invocation hiện tại rồi chuyển khỏi runnable queue sang `patchs/ignore/YYYY-MM-DD-<tên-file-gốc>`. Lần chạy sau `patchs/ignore/` không được discovery nên không hỏi/báo lại file đó.
- Move-to-ignore xác minh lại exact SHA-256 sau khi isolate input; `patchs/ignore` phải là real project-local directory. Collision được xử lý bằng tên date-prefixed unique mà không ghi đè file người dùng.
- Các item **chưa chạy do fail-fast** không phải duplicate skip và vẫn nằm trong `patchs/` để resume.
- PASS kết thúc bằng banner `PATCH COMPLETED` + tên PATCH vừa chạy ở cuối output.
- FAIL kết thúc bằng banner `PATCH FAILED` + tên PATCH/rc; khi terminal hỗ trợ ANSI/VT, banner dùng **nền đỏ + chữ vàng đậm**. Redirect/log dùng plain-text fallback để không chèn escape sequence.
- FAIL_HANDOFF, recovery COLLECT request và các path ZIP vẫn giữ nguyên contract/đường dẫn hiện hành.

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
