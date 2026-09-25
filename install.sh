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
TMP_ROOT=""
STAGED_RELEASE=""

cleanup() {
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
    local url="$1" output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fL --retry 3 --connect-timeout 10 --max-time 120 -H 'User-Agent: taskdeck-installer' -o "$output" "$url" && return 0
    fi
    if command -v wget >/dev/null 2>&1; then
        wget -q --timeout=120 --user-agent='taskdeck-installer' -O "$output" "$url" && return 0
    fi
    return 1
}

remote_revision() {
    local json revision
    json="$(http_stdout "$API_URL")" || die "Không lấy được revision mới nhất từ GitHub."
    revision="$(printf '%s\n' "$json" | grep -oE '"sha"[[:space:]]*:[[:space:]]*"[0-9a-fA-F]{40}"' | head -n 1 | sed -E 's/.*"([0-9a-fA-F]{40})".*/\1/' || true)"
    [[ "$revision" =~ ^[0-9a-fA-F]{40}$ ]] || die "GitHub trả về revision không hợp lệ."
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
    command -v python3 >/dev/null 2>&1 || die "Cần Python 3.10+ trong PATH để dùng Patch add-on."
    python3 -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 10) else 3)' >/dev/null 2>&1 ||
        die "Cần Python 3.10+ để dùng Patch add-on."
}

validate_release() {
    local release="$1" revision="$2"
    [[ -x "$release/taskdeck" ]] || return 1
    [[ -f "$release/patchtool/python_patch_entry.py" ]] || return 1
    [[ -f "$release/patchtool/_patch_lib/python_patch_queue_dispatcher.py" ]] || return 1
    [[ -f "$release/patchtool/HUONG_DAN_PYTHON_PATCH_TOOL.html" ]] || return 1
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
    mv -Tf -- "$tmp" "$link"
}

command -v go >/dev/null 2>&1 || die "Cần Go toolchain trong PATH để cài TaskDeck."
command -v tar >/dev/null 2>&1 || die "Cần lệnh tar để cài TaskDeck."
python_gate
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
        tar --no-same-owner --no-same-permissions -xzf "$archive" -C "$extract"
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
    "$SOURCE_ROOT/python_patch_entry.py" \
    "$SOURCE_ROOT/run_python_patches.sh" \
    "$SOURCE_ROOT/_patch_lib/python_patch_queue_dispatcher.py" \
    "$SOURCE_ROOT/HUONG_DAN_PYTHON_PATCH_TOOL.html"; do
    [[ -f "$required" ]] || die "Source thiếu runtime bắt buộc: $required"
done

echo "TaskDeck: chạy test trước khi cài..."
(
    cd "$SOURCE"
    GOPROXY=off GOSUMDB=off go test ./...
)
(
    cd "$SOURCE_ROOT"
    python3 test_python_patch_entry.py
    python3 -m py_compile python_patch_entry.py
)

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
    cp "$SOURCE_ROOT/HUONG_DAN_PYTHON_PATCH_TOOL.html" "$STAGED_RELEASE/patchtool/HUONG_DAN_PYTHON_PATCH_TOOL.html"
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
case ":${PATH:-}:" in
    *":$BIN_DIR:"*) ;;
    *) echo "LƯU Ý: $BIN_DIR chưa có trong PATH. Hãy thêm nó để chạy lệnh: taskdeck" ;;
esac
