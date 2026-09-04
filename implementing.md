# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.13.0**  
Trạng thái: **PHASE A → PHASE B → PHASE C COMPLETE — STOP và chờ người dùng xác nhận**.

## Mục tiêu được người dùng phê duyệt

Thực hiện đúng thứ tự đã thống nhất, ưu tiên correctness trước usability; không mở thêm scope:

### Phase A — PATCH correctness

| ID | Task | Trạng thái | Acceptance |
|---|---|---|---|
| A1 | Exact machine-readable PATCH package schema | **COMPLETE** | `tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json`; manifest unknown field/type/value fail-closed. |
| A2 | PATCH preflight trước project modification | **COMPLETE** | Kiểm tra payload/manifest/resources/declared hash+anchor/post-command/Git requirement trước payload; lỗi in `PREFLIGHT FAIL — project unchanged`. |
| A3 | PATCH Tool version compatibility negotiation | **COMPLETE** | `compatibility.min_tool_version/max_tool_version/max_tested_version`; min/max incompatible reject trước payload. |
| A4 | Partial-modification detection | **COMPLETE** | Khi payload/post/Git/archive fail, structured result xác định project delta bằng Git + declared targets; cảnh báo rõ `PARTIAL MODIFICATION DETECTED`, hoặc `unknown` nếu không đủ evidence. Không tự rollback suy đoán. |

### Phase B — FAIL → AI recovery

| ID | Task | Trạng thái | Acceptance |
|---|---|---|---|
| B1 | Structured `LAST_RUN.json` | **COMPLETE** | `artifacts/patch_tool/LAST_RUN.json` chứa selection/order/results/duplicates/not-executed/rc/timing. |
| B2 | Automatic failure diagnosis | **COMPLETE** | Structured diagnosis cho schema/version/source-drift/anchor/payload/syntax/python/post/Git/archive/interruption; legacy console được enrich có giới hạn. |
| B3 | Automatic `FAIL_HANDOFF.zip` | **COMPLETE** | PATCH FAIL tạo một ZIP upload cho AI gồm patch, console, structured summary, tool context, relevant safe current source và recovery request nếu có. Có thể opt-out bằng manifest recovery. |
| B4 | Source-drift/anchor mismatch → prepare COLLECT request | **COMPLETE** | Tự sinh `CODE_COLLECTION_REQUEST_patch_recovery_<sha>.zip` dùng `pack`, **không tự chạy**; người dùng chọn ở invocation kế tiếp. |

### Phase C — usability / audit

| ID | Task | Trạng thái | Acceptance |
|---|---|---|---|
| C1 | Bounded run history | **COMPLETE** | `artifacts/patch_tool/history/*.json`, local-only, giữ tối đa 30 run. |
| C2 | Resume indication sau fail-fast | **COMPLETE** | Lần mở sau báo các selected item chưa chạy; không auto-select/auto-run. Cancel/IDLE không làm mất hint còn unresolved. |
| C3 | Inspect / dry-run | **COMPLETE** | `i` trong TTY hoặc `i <index>` line selector; chạy exact preflight, hiển thị targets/commands/Git/compatibility nhưng không payload, không archive. |
| C4 | COLLECT quality summary | **COMPLETE** | PASS in `files/source/reports/zip/truncated/missing`; truncation được ghi manifest và cảnh báo người dùng/AI. |

## Invariants giữ nguyên

- Public workflow bình thường vẫn chỉ: `./tools/run_python_patches.sh`.
- PATCH chạy **in-place**; SANDBOX/worktree transaction bị loại bỏ vĩnh viễn.
- Không project/process lock; terminal khác có thể chạy độc lập do người dùng chủ động.
- Một invocation chỉ chạy tối đa 1 COLLECT và không trộn COLLECT với PATCH.
- Duplicate trong queue hiện tại vẫn collapse/remove exact bytes; local-history duplicate vẫn local project only.
- Không tự suy đoán action COLLECT ngoài exact schema hiện hành.
- Full self-contained package vẫn là acceptance bắt buộc.

## Release stop condition

**COMPLETE. DỪNG. Không tự bắt đầu task/tính năng tiếp theo. Hỏi người dùng muốn làm gì tiếp.**
