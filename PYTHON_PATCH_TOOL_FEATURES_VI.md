# Danh sách tính năng Python Patch Tool — v6.16.0

> Chỉ ghi COMPLETE khi có acceptance/regression tương ứng.

## Workflow / selector

| Tính năng | Trạng thái |
|---|---|
| Linux `./tools/run_python_patches.sh` | COMPLETE |
| Windows `tools\run_python_patches.bat` / `.ps1` | COMPLETE |
| Internal dispatch không phụ thuộc Bash trên Windows | **COMPLETE v6.16.0** |
| Fullscreen POSIX selector | COMPLETE |
| Fullscreen Windows selector `msvcrt` + VT, safe line fallback | **COMPLETE v6.16.0** |
| Arrow/Space, priority 0–9, delete, inspect, validate, health | **COMPLETE v6.16.0** |
| Exactly one COLLECT / không trộn PATCH | COMPLETE |
| Không process/project lock | COMPLETE / BY REQUIREMENT |
| Local-history duplicate: báo 1 lần rồi move `patchs/ignore/YYYY-MM-DD-*` | **COMPLETE v6.16.0** |
| `patchs/ignore/` không tham gia queue discovery | **COMPLETE v6.16.0** |
| Final PASS banner highlight tên PATCH vừa chạy | **COMPLETE v6.16.0** |
| Final FAIL banner nền đỏ/chữ vàng trên TTY + plain fallback | **COMPLETE v6.16.0** |
| Batch result table: PASS/FAIL/NOT_EXECUTED/SKIPPED theo từng PATCH | **COMPLETE v6.16.0** |
| Persistent per-PATCH detail logs | **COMPLETE v6.16.0** |
| Persistent aggregate `batch.log` + `SUMMARY.txt` | **COMPLETE v6.16.0** |
| Interactive batch report menu + reopen `report` | **COMPLETE v6.16.0** |
| `report --list` / `report --run-id` history navigation | **COMPLETE v6.16.0** |

## PATCH diagnostics / validation

| Tính năng | Trạng thái |
|---|---|
| Exact `PATCH_PACKAGE_SCHEMA.json` | COMPLETE |
| Multi-error schema lint | **COMPLETE v6.16.0** |
| Migration hint `source_baseline` → `preflight.files` | **COMPLETE v6.16.0** |
| Báo cùng lượt invalid timeout/schema fields | **COMPLETE v6.16.0** |
| `validate --patch` read-only | **COMPLETE v6.16.0** |
| Inspect/validate classification: READY/PATCH_INVALID/SOURCE_DRIFT/TOOL_ERROR | **COMPLETE v6.16.0** |
| Aggregate existence/SHA/anchor mismatch theo nhiều file | **COMPLETE v6.16.0** |
| expected/actual SHA trong diagnostics | **COMPLETE v6.16.0** |
| Data-only OPS sequential dry-run trên temporary mirror | **COMPLETE v6.16.0** |
| OPS anchor/source mismatch fail trước payload | **COMPLETE v6.16.0** |
| Recovery COLLECT chỉ pack affected source paths | **COMPLETE v6.16.0** |
| Normal execution re-preflight trước payload | COMPLETE |

## Execution / recovery / safety

| Tính năng | Trạng thái |
|---|---|
| PATCH in-place; SANDBOX/worktree removed | COMPLETE |
| Exact PATCH/COLLECT input snapshot lifecycle | COMPLETE |
| Fail-fast selected batch | COMPLETE |
| POSIX child process-group containment | COMPLETE |
| Windows `CREATE_NEW_PROCESS_GROUP` + `taskkill /T /F` containment | **COMPLETE v6.16.0** |
| Reparse/junction-aware project path rejection on Windows | **COMPLETE v6.16.0** |
| Metadata-driven rollback opt-in/fail-closed | COMPLETE |
| Structured FAIL_HANDOFF | COMPLETE |
| LAST_RUN/history/resume | COMPLETE |
| Batch report history/log retention bounded by RUN_HISTORY_LIMIT | **COMPLETE v6.16.0** |

## Docs / package / health

| Tính năng | Trạng thái |
|---|---|
| `PATCH_PACKAGE_CHECKLIST.json` machine-readable AI checklist | **COMPLETE v6.16.0** |
| PATCH/COLLECT schemas + AI guides | COMPLETE |
| Windows portable guide | COMPLETE |
| `SHA256SUMS` exact coverage + no pycache | COMPLETE |
| Tool Health version/checksum/runtime/launcher/schema audit | COMPLETE |
| HTML user guide tối giản | UPDATED chỉ cho Windows selector/validate cần thiết |

## Giới hạn có chủ đích

- Python PATCH tùy ý không thể được mô phỏng an toàn như OPS; inspect vẫn kiểm schema/preflight/resources/post-command/Git, còn payload Python chỉ chạy khi người dùng thực thi PATCH.
- External `post_patch.commands[].argv` phải tồn tại trên OS hiện tại; tool không tự chuyển Bash thành PowerShell.
- Windows fullscreen cần console TTY + VT; nếu không đáp ứng sẽ dùng line selector.
- Rollback không được suy đoán khi thiếu exact metadata.
- Batch execution vẫn fail-fast. Vì vậy trong run thật v6.16.0, một mixed-result batch có dạng `PASS... → FAIL → NOT_EXECUTED`; nhiều FAIL trong cùng run chỉ là capability của report renderer cho policy tương lai, không phải execution policy hiện tại.

## v6.16.0 stop condition

Batch report, aggregate/detail log viewer, output clarity và regression hoàn tất. **Dừng.**
