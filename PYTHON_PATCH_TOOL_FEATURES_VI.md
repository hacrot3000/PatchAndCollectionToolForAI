# Danh sách tính năng Python Patch Tool — v6.17.7

## Workflow / batch engine

| Tính năng | Trạng thái |
|---|---|
| Linux `.sh`, Windows `.bat` / `.ps1` | COMPLETE |
| PATCH in-place, SANDBOX/worktree removed | COMPLETE |
| COLLECT độc lập, không trộn PATCH | COMPLETE |
| Duplicate-local báo 1 lần → `patchs/ignore/YYYY-MM-DD-*` | COMPLETE |
| Recovery bind exact `requeued_as + SHA-256`; không thao tác nhầm file mới chiếm tên predecessor | **COMPLETE v6.17.6** |
| Batch rollback exact replay được bảo vệ khỏi session/history duplicate filter bằng report identity | **COMPLETE v6.17.6** |
| Failed PATCH không declared target + Git-clean fingerprint => partial state `unknown`, safety-stop | **COMPLETE v6.17.6** |
| Unsafe queue/artifact/lock filesystem boundary => fail-closed rc=2, không traceback ngoài ý muốn | **COMPLETE v6.17.6** |
| `continue_independent` mặc định; `fail_fast` là explicit opt-in | **COMPLETE v6.17.5** |
| `continue_independent` có safety-stop | **COMPLETE v6.17.5** |
| Dependency `batch.depends_on` + stable topological order | **COMPLETE v6.17.5** |
| Dependency runtime FAIL → `BLOCKED` mặc định | **COMPLETE v6.17.5** |
| Successor bắt buộc xử lý predecessor FAIL | **COMPLETE v6.17.5** |
| `previous_failure`: delete/retry_before/run_after/block | **COMPLETE v6.17.5** |
| Whole-batch preflight trước payload đầu tiên | **COMPLETE v6.17.5** |
| `transaction_policy=patch` | COMPLETE |
| `transaction_policy=batch` effective-target rollback + verify | **COMPLETE v6.17.5** |
| Batch target/package snapshot generation checks + POSIX dir-fd restore | **COMPLETE v6.17.5** |
| Batch rollback requeue exact package atomic no-overwrite + `REQUEUE_FAILED` rc=71 | **COMPLETE v6.17.5** |
| Smart Resume ↑/↓ + mô tả động + all/failed/remaining | **COMPLETE v6.17.5** |
| Recovery multi-select PATCH lỗi cho Retry/COLLECT/Delete | **COMPLETE v6.17.5** |
| Recovery COLLECT source cho bất kỳ PATCH FAIL | **COMPLETE v6.17.5** |
| Recovery Delete chuyển PATCH lỗi an toàn sang `patchs/ignore` | **COMPLETE v6.17.5** |
| PATCH sau overlap effective target với PATCH lỗi → BLOCKED; PATCH độc lập tiếp tục | **COMPLETE v6.17.5** |
| Project mutation lock chống lost-update; batch giữ lock xuyên transaction | **COMPLETE v6.17.5** |
| `internal_error`/tool failure: continue chỉ khi project proven unchanged; partial/unknown hard-stop | **COMPLETE v6.17.5** |
| Planned package SHA binding → verify trước batch snapshot + child spawn | **COMPLETE v6.17.5** |
| Batch replay snapshot SHA/size verification trước requeue | **COMPLETE v6.17.5** |
| Mutation lock no-follow / reject symlink-reparse | **COMPLETE v6.17.5** |
| Artifact subdirectory real-dir guard + FAIL_HANDOFF fallback | **COMPLETE v6.17.5** |

## Report / diagnostics

| Tính năng | Trạng thái |
|---|---|
| Batch statuses PASS/FAIL/BLOCKED/PREFLIGHT_FAIL/NOT_EXECUTED/SKIPPED | **COMPLETE v6.17.5** |
| Full per-PATCH logs | COMPLETE |
| Aggregate `batch.log` + `SUMMARY.txt` | COMPLETE |
| Report browser reopen `report` | COMPLETE |
| Filter PASS / problems / changed | **COMPLETE v6.17.5** |
| Source before/after unified diff | **COMPLETE v6.17.5** |
| Support bundle ZIP từ từng report item | **COMPLETE v6.17.5** |
| History list/pin/unpin/export/delete/cleanup | **COMPLETE v6.17.5** |
| Pinned run được retention bảo vệ | **COMPLETE v6.17.5** |
| FAIL_HANDOFF tự quét + snapshot ổn định source + DETAIL.log cho **mọi PATCH FAIL** | **COMPLETE v6.17.5** |
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
| `batch` manifest metadata | **COMPLETE v6.17.5** |
| AI bắt buộc khai báo predecessor failure action | **COMPLETE v6.17.5** |
| Machine-readable checklist cập nhật batch contract | **COMPLETE v6.17.5** |
| Explicit `already` semantics, không suy từ `new` ở vị trí bất kỳ | **COMPLETE v6.17.5** |
| OPS managed timeout + atomic source write | **COMPLETE v6.17.5** |
| Archive extraction budgets + collision/Windows path hardening | **COMPLETE v6.17.5** |
| Git auto-commit bảo vệ dirty target + strict rc + không leak staging khi commit FAIL | **COMPLETE v6.17.5** |
| COLLECT hard ceilings/self-output exclusion/sensitive warning | **COMPLETE v6.17.5** |
| COLLECT regex worker + hard timeout | **COMPLETE v6.17.5** |

## Windows

| Tính năng | Trạng thái |
|---|---|
| Native fullscreen selector + line fallback | COMPLETE |
| Process-tree containment `taskkill /T /F` | COMPLETE |
| Reparse/junction safety | COMPLETE |
| Native Windows runtime test lane packaged | **COMPLETE v6.17.5** |
| Native lane đã chạy trên host phát hành Linux | **N/A — cần Windows host thật** |

## Không thực hiện theo yêu cầu

- **Static target-overlap/conflict analyzer: IMPLEMENTED v6.17.7.** Analyzer cảnh báo overlap theo effective target và phân biệt overlap đã dependency-order với order-dependent overlap; không tự suy đoán arbitrary Python side effects.
- **Local identity/provenance-light: IMPLEMENTED v6.17.7.** Có project key, patch ledger `id+SHA` và recipe SHA binding. Cryptographic signature/PKI/remote trust registry vẫn NOT IMPLEMENTED.

## Giới hạn an toàn

- `continue_independent` không tiếp tục nếu source partial/unknown hoặc rollback/tool integrity không an toàn.
- Whole-batch source validation của PATCH phụ thuộc có thể defer source check tới sau dependency; runner vẫn re-preflight ngay trước execution.
- Batch atomicity bao phủ effective target set resolve trước (`targets` + preflight/recovery/OPS targets); Python side effects ngoài set vẫn không được suy đoán, nên atomic mode từ chối post-command và Git side effects.
- Arbitrary Python payload không được chạy trong preflight/sandbox để “đoán” post-dependency state.
- Regex COLLECT chạy trong worker subprocess riêng và bị hard-timeout toàn search action; catastrophic Python `re` không thể treo collector vô hạn.
- Mọi PATCH FAIL đều tạo FAIL_HANDOFF và chạy source discovery tự động: structured targets/preflight/rollback evidence → path trong traceback/compiler/full DETAIL log → bounded basename scan → one-hop local reference/same-stem companion. Source được snapshot generation-checked/no-follow theo từng file trước khi ZIP, có SHA-256; file đổi/mất chỉ bị skip và không được phép xóa snapshot của file đã thu trước đó. DETAIL log <=64 MiB được nhúng nguyên; log lớn giữ đầu+cuối. `recovery.fail_handoff=false` chỉ còn được nhận để tương thích nhưng bị ignore.
- Package SHA binding chỉ bảo đảm **byte package đã plan = byte chuẩn bị execute** trong cùng invocation; nó không tuyên bố nguồn gốc/tác giả/provenance của PATCH.

## Stop condition

Sau full regression + clean package integrity: **COMPLETE / STOP**.

## Bổ sung v6.17.7

| Tính năng | Trạng thái |
|---|---|
| `manifest.project.key` được enforce với `.python_patch_tool.json` | **COMPLETE v6.17.7** |
| Trusted `validation.profiles` resolve command từ local project config | **COMPLETE v6.17.7** |
| Persistent `UNRESOLVED_FAILURES.json` qua nhiều LAST_RUN | **COMPLETE v6.17.7** |

Registry failure là **relation-aware**: PATCH độc lập vẫn chạy; dependency hoặc effective-target overlap với failure cũ phải xử lý `batch.previous_failure`.
| Static target-overlap analyzer trước execution | **COMPLETE v6.17.7** |
| `plan` read-only + OPS private-mirror unified diff | **COMPLETE v6.17.7** |
| Patch ledger `patch.id + SHA` + ID reuse warning | **COMPLETE v6.17.7** |
| Batch recipe export/replay exact SHA | **COMPLETE v6.17.7** |
| Disk/resource preflight trước source write | **COMPLETE v6.17.7** |
| Selector `/` search/filter theo name/id/summary/target | **COMPLETE v6.17.7** |
| Cryptographic provenance/signature/PKI | **NOT IMPLEMENTED** |
