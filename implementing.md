# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.12.0**  
Trạng thái tài liệu: **LIVE TASK TRACKER — phải cập nhật ở mỗi release khi task thay đổi trạng thái.**

## Yêu cầu đang thực hiện của người dùng

> 1. Tạo file `implementing.md` cập nhật danh sách các task, mục tiêu đang thực hiện; file nằm cùng cấp với `run_python_patches.sh`.
> 2. Tạo document Markdown bằng tiếng Việt chứa danh sách tính năng và cập nhật liên tục theo tình trạng thực hiện.
> 3. Tạo hướng dẫn sử dụng HTML ngắn gọn bằng tiếng Việt, cùng cấp với `run_python_patches.sh`; hướng dẫn cách gửi toàn bộ `tools/_patch_lib/docs/` cho AI và có prompt mẫu tiếng Việt, tiếng Anh, tiếng Nga.
> 4. Sau đó hoàn thiện: kiểm tra/triệt duplicate PATCH ngay trong phiên; hỗ trợ `overview` theo schema chính xác thay vì đoán; Exact collector action schema; Full self-contained Patch Tool package. Sau khi hoàn tất phải dừng và hỏi người dùng hướng tiếp theo.

## Trạng thái

| ID | Task | Trạng thái | Acceptance |
|---|---|---|---|
| DOC-1 | `tools/implementing.md` | **COMPLETE** | Có task/mục tiêu/trạng thái của phiên hiện tại và chỉ dẫn dừng sau khi hoàn tất. |
| DOC-2 | `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md` | **COMPLETE** | Feature matrix tiếng Việt, versioned, bắt buộc cập nhật mỗi release. |
| DOC-3 | `tools/HUONG_DAN_PYTHON_PATCH_TOOL.html` | **COMPLETE** | Hướng dẫn ngắn gọn tiếng Việt + gửi toàn bộ docs cho AI + prompt VI/EN/RU. |
| DUP-1 | Duplicate PATCH ngay trong queue hiện tại | **COMPLETE** | Exact size+SHA-256; giữ item đầu theo natural order; bản trùng tên khác bị loại khỏi selector và xóa khỏi `patchs/`; không tác động COLLECT. |
| COLLECT-1 | Action `overview` | **COMPLETE** | Read-only, bounded, schema chính thức; không còn phụ thuộc private collector. |
| COLLECT-2 | Exact collector action schema | **COMPLETE** | `COLLECT_ACTION_SCHEMA.json`; preflight reject action/field không hỗ trợ trước execution. |
| COLLECT-3 | Các action cần cho request `server-log-root-causes` đã quan sát | **COMPLETE** | `overview`, `find`, `search`, `git`, `pack` self-contained. |
| CORE-1 | Full self-contained package | **COMPLETE** | Release chứa runner, utils, readonly collector, schema, dispatcher/progress và docs; clean project không cần private core cũ cho contract v6.12.0. |
| TEST-1 | Regression + clean-release verification | **COMPLETE** | Self-contained PATCH Python/OPS, COLLECT schema/actions, duplicate session, checksum, executable launcher. |
| STOP-1 | Dừng sau phạm vi này | **PENDING USER CONFIRMATION** | Không tự bắt đầu task/tính năng tiếp theo; hỏi người dùng muốn làm gì tiếp. |

## Phạm vi self-contained v6.12.0

Self-contained nghĩa là **không cần các private core file từ bản cũ để chạy contract được tài liệu v6.12.0 công bố**. Những format lịch sử không nằm trong contract hiện hành sẽ fail-closed thay vì được mô phỏng bằng suy đoán.

## Next action

**STOP. Chờ người dùng xác nhận task/tính năng tiếp theo.**
