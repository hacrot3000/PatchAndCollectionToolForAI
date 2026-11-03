# Self update

TaskDeck hỗ trợ tự cập nhật từ public repository:

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

## Khi GitHub API trả `403 Forbidden` hoặc `429 Too Many Requests`

TaskDeck **ưu tiên Git HTTPS** qua `git ls-remote` khi tra commit mới nhất và danh sách branch. Với repository public, cách này không cần token, không dùng quota REST API và không sửa working tree. Tên ref và SHA 40 ký tự hex được đối chiếu chặt chẽ. **Chỉ khi Git HTTPS không khả dụng**, chương trình mới thử GitHub REST API để tương thích với môi trường chưa cài Git hoặc bị chặn Git HTTPS. Nếu cả hai đường đều lỗi, TaskDeck báo nguyên nhân của từng đường; không đoán revision và không kích hoạt bản cài không xác định.

Nếu cả API và git HTTPS đều lỗi, kiểm tra mạng tại host đang chạy daemon:

```bash
curl -si -H 'User-Agent: taskdeck-diagnostic' \
  https://api.github.com/rate_limit | grep -iE 'HTTP/|x-ratelimit-(remaining|reset)'
git ls-remote --heads --exit-code \
  https://github.com/hacrot3000/PatchAndCollectionToolForAI.git refs/heads/main
```

**Bootstrap bản cũ chưa có fallback:** Nếu `taskdeck --self-update` vẫn dừng ở API 403, cần dùng installer mới một lần. Lấy source sạch bằng Git HTTPS và cài từ source (không phụ thuộc GitHub REST API):

```bash
tmpdir="$(mktemp -d)"
git clone --depth 1 --branch main \
  https://github.com/hacrot3000/PatchAndCollectionToolForAI.git "$tmpdir"
bash "$tmpdir/install.sh"
rm -rf "$tmpdir"
```

Installer cài global release theo commit và giữ bản cũ trong `~/.local/lib/taskdeck/releases/`. Lệnh installer thay symlink nhưng không tự thay daemon đang chạy; chỉ restart daemon khi phù hợp với công việc đang chạy. **Không dùng `--restart-daemon` khi còn task/terminal cần giữ**, vì lệnh đó chủ động dừng broker sessions. Sau bootstrap, các lần cập nhật sau dùng `taskdeck --self-update` như thường lệ.

## Chọn branch cập nhật

Mặc định TaskDeck cập nhật từ branch `main`:

```ini
[self_update]
branch = main
```

Có thể đổi trong **Settings → UPDATE → Self-update branch**. Dropdown tải danh sách branch hiện có từ GitHub; sau khi chọn, bấm **Save**. Setting được lưu theo workspace trong `.vscode/vscode_tasks_menu.ini`.

Ví dụ:

```ini
[self_update]
branch = feat/test-update
```

Cả **Check update** và lệnh self-update đều dùng branch đã lưu. Updater trước tiên resolve HEAD của branch thành commit SHA, sau đó download/build đúng SHA đó; vì vậy source cài đặt được cố định theo commit trong suốt một lần update.

## Quy trình

1. Đọc branch self-update từ `[self_update] branch` (mặc định `main`) rồi lấy revision mới nhất bằng Git HTTPS; chỉ fallback sang GitHub REST API khi Git HTTPS không khả dụng.
2. Nếu revision hiện tại đã trùng bản mới nhất thì thoát ngay.
3. `--self-update` được xem là xác nhận trực tiếp từ CLI; updater tạo request ở trạng thái đã xác nhận và bắt đầu dry-run ngay, không phụ thuộc browser/UI.
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

Trên Linux/macOS, code có cơ chế thử duplicate TCP listener thành file descriptor và truyền cho daemon mới. Đây là đường ưu tiên để giữ nguyên socket/port khi môi trường hỗ trợ; **trên macOS tính tương thích và session-preserving handoff chưa có kết quả smoke thực tế nên không được coi là bảo đảm**.

Nếu listener handoff thất bại, updater trước tiên yêu cầu daemon cũ **detach nhưng giữ broker**, chờ daemon lock/state cũ được nhả rồi start binary mới trên old Address. Chỉ khi control path này cũng không hoạt động mới dùng legacy stop. Nếu bước cuối phải chọn port khác, update state ghi `target_url`; browser đang mở tự redirect sang URL đó.

## Khi không có daemon

Dù daemon đang chạy hay chưa, CLI `--self-update` không yêu cầu browser confirmation. Updater vẫn thực hiện đầy đủ download -> test -> build -> validate -> atomic replace; nếu daemon đang chạy thì tiếp tục handoff/restart, còn nếu chưa chạy thì binary mới sẽ được dùng ở lần start tiếp theo.

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

## macOS native và Docker Desktop

- **Native Intel/Apple Silicon:** `main` đã có backend Darwin nhưng vẫn cần kiểm thử nghiệm thu. Yêu cầu **Go 1.25+**, Git/Bash, mạng HTTPS để download và Python 3.10+ cho Patch Tool đầy đủ. Khi chạy global installer, dùng `~/.local/bin/taskdeck --workspace "$PWD"`; sau khi xác minh mới dùng `taskdeck --self-update`/nút Update trên Mac thật.
- **Bảo toàn phiên:** listener handoff/broker detach được thiết kế dùng chung Unix nhưng phải chạy acceptance macOS: mở terminal/task chạy dài, update candidate, kiểm tra session ID, PTY, scrollback, port, reattach, TLS, rollback nếu thất bại. Không coi một build thành công là xác minh runtime.
- **Docker Desktop trên macOS:** container chạy Linux, **không** dùng native self-update để thay container. Cập nhật bằng `./deploy/macos/taskdeck.sh update <project>` từ repo TaskDeck hoặc build/pull Docker image theo Compose; giữ named home volume.
- **Windows:** Docker/WSL2 tương tự, không nhầm native Windows Go với launcher.
- **Standalone legacy `self-install-and-update.sh`:** vẫn có giả định GNU/Linux; trên Mac hãy dùng global `install.sh`, không coi hai scripts tương đương.

Hướng dẫn: [docs/MACOS.md](../docs/MACOS.md), [bảng nền tảng](../docs/PLATFORM_SUPPORT.md), [handoff kiểm thử](../MACOS_PORTABILITY_HANDOFF.md).

## Yêu cầu

- **Nhiều project / nhiều daemon:** updater ưu tiên Go trong `PATH`, `TASKDECK_GO` (nếu có), Go cài theo user/hệ thống, và đường dẫn compiler thực tế đã được một TaskDeck khác cùng người dùng xác nhận tại `~/.local/lib/taskdeck/toolchains/.last-working-go`. Không đọc môi trường tiến trình khác, không chạy shell profile hoặc quét `/home`.
- **Tự xử lý khi thiếu Go:** nếu toàn bộ phương án tìm Go thất bại và người dùng không đặt `TASKDECK_GO`, updater tải Go SDK `go1.27.3` chính thức từ `go.dev`, kiểm tra SHA-256 đã ghim (cùng các checksum trong root `install.sh`), giới hạn kích thước, chỉ giải nén file/thư mục an toàn rồi xác minh `go version` trước khi kích hoạt. SDK được cài dưới `~/.local/lib/taskdeck/toolchains/` và dùng chung cho những TaskDeck của cùng tài khoản; không cần sudo và không sửa PATH của daemon. Cần HTTPS tới `go.dev`/server redirect chính thức ở lần đầu bootstrap. Nếu đã cài hoặc compiler hợp lệ được tìm thấy thì không tải lại.
- Nếu Go ở vị trí tùy chỉnh, đặt biến môi trường `TASKDECK_GO` thành **đường dẫn tuyệt đối** đến executable `go` rồi khởi động lại **daemon của project gặp lỗi**; shell trong tab terminal có thể có `PATH` khác với daemon. Ví dụ `TASKDECK_GO=/usr/local/go/bin/go`.
- Khi update từ trình duyệt, `go test` và `go build` dùng cùng Go executable đã tìm được; subprocess được thêm đường dẫn compiler vào `PATH`, đồng thời ghim `GOROOT` theo thư mục SDK thực tế nếu xác định được, tránh kế thừa `GOROOT` sai từ daemon project khác. Không sửa môi trường của toàn bộ TaskDeck.
- **Khóa cập nhật toàn máy theo global app root:** mỗi updater giữ khóa thư mục `~/.local/lib/taskdeck/.self-update-global.lock` (theo tài khoản OS, độc lập với `TASKDECK_APP_DIR` của từng project) trong suốt prepare → activation → handoff → rollback. Các daemon khác sẽ chờ khóa rồi đọc lại release hiện tại; không build/rollback chồng lên nhau. Khóa SDK riêng ngăn hai project cài Go đồng thời. Trên Linux/macOS dùng khóa file do kernel quản lý (`flock`): khi updater chết hoặc bị kill thì khóa tự nhả, không phải chờ thời gian timeout. Các nền tảng không có `flock` dùng khóa thư mục có cơ chế phục hồi khóa stale sau 35 phút; tiến trình đang hoạt động không bị dừng.
- Có HTTPS access tới `github.com` (Git smart protocol) và `codeload.github.com`; `api.github.com` chỉ cần cho đường REST dự phòng.
- Binary hiện tại phải có quyền ghi/thay thế file `.build/vscode_tasks_menu`.
- Repo upstream là public nên self-update không yêu cầu GitHub token.

## Trạng thái runtime

Progress được lưu atomic trong runtime directory riêng của workspace ở file `self-update.json`. Các trạng thái chính gồm:

```text
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

## Thông báo riêng khi hệ điều hành thiếu Go

Nếu self-update thất bại vì không thể tìm Go compiler (bao gồm thông báo cũ `Go toolchain not found for self-update` hoặc `exec: "go": executable file not found in $PATH`), giao diện hiển thị **TaskDeck update · Thiếu Go** cùng hướng dẫn cài đặt **trên host chạy TaskDeck**, không phải cài trong thư mục từng project. Có hai mục hướng dẫn chọn theo hệ điều hành:

- **Ubuntu / Debian:** `sudo apt update && sudo apt install -y golang-go`, sau đó thử `/usr/bin/go version`, `type -a go`, `go version`, `go env GOROOT`.
- **macOS:** `brew install go`, sau đó thử `"$(brew --prefix go)/bin/go" version`, `command -v go`, `go env GOROOT`.

Nếu bản phân phối Go của hệ điều hành quá cũ, dùng bộ cài chính thức tại <https://go.dev/doc/install>. Nếu Go đã được cài mà một daemon vẫn lỗi, kiểm tra `PATH`/`GOROOT` trong môi trường khởi chạy daemon, không chỉ ở terminal. Trên host có nhiều bản Go, `type -a go` cho biết bản nào đang được ưu tiên.

Thông báo này chỉ xuất hiện với lỗi **thiếu compiler Go** hoặc backend trả mã `GO_TOOLCHAIN_MISSING`; lỗi Go build thông thường (sai code, test fail) vẫn dùng thông báo thất bại cũ. **Copy error details** bao gồm nguyên nhân gốc và các bước cài Go theo hệ điều hành. Khi chuyển sang lỗi khác, phần hướng dẫn Go được ẩn, không làm kẹt nút Close.

## Lỗi `go build failed: exec: "go": executable file not found in $PATH`

Một project có thể update thành công nhưng project khác thất bại vì PATH/HOME của mỗi daemon khác nhau. **Từ bản sửa này**, updater tự tìm Go đã được daemon khác xác nhận; nếu không có, tự cài SDK chính thức đã kiểm tra checksum và dùng chung theo người dùng. Không cần cài compiler riêng cho từng project, cũng không cần restart daemon chỉ để thay đổi PATH. `TASKDECK_GO` vẫn là tùy chọn ghim compiler thủ công; nếu đã cấu hình sai, updater sẽ báo lỗi rõ ràng chứ không bỏ qua tùy chọn này.

**Nâng cấp lần đầu cho daemon đang chạy mã cũ:** daemon lỗi từ trước khi commit sửa sẽ chưa tự biết cách bootstrap Go. Sau khi một TaskDeck khác đã cài bản global mới, có thể chạy chính executable global mới cho workspace còn lỗi mà không cần đổi PATH trong daemon cũ:

```bash
"$HOME/.local/bin/taskdeck" --workspace "/duong/dan/project-con-loi" --self-update
```

Lệnh trên dùng quy trình handoff đã có để cập nhật daemon của workspace được chỉ định. Không chạy `--stop-daemon` hoặc `--restart-daemon` nếu vẫn muốn giữ các terminal đang chạy. Nếu bản global vẫn cũ, dùng bootstrap installer một lần như phần đầu tài liệu. **Lưu ý bootstrap:** self-update đang thất bại vì thiếu Go không thể nhận bản sửa auto-discovery mới cho tới khi bản daemon đang chạy tìm được Go. Trên chính host đó chạy `go env GOROOT`, kiểm tra `$(go env GOROOT)/bin/go version`, rồi đặt `TASKDECK_GO` thành đường dẫn tuyệt đối `<GOROOT>/bin/go` trong môi trường khởi chạy daemon đang lỗi và khởi động lại daemon đó. Nếu chạy bằng `systemd --user`, dùng override `Environment=TASKDECK_GO=/path/to/go` cho đúng unit và restart unit. `export` trong một terminal khác không thay đổi môi trường của daemon đã chạy. Nếu hai TaskDeck chạy dưới người dùng khác nhau, cấu hình và kiểm tra từng daemon riêng; không dùng đường dẫn Go của máy khác.

Nếu gặp trạng thái lỗi trong dialog, **Close** phải đóng thông báo ngay dù ACK tới backend bị gián đoạn. Việc đóng không kích hoạt lại, hủy hoặc rollback một update; candidate đã thất bại thì release đang chạy vẫn được giữ nguyên. Lần update thất bại tiếp theo vẫn có nút Close khả dụng.
