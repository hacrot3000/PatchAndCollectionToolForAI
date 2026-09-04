# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.16.0**  
Trạng thái: **BATCH REPORT + AGGREGATE/DETAIL LOG VIEWER — COMPLETE / STOP**

## Baseline

Baseline kỹ thuật: **v6.15.1**. Release này tập trung vào kết quả khi chọn nhiều PATCH: persistent batch report, aggregate/detail logs và report browser; giữ nguyên fail-fast, PATCH/COLLECT contract fail-closed, in-place execution, diagnostics, Windows parity và zero-argument workflow.

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

## v6.16.0 output clarity acceptance

- `SKIPPED:DUPLICATE_LOCAL` được báo đúng một record trong invocation hiện tại rồi chuyển khỏi runnable queue sang `patchs/ignore/YYYY-MM-DD-<tên-file-gốc>`. Lần chạy sau `patchs/ignore/` không được discovery nên không hỏi/báo lại file đó.
- Move-to-ignore xác minh lại exact SHA-256 sau khi isolate input; `patchs/ignore` phải là real project-local directory. Collision được xử lý bằng tên date-prefixed unique mà không ghi đè file người dùng.
- Các item **chưa chạy do fail-fast** không phải duplicate skip và vẫn nằm trong `patchs/` để resume.
- PASS kết thúc bằng banner `PATCH COMPLETED` + tên PATCH vừa chạy ở cuối output.
- FAIL kết thúc bằng banner `PATCH FAILED` + tên PATCH/rc; khi terminal hỗ trợ ANSI/VT, banner dùng **nền đỏ + chữ vàng đậm**. Redirect/log dùng plain-text fallback để không chèn escape sequence.
- FAIL_HANDOFF, recovery COLLECT request và các path ZIP vẫn giữ nguyên contract/đường dẫn hiện hành.

## v6.16.0 batch report acceptance

- Một run chọn nhiều PATCH luôn có bảng tổng hợp cuối với từng PATCH và trạng thái `PASS`, `FAIL`, `NOT_EXECUTED` hoặc `SKIPPED_*`.
- Fail-fast **không đổi**: nếu PATCH thứ N FAIL thì các PATCH sau là `NOT_EXECUTED`, tuyệt đối không được báo như FAIL.
- Report renderer hỗ trợ nhiều FAIL trong dữ liệu report để không khóa kiến trúc nếu sau này có policy continue-on-failure, nhưng v6.16.0 không tự bật policy đó.
- Mỗi PATCH đã thực thi có persistent detail log tại `artifacts/patch_tool/runs/<run_id>/items/`.
- Mỗi run có `SUMMARY.txt` và `batch.log` tổng hợp dưới `artifacts/patch_tool/runs/<run_id>/`.
- `report` mở lại batch gần nhất có selected work; một health/IDLE run sau đó không được che mất batch report hữu ích gần nhất.
- `report --list` liệt kê lịch sử; `report --run-id <id>` mở một run cụ thể.
- Trên TTY, sau batch nhiều PATCH tool mở menu report: nhập số để xem detail log của PATCH đó, `a` để xem aggregate log, `q` để thoát.
- Trên non-TTY/redirect, tool không chờ input; vẫn in bảng batch + các path log và lệnh mở lại report.
- Lịch sử report/log được giới hạn cùng `RUN_HISTORY_LIMIT` để không tăng artifact vô hạn.

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

Open the most recent useful batch report:

```bash
./tools/run_python_patches.sh report
```

```bat
tools\run_python_patches.bat report
```

## Windows parity / robustness

- Dispatcher no longer calls the POSIX `.sh` launcher internally. PATCH, COLLECT, inspect and validate route directly through the packaged Python runtime with `sys.executable`, so zero-argument Windows use does not require Bash/Git-Bash.
- Payload/post-command subprocesses use `CREATE_NEW_PROCESS_GROUP`; timeout/Ctrl+C containment escalates to Windows `taskkill /T /F` before rollback/result publication.
- Windows project safety rejects reparse-point paths where a symlink/junction-like redirection could escape the validated project path.
- Native Windows console selector uses `msvcrt` and VT output when available: ↑/↓, Space, 0–9 priority, `a`, `n`, `d`, `i`, `v`, `h`, Enter, q/Esc. If a console cannot support the safe fullscreen path, tool automatically falls back to the stable line selector.

## Release stop condition

**COMPLETE / STOP.** Batch report tests, old regression, clean-extract public workflow và package checksum đều phải PASS trước khi phát hành artifact; không tự mở thêm capability sau release này.
