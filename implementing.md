# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.14.0**  
Trạng thái: **HOÀN TẤT CÁC HẠNG MỤC CÒN LẠI TRONG DANH SÁCH ĐÃ ĐƯỢC PHÊ DUYỆT — STOP**.

## Mục tiêu của phiên này

Người dùng yêu cầu tiếp tục **đúng thứ tự danh sách tính năng đã đề xuất trước đó**, không lặp lại các mục đã COMPLETE ở v6.13.0, cập nhật `implementing.md` và `PYTHON_PATCH_TOOL_FEATURES_VI.md`, chỉ sửa HTML nếu thật sự cần, rồi tự động dừng và hỏi ý kiến.

Đối chiếu v6.13.0 với danh sách 12 mục ban đầu cho thấy các mục **2→11 đã COMPLETE**. Hai phần còn thiếu đúng nghĩa được thực hiện theo thứ tự cũ:

| Thứ tự gốc | ID | Task | Trạng thái | Acceptance |
|---|---|---|---|---|
| 1 | D1 | Metadata-driven safe rollback cho PATCH in-place | **COMPLETE** | Chỉ opt-in khi `recovery.rollback` khai báo exact targets và mỗi target có baseline preflight đầy đủ. Snapshot bounded trước payload. Payload/post-patch FAIL có thể restore exact declared targets; Git fingerprint phát hiện thay đổi ngoài scope và downgrade thành `PARTIAL`. Không rollback Git-policy failure, không dùng `git reset`, SANDBOX hay worktree. |
| 12 | D2 | Tool Health / self-audit trong zero-argument workflow | **COMPLETE** | `h` trong TTY/line selector chạy read-only audit; queue trống tự in health compact. Kiểm tra VERSION, managed SHA256SUMS, required runtime, executable launcher và authoritative schemas; corruption/missing file → FAIL. Không tải update và không thực thi PATCH. |

## Các mục 2→11 từ danh sách gốc

Giữ trạng thái **COMPLETE từ v6.13.0**: PATCH schema/preflight/version negotiation, diagnosis/FAIL_HANDOFF, source recollection, LAST_RUN/history/resume, inspect/dry-run và COLLECT quality summary. Không triển khai lại hoặc thay đổi semantics ngoài regression cần thiết cho D1/D2.

## Invariants giữ nguyên

- Public workflow bình thường: `./tools/run_python_patches.sh`.
- PATCH vẫn chạy **in-place**; SANDBOX/detached worktree bị loại bỏ vĩnh viễn.
- Không project/process lock; người dùng có thể chủ động chạy terminal khác.
- Một invocation tối đa 1 COLLECT và không trộn PATCH.
- Duplicate queue/local-history rules giữ nguyên.
- COLLECT/PATCH exact schemas vẫn authoritative; không tự suy đoán field/action.
- Full self-contained package là acceptance bắt buộc.
- Rollback **không mặc định bật** và không được coi là generic transaction: chỉ chạy khi manifest cung cấp contract đủ an toàn.

## Release stop condition

**COMPLETE. DỪNG. Không tự bắt đầu task/tính năng tiếp theo. Hỏi người dùng muốn làm gì tiếp.**
