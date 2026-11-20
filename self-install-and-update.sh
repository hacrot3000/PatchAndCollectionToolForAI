#!/usr/bin/env bash
set -Eeuo pipefail

# ============================================================
# PatchAndCollectionToolForAI
# Self install / update
#
# Source:
#   https://github.com/hacrot3000/PatchAndCollectionToolForAI
#
# Branch:
#   main
#
# Có 2 chế độ:
#
# 1. REPOSITORY MODE
#
#    Nếu script đang nằm ngay tại root của chính repo:
#
#      hacrot3000/PatchAndCollectionToolForAI
#
#    và branch hiện tại là main:
#
#      git pull --ff-only origin main
#
#
# 2. PORTABLE INSTALL/UPDATE MODE
#
#    Nếu:
#      - thư mục chứa script không phải Git repo, hoặc
#      - nó nằm bên trong Git repo khác, hoặc
#      - chính thư mục đó là root của một repo khác
#
#    thì:
#
#      clone repo PatchAndCollectionToolForAI vào /tmp
#      và update code vào THƯ MỤC CHỨA SCRIPT.
#
#
# QUAN TRỌNG:
#
#   Target được xác định bằng vị trí của SCRIPT,
#   KHÔNG phải $(pwd).
#
# Vì vậy có thể gọi script từ bất kỳ đâu.
# ============================================================

REPO_SLUG="hacrot3000/PatchAndCollectionToolForAI"
REPO_URL="https://github.com/${REPO_SLUG}.git"
BRANCH="main"

# ------------------------------------------------------------
# Helpers
# ------------------------------------------------------------

die() {
    echo
    echo "[ERROR] $*" >&2
    exit 1
}

info() {
    echo "[INFO] $*"
}

ok() {
    echo "[ OK ] $*"
}

# ------------------------------------------------------------
# Resolve vị trí thực của script.
# ------------------------------------------------------------

SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(dirname -- "$SCRIPT_PATH")"
SCRIPT_NAME="$(basename -- "$SCRIPT_PATH")"

TARGET_DIR="$SCRIPT_DIR"

# ------------------------------------------------------------
# Dependencies
# ------------------------------------------------------------

command -v git >/dev/null 2>&1 ||
    die "Không tìm thấy lệnh git."

command -v mktemp >/dev/null 2>&1 ||
    die "Không tìm thấy mktemp."

command -v cp >/dev/null 2>&1 ||
    die "Không tìm thấy cp."

command -v cmp >/dev/null 2>&1 ||
    die "Không tìm thấy cmp."

[[ -d "$TARGET_DIR" ]] ||
    die "Target directory không tồn tại: $TARGET_DIR"

[[ -w "$TARGET_DIR" ]] ||
    die "Không có quyền ghi vào: $TARGET_DIR"

# ------------------------------------------------------------
# Git wrapper.
#
# Xóa các biến môi trường Git có thể được inherited từ:
#
#   hook
#   IDE
#   shell
#   project khác
#
# để không vô tình thao tác nhầm repository.
# ------------------------------------------------------------

git_clean() {
    env \
        -u GIT_DIR \
        -u GIT_WORK_TREE \
        -u GIT_INDEX_FILE \
        -u GIT_OBJECT_DIRECTORY \
        -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
        git "$@"
}

# ------------------------------------------------------------
# Kiểm tra một remote URL có phải chính xác repo của chúng ta.
#
# Hỗ trợ:
#
#   https://github.com/hacrot3000/PatchAndCollectionToolForAI.git
#   https://github.com/hacrot3000/PatchAndCollectionToolForAI
#   git@github.com:hacrot3000/PatchAndCollectionToolForAI.git
#   ssh://git@github.com/hacrot3000/PatchAndCollectionToolForAI.git
#
# ------------------------------------------------------------

is_expected_repo_url() {
    local url="${1:-}"

    url="${url%/}"
    url="${url%.git}"

    case "$url" in
        "https://github.com/${REPO_SLUG}")
            return 0
            ;;

        "http://github.com/${REPO_SLUG}")
            return 0
            ;;

        "git@github.com:${REPO_SLUG}")
            return 0
            ;;

        "ssh://git@github.com/${REPO_SLUG}")
            return 0
            ;;

        *)
            return 1
            ;;
    esac
}

# ------------------------------------------------------------
# Read version
# ------------------------------------------------------------

get_version() {
    local root="$1"

    if [[ -f "$root/_patch_lib/VERSION" ]]; then
        head -n 1 "$root/_patch_lib/VERSION" |
            tr -d '\r\n'
    else
        printf '%s' "unknown"
    fi
}

CURRENT_VERSION="$(get_version "$TARGET_DIR")"

echo
echo "============================================================"
echo " PatchAndCollectionToolForAI"
echo " Self install / update"
echo "============================================================"
echo
echo "Script       : $SCRIPT_PATH"
echo "Target       : $TARGET_DIR"
echo "Repository   : $REPO_URL"
echo "Branch       : $BRANCH"
echo "Version      : $CURRENT_VERSION"
echo

# ============================================================
# MODE DETECTION
# ============================================================

GIT_ROOT=""
ORIGIN_URL=""
CURRENT_BRANCH=""

if GIT_ROOT="$(
    git_clean -C "$TARGET_DIR" rev-parse --show-toplevel 2>/dev/null
)"; then

    GIT_ROOT="$(readlink -f -- "$GIT_ROOT")"

    ORIGIN_URL="$(
        git_clean -C "$GIT_ROOT" remote get-url origin 2>/dev/null || true
    )"

    CURRENT_BRANCH="$(
        git_clean -C "$GIT_ROOT" symbolic-ref \
            --quiet \
            --short HEAD 2>/dev/null || true
    )"
fi

# ============================================================
# MODE 1:
# Script đang nằm ngay root của chính repository.
# ============================================================

if [[ -n "$GIT_ROOT" ]] &&
   [[ "$GIT_ROOT" == "$TARGET_DIR" ]] &&
   is_expected_repo_url "$ORIGIN_URL"; then

    echo "Mode         : GIT REPOSITORY"
    echo "Git root     : $GIT_ROOT"
    echo "Origin       : $ORIGIN_URL"
    echo "Current branch: ${CURRENT_BRANCH:-DETACHED}"
    echo

    # --------------------------------------------------------
    # Đây đúng là repo của Patch Tool.
    #
    # Không dùng clone/copy đè vì như vậy sẽ phá semantics
    # của working tree.
    # --------------------------------------------------------

    if [[ "$CURRENT_BRANCH" != "$BRANCH" ]]; then
        die "Đây là repo ${REPO_SLUG}, nhưng đang ở branch '${CURRENT_BRANCH:-DETACHED}'.

Hãy chuyển sang branch '$BRANCH' trước:

  git switch $BRANCH

sau đó chạy lại:

  ./$SCRIPT_NAME

Script không tự overwrite một branch khác."
    fi

    BEFORE_COMMIT="$(
        git_clean -C "$TARGET_DIR" rev-parse HEAD
    )"

    BEFORE_SHORT="$(
        git_clean -C "$TARGET_DIR" rev-parse --short HEAD
    )"

    info "Đã phát hiện chính repository PatchAndCollectionToolForAI."
    info "Update trực tiếp bằng git pull --ff-only..."

    echo
    git_clean -C "$TARGET_DIR" pull --ff-only origin "$BRANCH"
    echo

    AFTER_COMMIT="$(
        git_clean -C "$TARGET_DIR" rev-parse HEAD
    )"

    AFTER_SHORT="$(
        git_clean -C "$TARGET_DIR" rev-parse --short HEAD
    )"

    NEW_VERSION="$(get_version "$TARGET_DIR")"

    echo "============================================================"
    echo " UPDATE SUCCESS"
    echo "============================================================"
    echo
    echo "Mode            : git pull"
    echo "Previous commit : $BEFORE_SHORT"
    echo "Current commit  : $AFTER_SHORT"
    echo "Version         : $NEW_VERSION"
    echo "Branch          : $BRANCH"
    echo "Repository      : $TARGET_DIR"
    echo

    if [[ "$BEFORE_COMMIT" == "$AFTER_COMMIT" ]]; then
        echo "Status          : Already up to date"
    else
        echo "Status          : Updated"
    fi

    echo
    exit 0
fi

# ============================================================
# Nếu tới đây thì KHÔNG được git pull repository hiện tại.
# ============================================================

echo "Mode         : PORTABLE INSTALL / UPDATE"

if [[ -z "$GIT_ROOT" ]]; then

    echo "Git context  : target không nằm trong Git repository"

elif [[ "$GIT_ROOT" != "$TARGET_DIR" ]]; then

    echo "Git context  : target đang nằm bên trong Git repo khác"
    echo "Other repo   : $GIT_ROOT"

else

    echo "Git context  : target là root của một Git repo khác"
    echo "Other repo   : $GIT_ROOT"
    echo "Other origin : ${ORIGIN_URL:-<none>}"

fi

echo
echo "Repository hiện tại sẽ KHÔNG bị git pull/reset/checkout."
echo

# ============================================================
# MODE 2:
# Portable install / update
# ============================================================

TMP_DIR=""

cleanup() {
    local rc=$?

    if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
        rm -rf -- "$TMP_DIR"
    fi

    return "$rc"
}

trap cleanup EXIT

TMP_DIR="$(
    mktemp -d "${TMPDIR:-/tmp}/patchtool-self-update.XXXXXXXX"
)"

SOURCE_DIR="$TMP_DIR/source"

# ------------------------------------------------------------
# Clone source.
# ------------------------------------------------------------

info "Clone $REPO_SLUG branch '$BRANCH' vào thư mục tạm..."

git_clean clone \
    --quiet \
    --depth 1 \
    --single-branch \
    --branch "$BRANCH" \
    "$REPO_URL" \
    "$SOURCE_DIR"

REMOTE_COMMIT="$(
    git_clean -C "$SOURCE_DIR" rev-parse HEAD
)"

REMOTE_SHORT_COMMIT="$(
    git_clean -C "$SOURCE_DIR" rev-parse --short HEAD
)"

REMOTE_VERSION="$(get_version "$SOURCE_DIR")"

# ------------------------------------------------------------
# Verify repository vừa clone.
# ------------------------------------------------------------

[[ -f "$SOURCE_DIR/run_python_patches.sh" ]] ||
    die "Repository source không có run_python_patches.sh"

[[ -d "$SOURCE_DIR/_patch_lib" ]] ||
    die "Repository source không có _patch_lib/"

[[ -f "$SOURCE_DIR/_patch_lib/VERSION" ]] ||
    die "Repository source không có _patch_lib/VERSION"

ok "Clone thành công."

echo
echo "Remote version: $REMOTE_VERSION"
echo "Remote commit : $REMOTE_SHORT_COMMIT"
echo

# ------------------------------------------------------------
# Lấy CHỈ tracked files.
#
# Vì vậy:
#
#   .git/
#
# không bao giờ được copy.
# ------------------------------------------------------------

mapfile -d '' -t TRACKED_FILES < <(
    git_clean -C "$SOURCE_DIR" ls-files -z
)

((${#TRACKED_FILES[@]} > 0)) ||
    die "Repository source không có tracked file."

# ============================================================
# Update _patch_lib
# ============================================================
#
# _patch_lib được replace nguyên directory.
#
# Lý do:
#
# Nếu version mới xóa:
#
#   _patch_lib/old_file.py
#
# thì copy đè thông thường sẽ khiến old_file.py vẫn còn.
# ============================================================

NEW_LIB="$TARGET_DIR/.patch_lib.new.$$"
OLD_LIB="$TARGET_DIR/.patch_lib.old.$$"

rm -rf -- "$NEW_LIB" "$OLD_LIB"

info "Chuẩn bị _patch_lib mới..."

cp -a \
    "$SOURCE_DIR/_patch_lib" \
    "$NEW_LIB"

[[ -f "$NEW_LIB/VERSION" ]] ||
    die "Staging _patch_lib không hợp lệ."

info "Cập nhật _patch_lib..."

if [[ -e "$TARGET_DIR/_patch_lib" ||
      -L "$TARGET_DIR/_patch_lib" ]]; then

    mv \
        "$TARGET_DIR/_patch_lib" \
        "$OLD_LIB"
fi

if ! mv "$NEW_LIB" "$TARGET_DIR/_patch_lib"; then

    echo "[ERROR] Không thể activate _patch_lib mới." >&2

    rm -rf -- "$TARGET_DIR/_patch_lib"

    if [[ -e "$OLD_LIB" || -L "$OLD_LIB" ]]; then

        mv \
            "$OLD_LIB" \
            "$TARGET_DIR/_patch_lib"

        echo "[ROLLBACK] Đã phục hồi _patch_lib cũ." >&2
    fi

    exit 1
fi

rm -rf -- "$OLD_LIB"

# ============================================================
# Copy toàn bộ tracked files còn lại
# ============================================================

info "Cập nhật public files..."

for rel in "${TRACKED_FILES[@]}"; do

    # _patch_lib đã được xử lý nguyên cây ở trên.
    case "$rel" in
        _patch_lib/*)
            continue
            ;;
    esac

    # --------------------------------------------------------
    # Reject path bất thường.
    # --------------------------------------------------------

    case "$rel" in
        /*|../*|*/../*|*/..)
            die "Repository chứa path không an toàn: $rel"
            ;;
    esac

    src="$SOURCE_DIR/$rel"
    dst="$TARGET_DIR/$rel"

    mkdir -p -- "$(dirname -- "$dst")"

    # --------------------------------------------------------
    # Không tự xóa directory lạ để thay bằng file.
    # --------------------------------------------------------

    if [[ -d "$dst" && ! -L "$dst" ]]; then
        die "Không thể overwrite directory bằng file:

  $dst"
    fi

    if [[ -L "$src" ]]; then

        rm -f -- "$dst"
        cp -a -- "$src" "$dst"

    elif [[ -f "$src" ]]; then

        # Copy tới temp file cùng filesystem,
        # rồi rename để tránh file dở dang.
        tmp_dst="${dst}.patchtool-new.$$"

        rm -f -- "$tmp_dst"

        cp -p \
            -- "$src" "$tmp_dst"

        mv -f \
            -- "$tmp_dst" "$dst"

    else

        die "Tracked path không phải regular file/symlink:

  $rel"
    fi
done

# ------------------------------------------------------------
# Launcher phải executable.
# ------------------------------------------------------------

if [[ -f "$TARGET_DIR/run_python_patches.sh" ]]; then
    chmod +x "$TARGET_DIR/run_python_patches.sh"
fi

if [[ -f "$TARGET_DIR/$SCRIPT_NAME" ]]; then
    chmod +x "$TARGET_DIR/$SCRIPT_NAME"
fi

# ============================================================
# Verify sau update
# ============================================================

info "Verify installed files..."

VERIFY_FAILED=0

for rel in "${TRACKED_FILES[@]}"; do

    src="$SOURCE_DIR/$rel"
    dst="$TARGET_DIR/$rel"

    if [[ -L "$src" ]]; then

        if [[ ! -L "$dst" ]]; then
            echo "[VERIFY FAILED] $rel: destination không phải symlink"
            VERIFY_FAILED=1
            continue
        fi

        src_link="$(readlink -- "$src")"
        dst_link="$(readlink -- "$dst")"

        if [[ "$src_link" != "$dst_link" ]]; then
            echo "[VERIFY FAILED] $rel: symlink target khác source"
            VERIFY_FAILED=1
        fi

    elif [[ -f "$src" ]]; then

        if [[ ! -f "$dst" ]]; then
            echo "[VERIFY FAILED] Missing: $rel"
            VERIFY_FAILED=1
            continue
        fi

        if ! cmp -s -- "$src" "$dst"; then
            echo "[VERIFY FAILED] Content differs: $rel"
            VERIFY_FAILED=1
        fi
    fi
done

if (( VERIFY_FAILED != 0 )); then
    die "Verify thất bại."
fi

INSTALLED_VERSION="$(get_version "$TARGET_DIR")"

echo
echo "============================================================"
echo " INSTALL / UPDATE SUCCESS"
echo "============================================================"
echo
echo "Mode             : portable clone + update"
echo "Previous version : $CURRENT_VERSION"
echo "Installed version: $INSTALLED_VERSION"
echo "Source branch    : $BRANCH"
echo "Source commit    : $REMOTE_COMMIT"
echo "Target           : $TARGET_DIR"
echo
echo "Repository chứa target, nếu có, KHÔNG bị thao tác Git."
echo
