# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.14.1**  
Trạng thái: **ROBUSTNESS AUDIT — COMPLETE / STOP**

## Mục tiêu của phiên này

Người dùng yêu cầu tiếp tục **audit bug/robustness của v6.14.0**, ưu tiên sửa lỗi có bằng chứng, không mở tính năng mới. Các invariants công khai giữ nguyên: zero-argument, PATCH in-place, không SANDBOX/worktree, không project/process lock, một invocation tối đa một COLLECT và không trộn PATCH.

## Các lỗi đã xác nhận và sửa

| ID | Hạng mục audit | Trạng thái | Acceptance |
|---|---|---|---|
| RB-1 | Rollback target có ancestor symlink có thể thoát project | **COMPLETE** | Preflight/snapshot/restore đi từng path component, reject symlink/non-directory ancestor; restore POSIX dùng pinned directory FD + `O_NOFOLLOW`. |
| RB-2 | `exists:false` dưới parent chưa tồn tại có thể để lại directory mới nhưng báo rollback PASS | **COMPLETE** | Parent của rollback target ban đầu phải tồn tại, là directory thật và không symlink trước payload; tool không tự đoán/xóa directory. |
| RB-3 | TOCTOU giữa preflight và rollback snapshot | **COMPLETE** | Snapshot re-check exact baseline ngay trước payload; drift → fail-closed trước execution. |
| SIG-1 | Chỉ signal dispatcher có thể để PATCH child/descendant tiếp tục chạy | **COMPLETE** | Dispatcher tạo process group riêng, forward SIGINT/SIGTERM; runner xử lý controlled interruption, rollback rồi trả rc130/143. |
| PROC-1 | Payload/post-command timeout chỉ giết parent, descendant tiếp tục sửa project sau rollback | **COMPLETE** | Python payload và post command chạy trong process group riêng; timeout/interruption terminate toàn group trước rollback/return. |
| LIFE-1 | PATCH tự thay thế chính queue ZIP có thể làm archive bytes chưa từng thực thi | **COMPLETE** | Snapshot exact input trước preflight; execute/archive đúng snapshot; same-name replacement khác bytes được giữ lại trong queue. |
| LIFE-2 | COLLECT request có cùng identity race | **COMPLETE** | Snapshot exact request; execute/archive exact snapshot; same-name replacement khác bytes được giữ lại để lần sau chạy. |
| HANDOFF-1 | FAIL_HANDOFF có thể attach queue ZIP mới thay vì PATCH đã thực thi | **COMPLETE** | Chỉ attach current queue package khi SHA khớp structured executed SHA; nếu khác thì omit + ghi lý do. |
| HEALTH-1 | Xóa checksum row có thể che việc runtime file bị sửa | **COMPLETE** | Tool Health yêu cầu checksum coverage cho toàn bộ required runtime; missing row/corrupt file → FAIL. |
| HEALTH-2 | Runtime/checksum path có symlink ancestor chưa được audit đủ chặt | **COMPLETE** | Tool Health lstat-walk và reject symlink ancestor cho required/checksummed paths. |
| HEALTH-3 | Normal launcher tự sinh `__pycache__` khiến Tool Health WARN ngay trên install sạch | **COMPLETE** | Launcher export `PYTHONDONTWRITEBYTECODE=1`; zero-arg IDLE không tạo cache và Tool Health PASS không cần env test đặc biệt. |
| QUEUE-1 | `patchs/` symlink có thể route queue ra ngoài project | **COMPLETE** | Discovery fail-closed `QUEUE SAFETY ERROR`; không chạy item bên ngoài project. |
| DUP-1 | Current-session duplicate có race giữa hash và unlink trong multi-terminal mode | **COMPLETE** | Duplicate được isolate atomically rồi re-hash canonical + candidate; nếu identity đổi thì restore/preserve thay vì xóa nhầm. |
| TEST-1 | Robustness regression + clean artifact verification | **COMPLETE** | Có `self_test_robustness_v6_14_1.py`; full regression và clean extraction bắt buộc PASS trước release. |
| STOP-1 | Dừng sau audit | **COMPLETE** | Không tự bắt đầu feature mới; hỏi người dùng hướng tiếp theo. |

## Invariants giữ nguyên

- Public command: `./tools/run_python_patches.sh`.
- PATCH chạy **in-place**; SANDBOX/detached worktree bị loại bỏ vĩnh viễn.
- Không project/process lock; nhiều terminal vẫn do người dùng chủ động điều khiển.
- Một invocation tối đa 1 COLLECT và không trộn PATCH.
- Duplicate local-history vẫn local-project-only; không dùng global/server history.
- PATCH/COLLECT schemas vẫn authoritative và fail-closed.
- Full self-contained package vẫn là acceptance bắt buộc.
- Safe rollback vẫn chỉ opt-in khi manifest cung cấp exact metadata; không trở thành generic transaction.

## Release stop condition

**COMPLETE. DỪNG. Không tự bắt đầu task/tính năng tiếp theo. Hỏi người dùng muốn làm gì tiếp.**
