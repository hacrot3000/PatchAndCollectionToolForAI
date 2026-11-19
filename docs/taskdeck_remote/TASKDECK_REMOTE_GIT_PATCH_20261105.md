# TaskDeck Remote — Git diff chi tiết có phân trang

Nhánh triển khai: `feature/taskdeck-remote-access-architecture`.

Native Remote GUI có thêm **Xem diff chi tiết** và **Trang diff tiếp**.
Hai nút dùng `git.patch.start` / `git.patch.next` trong E2EE của
TaskDeck, qua PHP Portal/Relay chỉ truyền ciphertext. Không có API
nhận tham số filename, branch, ref, offset, SHA, command hoặc Git argv.

## Quyền và opt-in

- Chỉ Linux/macOS, repository gốc được TaskDeck cấu hình chính xác; Agent
  từ chối repository cấp cha nếu workspace là thư mục lồng.
- Yêu cầu quyền `git.diff`, xác thực principal TaskDeck thật, approved
  client key, Portal grant hiện còn hiệu lực và audit đọc tại host
  **cho từng trang**. PHP không được tự cấp quyền.
- Cờ `TASKDECK_REMOTE_GIT_PATCH=1` phải bật riêng (mặc định OFF).
  Bật `git.diff` dạng `--stat` không tự bật patch.
- Lệnh cố định: `git diff --no-ext-diff --no-textconv --no-color
  --no-renames --ignore-submodules=all --unified=1 HEAD --`,
  đọc nội dung tracked trong working tree + index so với HEAD.
  Không đọc untracked, không gọi shell, external diff, textconv, network,
  không ghi lại file hoặc index.
- Diff lớn hơn **4096 byte** bị từ chối toàn bộ, không truyền bản
  bị cắt mất phần cuối. Diff có ANSI/control/bidi hoặc UTF-8 sai bị
  từ chối. Mỗi trang tối đa 320 byte UTF-8 và được co tự động để vừa
  mailbox E2EE gốc **512 byte** sau JSON escaping.
- Agent giữ duy nhất số trang, offset và SHA-256 của
  `canonical_workspace + separator + toàn bộ diff` trong phiên đã
  xác thực. Ở mỗi `git.patch.next`, Agent tính lại diff và so sánh
  SHA-256; nếu file/workspace thay đổi, từ chối và hủy cursor.
- Trang cuối xóa cursor. Không có bookmark hoặc bearer token để đọc
  tiếp ở phiên khác. Giới hạn yêu cầu của native GUI vẫn là 16 lượt/phiên;
  nếu hết lượt giữa diff, phải đăng nhập phiên mới và đọc lại từ đầu.
- Nội dung chỉ hiển thị qua `textContent` trong native GUI, không HTML.
  Xóa khỏi màn hình khi đăng xuất hoặc sau các thao tác Git ghi.

## Kiểm thử mã nguồn

- Roundtrip E2EE, phân trang UTF-8, escape JSON, độc lập capability và
  từ chối path/ref/offset, duplicate-key, forged response, control chars.
- Repository Git thật: ghép lại các trang, SHA-256 fail-closed khi file
  thay đổi giữa các lần đọc, HEAD/working tree không thay đổi,
  từ chối repo cấp cha và patch vượt hạn mức.
- API native localhost: từ chối dữ liệu request body, query string
  và nguồn HTTP của Portal.
- JavaScript GUI: đọc trang đầu/tiếp, khóa nút khi hết trang, không
  nới quyền push/pull, xóa diff khi logout.
- CI Go Agent chạy test với `-race`, CI PHP Portal độc lập.

## Code chưa hoàn tất

Diff cực lớn hơn 4 KiB, khôi phục cursor giữa nhiều phiên, diff riêng
từng file/hunk và hunk staging vẫn chưa hỗ trợ. Git branch create/
checkout/delete/merge, Terminal nâng cao và task dependencies/secrets
cùng các GUI Editor/Files còn ở backlog. Code hoàn tất không cần thử
trên thiết bị thật để được tính hoàn thành, nhưng cần review bảo mật
trước khi bật ở production.
