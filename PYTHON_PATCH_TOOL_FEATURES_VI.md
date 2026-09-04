# Danh sách tính năng Python Patch Tool — v6.17.1

## Workflow / batch engine

| Tính năng | Trạng thái |
|---|---|
| Linux `.sh`, Windows `.bat` / `.ps1` | COMPLETE |
| PATCH in-place, SANDBOX/worktree removed | COMPLETE |
| COLLECT độc lập, không trộn PATCH | COMPLETE |
| Duplicate-local báo 1 lần → `patchs/ignore/YYYY-MM-DD-*` | COMPLETE |
| `fail_fast` mặc định | COMPLETE |
| `continue_independent` có safety-stop | **COMPLETE v6.17.1** |
| Dependency `batch.depends_on` + stable topological order | **COMPLETE v6.17.1** |
| Dependency runtime FAIL → `BLOCKED` mặc định | **COMPLETE v6.17.1** |
| Successor bắt buộc xử lý predecessor FAIL | **COMPLETE v6.17.1** |
| `previous_failure`: delete/retry_before/run_after/block | **COMPLETE v6.17.1** |
| Whole-batch preflight trước payload đầu tiên | **COMPLETE v6.17.1** |
| `transaction_policy=patch` | COMPLETE |
| `transaction_policy=batch` effective-target rollback + verify | **COMPLETE v6.17.1** |
| Batch target/package snapshot generation checks + POSIX dir-fd restore | **COMPLETE v6.17.1** |
| Batch rollback requeue exact package để replay | **COMPLETE v6.17.1** |
| Smart Resume all/failed/remaining | **COMPLETE v6.17.1** |
| Project mutation lock chống lost-update; batch giữ lock xuyên transaction | **COMPLETE v6.17.1** |
| `internal_error` hard-stop + recompute partial state | **COMPLETE v6.17.1** |

## Report / diagnostics

| Tính năng | Trạng thái |
|---|---|
| Batch statuses PASS/FAIL/BLOCKED/PREFLIGHT_FAIL/NOT_EXECUTED/SKIPPED | **COMPLETE v6.17.1** |
| Full per-PATCH logs | COMPLETE |
| Aggregate `batch.log` + `SUMMARY.txt` | COMPLETE |
| Report browser reopen `report` | COMPLETE |
| Filter PASS / problems / changed | **COMPLETE v6.17.1** |
| Source before/after unified diff | **COMPLETE v6.17.1** |
| Support bundle ZIP từ từng report item | **COMPLETE v6.17.1** |
| History list/pin/unpin/export/delete/cleanup | **COMPLETE v6.17.1** |
| Pinned run được retention bảo vệ | **COMPLETE v6.17.1** |
| FAIL banner nền đỏ/chữ vàng | COMPLETE |
| PASS banner highlight PATCH cuối | COMPLETE |

## PATCH package / AI contract

| Tính năng | Trạng thái |
|---|---|
| Exact PATCH schema + multi-error lint | COMPLETE |
| `validate --patch` read-only | COMPLETE |
| READY/PATCH_INVALID/SOURCE_DRIFT/TOOL_ERROR | COMPLETE |
| Expected/actual SHA + anchor diagnostics | COMPLETE |
| OPS sequential dry-run | COMPLETE |
| `batch` manifest metadata | **COMPLETE v6.17.1** |
| AI bắt buộc khai báo predecessor failure action | **COMPLETE v6.17.1** |
| Machine-readable checklist cập nhật batch contract | **COMPLETE v6.17.1** |
| Explicit `already` semantics, không suy từ `new` ở vị trí bất kỳ | **COMPLETE v6.17.1** |
| OPS managed timeout + atomic source write | **COMPLETE v6.17.1** |
| Archive extraction budgets + collision/Windows path hardening | **COMPLETE v6.17.1** |
| Git auto-commit bảo vệ dirty target + strict commit rc | **COMPLETE v6.17.1** |
| COLLECT hard ceilings/self-output exclusion/sensitive warning | **COMPLETE v6.17.1** |
| COLLECT regex worker + hard timeout | **COMPLETE v6.17.1** |

## Windows

| Tính năng | Trạng thái |
|---|---|
| Native fullscreen selector + line fallback | COMPLETE |
| Process-tree containment `taskkill /T /F` | COMPLETE |
| Reparse/junction safety | COMPLETE |
| Native Windows runtime test lane packaged | **COMPLETE v6.17.1** |
| Native lane đã chạy trên host phát hành Linux | **N/A — cần Windows host thật** |

## Không thực hiện theo yêu cầu

- **Target-overlap/conflict analyzer: NOT IMPLEMENTED.** Tool không phân tích hai PATCH có cùng target/anchor để quyết định conflict.
- **Patch provenance / identity: NOT IMPLEMENTED.** Dependency chỉ dùng `manifest.patch.id` đã tồn tại; không thêm fingerprint/provenance subsystem.

## Giới hạn an toàn

- `continue_independent` không tiếp tục nếu source partial/unknown hoặc rollback/tool integrity không an toàn.
- Whole-batch source validation của PATCH phụ thuộc có thể defer source check tới sau dependency; runner vẫn re-preflight ngay trước execution.
- Batch atomicity bao phủ effective target set resolve trước (`targets` + preflight/recovery/OPS targets); Python side effects ngoài set vẫn không được suy đoán, nên atomic mode từ chối post-command và Git side effects.
- Arbitrary Python payload không được chạy trong preflight/sandbox để “đoán” post-dependency state.
- Regex COLLECT chạy trong worker subprocess riêng và bị hard-timeout toàn search action; catastrophic Python `re` không thể treo collector vô hạn.

## Stop condition

Sau full regression + clean package integrity: **COMPLETE / STOP**.
