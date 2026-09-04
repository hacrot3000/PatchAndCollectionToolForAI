# Danh sách tính năng Python Patch Tool — v6.17.14

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
| Recovery planner ràng buộc predecessor theo từng successor liên quan, không theo vị trí item đầu | **COMPLETE v6.17.10** |
| Multiple unresolved related predecessors → fail-closed, yêu cầu Smart Resume | **COMPLETE v6.17.10** |
| `run_anyway` legacy được ignore; dependency/target relation FAIL luôn BLOCKED | **COMPLETE v6.17.10** |
| Recipe/plan giữ đúng effective failure + transaction policy | **COMPLETE v6.17.10** |
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

## Bổ sung v6.17.8 — command khi FAIL và execution robustness

| Tính năng | Trạng thái |
|---|---|
| `manifest.on_failure.commands` — chỉ chạy sau execution failure | **COMPLETE v6.17.8** |
| Failure command chạy sau rollback attempt; giữ nguyên RC/lỗi PATCH gốc | **COMPLETE v6.17.8** |
| `on_failure` FAIL/timeout/lingering => safety-stop continuation | **COMPLETE v6.17.8** |
| Structured `on_failure` / `post_patch` / validation timeout-vs-exit-code evidence | **COMPLETE v6.17.8** |
| `post_patch.run_when_no_changes` — mặc định skip post command khi PATCH không tạo delta; `true` mới chạy cả no-op | **DOCUMENTED/ENFORCED v6.17.10** |
| `patch.version/phase/phase_under_test/summary/regression_scope` là metadata, không phải implicit runtime gate | **DOCUMENTED v6.17.10** |
| `git.timeout_seconds` + managed Git hook/push process-tree containment | **COMPLETE v6.17.8** |
| `exit 124` không còn bị nhận nhầm là timeout | **COMPLETE v6.17.8** |
| POSIX leader exit nhưng child nền còn sống => cleanup + không false PASS; Windows native normal-exit detection chưa được tuyên bố PASS | **COMPLETE POSIX v6.17.8 / WINDOWS NATIVE VERIFY PENDING** |
| Ctrl+C trong post/on-failure command propagate thành global interruption | **COMPLETE v6.17.8** |
| Managed commands non-interactive (`stdin=DEVNULL`) | **COMPLETE v6.17.8** |
| Internal `PTV_*` result/lock channels không leak vào project commands | **COMPLETE v6.17.8** |
| Batch preflight timeout cleanup nested runner/OPS process tree | **COMPLETE v6.17.8** |
| COLLECT Git context fail-visible + fsmonitor/external-diff/textconv disabled | **COMPLETE v6.17.8** |

`on_failure.commands` không chạy cho package/schema/preflight rejection hoặc user interruption. `transaction_policy=batch` reject failure commands do side effect không target-bounded.

### Bổ sung audit cuối v6.17.8

| Tính năng | Trạng thái |
|---|---|
| Dispatcher `inspect/preview/validate` có outer timeout + process-tree/signal containment | **COMPLETE v6.17.8** |
| Dispatcher COLLECT foreground không còn dùng bare `subprocess.run`; Ctrl+C/SIGTERM forward theo tree | **COMPLETE v6.17.8** |
| `internal_error` sau khi execution bắt đầu => rollback phù hợp → `on_failure`, giữ lỗi gốc primary | **COMPLETE v6.17.8** |


## Sửa lỗi v6.17.9 — batch preflight continuation

| Hành vi | Trạng thái |
|---|---|
| `continue_independent` + `transaction=patch`: PREFLIGHT_FAIL chỉ fail item đó | **COMPLETE v6.17.9** |
| Dependency/effective-target relation với PREFLIGHT_FAIL => `BLOCKED` | **COMPLETE v6.17.9** |
| PATCH độc lập vẫn chạy sau item-local PREFLIGHT_FAIL | **COMPLETE v6.17.9** |
| `transaction=batch`, global preflight error, explicit `fail_fast` vẫn fail-closed | **COMPLETE v6.17.9** |
| Report đếm đúng `PREFLIGHT_FAIL` và `failed_item`, không gán sai `NOT_EXECUTED` | **COMPLETE v6.17.9** |

## v6.17.12 — Lịch sử zero-argument và live status

| Tính năng | Trạng thái |
|---|---|
| Selector zero-argument có dòng `HISTORY`, mở bằng `↑/↓ + Enter` | **COMPLETE v6.17.12** |
| Queue rỗng: in warning/AUTO STATUS/Tool Health rồi tự mở history ở TTY | **COMPLETE v6.17.12** |
| HISTORY/report overview in sẵn `Important files` với path tuyệt đối cho COLLECT result/request, FAIL_HANDOFF, recovery/replay/archive và log chẩn đoán; artifact đã mất được đánh dấu `[missing]` | **COMPLETE v6.17.12** |
| History mặc định chọn lần PASS gần nhất có công việc thực sự | **COMPLETE v6.17.12** |
| Reopen report xem archived PATCH ZIP, COLLECT result/request, FAIL_HANDOFF, recovery COLLECT, detail/aggregate log, source diff, support ZIP | **COMPLETE v6.17.12** |
| Fixed live PATCH status header `WAITING/RUNNING/PASS/FAILED/BLOCKED/...` khi TTY hỗ trợ | **COMPLETE v6.17.12 (best-effort + fallback)** |
| `PTV_DISABLE_LIVE_STATUS=1` tắt live header; raw log trên disk giữ nguyên | **COMPLETE v6.17.12** |



## v6.17.13 — History hữu ích + Smart Resume đúng ngữ nghĩa

| Hành vi | Trạng thái |
|---|---|
| HISTORY mặc định ẩn toàn bộ run `IDLE`; IDLE mới chỉ cập nhật `LAST_RUN`, không tạo thêm `history/*.json` | **COMPLETE v6.17.13** |
| Dòng HISTORY hiển thị `tên PATCH/COLLECT → ngày giờ → trạng thái`, không dùng run-id/counts làm thông tin chính | **COMPLETE v6.17.13** |
| Queue từng có package nhưng tất cả bị duplicate/auto-filter: in `QUEUE CLEANUP SUMMARY` và chờ Enter trước khi mở HISTORY | **COMPLETE v6.17.13** |
| Zero-work zero-argument không tạo run/LAST_RUN/history/log; không tự mở HISTORY | **COMPLETE v6.17.14** |
| SMART RESUME tự bật chỉ khi LAST_RUN FAIL còn recovery item thực sự trong queue; failure cũ không chiếm màn hình PATCH/COLLECT mới độc lập | **COMPLETE v6.17.14** |
| Unresolved failure cũ vẫn được planner enforce cho successor liên quan và replay package vẫn được bảo vệ khỏi duplicate suppression | **COMPLETE v6.17.13** |
| Report menu dùng `1..N=detail`; run không có item không hiện action detail/diff/support vô nghĩa | **COMPLETE v6.17.13** |

- Cleanup v6.17.13 ưu tiên xóa IDLE unpinned cũ trước để chúng không chiếm quota 30 run PATCH/COLLECT hữu ích.

- HISTORY hiển thị ngày giờ theo timezone local của máy chạy tool; JSON vẫn lưu UTC chuẩn.
