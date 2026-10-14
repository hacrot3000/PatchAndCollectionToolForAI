# TaskDeck trên Windows (Docker Desktop hoặc WSL2)

> Trạng thái 2026-10-14: **Docker/WSL2 là hướng hỗ trợ Windows sử dụng backend Linux**.
> Windows native Go/ConPTY chưa được xác nhận. Bảng kiểm và các giới hạn: [WINDOWS_PORTABILITY_HANDOFF.md](../WINDOWS_PORTABILITY_HANDOFF.md).

## 1. Chạy bằng Docker Desktop (ưu tiên)

Yêu cầu: Windows 10/11 x64, Docker Desktop sử dụng WSL2 engine và Firefox/Edge.
Lần đầu cần tải base image Debian/Python/Go để build. Container chỉ chạy Linux backend;
project **không** bị copy vào image mà được mount từ ổ Windows.

Mở PowerShell từ root repository:

```powershell
.\deploy\windows\taskdeck.ps1 -Action Doctor -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Start  -ProjectPath 'D:\Projects\M3'
```

Windows CMD:

```bat
deploy\windows\taskdeck.cmd -Action Start -ProjectPath "D:\Projects\M3"
```

Launcher khởi tạo Docker image `taskdeck:local` nếu cần, đặt container riêng
cho mỗi đường dẫn project, mount project vào `/workspace`, lưu home runtime vào
named volume, expose **chỉ** `127.0.0.1:42882` và mở trình duyệt tới
`https://localhost:42882`. Không cần cài Go/Python/Git/SSH trên Windows host.

Các lệnh thường dùng (giữ nguyên `-ProjectPath`):

```powershell
.\deploy\windows\taskdeck.ps1 -Action Status      -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Logs        -ProjectPath 'D:\Projects\M3' -Follow
.\deploy\windows\taskdeck.ps1 -Action Stop        -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Update      -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Credentials -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Start       -ProjectPath 'D:\Projects\M3' -Profile dev
```

`Update` build lại image từ checkout hiện tại và tạo lại container, **không**
xóa project hay named volume. Để dùng image đã publish thay vì build local:
truyền `-Image <registry/repository:tag> -PullImage`. Chỉ dùng image bạn tin cậy.
Nếu chọn `-Profile dev`, build image với Node.js/npm, GCC/G++, gdb;
profile mặc định `runtime` không có compiler hay Node.js để giảm dung lượng.

### Credentials / bảo mật

Khi bạn cho phép khởi tạo workspace, entrypoint chỉ tạo `.vscode/tasks.json`
nếu chưa có và không thay đổi các task đang tồn tại. Nếu chưa có
`.vscode/vscode_tasks_menu.ini`, entrypoint tạo cấu hình **HTTPS + auth bật**
và password ngẫu nhiên 192-bit, mode 0600. Xem password tại:

```text
D:\Projects\M3\.vscode\vscode_tasks_menu.ini
```

Hoặc `-Action Credentials -RevealPassword`, có xác nhận trước khi in ra.
**Không commit file ini có password.** Kiểm tra `git status`/gitignore của project.
Chứng chỉ HTTPS tự ký sẽ gây cảnh báo Firefox; cần đối chiếu fingerprint trước
khi trust. Không dùng chế độ mặc định để publish ra LAN/public Internet.
File config đang tồn tại được giữ nguyên; nếu nó bind `127.0.0.1`, hãy
tự chỉnh `bind = 0.0.0.0` cùng `auth.enabled = true` và password mạnh,
vì Docker publish port qua namespace mạng của container.

### Công cụ trong container

`runtime`: Bash, Git/Git LFS, OpenSSH client (ssh/scp/sftp/ssh-keygen),
Python 3.11 + Python Patch Tool, curl/wget, rsync, find, grep, sed/awk,
jq, tar/zip/unzip/gzip/xz, ps, ip/ping, make, nano, vim-tiny, less.
`dev` thêm GCC/G++, make/build-essential, gdb, Node.js/npm, Python headers.

SSH tới remote server hoạt động qua OpenSSH client hoặc các profile TaskDeck;
private key **không được** nhúng vào image. Nếu cần sử dụng key SSH của Windows,
hãy cấu hình credential/agent riêng cho mỗi project/container, sao chép một key
được cấp phép vào volume home rồi giới hạn quyền chmod 0600, hoặc sử dụng
cơ chế agent forwarding an toàn. Đừng bind `C:\Users\...\.ssh` vào container
theo kiểu ghi được hoặc đưa key vào image; permission trên NTFS khác POSIX.

### Dùng Docker Compose thay launcher

```powershell
Copy-Item deploy/docker/.env.example deploy/docker/.env
# Sửa TASKDECK_WORKSPACE=D:/Projects/M3 trong deploy/docker/.env
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml logs -f
```

Compose dùng project `/workspace`, home named volume, bind host loopback,
không mount Docker socket. `docker compose down` giữ named volume;
**không** chạy `down --volumes` nếu muốn giữ SSH/config home.

## 2. Chạy WSL2 không qua Docker

```powershell
wsl --install -d Ubuntu
```

Trong WSL Ubuntu: cài `git`, `openssh-client`, `python3`, `go` tương thích,
clone TaskDeck và chạy `./install.sh`. Sau đó:

```powershell
.\deploy\windows\taskdeck-wsl.ps1 -Action Doctor -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck-wsl.ps1 -Action Start -ProjectPath 'D:\Projects\M3'
```

Script chuyển đường dẫn sang `/mnt/d/Projects/M3`, gọi `taskdeck` trong
WSL với tham số đã quote an toàn. Các task thực thi bằng Bash/Linux.
Với repo cực lớn hoặc hàng trăm nghìn file, chuyển repository vào
filesystem riêng của WSL2 (`~/projects`) để tăng hiệu năng Git/IO; khi đó
khởi chạy TaskDeck trực tiếp trong shell WSL.

## 3. Những khác biệt cần biết

- Docker/WSL chạy **Linux commands**, không phải PowerShell/CMD bên trong terminal;
  các task Windows-only (`.exe`, MSBuild, PowerShell module Windows, registry)
  cần lane Windows native trong giai đoạn sau.
- Docker container không có quyền tự điều khiển Docker host, USB/COM device
  hay toàn bộ ổ cứng Windows. Muốn hỗ trợ thêm mount phải cấp rõ ràng.
- Một repository Windows dùng chung với Linux container có thể gặp vấn đề
  file permission, case sensitivity, symlink, CRLF và hiệu năng `node_modules`.
  Không thực hiện song song các lệnh Git ghi từ Windows và container vào một repo.
- Container tự cập nhật **bằng rebuild/pull image**, không bằng nút self-update
  Linux bên trong container; volume home và mount project được giữ.
- CI build/smoke: [taskdeck_windows_docker.yml](../.github/workflows/taskdeck_windows_docker.yml).
  Khả năng build Windows native Go chưa tương đương và sẽ không được quảng bá là
  hoàn thành cho tới khi ConPTY/process implementation và Windows smoke PASS.

## 4. Tải Windows GUI Launcher dạng ZIP/.exe

Ở GitHub Actions, mở workflow **TaskDeck Docker and Windows launchers**
và chọn run thành công mới nhất. Trong mục **Artifacts**, tải
`TaskDeckLauncher-win-x64`. Từ bản ZIP mới, chỉ cần giải nén sang thư mục
bất kỳ trên Windows và chạy `TaskDeckLauncher.exe`.

Gói này có GUI WinForms .NET self-contained, `taskdeck.ps1`,
`taskdeck.cmd`, `taskdeck-wsl.ps1`, cùng **Dockerfile và source Go/Python
tối thiểu** để build image ngay trên máy Windows có Docker Desktop.
Bạn không cần cài Go/.NET SDK riêng để chạy GUI. Gói có thể khoảng
70 MB do .NET Windows runtime được nhúng vào EXE; kích thước GUI không làm
lớn Docker runtime image.

GUI cho Browse folder, profile runtime/dev, image name, port, Start,
Stop, Status, Update, Doctor, Logs, Open browser. Nếu muốn tải image từ
registry thay vì build source đóng gói, chọn **Pull image (registry)** và
chỉ nhập tên image/version đã được phát hành, xác minh nguồn tin cậy.
`Update` là thao tác chủ động; không tự động sửa các file project.

**Không nhầm** Windows Launcher `.exe` này với Windows-native TaskDeck
backend. EXE quản lý Docker Linux nên không cần ConPTY trên Windows.

## 5. Phát hành Docker image từ GHCR (optional)

Workflow [taskdeck_docker_release.yml](../.github/workflows/taskdeck_docker_release.yml)
hỗ trợ build `linux/amd64` và `linux/arm64` cho profile `runtime` và
`dev`. Chỉ publish GHCR nếu chọn `workflow_dispatch publish=true` hoặc
tạo Git tag `taskdeck-docker-v*`. Không tự động public image trên mỗi
commit main; tên/tag image chỉ hợp lệ sau một workflow publish PASS.

Ví dụ **sau khi đã publish thật**:

```powershell
.\deploy\windows\taskdeck.ps1 -Action Start -ProjectPath 'D:\Projects\M3' `
  -Image 'ghcr.io/hacrot3000/taskdeck:2026.10.14-runtime' -PullImage
```

Tag ở trên chỉ là ví dụ cú pháp, không xác nhận một image đã tồn tại.
