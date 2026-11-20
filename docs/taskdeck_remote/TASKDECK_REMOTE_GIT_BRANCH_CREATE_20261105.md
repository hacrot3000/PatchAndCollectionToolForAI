# TaskDeck Remote — Tạo một nhánh Git cục bộ được host duyệt

Nhánh mã nguồn: `feature/taskdeck-remote-access-architecture`.

## Chức năng

Remote GUI có nút **Tạo nhánh đã cấu hình trên host**. Yêu cầu E2EE
`git.branch_create` **không có tham số**. Agent tạo một nhánh mới
từ commit `HEAD` hiện tại, **không checkout**, không sửa working tree,
không push, fetch, merge hay xóa ref.

Để bật trên Agent Linux/macOS:

```sh
TASKDECK_REMOTE_GIT_BRANCH_CREATE=1
TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME=remote-topic
```

Cả hai biến phải do quản trị viên cấu hình **trên máy Agent**, trước
khi bắt đầu phiên. Không đặt tên nhánh hoặc giá trị opt-in trong PHP
Portal/Relay, URL, request body, browser, query string hoặc dữ liệu
E2EE của client. Cờ mặc định OFF. Tên phải thuộc biểu thức
`[A-Za-z0-9][A-Za-z0-9._-]{0,63}`, không được là `HEAD`, không
được chứa `..`, không kết thúc `.` hoặc `.lock` và phải qua
`git check-ref-format --branch`.

## Điều kiện bắt buộc

- TaskDeck native E2EE đã xác thực password + approved client-key và
  Portal live grant; `git.write` đúng project được kiểm tra lại mỗi lượt.
- Workspace phải đúng repository root sau khi resolve symlink;
  workspace lồng trong repo khác bị từ chối.
- Repository phải có commit HEAD hợp lệ, nhánh hiện tại không detached.
- Working tree, index và file untracked phải sạch; tên nhánh đích chưa
  tồn tại. Nếu biến host không an toàn hoặc host đang có task dài/PTY
  đang hoạt động, từ chối.
- Không cho client chọn shell, file path, Git ref, revision, argv,
  remote, target commit hay force flag.
- Agent dùng `git branch --no-track <host-approved-name> <verified-HEAD>`
  dưới subprocess có timeout, không qua shell, disabled Git hooks,
  env tối thiểu và process-group cancellation.
- Audit TaskDeck-local `remote.action.git_branch_create` với kết quả
  `attempted` ghi **trước khi tạo**, không log tên nhánh và không lưu
  credentials.
- Sau khi thành công, kiểm tra nhánh mới có tip trùng với HEAD ban đầu,
  nhánh checkout và HEAD hiện tại không đổi. Không retry thao tác ghi.
- Phản hồi thành công có đúng tên nhánh đã tạo và SHA HEAD 40 hex
  trong E2EE, nhằm để GUI hiển thị kết quả rõ ràng.

## Kiểm thử

Test giao thức: loại bỏ JSON lạ, duplicate, ID/reply bị tráo, tên ref
không an toàn, tham số Git bổ sung. Test Git thực: mặc định tắt, worktree
bẩn, ref trùng, chính xác SHA, không checkout, audit trước thao tác,
giữ nguyên file và root confinement. Test GUI native: confirmation
bắt buộc, không có request body/URL parameter, chặn PHP origin, tách
khỏi quyền pull/push. CI Go Agent chạy test với race detector.

Do giới hạn E2EE plaintext **512 byte**, cờ giao thức mới mang tên
`git_new` (compact), còn GUI/native API dùng tên rõ nghĩa
`git_branch_create`. Test tất cả cờ cùng bật bảo đảm không tràn bản tin.

## Chưa hoàn tất

Các thao tác checkout, xóa nhánh, tạo nhánh với lựa chọn người dùng đã
được phê duyệt, merge, full diff vượt 4 KiB và hunk staging vẫn chưa
được hoàn thiện. Terminal streaming/reconnect, task dependencies/secret
input/output replay và các GUI Editor/Files vẫn ở backlog. Không làm
hỗ trợ Windows riêng theo yêu cầu hiện tại.
