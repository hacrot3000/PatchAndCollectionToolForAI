#!/usr/bin/env bash
set -Eeuo pipefail

# ============================================================
# Python Patch Tool - self updater / installer
#
# Source:
#   https://github.com/hacrot3000/PatchAndCollectionToolForAI
#
# Branch:
#   main
#
# QUAN TRỌNG:
#   TARGET_DIR = thư mục CHỨA SCRIPT này.
#
# Ví dụ:
#
#   /home/user/project/
#   ├── tools/
#   │   ├── update_patch_tool.sh     <-- script này
#   │   ├── run_python_patches.sh
#   │   └── _patch_lib/
#   └── .git/
#
# Có thể đứng ở bất kỳ đâu:
#
#   cd /tmp
#   /home/user/project/tools/update_patch_tool.sh
#
# Patch Tool vẫn chỉ update:
#
#   /home/user/project/tools/
#
# Nó KHÔNG dùng pwd làm target.
# Nó KHÔNG thao tác Git repo chứa project.
# ============================================================

REPO_URL="https://github.com/hacrot3000/PatchAndCollectionToolForAI.git"
BRANCH="main"

# ------------------------------------------------------------
# Helper
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
# Resolve chính xác vị trí script.
#
# Dùng readlink -f để nếu gọi:
#
#   ./tools/update_patch_tool.sh
#
# hoặc:
#
#   /absolute/path/tools/update_patch_tool.sh
#
# thì kết quả vẫn giống nhau.
# ------------------------------------------------------------

SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(dirname -- "$SCRIPT_PATH")"
SCRIPT_NAME="$(basename -- "$SCRIPT_PATH")"

TARGET_DIR="$SCRIPT_DIR"

# ------------------------------------------------------------
# Dependency
# ------------------------------------------------------------

command -v git >/dev/null 2>&1 ||
    die "Không tìm thấy git."

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
# Không để Git context của project hiện tại ảnh hưởng clone.
#
# Điều này đặc biệt hữu ích nếu script được gọi từ:
#
#   - một Git repo khác
#   - git hook
#   - shell có GIT_DIR/GIT_WORK_TREE
#
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
# Lock chống chạy hai updater cùng lúc
# ------------------------------------------------------------

LOCK_DIR="$TARGET_DIR/.patchtool-update.lock"

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    die "Có vẻ Patch Tool updater khác đang chạy: $LOCK_DIR"
fi

TMP_DIR=""

cleanup() {
    local rc=$?

    if [[ -n "${TMP_DIR:-}" && -d "$TMP_DIR" ]]; then
        rm -rf -- "$TMP_DIR"
    fi

    rm -rf -- "$LOCK_DIR"

    exit "$rc"
}

trap cleanup EXIT INT TERM

# ------------------------------------------------------------
# Current version
# ------------------------------------------------------------

CURRENT_VERSION="not-installed"

if [[ -f "$TARGET_DIR/_patch_lib/VERSION" ]]; then
    CURRENT_VERSION="$(
        head -n 1 "$TARGET_DIR/_patch_lib/VERSION" \
        | tr -d '\r\n'
    )"
fi

echo
echo "============================================================"
echo " Python Patch Tool updater"
echo "============================================================"
echo
echo "Source        : $REPO_URL"
echo "Branch        : $BRANCH"
echo "Script        : $SCRIPT_PATH"
echo "Install target: $TARGET_DIR"
echo "Current       : $CURRENT_VERSION"
echo

# ------------------------------------------------------------
# Clone vào TEMP.
#
# Tuyệt đối không clone vào TARGET_DIR.
# Tuyệt đối không git init/pull/reset project hiện tại.
# ------------------------------------------------------------

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/patchtool-update.XXXXXXXX")"
SOURCE_DIR="$TMP_DIR/source"

info "Đang lấy branch '$BRANCH' từ GitHub..."

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

# ------------------------------------------------------------
# Verify source trước khi đụng đến bản đang chạy
# ------------------------------------------------------------

[[ -f "$SOURCE_DIR/run_python_patches.sh" ]] ||
    die "Source không có run_python_patches.sh"

[[ -d "$SOURCE_DIR/_patch_lib" ]] ||
    die "Source không có _patch_lib/"

[[ -f "$SOURCE_DIR/_patch_lib/VERSION" ]] ||
    die "Source không có _patch_lib/VERSION"

REMOTE_VERSION="$(
    head -n 1 "$SOURCE_DIR/_patch_lib/VERSION" \
    | tr -d '\r\n'
)"

[[ -n "$REMOTE_VERSION" ]] ||
    die "_patch_lib/VERSION rỗng."

ok "Đã tải source."
echo
echo "Remote version : $REMOTE_VERSION"
echo "Remote commit  : $REMOTE_SHORT_COMMIT"
echo

# ------------------------------------------------------------
# Lấy danh sách file Git thực sự quản lý.
#
# Không copy:
#
#   source/.git/
#
# và không dựa vào `find` toàn bộ clone.
# ------------------------------------------------------------

mapfile -d '' -t TRACKED_FILES < <(
    git_clean -C "$SOURCE_DIR" ls-files -z
)

((${#TRACKED_FILES[@]} > 0)) ||
    die "Repository không có tracked file."

# ------------------------------------------------------------
# _patch_lib là private runtime của Patch Tool.
#
# Ta thay TOÀN BỘ directory này để tránh trường hợp:
#
# v6.x:
#   _patch_lib/old_module.py
#
# v6.y:
#   old_module.py đã bị xóa khỏi GitHub
#
# Nếu chỉ cp đè, old_module.py sẽ còn lại và có thể gây lỗi.
# ------------------------------------------------------------

NEW_LIB="$TARGET_DIR/.patch_lib.new.$$"
OLD_LIB="$TARGET_DIR/.patch_lib.old.$$"

rm -rf -- "$NEW_LIB" "$OLD_LIB"

info "Chuẩn bị _patch_lib mới..."

cp -a \
    "$SOURCE_DIR/_patch_lib" \
    "$NEW_LIB"

[[ -f "$NEW_LIB/VERSION" ]] ||
    die "Copy _patch_lib staging thất bại."

# ------------------------------------------------------------
# Đổi _patch_lib theo kiểu gần-atomic.
#
# Bản cũ được giữ tạm để rollback nếu mv bản mới thất bại.
# ------------------------------------------------------------

info "Cập nhật _patch_lib/..."

if [[ -e "$TARGET_DIR/_patch_lib" || -L "$TARGET_DIR/_patch_lib" ]]; then

    mv \
        "$TARGET_DIR/_patch_lib" \
        "$OLD_LIB"
fi

if ! mv "$NEW_LIB" "$TARGET_DIR/_patch_lib"; then

    echo "[ERROR] Không thể activate _patch_lib mới." >&2

    rm -rf -- "$TARGET_DIR/_patch_lib"

    if [[ -e "$OLD_LIB" ]]; then
        mv \
            "$OLD_LIB" \
            "$TARGET_DIR/_patch_lib"

        echo "[ROLLBACK] Đã phục hồi _patch_lib cũ." >&2
    fi

    exit 1
fi

rm -rf -- "$OLD_LIB"

# ------------------------------------------------------------
# Copy các tracked file còn lại.
#
# _patch_lib/* bỏ qua vì đã thay nguyên directory phía trên.
#
# Với file root:
#
#   run_python_patches.sh
#   run_python_patches.ps1
#   README.md
#   implementing.md
#   ...
#
# sẽ overwrite.
#
# Các file/folder KHÔNG thuộc repo:
#
#   patchs/
#   patched/
#   artifacts/
#   logs/
#   .python_patch_tool.json
#   source code của project
#   .git/
#
# không bị xóa.
# ------------------------------------------------------------

info "Cập nhật launcher, docs và các file public..."

for rel in "${TRACKED_FILES[@]}"; do

    case "$rel" in
        _patch_lib/*)
            continue
            ;;
    esac

    src="$SOURCE_DIR/$rel"
    dst="$TARGET_DIR/$rel"

    # Safety: path từ git ls-files không được escape target.
    case "$rel" in
        /*|../*|*/../*|*/..)
            die "Repository chứa path không an toàn: $rel"
            ;;
    esac

    mkdir -p -- "$(dirname -- "$dst")"

    # Nếu destination là directory nhưng source là file,
    # không tự ý rm một directory lạ.
    if [[ -d "$dst" && ! -L "$dst" ]]; then
        die "Không overwrite directory bằng file: $dst"
    fi

    if [[ -L "$src" ]]; then

        rm -f -- "$dst"
        cp -a -- "$src" "$dst"

    elif [[ -f "$src" ]]; then

        # temp file cùng directory để rename atomic
        tmp_dst="${dst}.patchtool-new.$$"

        rm -f -- "$tmp_dst"

        cp -p -- "$src" "$tmp_dst"

        mv -f -- "$tmp_dst" "$dst"

    else
        die "Tracked path không phải regular file/symlink: $rel"
    fi
done

# ------------------------------------------------------------
# Đảm bảo shell launcher executable.
# Git clone/cp thường giữ mode, nhưng enforce lại cho chắc.
# ------------------------------------------------------------

if [[ -f "$TARGET_DIR/run_python_patches.sh" ]]; then
    chmod +x "$TARGET_DIR/run_python_patches.sh"
fi

# ------------------------------------------------------------
# Verify tất cả tracked files.
# ------------------------------------------------------------

info "Kiểm tra kết quả..."

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
            echo "[VERIFY FAILED] $rel: symlink khác nhau"
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
    die "Một hoặc nhiều file không khớp source GitHub."
fi

INSTALLED_VERSION="$(
    head -n 1 "$TARGET_DIR/_patch_lib/VERSION" \
    | tr -d '\r\n'
)"

# ------------------------------------------------------------
# Thành công
# ------------------------------------------------------------

echo
echo "============================================================"
echo " UPDATE SUCCESS"
echo "============================================================"
echo
echo "Previous version : $CURRENT_VERSION"
echo "Installed version: $INSTALLED_VERSION"
echo "Branch           : $BRANCH"
echo "Commit           : $REMOTE_COMMIT"
echo "Target           : $TARGET_DIR"
echo
echo "Launcher:"
echo
echo "  $TARGET_DIR/run_python_patches.sh"
echo
