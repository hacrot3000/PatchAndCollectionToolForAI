# TaskDeck Remote — native interactive console handoff (31/10/2026)

## Mục tiêu hiện tại

Tập trung **tương tác**, Terminal TaskDeck đã được host duyệt, Git read-only,
xem/sửa file; **chưa triển khai giao diện tải file** và không mở Terminal PTY,
HTTP/WS proxy, shell tùy ý, Git push/fetch/pull.

Remote Control tổng quát **NO-GO**. `remote_control_available=false` không đổi.
Không dùng PHP để nhận mật khẩu TaskDeck, tải binary client hoặc cấp thêm khóa
TaskDeck. Client native phải được xác minh độc lập PHP; Agent Device ID phải
được pin từ host, và host CLI mới có quyền duyệt client public key.

## Tiến độ code và điểm đã thực hiện

- PR #28 (đã merge main): `replace <file>` thực hiện read 4KiB,
  yêu cầu đúng 1 match, xem đầy đủ kết quả dự kiến, xác nhận `YES`,
  ghi `project.file.replace_text` bằng SHA-256 CAS đã có; hiển thị
  dòng mới, thoát ANSI/C0/C1/bidi trước khi ghi ra terminal.
- PR #29 (đã merge `main` tại `c788bef25f2d6349ebbff37385e6821c292056cc`): `find <file>` đọc **toàn bộ** UTF-8 <=4096 bytes trong một
  E2EE action, hiển thị số lần khớp và số dòng (không gọi host grep).
- PR #29 (đã merge): `replace-all <file>` đọc đầy đủ file, thay mọi chuỗi literal,
  yêu cầu xác nhận **`YES N`** với số match N chính xác; trả về lỗi
  local nếu không tìm thấy, không thay đổi, dài hơn 4096 bytes, quota
  chưa đủ hoặc nhập ký tự điều khiển.
- Phòng ngừa Terminal spoof: không in trực tiếp ANSI/Unicode format &
  line/paragraph separators từ kết quả file/Git/Task.
- `find/replace/replace-all/edit` vẫn chỉ thực hiện action typed E2EE
  và dùng TaskDeck local RBAC, client key, session grant và audit trước
  lệnh ghi. Không nhận lệnh shell từ CLI, PHP hoặc Relay.

## Test (run trên đúng HEAD và ghi lại kết quả)

```sh
cd vscode_tasks_menu_go
go test -race ./cmd/taskdeck_remote_login -run TestInteractive -count=1
go test -race ./internal/remotetransport ./internal/remoteportal ./internal/remoterelay
go test -race ./internal/server -run TestRemote -count=1
```

PR #29: https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/29

Go Workflow: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/workflows/taskdeck_remote_agent.yml
PHP Workflow: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/workflows/taskdeck_remote_portal.yml

CI xanh chứng minh phạm vi được test, **không phải** release approval.
Không merge nếu có Go/PHP workflow FAIL trên đúng mã nguồn PR.
Host phải bật riêng `TASKDECK_REMOTE_FILE_REPLACE=1` và
`TASKDECK_REMOTE_TEXT_EDIT=1` để thực sự ghi; giữ mặc định OFF.

## Phần còn thiếu

1. Terminal PTY thực sự: quản lý stdin/stdout/stderr, resize, detach/close,
   backpressure, stream framing, quota, cancellation và RBAC; chưa phát hành.
2. Đọc/sửa file lớn hơn 4KiB theo dạng editor, giữ phiên không ghi đè
   dữ liệu mới, hủy tác vụ giữa chừng, và phối hợp local mutation lock.
3. Git write/commit/push phải có approval riêng và cơ chế revocation, không
   dùng trực tiếp arbitrary shell command.
4. Kiểm thử hostile PHP/Relay end-to-end trên mỗi action/stream mới, Linux/
   macOS thật, NAT/CGNAT, kiểm thử hiệu năng và pentest độc lập.
5. Bản phát hành có provenance, hướng dẫn triển khai rõ ràng và rollback.
   Chưa tự ý bật Remote Control LIVE trên các điểm hiện tại.

## Bổ sung 01/11/2026 — view/edit một dòng trên native client

Branch: `feature/remote-native-console-line-edit-20261101`. Dựa trên `main` commit `c788bef25f2d6349ebbff37385e6821c292056cc`.

- `lines <file>`: in số thứ tự các dòng từ một lần preview <=4KiB.
- `edit-line <file>`: hiện tất cả dòng, nhập số dòng và literal UTF-8 mới,
  hiển thị cũ/mới, xác nhận riêng `YES N`, dùng **SHA-256 CAS toàn file**.
- Các byte khác được giữ nguyên, bao gồm LF/CRLF và newline cuối.
- Từ chối số dòng không tồn tại, dòng ảo sau newline cuối, no-op,
  ký tự điều khiển/Unicode vô hình, vượt 4KiB, sai xác nhận,
  không đủ hai lượt E2EE hoặc đường dẫn không hợp lệ.
- Không thay đổi PHP/Relay/Agent dispatcher; không nâng quyền,
  không mở network API, PTY hay tải file.

Test mới nằm trong
`vscode_tasks_menu_go/cmd/taskdeck_remote_login/interactive_test.go`
và được workflow Go hiện có chạy với `go test -race ... -run TestInteractive`
cho các trường hợp tích hợp; kiểm tra thêm test helper
`TestConsoleReplaceLine` khi nghiệm thu.

**Vẫn NO-GO cho Remote Control tổng quát.**

## Bổ sung 01/11/2026 — Chèn/xóa dòng an toàn

Nhánh: `feature/remote-native-console-line-insert-delete-20261101`, bắt đầu từ main `0feb73467586fea119b8e5a6be83020e4baab33d`.

- `insert-line <file>`: đọc nguyên file <=4096 bytes, chọn chèn
  trước dòng N hoặc nối cuối N=last+1, xem toàn bộ kết quả, yêu cầu
  `INSERT N` trước khi ghi.
- `delete-line <file>`: chọn dòng N đang tồn tại, xem nội dung bị xóa
  và toàn file sau xóa, yêu cầu `DELETE N`.
- Bảo toàn byte các dòng còn lại, LF/CRLF, newline cuối; không tự
  tạo file mới hay nhận lệnh tùy ý từ PHP/Relay.
- Chỉ dùng `project.file.replace_text` đã có: mật khẩu TaskDeck,
  client public key được host CLI duyệt, RBAC FilesWrite, Portal grant,
  local audit, mutation lock và SHA-256 CAS đều bắt buộc; host phải
  bật **hai cờ opt-in độc lập**; mặc định OFF.
- Negative tests: khoảng dòng, dòng ảo sau newline, empty/CRLF,
  quá 4096 bytes, Unicode spoof, sai xác nhận, hết quota, traversal.
- CI và review bảo mật vẫn là điều kiện merge. Không tự tuyên bố LIVE
  khi chỉ có test native console xanh.
