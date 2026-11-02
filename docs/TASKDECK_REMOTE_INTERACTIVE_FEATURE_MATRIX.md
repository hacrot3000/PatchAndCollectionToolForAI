# TaskDeck Remote — bảng trạng thái tính năng (cập nhật 01/11/2026)

**Mục tiêu hiện tại:** chuyển trọng tâm sang trusted native Remote GUI trên Linux/macOS; Files view, Git metadata/diff và host-approved Tasks qua E2EE; **không mở rộng UX tải file**. CLI vẫn còn để nghiệm thu các chức năng khác.
**Tuyệt đối không diễn giải feature đang có thành đã được phép release LIVE.** `remote_control_available=false` là chính sách hiện tại. Các nhận định bên dưới là trạng thái code/CI, chưa phải nghiệm thu thực địa.

## Đã có trong main (staging, chưa LIVE)

| Lĩnh vực | Đã triển khai (experimental / staging) | Giới hạn |
| --- | --- | --- |
| Native Remote GUI (PR chờ CI) | Chạy native client với `--gui` sau khi xác thực Agent ghim Ed25519 + password TaskDeck | Trang Files (duyệt/xem), Git (status/log/branches/diff), Tasks (host-approved review/run/start/status/output/cancel); local-only 127.0.0.1, CSP, Origin+CSRF token, không mã PHP, 6 lượt E2EE/phiên, 5 phút; **chưa phải GUI tương đương local** |\n| Kết nối & đăng nhập | Native CLI E2EE, Agent Device ID được pin ngoài PHP, client Ed25519 key được host CLI duyệt, mật khẩu TaskDeck, TaskDeck RBAC, Portal grant và audit trên host | Cần chính sách triển khai và kiểm thử an ninh thực tế |
| Điều hướng | `pwd`, `cd`, `ls`; định tuyến đường dẫn tương đối trong workspace | `cd` chỉ là trạng thái local, không phải remote shell |
| Xem & tìm file | `view`, `lines`, `find` | Preview UTF-8 đầy đủ tối đa 4096 bytes, không đọc partial trong editor |
| Xem trang dòng lớn | `view-lines <file> <start-line>` | Đã merge main PR #41 (`0a61f04e`); chỉ đọc nguồn UTF-8 <=2MiB, hash SHA-256 toàn file; mỗi trang <=4KiB |
| Xem trang tiếp | `view-next` | Đã merge main PR #42 (`d6dc274b`), dùng NEXT_LINE đã xác thực; không tải file |
| Tìm literal trong file lớn | `find-lines <file> :: <literal>` | Đã merge main PR #43 (`19c95c6e`); nguồn UTF-8 <=2MiB, tất cả dòng khớp phải vừa 4KiB, E2EE FilesRead |
| Chỉnh sửa 1 dòng file lớn (staging trên main) | `edit-large-line <file> <line>` | UTF-8 <=2MiB, 1 dòng <=512B, full-file SHA-256 CAS, khóa ghi/confirm; host có 3 cờ opt-in OFF mặc định; **đã merge PR #50 (`c2e6f621`)** |\n| Chỉnh sửa file | `edit`, `edit-line`, `insert-line`, `delete-line`, `replace`, `replace-all`, `edit-range`, `move-lines` | SHA-256 CAS, phải xác nhận, FilesWrite và 2 host flags; tối đa 4096 bytes |
| Git chỉ đọc | `git status`, `git log`, `git branches`, `git diff <file>`, `git diff --staged <file>`, `git review <file>`, `git diff --head <file>` | Output tối đa 4KiB; từ chối Git config có executable filters |
| Git stage (experimental) | `git stage <file>` | Đã vào main trong PR #36; host mặc định OFF; GitWrite + FilesRead + FilesWrite, SHA-256 CAS, chỉ file UTF-8 <=4KiB |
| Git unstage (experimental) | `git unstage <file>` | Đã vào main trong PR #37; host mặc định OFF; yêu cầu SHA-256 staged diff, GitWrite + FilesRead + FilesWrite và xác nhận |
| Git commit-review | `git commit-review` | Đã vào main trong PR #38 (`f2357459`), chỉ xem toàn bộ staged diff <=4KiB, không ghi |
| Git commit (experimental) | `git commit` | Đã vào main trong PR #39 (`4241e633`), phải host opt-in, review+HEAD/index CAS, index.lock; không tự bật LIVE |
| Git branch-create (experimental) | `git branch-review`, `git branch-create <slug>` | Đã vào main PR #45 (`6ee632f1`), chỉ tạo `refs/heads/taskdeck/<slug>` qua HEAD/CAS, cần 2 host flags; không checkout |
| Git branch-delete cùng HEAD (staging trên main) | `git branch-delete taskdeck/<slug>` | Chỉ xóa ref dư thừa cùng HEAD khi sạch, không phải branch đang dùng; review/CAS, host 2 flags mặc định OFF; **đã merge PR #49 (`5c9891ac`)** |\n| Git switch cùng HEAD (experimental) | `git switch taskdeck/<slug>` | Đã merge PR #47 (`79b1818b`); chỉ đổi symbolic HEAD khi target cùng commit, index/worktree sạch; 2 host flags mặc định OFF, không checkout commit khác |
| Terminal task giới hạn | `terminal tasks`, `terminal task <ID>` | Chỉ task host đã phê duyệt; không có stdin/PTY/shell tùy ý; output tối đa 4 KiB |
| Managed task output theo cursor (đã merge #53 vào main, staging) | `terminal task-output <handle> <byte-offset>` | Mỗi lượt chỉ tối đa 2048 byte UTF-8, có OFFSET/NEXT/TOTAL/EOF; offset cũ bị evict phải báo lỗi, không tự bỏ qua dữ liệu; broker dùng Session Manager cục bộ; Go/PHP CI PASS; chưa LIVE; không phải streaming/PTY, giới hạn 6 lượt E2EE/phiên và cần đọc trước khi dùng terminal task-status kết thúc |\n| Managed terminal task (đã merge #52 vào main, staging) | `terminal task-start <ID>`, `terminal task-status <handle>`, `terminal task-cancel <handle>` | Task host phê duyệt, 30 giây tối đa, 1 task/phiên, mã E2EE ngẫu nhiên, cleanup theo session, cờ host riêng mặc định OFF; CI Go/PHP PASS, chưa LIVE; không shell, stdin, PTY |\n| Terminal report (đã merge #51 vào main, staging) | `terminal task-report <ID>` | Exit code lấy từ metadata host, output UTF-8 <=3500B, 2 lượt xác nhận E2EE; task <=7 giây, không mở stdin/PTY; Go/PHP CI PASS; chưa LIVE |
| Phiên thao tác | Typed E2EE application actions | 6 actions / 5 phút; thao tác sửa thường tốn 2 lượt |

## Đang làm dở / chưa merge main

| Hạng mục | Vị trí | Trạng thái và điều kiện hoàn thành |
| --- | --- | --- |

| Nghiệm thu bảo mật, NAT/CGNAT, host PHP thật | Chưa có kết quả nghiệm thu toàn diện | Phải thử với PHP/Relay bị chiếm quyền, Linux/macOS, sai/revoke grant, replay, audit |
| Phát hành LIVE | Chặn | Không bật `remote_control_available` khi các gate chưa chứng minh đạt |

**Lưu ý:** PR #32 (`edit-range`, `4a03a9fc`), PR #33 (`move-lines`, `3345a7ca`), PR #34 (`git review`, `b7211c32`) PR #35 (`git diff --head`, `bae2795d`) và PR #36 (`git stage`, `4bc4b352`) đã merge main sau Go/PHP CI PASS. PR #37 (`git unstage`, `2152e8cc`) đã merge. PR #38 (`git commit-review`, `f2357459`) đã merge. PR #39 (`git commit`, `4241e633`) đã merge. PR #40 (`terminal task` xác nhận, `aae6203a`) và PR #41 (`view-lines`, `0a61f04e`) đã merge. PR #42 (`view-next`, `d6dc274b`) đã merge. PR #43 (`find-lines`, `19c95c6e`) đã merge sau Go/PHP CI PASS. PR #45 (`git branch-create`, `6ee632f1`) và PR #47 (`git switch` cùng HEAD, `79b1818b`) đã merge sau Go/PHP CI PASS trên commit cuối.

## Chưa hỗ trợ trong remote interactive console

1. Terminal PTY/shell tương tác thực sự: stdin/stdout/stderr streaming, resize, attach/detach, process cancellation, giới hạn luồng và backpressure.
2. Git ghi: `stage` đã merge main nhưng host OFF theo mặc định; `unstage` đã merge main (host OFF); `commit-review` đã merge main; `commit` đã merge main nhưng host OFF; amend, checkout/switch sang commit khác, branch deletion giới hạn cùng HEAD đã merge PR #49; merge/rebase chưa triển khai; chỉ tạo branch mới và switch cùng HEAD trong `taskdeck/` đã merge, host mặc định OFF; push/pull/fetch cần policy riêng. Không cho nhập command shell tự do để “giả lập Git”.
3. Edit nguồn UTF-8 >4KiB: thao tác thay đúng một dòng có full-file SHA-CAS đã vào main (PR #50), mặc định OFF; editor nhiều dòng, sửa vùng bất kỳ và chỉnh nhiều file trong cùng transaction chưa hỗ trợ.
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

## Bổ sung 01/11/2026 — Git commit có CAS (PR chưa merge)

- `feature/remote-git-pinned-commit-20261101`: `git commit` không có
  flags, cần `git.commit_review` đầy đủ trước, xác nhận
  `COMMIT <12-hex-HEAD>` và commit message ASCII 3–96 ký tự.
- Hai typed E2EE actions, review SHA-256 toàn bộ, request/response tách
  kiểu và replay protection. PHP/Relay không biết message/hash.
- Host mặc định OFF: cần đồng thời `TASKDECK_REMOTE_GIT_WRITE=1` và
  `TASKDECK_REMOTE_GIT_COMMIT=1`. Cần local TaskDeck password,
  approved Ed25519 client key, Agent pin, Portal grant live, GitWrite,
  FilesRead, FilesWrite, audit trước side effect, session revoke lock.
- Host tạo tree từ **bản sao Git index đúng SHA**; chạy fixed
  `write-tree`, `commit-tree` với identity host, cấm hooks/filters/
  signing/editor, rồi `update-ref` bằng expected-old-OID. Không cho
  PHP/user truyền arbitrary Git argv, branch, parent hoặc script.
- Refuse HEAD/index thay đổi, clean filter, Git alternates, symlink
  object store, review không đầy đủ/binary, index quá 1MiB, không staged
  changes; giữ working tree và Git index nguyên vẹn.
- Giới hạn cần nghiệm thu: Git process bên ngoài TaskDeck có thể sửa
  index giữa lần xác minh CAS cuối và `update-ref`; CAS branch vẫn
  chống ghi đè HEAD thay đổi nhưng chưa atomic cùng index. Có thể
  tạo orphan Git objects nếu update-ref thất bại.
- Có regression protocol, UI, Git real filesystem, hooks, denied
  viewer/PHP-only, opt-out, stale/replay, CAS. Chỉ merge sau exact-head
  Go/PHP CI PASS. `remote_control_available=false`; LIVE NO-GO.

- Bổ sung khóa `.git/index.lock` với O_EXCL cho toàn bộ kiểm tra
  index, tạo commit và update-ref. Từ chối nếu có lock của tiến trình
  Git khác, xóa lock của TaskDeck sau khi xong. Chỉ bảo vệ các writer
  tuân thủ Git lock; cần nghiệm thu writer sửa file Git trực tiếp.

## Bổ sung — Terminal TaskDeck có xác nhận host (PR chờ CI)

- Nhánh `feature/remote-terminal-reviewed-task-20261101`: lệnh
  `terminal task <ID>` yêu cầu trước đó một `task.list` authenticated
  E2EE trên chính phiên, hiển thị tên task từ host, nhập chính xác
  `RUN TASK <ID>` rồi mới gọi `task.run_text`.
- Không cho chạy task không có trong catalogue, đầu vào có dấu điều
  khiển/bidi, danh mục thiếu newline, trùng ID/không canonical và vượt
  quota 6 lượt E2EE; từ chối fail-closed, không tự thực thi.
- Host **vẫn** xác thực lại toàn bộ hash định nghĩa task và cờ host,
  principal TaskDeck, TasksRun, Portal live grant, process allowlist
  và audit trước side effect. Tính năng không mở shell tuỳ ý.
- Có test native thao tác 2 lượt, cancel, malformed list, zero/leading
  zero/overflow ID, terminal spoof, quota. Cần Go/PHP CI cuối PASS
  trước merge. LIVE tiếp tục NO-GO.

## Bổ sung — xem file nguồn theo trang dòng E2EE (PR chưa merge)

- Nhánh `feature/remote-native-file-line-pages-20261101`: lệnh
  `view-lines <file> <start-line>` đọc từng trang từ file nguồn
  UTF-8 **tối đa 2MiB**, trang E2EE đầy đủ <=4KiB.
- Host pin descriptor vào workspace, đối chiếu độ ổn định trước/sau,
  tính SHA-256 **toàn bộ file**, đếm tổng số dòng, NEXT_LINE và
  chỉ in các dòng hoàn chỉnh (không cắt giữa dòng).
- Client xác thực metadata chặt chẽ, nhớ hash theo file và dừng in
  nếu source thay đổi giữa hai trang trong cùng phiên; output escape
  ANSI/bidi/control. Cần local TaskDeck login, FilesRead, live Portal
  grant, E2EE và host audit. Không tự download, không sửa file.
- File edit và view đầy đủ vẫn yêu cầu <=4KiB; lệnh mới chỉ đọc.
  Chưa hoàn thiện GUI, scrolling dài hạn hay transactional edit lớn.
  PR cần Go/PHP CI PASS trước merge, LIVE vẫn NO-GO.

## Bổ sung — điều hướng view-next (đã merge PR #42)

- `feature/remote-native-file-view-next-20261101` (xây trên PR #41):
  `view-next` không có đối số, tự dùng đúng `NEXT_LINE` và đường dẫn
  file từ trang gần nhất đã xác thực E2EE.
- Trước trang đầu, sau EOF hoặc khi SHA nguồn đổi thì không gửi yêu
  cầu đọc tiếp. Không thay đổi đường dẫn bởi local `cd`, không có
  quyền shell, download hoặc file mutation.
- Mỗi trang là 1 E2EE action, giữ quota 6 lượt, cùng FilesRead,
  grant live và audit ở host. Kiểm thử có EOF, SHA mismatch, local cwd,
  shell injection, quota và không in trang của phiên bản mới.
- PR #41 đã merge main, PR #42 đã merge; chưa được phép LIVE.

## Bổ sung — find-lines trên file nguồn đến 2MiB (đã merge PR #43)

- Nhánh `feature/remote-native-find-lines-20261101`:
  `find-lines <file> :: <literal>` tìm chính xác chuỗi UTF-8 (1–96 bytes)
  trong **toàn bộ** file nguồn tối đa 2MiB, không chỉ 4KiB đầu.
- Kết quả gồm hash SHA256 toàn file, hash chuỗi tìm kiếm, số dòng tổng,
  **tổng số dòng khớp** và nội dung đầy đủ từng dòng khớp có số thứ tự.
  Nếu không vừa 4KiB thì từ chối tất cả, không hiện kết quả bị cắt.
- Host kiểm tra FilesRead, E2EE native principal, key được phép, grant
  Portal còn hiệu lực, audit và pinned descriptor filesystem; không
  shell, regex thực thi, arbitrary path, tải file hay ghi đè source.
- Client xác minh hash toàn file và truy vấn, tổng matches/line numbers,
  không in kết quả từ file đã thay SHA giữa các trang đã xem.
- Go/PHP CI đã PASS trên exact HEAD và PR #43 đã merge; vẫn
  `remote_control_available=false` và LIVE NO-GO.

## Bổ sung — Git branch-create có CAS (đã merge PR #45)

- `feature/remote-git-branch-create-20261101`: native
  `git branch-create <slug>` không nhận Git flags/argv, luôn đọc
  `git.branch_review` (HEAD + branch hiện hành), kiểm tra SHA-256 và
  yêu cầu nhập `CREATE BRANCH taskdeck/<slug> <12 ký tự HEAD>`.
- Host chỉ tạo `refs/heads/taskdeck/<slug>` qua `git update-ref`
  có expected-old-OID toàn số 0, từ chối nhánh đã tồn tại; không
  checkout, không đụng Git index/working tree, không chạy hooks.
- Chỉ hoạt động khi host bật đồng thời
  `TASKDECK_REMOTE_GIT_WRITE=1` và
  `TASKDECK_REMOTE_GIT_BRANCH_CREATE=1`. Cần local TaskDeck login,
  Ed25519 key duyệt tại host, Agent pin, Portal grant live, GitWrite,
  FilesRead, FilesWrite, audit trước ghi refs và session revoke mutex.
- Có regression E2EE, no-PHP-principal, RBAC viewer, opt-out flags,
  Git thực tế, existing branch, stale HEAD, Git hooks, symlink, alternates,
  không chuyển shell hoặc ref tùy ý. Go/PHP CI đã PASS trên
  exact HEAD; PR #45 đã merge; LIVE vẫn NO-GO.

## Bổ sung — git switch cùng HEAD (đã merge PR #47)

- Nhánh `feature/remote-git-same-head-switch-20261101`:
  `git switch taskdeck/<slug>` **chỉ** đổi symbolic HEAD khi target
  branch cùng commit HEAD, Git index/worktree/untracked đều sạch,
  không gọi `checkout` hay sửa nội dung dự án.
- E2EE 2 lượt: review chứa HEAD, source branch, target ref/OID,
  index SHA256, CLEAN yes; native xác nhận `SWITCH taskdeck/<slug> <12hex>`,
  host bắt buộc đối chiếu lại SHA toàn bộ review và kiểm tra sau khi
  lấy HEAD/ref lock. Chỉ target loose ref không symlink.
- Bật riêng bằng `TASKDECK_REMOTE_GIT_WRITE=1` và
  `TASKDECK_REMOTE_GIT_SWITCH_SAME_HEAD=1` trên host; còn lại
  TaskDeck password, approved Ed25519, Agent pin, GitWrite, FilesRead,
  FilesWrite, live Portal grant và audit trước ghi.
- Chưa hỗ trợ switch commit khác, checkout, force, merge, rebase,
  shell/PTY hoặc file transfer UX. CI Go/PHP đã PASS trên exact HEAD, PR #47 đã merge;
  vẫn cần nghiệm thu security thực tế; `remote_control_available=false`.


## Bổ sung 02/11/2026 — xóa một dòng file UTF-8 lớn (PR chờ CI)

- Nhánh `feature/remote-large-file-safe-line-delete-20261102`: native console có lệnh `delete-large-line <file> <line>` cho file nguồn hiện hữu UTF-8 tối đa 2 MiB.
- Đọc dòng mục tiêu qua `project.file.lines` với **SHA-256 toàn file**, hiển thị nguyên dòng và chỉ gửi lệnh ghi sau khi người dùng nhập chính xác `DELETE LINE <line> <12-hex-SHA>`.
- Host dùng typed E2EE `project.file.delete_line` riêng (không shell, PHP script, upload hay download); phản hồi xóa có loại receipt riêng, không thể tráo với receipt sửa dòng hoặc thay toàn file.
- Yêu cầu FilesWrite + FilesRead, client Ed25519 đã duyệt, mật khẩu TaskDeck, Portal grant đang hoạt động, audit trên host trước ghi; ba host flags `TASKDECK_REMOTE_FILE_REPLACE`, `TASKDECK_REMOTE_TEXT_EDIT`, `TASKDECK_REMOTE_LARGE_LINE_EDIT` đều phải bật rõ ràng (mặc định OFF).
- Xóa đúng dòng đã xem cùng newline theo sau nếu có; nếu dòng cuối không có newline thì xóa cả dấu phân cách LF/CRLF đứng trước, khớp lệnh `delete-line` file nhỏ. Các dòng còn lại giữ nguyên byte và quyền file. Ghi qua pinned fd + SHA-256 CAS + shared mutation lease và atomic rename, cấm path symlink/metadata/thoát workspace, từ chối file đổi sau review, nguồn không phải UTF-8 hoặc dòng dài hơn trang E2EE.
- Regression cho Go host/transport/native console, quyền bị từ chối, flags OFF, stale SHA, replay, symlink, CRLF và receipt type swap; bắt buộc Go/PHP CI exact PR HEAD PASS trước khi merge.
- **Chưa hỗ trợ:** xóa nhiều dòng file lớn trong một transaction, edit-range >4KiB, PTY, Git merge/rebase/push/pull, file transfer UX; remote vẫn `remote_control_available=false` và **LIVE NO-GO**.


## Bổ sung 02/11/2026 — chèn một dòng file lớn (PR #57, chưa merge)

- `delete-large-line` đã merge `main` qua PR #56 (`8c2cc30a`) sau Go/PHP/GUI CI xanh.
- Nhánh `feature/remote-large-file-safe-line-insert-20261102`: thêm lệnh native `insert-large-line <file> <line>` chỉ **chèn trước dòng có thật** của nguồn UTF-8 <=2MiB, không chèn vào dòng ảo cuối file và không mở file transfer.
- Xem dòng đích và hash toàn file từ typed E2EE `project.file.lines`, nhập chính xác `INSERT LINE <line> <12 hex của SHA-256>`; gửi typed `project.file.insert_line` mang một dòng literal <=512B và SHA-256 toàn file.
- E2EE result được domain-separated với sửa/xóa file, chống type swap; xác thực TaskDeck principal, FilesRead+FilesWrite, approved client Ed25519, Agent pin và live Portal grant. Audit trước ghi, shared mutation lock, session revoke lock, pinned full-file CAS, atomic rename. Ba host flag `TASKDECK_REMOTE_FILE_REPLACE`, `TASKDECK_REMOTE_TEXT_EDIT`, `TASKDECK_REMOTE_LARGE_LINE_EDIT` vẫn OFF mặc định và đều bắt buộc.
- Chèn dòng mới giữ toàn bộ byte có sẵn, dùng LF/CRLF của dòng đích hoặc dòng trước nếu dòng cuối thiếu newline; từ chối UTF-8 sai, symlink, quá 2MiB, target quá dài, stale SHA, role không đủ, cờ OFF, replay, request/schema giả.
- Đã thêm test host, native console, protocol và CI gate; cần cả Go, PHP, GUI CI trên exact PR head PASS mới được merge. `remote_control_available=false`, LIVE NO-GO.
- Chưa hỗ trợ edit-range nhiều dòng của file lớn, chèn vào EOF nếu không có dòng đích, các thao tác trên nhiều file cùng transaction, PTY/shell tương tác, Git merge/rebase/push/pull/fetch và UX tải file.


## Bổ sung 02/11/2026 — tìm literal qua native Remote GUI (PR #58 đã merge main)

- PR #57 đã merge `main` commit `739185bb` sau khi Go/PHP/native GUI CI xanh; console có `insert-large-line` giới hạn trước một dòng có thật trong file UTF-8 <=2MiB.
- Nhánh `feature/remote-native-gui-find-lines-20261102` mở rộng **Files** của native GUI độc lập, không tải JavaScript từ PHP. Có ô tìm một chuỗi literal (UTF-8, 1–96 byte) trong một file UTF-8 <=2 MiB; không phải regex, recursive scan, hay download.
- Local handler chỉ nhận typed `file_find_lines` kèm `path` + `literal`, kiểm soát localhost Host+Origin+CSRF local key, đường dẫn nằm trong workspace và không phải tệp ẩn/Git metadata. Từ chối trường mode/confirm/task_id/offset thừa và literal trái phép trong các action khác.
- Client E2EE có sẵn `project.file.find_lines`, host thực hiện FilesRead + TaskDeck principal + approved Ed25519 client key + pinned Agent + live Portal grant. Báo cáo chỉ được hiển thị nếu đủ FILE_SHA256/TERM_SHA256, đếm kết quả và dòng tương ứng, không truncation hoặc mismatch. Kết quả dùng `textContent`, tuyệt đối không thực thi HTML/JS lấy từ file.
- Mỗi lần tìm chỉ tốn 1 trong 6 lượt E2EE của phiên; giới hạn kết quả 4KiB, danh sách quá dài bị từ chối. Kiểm thử hành vi bình thường, unauthorized PHP Origin, đường dẫn thoát workspace, metadata ẩn, trường giả mạo, literal điều khiển, báo cáo lỗi/truncated.
- **Chưa hỗ trợ:** editor trực tiếp trên GUI, tìm regex/tìm nhiều file, terminal PTY, Git merge/rebase/push/pull, phiên dài/resume, UX tải file. Go/PHP/native GUI CI trên đúng PR head đều PASS; **PR #58 đã merge `main` tại `50426a2a`**. Remote vẫn `remote_control_available=false`, LIVE NO-GO.


## Tiếp tục 02/11/2026 — Native GUI Git writes (PR #59, chờ CI/merge)

- Native GUI độc lập PHP nay có luồng **xem trước và xác nhận một lần**
  cho Git `stage`, `unstage`, `commit`, `branch-create`,
  `branch-switch` và `branch-delete`. Mỗi thao tác ghi dùng hai lượt
  E2EE từ hạn mức sáu lượt: kết quả review lấy trực tiếp từ Agent sau
  xác thực, người dùng nhập chuỗi xác nhận chính xác, host kiểm tra
  SHA-256 của toàn bộ snapshot trước khi ghi.
- Stage: xem trước toàn bộ file UTF-8 <=4 KiB và CAS vào Git index.
  Unstage: xác thực staged diff trước khi khôi phục một index entry.
  Commit: xem HEAD, branch, index hash và *toàn bộ* staged diff,
  nhập message ASCII an toàn, sau đó host xác thực lại HEAD/index.
- Nhánh chỉ có tiền tố `taskdeck/`: create không checkout, switch
  chỉ khi target cùng HEAD + Git index/worktree sạch, delete chỉ
  với nhánh dư thừa chưa checkout cùng HEAD. Không cung cấp
  arbitrary `git` argv, push/pull/fetch, merge/rebase hay shell.
- Tất cả cờ host Git write riêng biệt vẫn mặc định OFF; quyền GitWrite,
  FilesRead, FilesWrite, approved native Ed25519 client key,
  TaskDeck login + grant Portal, audit, mutation lock và host CAS
  **không bị thay thế** bởi xác nhận trong trình duyệt.
- GUI chỉ bind trên loopback 127.0.0.1, có Origin + CSRF secret;
  PHP không cấp HTML/JS và không thấy nội dung yêu cầu E2EE.
  Review 45 giây tự hết hạn, chỉ dùng được một lần; invalid/replay
  bị từ chối. Regression âm/dương chạy trên Linux/macOS CI,
  cộng với Go Remote Agent và PHP Portal CI.
- **Trạng thái:** đã commit trên branch
  `feature/remote-native-gui-git-write-review-20261102`,
  PR [#59](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/59);
  không nhận là đã merge cho đến khi CI PASS trên exact HEAD và PR được merge.
  **Remote Control LIVE vẫn NO-GO**: chưa có nghiệm thu thực địa,
  security review độc lập, PTY stream, Git push/pull và GUI tương đương local.

## Tiếp tục 02/11 — GUI chỉnh sửa file nhỏ an toàn (PR #60)

- Native GUI **Files** bổ sung editor văn bản thuần UTF-8 (tệp hiện có <=4KiB).
  Đọc preview đầy đủ qua E2EE trước, hiển thị nội dung trong textarea
  local được đóng gói với binary native; người dùng sửa rồi nhập
  `EDIT <path> <12 hex SHA>` chính xác. Review chỉ sống 45 giây,
  dùng một lần và cần còn đủ **hai** lượt E2EE.
- Agent nhận `project.file.replace_text` đã có, tự kiểm tra lại SHA-256
  của toàn file/FS path/permissions, host opt-in, audit, khóa ghi;
  việc edit không bypass được RBAC hoặc Portal grant. Bản thay thế
  tối đa 4096 byte, UTF-8 hợp lệ không chứa NUL; chống ghi file
  thay đổi giữa review và ghi. Không mở file hidden/.git hoặc upload.
- Browser vẫn chạy trên 127.0.0.1 native-only, kiểm tra Origin và CSRF;
  PHP không cung cấp HTML/JS, mật khẩu hoặc nội dung file. Không có
  HTTP proxy, shell, arbitrary Git argv, download/transfer.
- Go GUI regression có test digest, expiry, confirmation/replay, malformed
  preview, oversized text, NUL, wrong Origin, path escape và quota.
  Linux/macOS GUI, Go Agent, PHP Portal CI bắt buộc hoàn tất PASS
  trên cùng source SHA rồi mới xét merge. Chưa phát hành LIVE.
- Editor nhiều tệp, thao tác trên file >4 KiB, file manager đầy đủ,
  PTY/interactive streaming và live security acceptance vẫn chưa hỗ trợ.

### Chốt nhánh và kiểm định

- PR #59 Git GUI đã merge `main` tại commit `add20633575e76462b6d1bce14dc05a5accd9d9f`; PR #60 chỉ chứa bổ sung editor file trên nền main đã merge.
- CI Linux/macOS native GUI, Go Agent, PHP Portal phải PASS trên **exact PR #60 head** sau khi cập nhật nhánh; không lấy PASS từ source SHA cũ. Đây chỉ là staging, không phải cho phép LIVE.

## Tiếp tục 02/11 — Git review staged + unstaged trong GUI (PR #61)

- Thêm nút `Review 2 chiều` tại Git panel, yêu cầu đường dẫn source hợp lệ.
  Kết quả phân thành **WORKTREE / UNSTAGED** và **INDEX / STAGED**.
- Lấy hai kết quả qua đúng hai typed E2EE actions đã có `git.diff` và
  `git.diff_staged`; TaskDeck host recheck RBAC, live Portal grant và
  local audit **riêng trên từng lượt**. Cả hai phải thành công và đủ
  <=4096 bytes/mỗi kết quả; lỗi tại lượt hai không được in kết quả
  chưa hoàn chỉnh của lượt đầu. Giới hạn quota cần còn 2/6 lượt.
- GUI vẫn từ binary native, loopback-only, Origin + CSRF và textContent;
  PHP không tạo UI/HTML hoặc lệnh Git tùy ý. Không làm phát sinh quyền ghi.
- Có kiểm thử thành công, thiếu quota, path/metadata/flag, giả định PHP
  bị chiếm và thu hồi quyền tại lượt đọc thứ hai. Chờ CI và merge; LIVE
  vẫn NO-GO.

- Kiểm định PR #61: sau khi đổi base sang main đã merge #60, CI phải chạy lại trên đúng head mới; không tái sử dụng kết quả của nhánh chồng trước đó.

## 02/11/2026 — Native GUI: large-file line edits (PR #62 staging)

- Trong Files, nhập tệp tương đối và số dòng 1..2,097,152, chọn
  **Review sửa dòng**, **Review chèn dòng** (trước dòng hiện có), hoặc
  **Review xóa dòng**. Mỗi lần chỉ xử lý một dòng nguồn của file
  UTF-8 <=2 MiB qua ba typed action đã có: `project.file.edit_line`,
  `project.file.insert_line`, `project.file.delete_line`.
- Một vòng `project.file.lines` lấy trang nguồn hoàn chỉnh và
  `FILE_SHA256` toàn file. Native GUI kiểm tra đầy đủ envelope,
  vị trí dòng và giới hạn nội dung, sau đó chỉ cho phép một lần ghi
  trong 45 giây qua xác nhận `EDIT/INSERT/DELETE LINE <N> <SHA12>`.
  Ghi dùng full-file SHA-CAS trên host; nếu một process khác sửa file
  sau review, Agent từ chối (không tự retry). Mỗi thao tác chiếm
  đúng 2 trong 6 lượt E2EE.
- Input dòng mới UTF-8 <=512 byte, cho phép tab nhưng không có LF,
  NUL, control/bidi. Metadata `.git`, hidden file, path traversal,
  shell/git argv, upload/download đều bị chặn. Host duy trì file
  write flag mặc định OFF, TaskDeck RBAC, live Portal grant và audit.
- Có test sửa/chèn/xóa, SHA, chống replay, sửa chế độ giữa các lượt,
  preview sai, thời hạn, origin PHP giả mạo, quota và path traversal.
  CI và nghiệm thu LIVE chưa được thay thế bởi code/unit test.

- PR #62 được tách lại trên main sau merge #61; kết quả CI trước rebase không thay thế kiểm định Linux/macOS, Go Agent và PHP trên commit hiện hành.

## 02/11/2026 — Native GUI: click-to-select workspace explorer (PR #63)

- Files panel có danh sách D/F (folder/file) đọc từ đúng
  `project.dir.list` E2EE. Mỗi lần chỉ hiển thị <=192 byte,
  <=64 entry được host giới hạn; parse lại tại native UI,
  chỉ cho phép tên visible/UTF-8 hợp lệ và đường dẫn không vượt
  workspace. Bất kỳ bản tin không đúng định dạng đều không tạo
  danh sách clickable.
- Click thư mục chỉ điền đường dẫn, muốn duyệt tiếp phải nhấn
  nút Duyệt (E2EE read mới). Click file chỉ điền đồng thời Files
  và Git path, không tự gửi lệnh hay thực thi. Có nút thư mục cha;
  không liệt kê file hidden, symlink hoặc URL do người dùng nhập.
- Tạo nút bằng `document.createElement` và `textContent`,
  không dùng `innerHTML`, eval, script từ PHP/Relay. Duy trì
  127.0.0.1 Origin + local CSRF, PIN Agent và quota 6 lượt E2EE.
  Đây là giao diện explorer thử nghiệm, không phải upload/download.

- CI gate #63: chạy lại GUI Linux/macOS, Go Agent, PHP Portal trên commit đã rebase theo main sau merge #62; không dùng PASS từ lịch sử branch chồng cũ.

## 02/11/2026 — Native GUI: approved Tasks list clickable (PR #64)

- Nhấn **Danh sách task** (typed E2EE `task.list`) để lấy danh sách
  được Agent chấp thuận; client xác minh đầy đủ định dạng
  `ID<TAB>task label`, ID 1..2147483647 dạng canonical, label
  <=120 byte UTF-8, không Unicode bidi/control; không trùng ID,
  <=64 dòng và <=4096 byte đầu ra.
- Thẻ Task chỉ điền số ID, không tự thực thi. **Xem trước chạy**
  hoặc **Xem trước 30 giây** vẫn bắt buộc lấy lại quyền từ host,
  và phải nhập exact confirmation trước khi gửi `task.run_report`
  hoặc `task.managed_start`. Các cờ chạy task trên host mặc định
  OFF, PIN Ed25519, TaskDeck login, RBAC, Portal grant, audit giữ nguyên.
- Đọc đủ 6/6 lượt E2EE vẫn giữ nguyên kết quả đã được xác thực cuối
  thay vì làm mất output; GUI chỉ báo hết lượt trong dòng trạng thái.
- Không hỗ trợ shell, argv tùy ý, task chạy tự động khi duyệt, hoặc
  nội dung HTML/script từ PHP/Agent. Đây là staging cho GUI,
  không chứng minh điều kiện remote LIVE đã đạt.

- CI gate PR #64: kiểm thử Linux/macOS GUI, Go Agent và PHP Portal lại trên đúng commit sau khi rebase từ main đã merge PR #63. Chưa đạt LIVE.

## 02/11/2026 — Native Git GUI: click-to-select changed file (PR #65)

- `git.status_full` đã có được dùng làm dữ liệu đọc độc lập, hiển thị
  file Git đã theo dõi trong danh sách chọn nhanh ngay dưới Status.
  Native client giới hạn <=4096 byte, <=65 dòng, path <=120 ký tự,
  không có `..`, path hidden/metadata, path Git được quote, rename
  biểu thức ` -> `, wildcard, control/Unicode dấu điều hướng.
- Click một mục chỉ điền đường dẫn hiện hành cho Git và Files;
  không thực thi thao tác ghi hay bất kỳ Git argv nào. Pending Git
  confirmation bị ẩn/hủy khi đổi file để tránh bấm nhầm.
- Git stage, unstage, commit vẫn phải qua bước review E2EE + xác nhận
  và host SHA-CAS/RBAC/Portal grant/host flags. Nội dung Git status
  từ Agent được hiển thị bằng `textContent`, không có HTML/script.
  Đây là staging, không phải mở Remote LIVE.

## 02/11/2026 — Clickable taskdeck/ branch list và UI integration tests

- `git.branches` (host readonly, fixed argv) được parse an toàn trên native
  client để tạo danh sách có thể chọn những branch thuộc `taskdeck/`.
  Slug strict ASCII, SHA Git đầy đủ 40/64 hex, không trùng;
  output <=4096 byte và <=65 dòng. Chọn một branch chỉ điền
  trường slug trong màn hình Git; không tự checkout/xóa/tạo nhánh.
  Pending xác nhận Git bị ẩn khi chọn lại; mọi thay đổi vẫn yêu cầu
  review E2EE riêng và host CAS.
- Trên Linux/macOS, workflow native GUI chạy `node --test` với
  UI JS thật trong isolated VM + synthetic E2EE responses: thử chọn
  thư mục, file, task được phê duyệt, file từ Git status, task
  duplicate, branch selector và lượt quota cuối; không phải chỉ
  tĩnh `node --check`. Không khẳng định kiểm định LIVE.

## 02/11/2026 — Strict pending-review invalidation in native GUI

- Các pending confirmation Git/file/file-line/task chỉ hợp lệ cho
  **thao tác kế tiếp đúng loại**. Thực hiện một action khác (kể cả
  read-only status/list) sẽ xóa pending authorization tương ứng
  ngay trong trusted local Go server. Trình duyệt cũng ẩn các panel
  xác nhận cũ; không được dùng lại khi đã đổi ngữ cảnh thao tác.
- Có native Go integration tests chứng minh một read xen giữa review
  và execute không thể kích hoạt Git stage, ghi file, ghi dòng hay
  chạy task. Bảo vệ này bổ sung, không thay thế CAS/host RBAC,
  approved client keys, giới hạn E2EE và audit.
- JS behavioral tests cũng xác nhận edit/large-line/stage/task không
  chạy trực tiếp khi chỉ chọn hoặc xem trước; sai confirmation
  không gửi write action. Remote LIVE vẫn NO-GO.

- CI gate PR #65 sau rebase PR #64: yêu cầu đồng thời Linux/macOS Native GUI (Node DOM + Go race), Go Agent security và PHP Portal CI trên đúng head mới, không tính các lượt PASS ở commit của branch chồng trước đó.

## 02/11/2026 — Managed task output pager in native GUI (PR #66)

- Tasks panel có bản ghi output dạng văn bản và nút **Đọc tiếp trang
  đã xác thực**, sử dụng typed read-only `task.managed_output`
  hiện hữu, không thêm PTY/shell hoặc HTTP endpoint trên Portal.
- Mỗi cửa sổ E2EE bắt buộc có đầy đủ `TASK_HANDLE`,
  `OUTPUT_OFFSET`, `OUTPUT_NEXT`, `OUTPUT_TOTAL`,
  `TASK_STATE`, `OUTPUT_EOF` và boundary marker chính xác.
  Browser kiểm tra handle 32-hex, offset dạng canonical, giới hạn
  2KiB/window, `next - offset == bytes UTF-8`, thứ tự
  offset/next/total và EOF/state; sai -> ngừng paging, không ghép
  một phần output. Chỉ append những trang liên tiếp của cùng handle,
  tối đa bốn trang trong RAM/DOM (8KiB nguồn).
- Nội dung task không bao giờ được render HTML. Control, C1 và
  Unicode bidi/control format được escape khi hiển thị. Nút Đọc tiếp
  yêu cầu người dùng nhấn thủ công, không timer/polling nền và không
  tự lặp sau lỗi. Mỗi trang tiêu tốn 1/6 lượt E2EE trong phiên;
  thay handle buộc đọc một snapshot khác.
- Node DOM tests chạy trong native GUI workflow Linux/macOS kiểm tra
  cursor hợp lệ, EOF, từ chối metadata sai, handle sai, giới hạn
  giữ output và khả năng hiển thị ESC an toàn. Không phát hành LIVE.
