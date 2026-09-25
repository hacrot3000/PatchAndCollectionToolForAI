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
3. Nếu daemon của workspace đang chạy, tạo self-update request và chờ xác nhận trên web UI.
4. Browser chỉ **persist snapshot** terminal/layout; không freeze UI hay terminal persistence trong lúc kiểm tra candidate.
5. **Dry-run/staging:** tải đúng source revision vào thư mục tạm, chạy `go test ./...`, Patch entry tests, compile candidate và chạy `--version` để xác minh binary.
6. Trong toàn bộ bước dry-run, daemon/release hiện tại vẫn chạy. Nếu download/test/build/validation lỗi thì không activate candidate; UI hiện lỗi dạng non-blocking và có thể đóng để tiếp tục làm việc.
7. Chỉ khi candidate PASS toàn bộ dry-run mới cài vào versioned release và atomic-switch global activation.
8. Trước activation, updater snapshot executable + revision global hiện tại. Nếu activation, handoff hoặc startup daemon mới thất bại, updater restore activation cũ; nếu daemon cũ đã rời listener thì restart trực tiếp executable cũ.
9. Khi release mới đã sẵn sàng, terminal snapshot mới được bảo vệ khỏi teardown writes trong cửa sổ `ready_restart/restarting`.
10. Nếu daemon đang chạy, ưu tiên handoff chính TCP listener hiện tại sang daemon mới để giữ URL/port.
11. Nếu listener handoff không khả dụng, fallback detach daemon nhưng giữ session broker rồi restart cùng địa chỉ cũ; legacy stop chỉ là last resort.
12. Browser chỉ reload/redirect sau khi daemon mới thật sự healthy.

Luồng an toàn:

```text
current release keeps serving
        |
        +--> candidate download/test/build/validate  [DRY-RUN]
        |         |
        |         +--> FAIL -> discard staging, keep current daemon, UI remains usable
        |
        +--> PASS -> snapshot current activation
                    -> atomic activate candidate
                    -> handoff/start new daemon
                            |
                            +--> PASS -> keep candidate
                            |
                            +--> FAIL -> restore previous activation
                                        -> restart previous daemon if needed
```

## Terminal/session khi update

Từ kiến trúc session broker, process/PTY không còn thuộc trực tiếp web daemon. Mỗi workspace có một **session broker process độc lập** giữ:

- process và process group của task/terminal;
- PTY master;
- session ID;
- trạng thái running/exited/stopped;
- scrollback;
- CWD/runtime metadata.

Web daemon chỉ kết nối tới broker qua Unix socket cục bộ. Vì vậy khi self-update thay binary và restart web daemon, broker vẫn tiếp tục chạy và **không nhận SIGINT/SIGHUP chỉ vì daemon được thay thế**.

Luồng bình thường:

```text
running task / terminal
    -> session broker vẫn sống
    -> daemon cũ detach
    -> daemon mới start
    -> daemon mới reconnect broker
    -> browser reload
    -> attach lại cùng session_id
```

Do giữ nguyên session ID, browser có thể phục hồi lại tab/order/active tab/split state đang lưu. Output phát sinh trong lúc daemon đang được thay vẫn được broker đưa vào scrollback và replay sau khi reconnect.

Browser vẫn snapshot project terminal state trước update để làm fallback cho trường hợp broker thực sự rỗng. Daemon mới **không recreate terminal** nếu broker đã còn session live; nó ưu tiên reconnect các session hiện hữu.

### Giới hạn chuyển tiếp từ bản cũ

Lần đầu nâng cấp từ một binary **pre-broker** sang bản có session broker là trường hợp đặc biệt: các process đã chạy trước lúc update vẫn do daemon cũ sở hữu, nên không thể chuyển ownership của PTY sang broker giữa chừng. Những process đó có thể mất trong chính lần chuyển tiếp đầu tiên.

Sau khi đã chạy phiên bản broker, các task/terminal được tạo mới thuộc broker và các self-update tiếp theo có thể giữ chúng xuyên qua daemon replacement.

### Fallback restart

Updater ưu tiên theo thứ tự:

```text
listener FD handoff, broker giữ nguyên
    -> broker-preserving daemon detach + restart cùng old Address
    -> legacy daemon stop (last resort)
    -> restart theo config hiện tại
```

Hai đường đầu không shutdown broker. Đường legacy stop chỉ còn là last resort nếu control path preserve-session không dùng được; trong trường hợp đó task đang chạy có thể bị dừng.

## Daemon và URL

Trên Linux/macOS, TCP listener có thể được duplicate thành file descriptor và truyền cho daemon mới. Đây là đường ưu tiên vì nó giữ nguyên socket đang listen và do đó giữ nguyên port.

Nếu listener handoff thất bại, updater trước tiên yêu cầu daemon cũ **detach nhưng giữ broker**, chờ daemon lock/state cũ được nhả rồi start binary mới trên old Address. Chỉ khi control path này cũng không hoạt động mới dùng legacy stop. Nếu bước cuối phải chọn port khác, update state ghi `target_url`; browser đang mở tự redirect sang URL đó.

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

Nếu dry-run lỗi, release/daemon hiện tại tiếp tục chạy và terminal persistence không bị freeze. Trạng thái `failed` được hiển thị non-blocking; người dùng có thể đóng thông báo và tiếp tục làm việc.

Nếu lỗi xảy ra sau khi candidate đã được activate, updater restore global activation trước đó. Nếu daemon cũ đã rời listener, updater khởi động lại trực tiếp executable cũ; candidate đã build vẫn được giữ trong thư mục versioned release để chẩn đoán hoặc retry sau.


## TLS identity khi self-update

Khi dùng HTTPS auto self-signed, TaskDeck lưu certificate/key theo workspace trong config directory
và tái sử dụng chúng qua các lần chạy.

Self-update có yêu cầu mạnh hơn startup thông thường: daemon replacement phải ưu tiên giữ nguyên
certificate identity đang phục vụ browser. Binary mới dùng `ResolveForSelfUpdate` để reuse pair hiện
tại miễn certificate/key còn hợp lệ. Việc tập network interface thay đổi (DHCP, IPv6 privacy address,
VPN, Tailscale...) không được phép tự làm đổi fingerprint trong self-update handoff.

Với wildcard bind (`0.0.0.0` / `::`), các interface address hiện tại vẫn có thể được thêm vào SAN
khi tạo certificate mới, nhưng chúng không còn là SAN bắt buộc để quyết định reuse certificate cũ.
Các SAN cấu hình ổn định như `advertise_host`, bind cụ thể, hostname và loopback vẫn được dùng để
quyết định rotation ở startup bình thường.

Certificate chỉ có thể thay trong self-update khi pair hiện tại thực sự không còn dùng được, ví dụ
file bị hỏng/mất hoặc certificate đã hết hạn. Trường hợp đó browser có thể yêu cầu trust lại
certificate mới.
