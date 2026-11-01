# TaskDeck Remote — bảng trạng thái tính năng (cập nhật 01/11/2026)

**Mục tiêu hiện tại:** tương tác E2EE trong trusted native console; tập trung Terminal TaskDeck, Git, view/edit; **không mở rộng UX tải file ở giai đoạn này**.
**Tuyệt đối không diễn giải feature đang có thành đã được phép release LIVE.** `remote_control_available=false` là chính sách hiện tại. Các nhận định bên dưới là trạng thái code/CI, chưa phải nghiệm thu thực địa.

## Đã có trong main (staging, chưa LIVE)

| Lĩnh vực | Đã triển khai (experimental / staging) | Giới hạn |
| --- | --- | --- |
| Kết nối & đăng nhập | Native CLI E2EE, Agent Device ID được pin ngoài PHP, client Ed25519 key được host CLI duyệt, mật khẩu TaskDeck, TaskDeck RBAC, Portal grant và audit trên host | Cần chính sách triển khai và kiểm thử an ninh thực tế |
| Điều hướng | `pwd`, `cd`, `ls`; định tuyến đường dẫn tương đối trong workspace | `cd` chỉ là trạng thái local, không phải remote shell |
| Xem & tìm file | `view`, `lines`, `find` | Preview UTF-8 đầy đủ tối đa 4096 bytes, không đọc partial trong editor |
| Chỉnh sửa file | `edit`, `edit-line`, `insert-line`, `delete-line`, `replace`, `replace-all`, `edit-range`, `move-lines` | SHA-256 CAS, phải xác nhận, FilesWrite và 2 host flags; tối đa 4096 bytes |
| Git chỉ đọc | `git status`, `git log`, `git branches`, `git diff <file>`, `git diff --staged <file>` | Output bounded 4 KiB, lệnh được cố định phía host |
| Terminal task giới hạn | `terminal tasks`, `terminal task <ID>` | Chỉ task host đã phê duyệt; không có stdin/PTY/shell tùy ý; output tối đa 4 KiB |
| Phiên thao tác | Typed E2EE application actions | 6 actions / 5 phút; thao tác sửa thường tốn 2 lượt |

## Đang làm dở / chưa merge main

| Hạng mục | Vị trí | Trạng thái và điều kiện hoàn thành |
| --- | --- | --- |
| `git review <file>` | PR #34, `feature/remote-native-console-git-review-20261101` | Đã commit code và regression; chờ Go/PHP CI PASS trên chính xác HEAD, review và merge |
| Nghiệm thu bảo mật, NAT/CGNAT, host PHP thật | Chưa có kết quả nghiệm thu toàn diện | Phải thử với PHP/Relay bị chiếm quyền, Linux/macOS, sai/revoke grant, replay, audit |
| Phát hành LIVE | Chặn | Không bật `remote_control_available` khi các gate chưa chứng minh đạt |

**Lưu ý:** PR #32 (`edit-range`, commit merge `4a03a9fc`) và PR #33 (`move-lines`, commit merge `3345a7ca`) đã vào main sau CI Go/PHP PASS. PR #34 chưa vào main.

## Chưa hỗ trợ trong remote interactive console

1. Terminal PTY/shell tương tác thực sự: stdin/stdout/stderr streaming, resize, attach/detach, process cancellation, giới hạn luồng và backpressure.
2. Git ghi: stage/unstage, commit, amend, checkout/switch, branch creation/deletion, merge/rebase; push/pull/fetch cần policy riêng. Không cho nhập command shell tự do để “giả lập Git”.
3. View/edit file nguồn UTF-8 lớn hơn 4KiB với editor/paging an toàn, transactional concurrency và conflict handling; chỉnh nhiều file trong một transaction.
4. Giao diện remote GUI/WebView đầy đủ tương đương TaskDeck local; hiện có native console giới hạn, không cung cấp arbitrary HTTP/WS proxy.
5. Remote upload / file manager toàn diện. Một số giao thức native scoped download tồn tại ở phần khác của mã staging, nhưng **không thuộc console tương tác hiện tại** và không được mở rộng UX theo ưu tiên người dùng.
6. Phiên dài, quản lý nhiều streams, phục hồi nối lại tiến trình sau mất mạng, cancellation an toàn cho thao tác không idempotent.
7. Nghiệm thu bảo mật độc lập, chính sách MFA/recovery/revocation live trên hạ tầng thật và kiểm thử triển khai macOS/Linux qua shared-host PHP để cấp phép LIVE.

## Nguyên tắc bảo mật bất biến

- Giả định PHP có thể bị sửa đổi sau lần pairing đáng tin: PHP không thể trở thành TaskDeck principal, thay Client/Agent key đã pin, hay có mật khẩu TaskDeck.
- Relay/PHP không giải mã được app data E2EE và không thể lách login của TaskDeck để thực thi lệnh.
- Mọi action phải được giới hạn đúng loại, có RBAC host, grant còn hiệu lực, audit, deadline/quota/replay guard; không chuyển raw URL, shell argv hay JS từ PHP thành lệnh host.
- File mutation dùng full preview + SHA-256 CAS + mutation lock; không tự ghi đè file đã đổi; user confirmation chỉ là bổ sung, không thay authorization của host.

## Quy tắc báo cáo mỗi lượt tiếp theo

Mỗi lần hoàn tất công việc phải nêu rõ: (1) code/commit/PR/nhánh và CI; (2) tính năng đã vào main vs còn PR; (3) **đang làm dở**; (4) **chưa hỗ trợ**; (5) cảnh báo release nếu còn gate NO-GO.


## Bổ sung 01/11/2026 — Git review hai chiều (PR đang kiểm thử)

- Nhánh `feature/remote-native-console-git-review-20261101`.
- `git review <file>` xem toàn bộ (mỗi diff tối đa 4KiB) thay đổi
  **chưa stage** và **đã stage** với tiêu đề riêng, cùng đường dẫn.
- Dùng hai action E2EE sẵn có `git.diff` và `git.diff_staged`;
  cả hai qua TaskDeck GitStatus + FilesRead, client key allowlist,
  live Portal grant, host audit, không nhận arbitrary shell/git argv.
- Hai lượt E2EE trên tổng sáu lượt; từ chối trước khi đọc nếu quota
  chỉ còn một, nếu lỗi/thu hồi quyền tại lượt hai thì không in
  kết quả dang dở. Escape ANSI/C0/C1/bidi khi hiển thị.
- Chưa có lệnh Git **stage/unstage/commit**. Chưa có giao diện tải file.
  Chưa phát hành LIVE và giữ `remote_control_available=false`.
- `edit-range` (#32) và `move-lines` (#33) đã merge main sau
  Go + PHP CI xanh trên PR head.


## Bổ sung 01/11/2026 — Git HEAD diff (nhánh đang CI)

- Nhánh `feature/remote-git-head-diff-20261101`.
- `git diff --head <file>` thêm typed action `git.diff_head`, đọc
  **tổng thay đổi HEAD → working tree** (bao gồm staged và unstaged).
  Giới hạn 4096 bytes UTF-8, chỉ một lượt E2EE, không cắt cụt output.
- Host chỉ cho revision cố định `HEAD`, path đã được giới hạn/pin
  trong workspace, GitStatus ∩ FilesRead, host local audit, mật khẩu
  TaskDeck, client Ed25519 được duyệt, live Portal grant, E2EE.
- Không chạy shell, hooks, external diff/textconv, không ghi Git index
  hoặc worktree. Các kiểu kết quả E2EE là độc lập, chống tráo type.
- Regression: typed protocol, từ chối path/revision sai, quá 4KiB,
  type swap, host Git index không đổi khi có cả staged/unstaged,
  symlink escape, không có TaskDeck principal; CI Go/PHP mới.
- `git.diff_head` chưa được tính là hỗ trợ trong `main` trước khi
  CI trên đúng SHA PASS và PR merge. Chưa stage/commit/PTY/LIVE.
