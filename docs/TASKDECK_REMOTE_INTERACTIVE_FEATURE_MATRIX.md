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
| Git chỉ đọc | `git status`, `git log`, `git branches`, `git diff <file>`, `git diff --staged <file>`, `git review <file>`, `git diff --head <file>` | Output tối đa 4KiB; từ chối Git config có executable filters |
| Git stage (experimental) | `git stage <file>` | Đã vào main trong PR #36; host mặc định OFF; GitWrite + FilesRead + FilesWrite, SHA-256 CAS, chỉ file UTF-8 <=4KiB |
| Git unstage (experimental) | `git unstage <file>` | Đã vào main trong PR #37; host mặc định OFF; yêu cầu SHA-256 staged diff, GitWrite + FilesRead + FilesWrite và xác nhận |
| Terminal task giới hạn | `terminal tasks`, `terminal task <ID>` | Chỉ task host đã phê duyệt; không có stdin/PTY/shell tùy ý; output tối đa 4 KiB |
| Phiên thao tác | Typed E2EE application actions | 6 actions / 5 phút; thao tác sửa thường tốn 2 lượt |

## Đang làm dở / chưa merge main

| Hạng mục | Vị trí | Trạng thái và điều kiện hoàn thành |
| --- | --- | --- |
| `git commit-review` | PR #38, `feature/remote-git-commit-review-20261101` | Đã triển khai lệnh chỉ đọc; chờ Go/PHP CI trên exact HEAD và merge; chưa có Git commit ghi |
| Nghiệm thu bảo mật, NAT/CGNAT, host PHP thật | Chưa có kết quả nghiệm thu toàn diện | Phải thử với PHP/Relay bị chiếm quyền, Linux/macOS, sai/revoke grant, replay, audit |
| Phát hành LIVE | Chặn | Không bật `remote_control_available` khi các gate chưa chứng minh đạt |

**Lưu ý:** PR #32 (`edit-range`, `4a03a9fc`), PR #33 (`move-lines`, `3345a7ca`), PR #34 (`git review`, `b7211c32`) PR #35 (`git diff --head`, `bae2795d`) và PR #36 (`git stage`, `4bc4b352`) đã merge main sau Go/PHP CI PASS. PR #37 (`git unstage`, `2152e8cc`) đã merge. PR #38 chưa merge.

## Chưa hỗ trợ trong remote interactive console

1. Terminal PTY/shell tương tác thực sự: stdin/stdout/stderr streaming, resize, attach/detach, process cancellation, giới hạn luồng và backpressure.
2. Git ghi: `stage` đã merge main nhưng host OFF theo mặc định; `unstage` đã merge main (host OFF); `commit-review` ở PR #38 chỉ đọc và chưa merge; commit, amend, checkout/switch, branch creation/deletion, merge/rebase chưa triển khai; push/pull/fetch cần policy riêng. Không cho nhập command shell tự do để “giả lập Git”.
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


## Bổ sung 01/11/2026 — Git stage an toàn (PR chưa merge)

- Nhánh `feature/remote-git-safe-stage-20261101`: `git stage <file>`
  stage **toàn bộ** một file UTF-8 hiện có trong workspace, <=4KiB,
  sau preview, xác nhận `STAGE <file>`, đối chiếu SHA-256 CAS trên host.
- Dùng typed action `git.stage` và kết quả E2EE riêng chỉ chứa blob
  OID; Relay/PHP không thấy file/path/hash hoặc lấy được TaskDeck principal.
- Phải có cả hai cờ host `TASKDECK_REMOTE_GIT_WRITE=1` và
  `TASKDECK_REMOTE_GIT_STAGE=1`, mặc định OFF. Cần quyền
  GitWrite + FilesRead + FilesWrite của người dùng TaskDeck,
  E2EE, approved client key, Agent ID pin, Portal grant đang sống,
  audit trước ghi, shared mutation lease, revoke lock.
- **Không dùng git add**: `hash-object --no-filters -w --stdin` rồi
  `update-index --add --cacheinfo`, path được kiểm tra/pin, hook và
  fsmonitor tắt, Git executable tin cậy, không shell/PTY/argv tùy ý.
  Chỉ hỗ trợ Git repo có thư mục `.git` thật, không linked worktrees.
- Test: strict E2EE schema, reject flag/command injection, host flags OFF,
  CAS stale, PHP không thể giả TaskDeck login, Git clean filter không
  chạy, index đúng nội dung, file không đổi, symlink escape, replay,
  confirmation/quota/path; CI Go/PHP trên đúng PR HEAD.
- Git unstage, commit, reset, pull, push, shell/PTY và file tải về
  **vẫn chưa hỗ trợ**. Chỉ merge khi toàn bộ CI đạt; không phát hành LIVE.


## Bổ sung bảo mật — chặn Git đọc kích hoạt clean filter

- Đã xác nhận Git `status` và `diff` có thể chạy `filter.*.clean`
  của repo dù chỉ đọc và dùng `--no-ext-diff`. Đây là đường thực thi
  không được cấp phép cho quyền GitStatus/FilesRead.
- PR #36 bổ sung `remoteGitReadOnlyConfigSafe`: kiểm tra toàn bộ
  Git config có nạp includes, từ chối status/status_full/log/branches/
  diff/diff_staged/diff_head nếu có bất kỳ driver
  `filter.*.(clean|process|smudge)`; không chạy filter.
- Có regression filter độc hại tạo marker, tất cả 7 read actions phải
  từ chối mà không tạo marker; includes cũng không bypass được.
- Tác động có chủ đích: repo có Git LFS/custom filters trong config
  sẽ không dùng các Git read views từ xa cho đến khi có thiết kế
  filter-free an toàn hơn. Stage typed riêng vẫn dùng `--no-filters`.
- **PR #36 đang CI, chưa vào main. LIVE NO-GO.**


## Bổ sung 01/11/2026 — Git unstage (PR đang CI)

- PR #36 `git stage` và chặn Git read clean filters đã merge `main`
  tại `4bc4b352`, sau Go và PHP CI trên SHA `97642d09` PASS.
- Nhánh `feature/remote-git-safe-unstage-20261101`: thêm lệnh
  `git unstage <file>` vào native E2EE console, yêu cầu staged diff
  đầy đủ tối đa 4KiB, xác nhận chính xác `UNSTAGE <file>`,
  CAS SHA-256 của staged diff, hai lượt trên tối đa sáu lượt E2EE.
- Host chỉ khôi phục **một** index entry về blob/mode của `HEAD`;
  file mới được stage sẽ bị loại khỏi index, giữ nguyên working tree.
  Không gọi `git reset`, hooks, external filters hay shell; không nhận
  revision/argv do PHP, Relay hoặc người dùng cung cấp.
- Có hai cờ host độc lập với stage:
  `TASKDECK_REMOTE_GIT_WRITE=1` và `TASKDECK_REMOTE_GIT_UNSTAGE=1`;
  mặc định tắt. Bắt buộc GitWrite, FilesRead, FilesWrite TaskDeck
  RBAC, client key được host duyệt, Agent pin, live Portal grant,
  audit trước side effect và revocation lock.
- Giới hạn: chỉ file còn trong working tree, Git root có thư mục
  `.git` thật, không nhận staged deletion, symlink, submodule,
  conflicted index, binary diff, Git LFS/custom executable filters,
  không hứa atomic với external Git writers ngoài TaskDeck.
- Kiểm thử E2EE, console, staged/unstaged khác nhau, file mới,
  file khác không đổi, denied viewer/flags, stale CAS, replay,
  symlink và parser index. Chưa merge cho tới Go/PHP CI trên exact
  PR head PASS. `remote_control_available=false` và LIVE NO-GO.

## Bổ sung — Git commit-review riêng (PR chưa merge)

- Nhánh `feature/remote-git-commit-review-20261101` phát triển
  `git commit-review` (đọc thuần, chưa có `git commit`). Trả về
  HEAD oid, branch ref, SHA-256 toàn bộ Git index và diff **tất cả**
  những gì đã stage, chỉ nếu kết quả đầy đủ <=4KiB UTF-8.
- Kiểm tra repo Git root thực, `.git/index` regular không symlink,
  giới hạn 1MiB index, từ chối object-store symlink/alternates và\n  replace refs, HEAD commit hiện hữu, branch symbolic (không
  detached), đối chiếu lại index/HEAD/branch trước phản hồi.
- Yêu cầu host GitWrite **và** FilesRead TaskDeck, live Portal grant,
  approved client key, mật khẩu TaskDeck native, E2EE authenticated
  và audit riêng. Host dùng trusted Git với env cách ly,
  chặn executable Git filter, không shell, hooks hay Git mutation.
- Test: schema E2EE no args, type swap, output quá 4KiB bị từ chối,
  host không sửa index/HEAD, nhiều trạng thái staged+unstaged,
  viewer/PHP-only bị chặn, clean filter và no staged diff bị chặn.
- `git commit` thực sự chưa triển khai: cần snapshot index cố định,
  commit-tree/update-ref CAS, audit trước side effect và an toàn
  đối với external Git writers. Remote LIVE vẫn NO-GO.
