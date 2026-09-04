# Danh sách tính năng Python Patch Tool — v6.12.1

> Quy tắc duy trì: file này **phải được cập nhật ở mỗi release** khi một tính năng đổi trạng thái. Không được ghi COMPLETE nếu chưa có acceptance test tương ứng.

## Hoàn thành

| Nhóm | Tính năng | Trạng thái |
|---|---|---|
| Workflow | Entry point duy nhất `./tools/run_python_patches.sh` | COMPLETE |
| Queue | Tự phân loại PATCH / COLLECT / non-runnable evidence | COMPLETE |
| Queue | COLLECT request phải là ZIP; raw JSON bị reject | COMPLETE |
| Selector | Space, ↑/↓, a/n/d, Enter, q/Esc | COMPLETE |
| Selector | Priority PATCH 0–9, số nhỏ chạy trước, cùng số giữ thứ tự | COMPLETE |
| Selector | Clip filename + Unicode cell width + viewport terminal | COMPLETE |
| COLLECT isolation | Mỗi invocation chỉ chọn đúng 1 COLLECT, không trộn PATCH | COMPLETE |
| Concurrency | Không project/process lock; terminal khác được chạy độc lập | COMPLETE |
| Duplicate local history | Exact SHA-256 so với `patchs/patched/`, local project only | COMPLETE |
| Duplicate current session | Exact size+SHA-256; giữ 1 canonical, tự xóa file queue trùng trước selector | COMPLETE / RE-VERIFIED v6.12.1 |
| PATCH | In-place, SANDBOX/worktree bị loại bỏ | COMPLETE |
| PATCH | Python package/standalone Python + safe archive extraction | COMPLETE |
| PATCH | Data-only `PATCH_TOOL_OPS.json` chuẩn | COMPLETE |
| PATCH | Post-patch argv command + timeout | COMPLETE |
| PATCH | Git manifest policy cơ bản `add/commit/push` fail-closed | COMPLETE |
| COLLECT | Highlight một `[PRIMARY - UPLOAD THIS FILE]` | COMPLETE |
| COLLECT | Verified ZIP + CRC + request archive lifecycle | COMPLETE |
| COLLECT | `pack`, `overview`, `find`, `search`, `git` readonly | COMPLETE |
| COLLECT schema | Không tự đoán/alias action; unsupported action/field bị reject trước execution | COMPLETE / RE-VERIFIED v6.12.1 |
| COLLECT schema | Machine-readable `COLLECT_ACTION_SCHEMA.json` | COMPLETE / RE-VERIFIED v6.12.1 |
| Packaging | Full self-contained runtime cho contract v6.12.1 | COMPLETE / RE-VERIFIED v6.12.1 |
| Packaging | `SHA256SUMS`, no `.pyc`/`__pycache__`, launcher 0755 | COMPLETE |
| Docs AI | AI contract + current collection guide + exact schema | COMPLETE |
| Docs workflow | `implementing.md` live task tracker | COMPLETE |
| Docs người dùng | Hướng dẫn HTML ngắn gọn + prompt VI/EN/RU | COMPLETE |
| Docs người dùng | Prompt có Select all / Copy; không bắt người dùng hiểu Action COLLECT | COMPLETE v6.12.1 |

## Một phần / bị giới hạn có chủ đích

| Tính năng | Trạng thái | Ghi chú |
|---|---|---|
| Tương thích mọi private-core lịch sử | PARTIAL / FAIL-CLOSED | v6.12.1 tự chứa contract hiện hành; format cũ không có bằng chứng được reject thay vì đoán. |
| Advanced historical COLLECT actions ngoài schema | NOT GUARANTEED | Không tự suy diễn action ngoài schema authoritative hiện hành. |
| Phase inference nâng cao | DEFERRED | Chỉ sửa khi runtime evidence cho thấy phase telemetry sai/thiếu. |
| LAST_RUN/private historical report parity | DEFERRED | Không phải acceptance của self-contained v6.12.1. |

## Exact COLLECT schema dành cho AI/tool

Schema máy đọc được: `tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json`.

AI/tool phải sử dụng schema này; **người dùng cuối không cần biết hoặc chọn action COLLECT**. Hướng dẫn HTML cố ý chỉ mô tả workflow ở mức người dùng.

## v6.12.1

- Không thêm runtime capability mới.
- Rà lại và khóa acceptance cho duplicate-in-session, exact action schema và self-contained package.
- Đơn giản hóa HTML người dùng: bỏ phần Action COLLECT, thêm tóm tắt workflow AI và nút Select all / Copy cho prompt mẫu.
