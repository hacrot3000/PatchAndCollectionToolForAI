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


## Bổ sung 01/11/2026 — sửa một vùng nhiều dòng an toàn

Nhánh: `feature/remote-native-console-edit-range-20261101`.

- `edit-range <file>` đọc **toàn bộ** file UTF-8 tối đa 4096 bytes,
  hiển thị số dòng, nhập vùng từ dòng A đến B (bao gồm hai đầu).
- Nhập nội dung mới nhiều dòng bằng `.save-noeol`, dùng `.save` nếu
  muốn thêm một dòng trống ở cuối vùng; nếu vùng kết thúc ở EOF không có
  newline thì `.save` bị từ chối để không vô tình đổi EOF. `.cancel`
  không gửi lệnh ghi.
- Kiểm tra đầy đủ nội dung dự kiến và chỉ ghi nếu người dùng nhập
  chính xác `RANGE A-B`. Giữ nguyên toàn bộ byte bên ngoài vùng,
  kết thúc dòng LF/CRLF của vùng đã chọn và trạng thái newline ở EOF.
  Mọi thay đổi làm đổi trạng thái newline EOF đều bị từ chối.
- Dùng lại `project.file.preview` + `project.file.replace_text`,
  quota hai lượt E2EE; CAS SHA-256 toàn file, host audit, TaskDeck RBAC,
  key duyệt trên host, Portal grant và **hai host opt-in flags** không đổi.
- Regression cases: CRLF/LF hỗn hợp, EOF, nhiều dòng, Unicode điều khiển,
  out of range, no-op, max 4096 bytes, path traversal, xác nhận sai,
  hết quota. Chạy `go test -race ./cmd/taskdeck_remote_login -run
  TestInteractive -count=1` trên đúng mã nguồn PR trước khi merge.
- Không triển khai tải file, Terminal PTY, Git write hoặc mở rộng Portal.
  **LIVE NO-GO** và `remote_control_available=false` vẫn giữ.


## Bổ sung 01/11/2026 — chuyển nhiều dòng giữ nguyên bytes

Nhánh: `feature/remote-native-console-move-lines-20261101`, dựa trên
`feature/remote-native-console-edit-range-20261101`.

- `move-lines <file>`: chọn dòng A..B, chuyển trước dòng N theo số
  dòng **trên bản gốc** (N = số dòng + 1 nghĩa là nối cuối).
- Toàn bộ file UTF-8 <=4096 bytes được đọc qua E2EE, giữ từng byte
  của tất cả dòng và LF/CRLF, kể cả khi file dùng EOL hỗn hợp.
- File phải có newline kết thúc ở **mọi** dòng; từ chối EOF không có
  newline, vùng sai, no-op, đường dẫn không hợp lệ, quota thiếu.
- Hiển thị toàn bộ kết quả, xác nhận đúng `MOVE A-B BEFORE N`,
  mới dùng SHA-256 CAS. Dùng 2/6 lượt E2EE.
- Giữ nguyên host TaskDeck password, client public key được duyệt
  bằng CLI, RBAC FilesWrite, live Portal grant, audit và mutation lock;
  hai host write flags vẫn mặc định OFF.
- Kiểm thử `TestInteractiveMoveLines*` bao phủ chuyển lên/xuống,
  EOL hỗn hợp, no-op, EOF, xác nhận sai, quota và path traversal.
- Chưa có remote PTY, Git stage/commit và file editor lớn. Chưa
  triển khai tải file vào giao diện. **LIVE NO-GO**, giữ
  `remote_control_available=false`.


## Bổ sung 01/11/2026 — Git review before stage/commit

- `git review <file>` kết hợp hai lần đọc E2EE typed `git.diff` và
  `git.diff_staged`. Hiển thị cả diff worktree và diff staged chỉ khi
  **cả hai** được phép và thành công; không tiết lộ một phần khi lượt
  thứ hai bị từ chối hoặc phiên bị thu hồi.
- Không thêm API/command phía host; không cho nhập lệnh Git, shell hay
  đường dẫn thoát workspace. Hai action dùng TaskDeck GitStatus ∩ FilesRead,
  local password/key, live grant, host audit riêng và giới hạn 6 lượt.
- Test `TestInteractiveGitReview*` kiểm tra đường dẫn, quota, chống
  terminal spoof và từ chối của từng lượt. Git stage/unstage/commit
  vẫn **chưa hỗ trợ**; không nới quyền `remote_control_available=false`.


## Bổ sung 01/11/2026 — HEAD diff E2EE cố định

- `git diff --head <file>`: so sánh HEAD với working tree của một
  file nguồn trong workspace. Được phép xem cả thay đổi đã stage và
  chưa stage qua một response type riêng `git.diff_head`.
- Host không nhận tên revision/flags từ client, chỉ chèn literal
  `HEAD` rồi `-- <path>`, chạy read-only Git không external
  diff/textconv; cần GitStatus ∩ FilesRead và các lớp E2EE, key,
  local login, live Portal grant, TaskDeck audit.
- Kết quả đầy đủ tối đa 4096 bytes, SHA-256 authenticated, không cắt
  cụt; client escape ký tự điều khiển khi hiển thị.
- Có regression protocol/host/client và CI yêu cầu kiểm thử GitHeadDiff.
  **Git stage/commit, PTY, upload, LIVE chưa mở**.


## Bổ sung 01/11/2026 — Git stage chính xác với CAS

- `git stage <file>`: preview nguyên file UTF-8 <=4KiB, yêu cầu
  xác nhận `STAGE <file>`, gửi E2EE typed `git.stage` với SHA-256.
- Host yêu cầu hai cờ riêng OFF theo mặc định:
  `TASKDECK_REMOTE_GIT_WRITE=1`, `TASKDECK_REMOTE_GIT_STAGE=1`.
  RBAC tối thiểu GitWrite + FilesRead + FilesWrite, client key được
  duyệt tại host, mật khẩu TaskDeck, Agent pin, live Portal grant,
  audit trước side effect và khóa revoke trước ghi Git index.
- Không dùng `git add` có clean filters; dùng trusted Git
  `hash-object --no-filters` rồi `update-index --cacheinfo`
  theo byte đã SHA-pinned, không chạy shell hay hook tùy ý.
- Chỉ hỗ trợ stage **toàn file** trong Git repository root thường
  (`.git` directory), không linked worktree, không stage hunk.
  Unstage/commit/PTY/LIVE vẫn chưa mở.


## Phát hiện 01/11/2026 — Git status/diff read có thể chạy code filter

- Đã tái hiện Git status và Git diff `--no-ext-diff` chạy clean
  filter khai báo trong repo. Với remote viewer đây là đường chạy lệnh
  trái phép nếu không phòng ngừa.
- Phải refuse Git read khi repo config (kể cả includes) có executable
  filter `clean/process/smudge`; host kiểm tra bằng trusted Git và
  environment cách ly trước mỗi read action. Chặn fail-closed kể cả
  filter có thể hợp lệ (Git LFS). Giữ nguyên giới hạn 4KiB và RBAC.
- Stage riêng dùng Git plumbing `hash-object --no-filters`, không
  gọi `git add`. Tests xác minh marker không bị tạo.


## Bổ sung 01/11/2026 — Giới hạn an toàn Git unstage

- `git unstage <file>`: diff staged E2EE đầy đủ, xác nhận
  `UNSTAGE <file>`, sha256 của diff được host xác minh lần cuối.
- Host cần `TASKDECK_REMOTE_GIT_WRITE=1` và
  `TASKDECK_REMOTE_GIT_UNSTAGE=1`, RBAC
  GitWrite + FilesRead + FilesWrite, TaskDeck login, key allowlist,
  live Portal grant, audit, mutation lease/revoke guard.
- Không dùng `git reset`; restore đúng mode/blob HEAD hoặc
  xóa entry mới stage qua fixed Git plumbing, không sửa working tree.
  Giới hạn: không conflicted records, staged deletion, submodule,
  symlink, filter, index external CAS race và >4KiB diff.
- Git commit/PTY/LIVE/upload/download UI tiếp tục không hỗ trợ.

## Bổ sung — git commit-review E2EE (chưa commit)

- `git commit-review`: chỉ đọc, một lượt E2EE, preview đầy đủ staged
  diff <=4KiB kèm HEAD/branch/index SHA-256 (chưa thực thi commit).
- Cần GitWrite + FilesRead TaskDeck, client Ed25519 key được duyệt,
  native local login, Agent pin, Portal live grant và local audit.
- Không cho arbitrary revision/path/Git CLI, không thực thi hooks,
  clean filter hoặc shell. Không có thay đổi vào index, HEAD, worktree.
- Trước khi cho phép `git commit`, bắt buộc revalidate toàn bộ
  HEAD/branch/index CAS rồi commit từ index snapshot cố định,
  update-ref theo expected-old-OID và có audit/revocation trước ghi.
  PR chưa merge, remote_control_available=false.

## Bổ sung 01/11/2026 — typed Git commit (experimental)

- Lệnh native E2EE `git commit` yêu cầu review toàn staged diff,
  message 3–96 ASCII, và xác nhận `COMMIT <12-hex HEAD>`.
- Cần `TASKDECK_REMOTE_GIT_WRITE=1`,
  `TASKDECK_REMOTE_GIT_COMMIT=1`, GitWrite + FilesRead + FilesWrite,
  host-local login, key allowlist, Agent pin, Portal live grant và
  audit trước Git object write.
- Commit từ snapshot index SHA-256 với `write-tree`, `commit-tree`
  và `update-ref oldOID CAS`, không shell/hooks/filters/signing;
  giữ nguyên working tree và Git index.
- Chưa an toàn production nếu external Git writers thay đổi index giữa
  final CAS và `update-ref`; còn cần policy lock đa tiến trình,
  kiểm thử Linux/macOS, xử lý orphan objects và nghiệm thu bảo mật.
  Chưa LIVE, chưa hỗ trợ Git push/pull/checkout/merge/rebase/PTY.

## Bổ sung — Terminal task confirmation

- `terminal task <ID>` mặc định 2 E2EE lượt: đọc full host-approved
  catalogue, verify canonical ID và label, xác nhận `RUN TASK <ID>`.
- Host vẫn chạy chỉ process task digest-approved, no input/no secrets,
  no shell, giới hạn 7s và output hoàn chỉnh <=4KiB.
- Không triển khai generic PTY, stdin streaming hay shell arbitrary.
  Tài liệu và implementation này không bật LIVE.

## Bổ sung — native `view-lines` E2EE, không tải file

- Chỉ đọc file UTF-8 hiện hữu <=2MiB; mỗi trang <=4KiB
  kèm SHA256 toàn bộ source, START/NEXT/TOTAL_LINES.
- Client từ chối trộn trang từ source hash khác và fail-closed trên
  header sai, dòng chưa đầy đủ, path vượt workspace hoặc page quá lớn.
- Không thay thế CAS full-file editor, không file-transfer mode,
  không mở shell/PTY; chưa LIVE.

## Bổ sung — native find-lines (2MiB read-only)

- `find-lines <file> :: <literal>`: một action E2EE, tìm literal từ
  file UTF-8 <=2MiB, chỉ trả trọn bộ dòng khớp <=4KiB; từ chối
  tràn output thay vì trả kết quả thiếu.
- File SHA, literal SHA, số dòng tổng và số dòng khớp được native
  kiểm chứng trước khi in; không nhận search từ PHP hoặc lệnh shell.
- Không thêm download, PTY, arbitrary Git, hoặc large-file editor;
  host authorization FilesRead và LIVE NO-GO giữ nguyên.

## Bổ sung — host-gated Git branch-create

- `git branch-review`: xem HEAD + symbolic branch, chỉ đọc E2EE.
- `git branch-create <slug>`: xem trước HEAD rồi xác nhận
  `CREATE BRANCH taskdeck/<slug> <12 hex HEAD>`; host revalidate CAS.
- 2 host flags, TaskDeck login/approved key, RBAC, Portal live grant,
  pre-write audit; `update-ref` expected-old-zero tạo ref mới,
  không ghi đè ref cũ. Không checkout/switch/push/pull/PTY.
- Chưa qua nghiệm thu LIVE, `remote_control_available=false`.

## Bổ sung — Git same-HEAD switch (PR chờ Go/PHP CI)

- Typed `git.switch_review` / `git.switch_same_head`: chỉ target
  `refs/heads/taskdeck/<slug>` cùng HEAD; status phải hoàn toàn sạch.
- Lock Git HEAD và target ref, revalidate full review SHA+index SHA
  trước khi nguyên tử thay thế symbolic HEAD. Không checkout tree,
  không Git hooks, không shell, không ghi index/worktree.
- Hai host flags mặc định OFF, TaskDeck host RBAC + live Portal grant,
  audit trước ghi; LIVE NO-GO.
