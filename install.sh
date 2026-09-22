#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="hacrot3000/PatchAndCollectionToolForAI"
BRANCH="main"
API_URL="https://api.github.com/repos/$REPOSITORY/commits/$BRANCH"
ARCHIVE_BASE_URL="https://codeload.github.com/$REPOSITORY/tar.gz"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${TASKDECK_INSTALL_DIR:-${HOME}/.local/bin}"
TARGET="$INSTALL_DIR/taskdeck"
MAX_ARCHIVE_BYTES=$((128 * 1024 * 1024))
TMP_ROOT=""

cleanup() {
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
    printf '%s\n' "$checkout/vscode_tasks_menu_go"
}

command -v go >/dev/null 2>&1 || die "Cần Go toolchain trong PATH để cài TaskDeck."
command -v tar >/dev/null 2>&1 || die "Cần lệnh tar để cài TaskDeck."
mkdir -p "$INSTALL_DIR"

SOURCE=""
REVISION=""
if [[ -f "$SCRIPT_DIR/vscode_tasks_menu_go/go.mod" ]]; then
    SOURCE="$SCRIPT_DIR/vscode_tasks_menu_go"
    if command -v git >/dev/null 2>&1 && git -C "$SCRIPT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        REVISION="$(git -C "$SCRIPT_DIR" rev-parse HEAD 2>/dev/null || true)"
        if [[ -n "$(git -C "$SCRIPT_DIR" status --porcelain --untracked-files=no 2>/dev/null || true)" ]]; then
            REVISION="dev"
        fi
    fi
fi

if [[ -z "$SOURCE" ]]; then
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
        SOURCE="$(dirname "$go_mod")"
    else
        echo "Tải codeload thất bại; thử fallback bằng git..." >&2
        SOURCE="$(stage_with_git "$REVISION" "$TMP_ROOT" || true)"
        [[ -n "$SOURCE" ]] || die "Không tải được source qua codeload hoặc git."
    fi
fi

[[ -n "$REVISION" ]] || REVISION="$(remote_revision)"
echo "TaskDeck: chạy test trước khi cài..."
(
    cd "$SOURCE"
    GOPROXY=off GOSUMDB=off go test ./...
)

staged="$(mktemp "$INSTALL_DIR/.taskdeck.new.XXXXXX")"
rm -f "$staged"
build_args=(build -buildvcs=false -trimpath)
if [[ "$REVISION" != "dev" ]]; then
    build_args+=(-ldflags "-X main.buildRevision=$REVISION")
fi
build_args+=(-o "$staged" ./cmd/vscode_tasks_menu)
(
    cd "$SOURCE"
    GOPROXY=off GOSUMDB=off go "${build_args[@]}"
)
chmod 755 "$staged"

version_output="$("$staged" --version)"
if [[ "$REVISION" != "dev" && "$version_output" != *"$REVISION"* ]]; then
    rm -f "$staged"
    die "Binary validation thất bại: $version_output"
fi
mv -f "$staged" "$TARGET"
if [[ "$REVISION" != "dev" ]]; then
    printf '%s\n' "$REVISION" > "$TARGET.revision.tmp"
    chmod 600 "$TARGET.revision.tmp"
    mv -f "$TARGET.revision.tmp" "$TARGET.revision"
else
    rm -f "$TARGET.revision"
fi

echo "Đã cài TaskDeck: $TARGET"
case ":${PATH:-}:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "LƯU Ý: $INSTALL_DIR chưa có trong PATH. Hãy thêm nó để chạy lệnh: taskdeck" ;;
esac
