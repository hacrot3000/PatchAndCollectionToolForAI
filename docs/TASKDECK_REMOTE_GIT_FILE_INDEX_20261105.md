# TaskDeck Remote — lựa chọn một file Git để stage/unstage

Triển khai trong nhánh `feature/taskdeck-remote-access-architecture`, riêng
Linux/macOS. Mục tiêu là thao tác chính xác một file từ Git Status của
Remote GUI mà không mở quyền chọn đường dẫn tùy ý trên Agent.

## Hợp đồng E2EE

- Hai operation cố định: `git.stage_file` và `git.unstage_file`.
- Native GUI cho chọn một dòng từ Git Status **đã giải mã và xác thực**.
  Payload có duy nhất `file_index` (1–12) và `snapshot` (trạng thái Git
  đầy đủ, tối đa 256 byte); không có tham số `path`, `remote`,
  `command`, revision hoặc môi trường tùy chọn.
- Trên Agent, ảnh chụp phải khớp **từng byte** với lần đọc Git Status mới
  nhất từ đúng thư mục gốc của repository host. Nếu thay đổi, yêu cầu bị
  từ chối và người dùng phải đọc Git Status mới.
- Agent tự giải mã dòng được chọn. Chỉ hỗ trợ tên đường dẫn ASCII đơn giản,
  không có `..`, thư mục, rename, conflict, tên quoted hoặc file untracked.
  Yêu cầu `git ls-files` xác minh một đường dẫn được index theo dõi,
  không cho pathspec khớp cả thư mục.
- Stage một file dùng `git add -u -- <file>`, không thêm file untracked;
  unstage một file dùng `git restore --staged -- <file>`, không thay đổi
  nội dung working tree.
- Kết quả E2EE chỉ chứa loại thao tác cố định và SHA của HEAD, không trả
  đường dẫn hoặc dữ liệu file. GUI xóa snapshot sau thao tác ghi.

## Quyền, opt-in và an toàn

- Yêu cầu `git.write` và `git.status` của TaskDeck, approved client
  software key, phiên E2EE còn sống, Portal live grant và xác nhận rõ
  của người dùng.
- Cờ bật **riêng biệt**, mặc định tắt:
  `TASKDECK_REMOTE_GIT_STAGE_FILE=1` và
  `TASKDECK_REMOTE_GIT_UNSTAGE_FILE=1`. Ngoài ra
  `TASKDECK_REMOTE_GIT_STATUS=1` phải bật.
- Audit `remote.action.git_stage_file` / `remote.action.git_unstage_file`
  ghi vào database TaskDeck **trước** hành động, không chứa tên file, đường
  dẫn hoặc nội dung file.
- Không chạy shell, Git hooks, filter clean/process có thể thực thi chương
  trình, hay lệnh Git do client tùy chọn. Hạn 8 giây, có cancellation theo
  phiên. Từ chối khi PTY hoặc task dài còn đang hoạt động.
- Nguy cơ thay đổi filesystem do tiến trình **cục bộ** cùng lúc không thể
  loại bỏ tuyệt đối bằng snapshot; chính vì vậy action kiểm tra snapshot
  trước khi audit/thực hiện và kiểm tra HEAD bất biến sau thao tác.

## Kiểm thử

Có unit test giao thức E2EE cho index và snapshot, từ chối fields lạ/trùng,
null và đường dẫn giả mạo; test Git repository tạm kiểm tra lựa chọn đúng
file, bảo toàn working tree, từ chối snapshot cũ và Git filter; test
loopback API chặn PHP origin/query/JSON sai; test trình duyệt native kiểm
tra lựa chọn file, xác nhận, trạng thái sau ghi và logout. CI Go Agent
thực hiện kiểm thử với race detector.

## Phần code còn lại

Nhóm Git: hunk-level staging, full diff có phân trang, branch/checkout,
quản lý merge và thao tác khác chưa được hỗ trợ. Nhóm Terminal: streaming
dài hạn/reconnect. Nhóm task: dependencies, nhập secret, replay output
qua phiên khác. Nhóm GUI: editor/files và các màn hình còn lại. Không
phát triển riêng cho Windows theo quyết định hiện tại.
