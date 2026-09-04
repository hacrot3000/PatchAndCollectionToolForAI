# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.12.1**  
Trạng thái tài liệu: **LIVE TASK TRACKER — phải cập nhật ở mỗi release khi task thay đổi trạng thái.**

## Yêu cầu đang thực hiện của người dùng

> Hoàn thiện đúng các mục đã chốt rồi dừng lại:
>
> 1. Cập nhật `implementing.md` và các tài liệu dành cho AI.
> 2. Chỉnh hướng dẫn HTML cho người dùng:
>    - loại bỏ phần giải thích **Action COLLECT** vì người dùng không cần quan tâm;
>    - thêm phần tóm tắt công cụ làm được gì và quy trình thông thường khi dùng cùng Chat AI;
>    - nếu có thể, thêm nút **Select all** và **Copy** cho từng prompt mẫu.
> 3. Rà lại và bảo đảm các capability đã yêu cầu trước đó vẫn hoàn chỉnh:
>    - tự phát hiện duplicate PATCH ngay trong queue hiện tại, khác tên nhưng exact checksum giống nhau thì giữ một và tự loại bản trùng;
>    - không tự đoán action như `overview`;
>    - Exact collector action schema;
>    - Full self-contained Patch Tool package.
> 4. Sau khi hoàn tất phải dừng, không tự bắt đầu tính năng khác.

## Trạng thái

| ID | Task | Trạng thái | Acceptance |
|---|---|---|---|
| DOC-UX-1 | Bỏ phần Action COLLECT khỏi HTML người dùng | **COMPLETE** | HTML không hiển thị danh sách/action schema kỹ thuật; trách nhiệm thuộc AI/tool. |
| DOC-UX-2 | Thêm tóm tắt chức năng + workflow với Chat AI | **COMPLETE** | Có phần ngắn gọn ở cuối hướng dẫn mô tả chức năng và chu trình làm việc thông thường. |
| DOC-UX-3 | Nút Select all / Copy cho prompt VI/EN/RU | **COMPLETE** | Mỗi prompt có hai nút; Copy có Clipboard API + fallback cho `file://`. |
| DOC-AI-1 | Đồng bộ tài liệu AI lên v6.12.1 | **COMPLETE** | AI contract/guide/schema/status vẫn là nguồn kỹ thuật authoritative; user guide cố ý ẩn chi tiết action. |
| DUP-1 | Duplicate PATCH ngay trong queue hiện tại | **COMPLETE / RE-VERIFIED** | Exact size+SHA-256; giữ item đầu theo natural order; bản trùng khác tên bị xóa khỏi `patchs/` trước selector; acceptance 3-file PASS. |
| COLLECT-1 | Không tự đoán action như `overview` | **COMPLETE / RE-VERIFIED** | `overview` là action có schema + implementation thực; unsupported action/field bị preflight reject. |
| COLLECT-2 | Exact collector action schema | **COMPLETE / RE-VERIFIED** | `COLLECT_ACTION_SCHEMA.json` là source of truth machine-readable và được queue preflight enforce. |
| CORE-1 | Full self-contained package | **COMPLETE / RE-VERIFIED** | Release chứa runner, utils, readonly collector, schema, dispatcher/progress/docs; clean project test không cần private core cũ. |
| TEST-1 | Regression + clean-release verification | **COMPLETE** | Master suite và clean-extraction artifact tests PASS; checksum/executable/docs UX đều được kiểm tra. |
| STOP-1 | Dừng sau phạm vi này | **COMPLETE** | Không tự bắt đầu task/tính năng tiếp theo; chờ người dùng xác nhận hướng tiếp theo. |

## Phạm vi self-contained v6.12.1

Self-contained nghĩa là **không cần các private core file từ bản cũ để chạy contract được tài liệu v6.12.1 công bố**. Những format lịch sử không nằm trong contract hiện hành vẫn fail-closed thay vì được mô phỏng bằng suy đoán.

## Next action

**STOP. Chờ người dùng xác nhận task/tính năng tiếp theo.**
