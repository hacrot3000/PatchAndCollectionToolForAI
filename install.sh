#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="hacrot3000/PatchAndCollectionToolForAI"
BRANCH="main"
API_URL="https://api.github.com/repos/$REPOSITORY/commits/$BRANCH"
ARCHIVE_BASE_URL="https://codeload.github.com/$REPOSITORY/tar.gz"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${TASKDECK_INSTALL_DIR:-${HOME}/.local/bin}"
APP_ROOT="${TASKDECK_APP_DIR:-${HOME}/.local/lib/taskdeck}"
RELEASES_DIR="$APP_ROOT/releases"
CURRENT_LINK="$APP_ROOT/current"
TARGET="$BIN_DIR/taskdeck"
MAX_ARCHIVE_BYTES=$((128 * 1024 * 1024))
GO_BOOTSTRAP_VERSION="go1.27.3"
GO_MAX_ARCHIVE_BYTES=$((160 * 1024 * 1024))
GO_TMP_ROOT=""
STAGED_GO=""
TMP_ROOT=""
STAGED_RELEASE=""
PYTHON_310_AVAILABLE=0
PYTHON_CMD=""

cleanup() {
    [[ -n "${STAGED_GO:-}" && -d "$STAGED_GO" ]] && rm -rf -- "$STAGED_GO"
    [[ -n "${GO_TMP_ROOT:-}" && -d "$GO_TMP_ROOT" ]] && rm -rf -- "$GO_TMP_ROOT"
    [[ -n "${STAGED_RELEASE:-}" && -d "$STAGED_RELEASE" ]] && rm -rf -- "$STAGED_RELEASE"
    [[ -n "${TMP_ROOT:-}" ]] && rm -rf -- "$TMP_ROOT"
}
trap cleanup EXIT

die() {
    echo "ERROR: $*" >&2
    exit 1
}

http_stdout() {
    local url="$1"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --retry 3 --connect-timeout 10 --max-time 30 -H 'Accept: application/vnd.github+json' -H 'User-Agent: taskdeck-installer' "$url" && return 0
    fi
    if command -v wget >/dev/null 2>&1; then
        wget -qO- --timeout=30 --header='Accept: application/vnd.github+json' --user-agent='taskdeck-installer' "$url" && return 0
    fi
    return 1
}

http_download() {
    local url="$1" output="$2" timeout="${3:-120}"
    if command -v curl >/dev/null 2>&1; then
        curl -fL --retry 3 --connect-timeout 10 --max-time "$timeout" -H 'User-Agent: taskdeck-installer' -o "$output" "$url" && return 0
    fi
    if command -v wget >/dev/null 2>&1; then
        wget -q --timeout="$timeout" --user-agent='taskdeck-installer' -O "$output" "$url" && return 0
    fi
    return 1
}

remote_revision_git() {
    command -v git >/dev/null 2>&1 || return 1
    local ref="refs/heads/$BRANCH" output line revision
    output="$(GIT_TERMINAL_PROMPT=0 GIT_ASKPASS= GCM_INTERACTIVE=never git -c credential.helper= ls-remote --heads --exit-code "https://github.com/$REPOSITORY.git" "$ref" 2>/dev/null)" || return 1
    revision=""
    while IFS= read -r line; do
        local sha name
        read -r sha name <<< "$line"
        if [[ "$name" == "$ref" && "$sha" =~ ^[0-9a-fA-F]{40}$ ]]; then
            [[ -z "$revision" ]] || return 1
            revision="$sha"
        fi
    done <<< "$output"
    [[ "$revision" =~ ^[0-9a-fA-F]{40}$ ]] || return 1
    printf '%s\n' "$revision"
}

remote_revision() {
    local json revision=""
    if json="$(http_stdout "$API_URL")"; then
        revision="$(printf '%s\n' "$json" | grep -oE '"sha"[[:space:]]*:[[:space:]]*"[0-9a-fA-F]{40}"' | head -n 1 | sed -E 's/.*"([0-9a-fA-F]{40})".*/\1/' || true)"
        [[ "$revision" =~ ^[0-9a-fA-F]{40}$ ]] && { printf '%s\n' "$revision"; return 0; }
        echo "WARNING: GitHub API trả về revision không hợp lệ; không sử dụng giá trị này." >&2
        return 1
    fi
    echo "GitHub API không truy cập được; thử git ls-remote HTTPS (không cần API token)..." >&2
    revision="$(remote_revision_git)" || die "Không lấy được revision qua cả GitHub API lẫn git HTTPS; kiểm tra kết nối github.com và git."
    printf '%s\n' "$revision"
}

stage_with_git() {
    local revision="$1" root="$2"
    command -v git >/dev/null 2>&1 || return 1
    local checkout="$root/git-source"
    mkdir -p "$checkout"
    git init -q "$checkout" || return 1
    git -C "$checkout" remote add origin "https://github.com/$REPOSITORY.git" || return 1
    GIT_TERMINAL_PROMPT=0 git -C "$checkout" fetch -q --depth 1 origin "$revision" || return 1
    git -C "$checkout" checkout -q --detach FETCH_HEAD || return 1
    [[ -f "$checkout/vscode_tasks_menu_go/go.mod" ]] || return 1
    printf '%s\n' "$checkout"
}

python_gate() {
    local candidate="" path="" version="" checked=""
    local candidates=()

    if [[ -n "${TASKDECK_PYTHON:-}" ]]; then
        candidates=("$TASKDECK_PYTHON")
    else
        candidates=(python3.13 python3.12 python3.11 python3.10 python3 python)
    fi

    for candidate in "${candidates[@]}"; do
        if [[ "$candidate" == */* ]]; then
            [[ -x "$candidate" ]] || continue
            path="$candidate"
        else
            path="$(command -v "$candidate" 2>/dev/null || true)"
            [[ -n "$path" ]] || continue
        fi

        version="$("$path" --version 2>&1 || true)"
        checked+="${checked:+; }$candidate=${version:-unknown}"
        if "$path" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 10) else 3)' >/dev/null 2>&1; then
            PYTHON_CMD="$path"
            PYTHON_310_AVAILABLE=1
            echo "TaskDeck: dùng Python runtime: $PYTHON_CMD ($version)"
            return 0
        fi
    done

    if [[ -n "${TASKDECK_PYTHON:-}" ]]; then
        echo "WARNING: TASKDECK_PYTHON=$TASKDECK_PYTHON không phải Python 3.10+ khả dụng." >&2
    else
        echo "WARNING: Không tìm thấy Python 3.10+ phù hợp. Đã kiểm tra: ${checked:-không có interpreter nào trong PATH}." >&2
    fi
    echo "WARNING: TaskDeck core vẫn sẽ được cài và có thể chạy bình thường." >&2
    echo "WARNING: Patch add-on và self-update Python validation vẫn cần Python 3.10+." >&2
    echo "WARNING: Shared identity SQLite sẽ tự fallback sang sqlite3 CLI tương thích (SQLite 3.7+) nếu Python 3.10+ không khả dụng." >&2
    echo "WARNING: Shared password hashing có thể dùng Python cũ nếu runtime đó có hashlib.scrypt; identity backup vẫn có thể cần Python mới hơn." >&2
}

# Install Python for the Patch add-on instead of silently deploying a partial release.
ensure_python() {
    python_gate
    if (( PYTHON_310_AVAILABLE )); then return 0; fi
    [[ -z "${TASKDECK_PYTHON:-}" ]] || die "TASKDECK_PYTHON không đáp ứng Python 3.10+; hãy chọn interpreter hợp lệ."
    local os
    os="$(uname -s)"
    echo "TaskDeck: tự cài Python 3.12 cho $os..."
    case "$os" in
        Darwin)
            command -v brew >/dev/null 2>&1 || die "macOS thiếu Homebrew. Cài Homebrew tại https://brew.sh rồi chạy lại installer."
            brew install python@3.12 || die "Homebrew không cài được Python 3.12."
            local brew_python
            brew_python="$(brew --prefix python@3.12)/bin/python3.12"
            [[ -x "$brew_python" ]] || die "Không tìm thấy Python sau khi Homebrew cài đặt."
            PYTHON_CMD="$brew_python"
            ;;
        Linux)
            if command -v apt-get >/dev/null 2>&1; then
                local apt=(apt-get)
                if (( EUID != 0 )); then
                    command -v sudo >/dev/null 2>&1 || die "Cần sudo để cài Python trên hệ thống apt."
                    apt=(sudo apt-get)
                fi
                "${apt[@]}" update && "${apt[@]}" install -y python3 || die "Không tự cài được Python qua apt."
            elif command -v dnf >/dev/null 2>&1; then
                local dnf=(dnf)
                if (( EUID != 0 )); then
                    command -v sudo >/dev/null 2>&1 || die "Cần sudo để cài Python trên hệ thống dnf."
                    dnf=(sudo dnf)
                fi
                "${dnf[@]}" install -y python3 || die "Không tự cài được Python qua dnf."
            else
                die "Linux không có apt-get/dnf. Hãy cài Python 3.10+ bằng package manager của hệ điều hành."
            fi
            python_gate
            (( PYTHON_310_AVAILABLE )) || die "Python hệ thống vẫn <3.10 sau khi cài. Cần repository cung cấp Python 3.10+."
            ;;
        *) die "Không hỗ trợ tự cài Python trên $os." ;;
    esac
    "$PYTHON_CMD" -c 'import sys; assert sys.version_info >= (3,10)' || die "Python mới cài không đạt 3.10+."
    PYTHON_310_AVAILABLE=1
    echo "TaskDeck: Python phù hợp: $("$PYTHON_CMD" --version)"
}

# Persist the command directory for new login and interactive shells.
# Running ./install.sh cannot change the parent terminal's environment.
ensure_command_path() {
    local user_shell escaped_dir profile
    local profiles=()
    user_shell="${SHELL##*/}"
    if [[ "$user_shell" == "zsh" ]]; then
        profiles=("$HOME/.zprofile" "$HOME/.zshrc")
    elif [[ "$user_shell" == "bash" ]]; then
        profiles=("$HOME/.bashrc")
        if [[ -e "$HOME/.bash_profile" ]]; then
            profiles+=("$HOME/.bash_profile")
        else
            profiles+=("$HOME/.profile")
        fi
    else
        echo "WARNING: Shell '$user_shell' chưa hỗ trợ cấu hình PATH tự động; TaskDeck đã cài tại $TARGET." >&2
        return 0
    fi
    printf -v escaped_dir '%q' "$BIN_DIR"
    for profile in "${profiles[@]}"; do
        if [[ -L "$profile" ]]; then
            echo "WARNING: Bỏ qua shell profile là symlink: $profile" >&2
            continue
        fi
        if [[ -e "$profile" && ! -f "$profile" ]]; then
            echo "WARNING: Bỏ qua shell profile không phải file: $profile" >&2
            continue
        fi
        if [[ -f "$profile" ]] && grep -Fq '# TaskDeck installer PATH' "$profile"; then
            continue
        fi
        if ! {
            printf '\n# TaskDeck installer PATH\n'
            printf 'case ":$PATH:" in *:%s:*) ;; *) export PATH=%s:"$PATH" ;; esac\n' "$escaped_dir" "$escaped_dir"
        } >> "$profile"; then
            echo "WARNING: Không ghi được PATH vào $profile" >&2
            continue
        fi
        echo "TaskDeck: đã cấu hình PATH trong $profile"
    done
    case ":$PATH:" in
        *:"$BIN_DIR":*) ;;
        *) export PATH="$BIN_DIR:$PATH" ;;
    esac
    hash -r 2>/dev/null || true
    echo "TaskDeck: để dùng lệnh ngay trong terminal đang mở, chạy: export PATH=$escaped_dir:\$PATH"
}

validate_release() {
    local release="$1" revision="$2"
    [[ -x "$release/taskdeck" ]] || return 1
    [[ -f "$release/patchtool/python_patch_entry.py" ]] || return 1
    [[ -f "$release/patchtool/_patch_lib/python_patch_queue_dispatcher.py" ]] || return 1
    if [[ "$revision" != "dev" && "$revision" != dev-* ]]; then
        "$release/taskdeck" --version 2>/dev/null | grep -Fq "$revision" || return 1
    fi
    return 0
}

atomic_symlink() {
    local target="$1" link="$2"
    local parent tmp
    parent="$(dirname "$link")"
    mkdir -p "$parent"
    tmp="$parent/.$(basename "$link").new.$$"
    rm -f -- "$tmp"
    ln -s "$target" "$tmp"
    # GNU mv -T is absent on macOS BSD mv; -h avoids following a
    # destination symlink that points at a directory.
    if [[ "$(uname -s)" == "Darwin" ]]; then
        mv -fh "$tmp" "$link"
    else
        mv -Tf -- "$tmp" "$link"
    fi
}


# Go is an installer-managed dependency, not a prerequisite.
go_version_supported() {
    local version major minor minimum
    command -v go >/dev/null 2>&1 || return 1
    version="$(go version 2>/dev/null || true)"
    [[ "$version" =~ go([0-9]+)\.([0-9]+) ]] || return 1
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    minimum=19
    [[ "$(uname -s)" != "Darwin" ]] || minimum=25
    (( major > 1 || (major == 1 && minor >= minimum) ))
}
sha256_file() {
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    elif command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$1" | awk '{print $NF}'
    else
        die "Không có shasum/sha256sum/openssl; không thể xác minh Go archive."
    fi
}
ensure_go() {
    if go_version_supported; then
        echo "TaskDeck: sử dụng $(go version)"
        return 0
    fi
    local os arch platform expected filename go_root go_final archive bytes actual version
    os="$(uname -s)"
    arch="$(uname -m)"
    case "$os/$arch" in
        Darwin/arm64|Darwin/aarch64)
            platform="darwin-arm64"
            expected="dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2"
            ;;
        Darwin/x86_64)
            platform="darwin-amd64"
            expected="b6f8e931e4ab497fcde6da8ae4499a58f9d258e55c8a80824a516eae5950a750"
            ;;
        Linux/x86_64|Linux/amd64)
            platform="linux-amd64"
            expected="65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40"
            ;;
        Linux/aarch64|Linux/arm64)
            platform="linux-arm64"
            expected="a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49"
            ;;
        *)
            die "Chưa hỗ trợ tự cài Go cho $os/$arch."
            ;;
    esac
    go_root="$APP_ROOT/toolchains"
    go_final="$go_root/$GO_BOOTSTRAP_VERSION-$platform"
    [[ ! -L "$go_final" ]] || die "Go directory không được là symlink: $go_final"
    if [[ -x "$go_final/bin/go" ]] && "$go_final/bin/go" version 2>/dev/null | grep -Fq " $GO_BOOTSTRAP_VERSION "; then
        echo "TaskDeck: dùng Go đã tự cài: $go_final"
    else
        [[ ! -e "$go_final" ]] || die "Go tại $go_final không hợp lệ; đổi tên thư mục này để cài lại."
        command -v tar >/dev/null 2>&1 || die "Cần tar để giải nén Go."
        mkdir -p "$go_root"
        GO_TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/taskdeck-go.XXXXXX")"
        filename="$GO_BOOTSTRAP_VERSION.$platform.tar.gz"
        archive="$GO_TMP_ROOT/$filename"
        echo "TaskDeck: tự tải $filename từ go.dev, không cần sudo..."
        http_download "https://go.dev/dl/$filename" "$archive" 600 || die "Không tải được Go chính thức."
        bytes="$(wc -c < "$archive" | tr -d '[:space:]')"
        [[ "$bytes" =~ ^[0-9]+$ ]] && (( bytes > 1000000 && bytes <= GO_MAX_ARCHIVE_BYTES )) || die "Kích thước Go archive không hợp lệ."
        actual="$(sha256_file "$archive")"
        [[ "$actual" == "$expected" ]] || die "SHA256 của Go không khớp checksum chính thức; từ chối giải nén."
        tar -tzf "$archive" | awk '
            !/^go\// { bad = 1 }
            /(^|\/)\.\.($|\/)/ { bad = 1 }
            END { exit bad }
        ' || die "Go archive chứa đường dẫn không an toàn."
        tar -xzf "$archive" -C "$GO_TMP_ROOT" || die "Không giải nén được Go."
        STAGED_GO="$go_root/.$GO_BOOTSTRAP_VERSION-$platform.new.$$"
        [[ ! -e "$STAGED_GO" && ! -L "$STAGED_GO" ]] || die "Go staging đã tồn tại: $STAGED_GO"
        mv "$GO_TMP_ROOT/go" "$STAGED_GO"
        [[ -x "$STAGED_GO/bin/go" ]] || die "Go archive không có executable."
        version="$("$STAGED_GO/bin/go" version 2>/dev/null || true)"
        [[ "$version" == *" $GO_BOOTSTRAP_VERSION "* ]] || die "Go archive chạy không đúng version: $version"
        mv "$STAGED_GO" "$go_final"
        STAGED_GO=""
        echo "TaskDeck: đã cài Go tại $go_final"
    fi
    export PATH="$go_final/bin:$PATH"
    hash -r 2>/dev/null || true
    go_version_supported || die "Go mới cài không chạy được."
}

ensure_go
command -v tar >/dev/null 2>&1 || die "Cần lệnh tar để cài TaskDeck."
ensure_python
mkdir -p "$BIN_DIR" "$RELEASES_DIR"

SOURCE_ROOT=""
REVISION=""
if [[ -f "$SCRIPT_DIR/vscode_tasks_menu_go/go.mod" ]]; then
    SOURCE_ROOT="$SCRIPT_DIR"
    if command -v git >/dev/null 2>&1 && git -C "$SCRIPT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        REVISION="$(git -C "$SCRIPT_DIR" rev-parse HEAD 2>/dev/null || true)"
        if [[ -n "$(git -C "$SCRIPT_DIR" status --porcelain --untracked-files=no 2>/dev/null || true)" ]]; then
            REVISION="dev"
        fi
    fi
fi

if [[ -z "$SOURCE_ROOT" ]]; then
    REVISION="$(remote_revision)"
    TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/taskdeck-install.XXXXXX")"
    archive="$TMP_ROOT/source.tar.gz"
    extract="$TMP_ROOT/extract"
    mkdir -p "$extract"
    if http_download "$ARCHIVE_BASE_URL/$REVISION" "$archive"; then
        size="$(wc -c < "$archive" | tr -d '[:space:]')"
        [[ "$size" =~ ^[0-9]+$ ]] || die "Không xác định được kích thước archive."
        (( size <= MAX_ARCHIVE_BYTES )) || die "Archive vượt giới hạn 128 MiB."
        if [[ "$(uname -s)" == "Darwin" ]]; then
            tar -xzf "$archive" -C "$extract"
        else
            tar --no-same-owner --no-same-permissions -xzf "$archive" -C "$extract"
        fi
        go_mod="$(find "$extract" -type f -path '*/vscode_tasks_menu_go/go.mod' -print -quit)"
        [[ -n "$go_mod" ]] || die "Archive thiếu vscode_tasks_menu_go/go.mod."
        SOURCE_ROOT="$(dirname "$(dirname "$go_mod")")"
    else
        echo "Tải codeload thất bại; thử fallback bằng git..." >&2
        SOURCE_ROOT="$(stage_with_git "$REVISION" "$TMP_ROOT" || true)"
        [[ -n "$SOURCE_ROOT" ]] || die "Không tải được source qua codeload hoặc git."
    fi
fi

[[ -n "$REVISION" ]] || REVISION="$(remote_revision)"
SOURCE="$SOURCE_ROOT/vscode_tasks_menu_go"
for required in \
    "$SOURCE/go.mod" \
    "$SOURCE/internal/dbsqlite/sqlite_helper.py" \
    "$SOURCE_ROOT/python_patch_entry.py" \
    "$SOURCE_ROOT/run_python_patches.sh" \
    "$SOURCE_ROOT/_patch_lib/python_patch_queue_dispatcher.py"; do
    [[ -f "$required" ]] || die "Source thiếu runtime bắt buộc: $required"
done

echo "TaskDeck: kiểm tra source trước khi cài..."
(
    cd "$SOURCE"
    if [[ "${TASKDECK_INSTALL_FULL_VALIDATION:-0}" == "1" ]] && (( PYTHON_310_AVAILABLE )); then
        echo "TaskDeck: chạy full Go regression suite theo TASKDECK_INSTALL_FULL_VALIDATION=1..."
        GOPROXY=off GOSUMDB=off go test ./...
    else
        if [[ "${TASKDECK_INSTALL_FULL_VALIDATION:-0}" == "1" ]] && (( ! PYTHON_310_AVAILABLE )); then
            echo "TaskDeck: đã yêu cầu full validation nhưng Python < 3.10; chuyển sang compile-only validation." >&2
        else
            echo "TaskDeck: installer dùng production-package build validation; đặt TASKDECK_INSTALL_FULL_VALIDATION=1 để chạy toàn bộ Go tests." >&2
        fi
        GOPROXY=off GOSUMDB=off go build ./...
    fi
)
if (( PYTHON_310_AVAILABLE )); then
    (
        cd "$SOURCE_ROOT"
        "$PYTHON_CMD" test_python_patch_entry.py
        "$PYTHON_CMD" -m py_compile python_patch_entry.py
    )
else
    echo "TaskDeck: bỏ qua validation runtime của Patch add-on vì thiếu Python 3.10+." >&2
fi

RELEASE_ID="$REVISION"
if [[ "$REVISION" == "dev" ]]; then
    RELEASE_ID="dev-$(date -u +%Y%m%dT%H%M%SZ)-$$"
fi
FINAL_RELEASE="$RELEASES_DIR/$RELEASE_ID"

if validate_release "$FINAL_RELEASE" "$REVISION"; then
    echo "TaskDeck release đã tồn tại và hợp lệ: $FINAL_RELEASE"
else
    if [[ -e "$FINAL_RELEASE" || -L "$FINAL_RELEASE" ]]; then
        die "Release đích đã tồn tại nhưng không hợp lệ: $FINAL_RELEASE"
    fi
    STAGED_RELEASE="$RELEASES_DIR/.$RELEASE_ID.new.$$"
    rm -rf -- "$STAGED_RELEASE"
    mkdir -p "$STAGED_RELEASE/patchtool"

    build_args=(build -buildvcs=false -trimpath)
    if [[ "$REVISION" != "dev" ]]; then
        build_args+=(-ldflags "-X main.buildRevision=$REVISION")
    fi
    build_args+=(-o "$STAGED_RELEASE/taskdeck" ./cmd/vscode_tasks_menu)
    (
        cd "$SOURCE"
        GOPROXY=off GOSUMDB=off go "${build_args[@]}"
    )
    chmod 755 "$STAGED_RELEASE/taskdeck"

    cp "$SOURCE_ROOT/python_patch_entry.py" "$STAGED_RELEASE/patchtool/python_patch_entry.py"
    cp "$SOURCE_ROOT/run_python_patches.sh" "$STAGED_RELEASE/patchtool/run_python_patches.sh"
    [[ ! -f "$SOURCE_ROOT/run_python_patches.ps1" ]] || cp "$SOURCE_ROOT/run_python_patches.ps1" "$STAGED_RELEASE/patchtool/run_python_patches.ps1"
    [[ ! -f "$SOURCE_ROOT/run_python_patches.bat" ]] || cp "$SOURCE_ROOT/run_python_patches.bat" "$STAGED_RELEASE/patchtool/run_python_patches.bat"
    cp -a "$SOURCE_ROOT/_patch_lib" "$STAGED_RELEASE/patchtool/_patch_lib"
    chmod 755 "$STAGED_RELEASE/patchtool/python_patch_entry.py" "$STAGED_RELEASE/patchtool/run_python_patches.sh"

    validate_release "$STAGED_RELEASE" "$REVISION" || die "Release staging validation thất bại."
    mv -- "$STAGED_RELEASE" "$FINAL_RELEASE"
    STAGED_RELEASE=""
fi

atomic_symlink "$FINAL_RELEASE" "$CURRENT_LINK"
atomic_symlink "$CURRENT_LINK/taskdeck" "$TARGET"

if [[ "$REVISION" != "dev" ]]; then
    printf '%s\n' "$REVISION" > "$TARGET.revision.tmp"
    chmod 600 "$TARGET.revision.tmp"
    mv -f "$TARGET.revision.tmp" "$TARGET.revision"
else
    rm -f "$TARGET.revision"
fi

echo "Đã cài TaskDeck release: $FINAL_RELEASE"
echo "TaskDeck current: $CURRENT_LINK"
echo "TaskDeck command: $TARGET"
echo "Patch add-on: $CURRENT_LINK/patchtool"
if (( ! PYTHON_310_AVAILABLE )); then
    echo "WARNING: Đã cài TaskDeck core. Một số module yêu cầu Python 3.10+ vẫn được đóng gói nhưng có thể không chạy trên Python hiện tại." >&2
fi
ensure_command_path
