# Self update

VS Code Tasks Menu hỗ trợ tự cập nhật từ public repository:

```text
https://github.com/hacrot3000/PatchAndCollectionToolForAI
```

Lệnh:

```bash
./vscode_tasks_menu --self-update
```

hoặc nếu launcher đã nằm trong `PATH`:

```bash
vscode_tasks_menu --self-update
```

## Quy trình

1. Query revision mới nhất của branch `main` qua GitHub HTTPS API.
2. Nếu revision hiện tại đã trùng bản mới nhất thì thoát ngay.
3. Nếu daemon của workspace đang chạy, tạo một self-update request và chờ người dùng xác nhận trên web UI.
4. Browser hiển thị dialog update. Khi người dùng bấm **Update now**, browser flush terminal project state ngay trước khi xác nhận.
5. CLI tải source đúng revision từ GitHub vào thư mục tạm. Không `git pull`, `git reset` hoặc sửa working tree hiện tại.
6. Chỉ extract subtree `vscode_tasks_menu_go`; path traversal và archive vượt giới hạn bị từ chối.
7. Chạy `go test ./...` trên source mới với dependency đã vendor/offline.
8. Compile binary mới vào file staging trong cùng thư mục với binary hiện tại.
9. Chạy binary staging với `--version` để xác minh revision vừa build.
10. Chỉ sau khi test/build/validation thành công mới atomic-replace `.build/vscode_tasks_menu`.
11. Nếu daemon đang chạy, ưu tiên handoff chính TCP listener hiện tại sang daemon mới. Cách này giữ nguyên URL/port kể cả khi config dùng `port = 0`.
12. Nếu listener handoff không khả dụng, fallback dừng daemon cũ và thử bind lại đúng địa chỉ/port cũ. Chỉ khi cách đó cũng không dùng được mới cho config chọn port mới.
13. Browser poll trạng thái update. Nếu URL giữ nguyên thì reload; nếu URL thay đổi thì redirect sang URL mới.

## Terminal/session khi update

Self-update không cố giữ process PTY cũ sống xuyên qua binary replacement. Trước khi update, browser ghi ngay terminal project state hiện tại, gồm:

- số terminal tab;
- thứ tự tab;
- CWD hiện tại của từng terminal;
- active tab;
- các split group, orientation và ratio.

Daemon cũ shutdown các PTY trước khi kết thúc; daemon mới dùng project state để dựng lại terminal tabs. Console/scrollback cũ không cần được giữ.

Task process đang chạy không được đảm bảo tiếp tục qua self-update. Nếu đang có task dài quan trọng, nên hoàn tất task trước khi xác nhận update.

## Daemon và URL

Trên Linux/macOS, TCP listener có thể được duplicate thành file descriptor và truyền cho daemon mới. Đây là đường ưu tiên vì nó giữ nguyên socket đang listen và do đó giữ nguyên port.

Nếu handoff thất bại, updater fallback theo thứ tự:

```text
same listener FD
    -> restart và bind lại old Address
    -> restart theo config hiện tại
```

Nếu bước cuối chọn port khác, update state ghi `target_url`; browser đang mở tự redirect sang URL đó.

## Khi không có daemon

Nếu daemon của workspace chưa chạy, không cần browser confirmation. Updater vẫn thực hiện đầy đủ download -> test -> build -> validate -> atomic replace, sau đó thoát. Daemon sẽ dùng binary mới ở lần start tiếp theo.

## An toàn working tree

Updater không sửa source checkout hiện tại. Nó không chạy:

```text
git pull
git reset
git checkout
git clean
```

Source mới chỉ tồn tại trong thư mục tạm. Vì vậy local edits hoặc branch đang checkout của repository cài đặt không bị thay đổi bởi self-update.

## Bootstrap lần đầu

Một binary cũ được build trước khi tính năng này tồn tại sẽ không hiểu flag `--self-update`. Vì vậy cần cập nhật thủ công **một lần duy nhất** đến revision có self-update, ví dụ:

```bash
cd /path/to/PatchAndCollectionToolForAI
git pull
./vscode_tasks_menu --restart-daemon
```

Từ revision đó trở đi có thể dùng:

```bash
./vscode_tasks_menu --self-update
```

mà không cần `git pull` chỉ để cập nhật binary.

## Yêu cầu

- Có `go` trong `PATH`.
- Có HTTPS access tới `api.github.com` và `codeload.github.com`.
- Binary hiện tại phải có quyền ghi/thay thế file `.build/vscode_tasks_menu`.
- Repo upstream là public nên self-update không yêu cầu GitHub token.

## Trạng thái runtime

Progress được lưu atomic trong runtime directory riêng của workspace ở file `self-update.json`. Các trạng thái chính gồm:

```text
awaiting_confirmation
confirmed
downloading
testing
building
installing
ready_restart
restarting
completed
failed
cancelled
```

Nếu update lỗi trước khi binary được thay, daemon hiện tại tiếp tục chạy. Nếu lỗi xảy ra trong giai đoạn restart/handoff, CLI cố fallback sang restart thường và giữ port cũ trước khi cho phép đổi URL.
