#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import select
import subprocess
import sys
import tarfile
import stat
import unicodedata
import zipfile
from dataclasses import dataclass
from pathlib import Path

try:
    import termios
    import tty
except Exception:
    termios = tty = None

try:
    import fcntl
except Exception:
    fcntl = None

VERSION = "6.9.3"
MAX_COLLECT_REQUEST_JSON_BYTES = 1024 * 1024
MAX_PATCH_MARKER_BYTES = 1024 * 1024
MAX_PATCH_MARKER_FILES = 8
COLLECT_JSON_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
PATCH_PY_RE = re.compile(r"^patch_.*\.py$", re.I)
PATCH_MARKERS = (b"python_patch_utils", b"run_patch", b"PATCH_NAME")
_ANSI_RE = re.compile(
    r"(?:\x1B\][^\x07]*(?:\x07|\x1B\\))"
    r"|(?:\x1B\[[0-?]*[ -/]*[@-~])"
    r"|(?:\x1B[@-_])"
)


@dataclass(frozen=True)
class QueueItem:
    name: str
    kind: str
    detail: str = ""


@dataclass(frozen=True)
class LocalDuplicate:
    item: QueueItem
    history_name: str
    sha256: str


def _safe_display(value: str) -> str:
    value = _ANSI_RE.sub("", str(value))
    out: list[str] = []
    for ch in value:
        if ch in "\r\n\t":
            out.append(" ")
            continue
        if unicodedata.category(ch) == "Cc":
            continue
        out.append(ch)
    return "".join(out)


def natural_name_key(value: str):
    return tuple(int(x) if x.isdigit() else x for x in re.split(r"(\d+)", value.lower()))



def _zip_has_root_patch_manifest(path: Path) -> bool:
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False
    try:
        with zipfile.ZipFile(path) as zf:
            return "PATCH_TOOL_MANIFEST.json" in {n for n in zf.namelist() if not n.endswith("/")}
    except Exception:
        return False



def _archive_nonrunnable_reason(names: list[str]) -> str | None:
    """Return a structural support/distribution reason for archive members.

    Supports both root archives and one-or-more-folder wrappers by matching
    marker paths relative to their common prefix.
    """
    normalized = [n.replace("\\", "/").strip("/") for n in names]

    # Tool distribution: launcher and _patch_lib under the same prefix.
    launcher_suffix = "tools/run_python_patches.sh"
    for name in normalized:
        if name == launcher_suffix:
            prefix = ""
        elif name.endswith("/" + launcher_suffix):
            prefix = name[: -len(launcher_suffix)]
        else:
            continue
        lib_prefix = prefix + "tools/_patch_lib/"
        if any(n.startswith(lib_prefix) for n in normalized):
            return "tool_distribution"

    # Handoff: the canonical marker pair shares the same archive parent.
    handoff_parents = {
        str(Path(n).parent).replace("\\", "/")
        for n in normalized
        if Path(n).name == "HANDOFF_README.md"
    }
    state_parents = {
        str(Path(n).parent).replace("\\", "/")
        for n in normalized
        if Path(n).name == "CURRENT_STATE.md"
    }
    if handoff_parents & state_parents:
        return "handoff_archive"
    return None


def _zip_known_nonrunnable(path: Path) -> tuple[bool, str]:
    """Recognize support/distribution ZIPs before COLLECT routing.

    A HANDOFF may legitimately preserve the original CODE_COLLECTION_REQUEST
    JSON as evidence. That must never make the handoff itself runnable as a
    new readonly collection request. Root PATCH manifest precedence is handled
    by discover_queue() before this helper.
    """
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False, ""
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
        reason = _archive_nonrunnable_reason(names)
        return (reason is not None, reason or "")
    except Exception:
        # Invalid ZIP handling remains with the normal classifier below.
        return False, ""

def inspect_collect_zip(path: Path):
    if path.suffix.lower() != ".zip" or not path.is_file():
        return False, ""
    try:
        with zipfile.ZipFile(path) as zf:
            req = [
                n
                for n in zf.namelist()
                if not n.endswith("/") and COLLECT_JSON_RE.match(Path(n).name)
            ]
            if len(req) != 1:
                return False, f"request_json_count={len(req)}"
            info = zf.getinfo(req[0])
            if info.file_size > MAX_COLLECT_REQUEST_JSON_BYTES:
                return False, f"request_too_large={info.file_size}"
            try:
                raw = zf.read(req[0])
                data = json.loads(raw.decode("utf-8"))
            except Exception as exc:
                return False, f"invalid_request_json:{type(exc).__name__}"
            if (
                not isinstance(data, dict)
                or not isinstance(data.get("actions"), list)
                or not data["actions"]
            ):
                return False, "invalid_request"
            return True, f"id={data.get('id', 'collect')} actions={len(data['actions'])}"
    except Exception as exc:
        return False, f"invalid_zip:{type(exc).__name__}"


def _has_patch_markers(data: bytes) -> bool:
    return any(marker in data for marker in PATCH_MARKERS)


def _zip_is_patch(path: Path):
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
            # v5+ standard package: manifest must be at package root. Requiring
            # the root prevents a HANDOFF that merely embeds a patch tree from
            # being mistaken for the patch itself.
            if "PATCH_TOOL_MANIFEST.json" in names:
                return True, "manifest"
            # Known distribution/support bundles are not runnable patches.
            # Resolve these signatures before scanning Python text for legacy
            # helper markers because support evidence may mention PATCH_NAME.
            nonrunnable_reason = _archive_nonrunnable_reason(names)
            if nonrunnable_reason:
                return False, nonrunnable_reason

            py_names = [n for n in names if n.lower().endswith(".py")]
            if any(PATCH_PY_RE.match(Path(n).name) for n in py_names):
                return True, "legacy_patch_script"
            # Original v4 fallback: patch-named archive with Python payload.
            if path.name.lower().startswith("patch_") and py_names:
                return True, "legacy_patch_archive"
            # Legacy helper-marker scripts, bounded so queue discovery cannot
            # turn into an unbounded archive scan.
            total = 0
            scanned = 0
            for name in py_names:
                if scanned >= MAX_PATCH_MARKER_FILES or total >= MAX_PATCH_MARKER_BYTES:
                    break
                try:
                    info = zf.getinfo(name)
                    if info.file_size > MAX_PATCH_MARKER_BYTES:
                        continue
                    room = MAX_PATCH_MARKER_BYTES - total
                    data = zf.read(name)[:room]
                except Exception:
                    continue
                total += len(data)
                scanned += 1
                if _has_patch_markers(data):
                    return True, "legacy_helper_marker"
            return False, "no_patch_signature"
    except Exception as exc:
        return False, f"invalid_zip:{type(exc).__name__}"


def _tar_is_patch(path: Path):
    try:
        with tarfile.open(path, "r:*") as tf:
            members = [m for m in tf.getmembers() if m.isfile()]
            names = [m.name for m in members]
            if "PATCH_TOOL_MANIFEST.json" in names:
                return True, "manifest"
            nonrunnable_reason = _archive_nonrunnable_reason(names)
            if nonrunnable_reason:
                return False, nonrunnable_reason
            py_members = [m for m in members if m.name.lower().endswith(".py")]
            if any(PATCH_PY_RE.match(Path(m.name).name) for m in py_members):
                return True, "legacy_patch_script"
            if path.name.lower().startswith("patch_") and py_members:
                return True, "legacy_patch_archive"
            total = 0
            scanned = 0
            for member in py_members:
                if scanned >= MAX_PATCH_MARKER_FILES or total >= MAX_PATCH_MARKER_BYTES:
                    break
                if member.size > MAX_PATCH_MARKER_BYTES:
                    continue
                try:
                    fh = tf.extractfile(member)
                    if fh is None:
                        continue
                    room = MAX_PATCH_MARKER_BYTES - total
                    data = fh.read(room)
                except Exception:
                    continue
                total += len(data)
                scanned += 1
                if _has_patch_markers(data):
                    return True, "legacy_helper_marker"
            return False, "no_patch_signature"
    except Exception as exc:
        return False, f"invalid_tar:{type(exc).__name__}"


def inspect_patch_candidate(path: Path):
    low = path.name.lower()
    if low.endswith(".zip"):
        return _zip_is_patch(path)
    if low.endswith((".tar.gz", ".tgz")):
        return _tar_is_patch(path)
    if low.endswith(".py"):
        if PATCH_PY_RE.match(path.name):
            return True, "legacy_patch_script"
        try:
            if path.stat().st_size > MAX_PATCH_MARKER_BYTES:
                return False, "standalone_python_too_large"
            data = path.read_bytes()
        except Exception as exc:
            return False, f"unreadable_python:{type(exc).__name__}"
        return (_has_patch_markers(data), "legacy_helper_marker" if _has_patch_markers(data) else "no_patch_signature")
    return False, "unsupported_extension"


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _split_local_duplicate_patches(root: Path, items: list[QueueItem]):
    """Split runnable items from PATCHes already present in local PASS history.

    Duplicate history is deliberately local-only: direct regular files under
    ``<project>/patchs/patched``.  No project key, network state, shared cache,
    Git history, or machine-external database participates.  Exact content
    SHA-256 is the authority, so a renamed copy is still a duplicate while a
    same-named package with different bytes remains runnable.

    Duplicate queue files are left untouched in ``patchs/``.  This mirrors an
    unselected item: the tool skips execution but does not delete or archive
    user input merely because a local historical copy exists.
    """
    queue_dir = root / "patchs"
    history_dir = queue_dir / "patched"

    # LOCAL means physically inside this resolved project tree.  Never follow
    # a symlinked patchs/ or patchs/patched/ into another project/shared cache:
    # doing so would make a patch executed elsewhere suppress this project.
    try:
        if queue_dir.is_symlink():
            return list(items), [], [
                "local duplicate check disabled: patchs/ is a symlink"
            ]
        if not queue_dir.is_dir():
            return list(items), [], []
        if history_dir.is_symlink():
            return list(items), [], [
                "local duplicate check disabled: patchs/patched/ is a symlink"
            ]
        if not history_dir.is_dir():
            return list(items), [], []
        resolved_root = root.resolve(strict=True)
        resolved_queue = queue_dir.resolve(strict=True)
        resolved_history = history_dir.resolve(strict=True)
        if resolved_queue.parent != resolved_root or resolved_history.parent != resolved_queue:
            return list(items), [], [
                "local duplicate check disabled: history escapes project-local patchs/"
            ]
    except OSError as exc:
        return list(items), [], [
            f"local duplicate history unavailable: patchs/patched ({type(exc).__name__})"
        ]

    by_size: dict[int, list[Path]] = {}
    warnings: list[str] = []
    try:
        history_entries = list(history_dir.iterdir())
    except OSError as exc:
        return list(items), [], [
            f"local duplicate history unavailable: patchs/patched ({type(exc).__name__})"
        ]

    for path in history_entries:
        try:
            if path.is_symlink() or not path.is_file():
                continue
            size = path.stat().st_size
        except OSError:
            continue
        by_size.setdefault(size, []).append(path)
    for paths in by_size.values():
        paths.sort(key=lambda x: natural_name_key(x.name))

    history_hash_cache: dict[Path, str | None] = {}
    runnable: list[QueueItem] = []
    duplicates: list[LocalDuplicate] = []

    for item in items:
        if item.kind != "PATCH":
            runnable.append(item)
            continue
        queued = root / "patchs" / item.name
        try:
            if queued.is_symlink() or not queued.is_file():
                runnable.append(item)
                continue
            size = queued.stat().st_size
        except OSError as exc:
            warnings.append(
                f"local duplicate check skipped for patchs/{item.name} ({type(exc).__name__})"
            )
            runnable.append(item)
            continue

        candidates = list(by_size.get(size, ()))
        if not candidates:
            runnable.append(item)
            continue
        # Prefer a same-name historical file in diagnostics, then natural name.
        candidates.sort(key=lambda x: (x.name != item.name, natural_name_key(x.name)))
        try:
            queued_hash = _sha256_file(queued)
        except OSError as exc:
            warnings.append(
                f"local duplicate check skipped for patchs/{item.name} ({type(exc).__name__})"
            )
            runnable.append(item)
            continue

        match: Path | None = None
        for historical in candidates:
            if historical not in history_hash_cache:
                try:
                    history_hash_cache[historical] = _sha256_file(historical)
                except OSError:
                    history_hash_cache[historical] = None
            if history_hash_cache[historical] == queued_hash:
                match = historical
                break
        if match is None:
            runnable.append(item)
            continue
        duplicates.append(LocalDuplicate(item, match.name, queued_hash))

    return runnable, duplicates, warnings


def _print_local_duplicate_skips(duplicates: list[LocalDuplicate], *, stream=None) -> None:
    if not duplicates:
        return
    out = stream or sys.stdout
    print("PATCHES SKIPPED / NOT EXECUTED:", file=out)
    for index, duplicate in enumerate(duplicates, 1):
        print(
            f"{index}. [SKIPPED:DUPLICATE_LOCAL] {_safe_display(duplicate.item.name)}",
            file=out,
        )
        print(
            f"   Local match: patchs/patched/{_safe_display(duplicate.history_name)}",
            file=out,
        )


def discover_queue(root: Path):
    directory = root / "patchs"
    directory.mkdir(parents=True, exist_ok=True)
    items: list[QueueItem] = []
    warnings: list[str] = []

    for path in directory.iterdir():
        try:
            if path.is_symlink():
                warnings.append(f"SKIPPED symlink queue entry: patchs/{path.name}")
                continue
            if not path.is_file():
                continue
        except OSError as exc:
            warnings.append(f"SKIPPED unreadable entry: patchs/{path.name} ({type(exc).__name__})")
            continue

        low = path.name.lower()
        if path.suffix.lower() == ".json" and COLLECT_JSON_RE.match(path.name):
            warnings.append(f"RAW JSON REJECTED: patchs/{path.name}")
            continue

        # A root PATCH manifest is the strongest package signature. A PATCH may
        # legitimately carry a collection request as a resource; do not route
        # that resource ZIP into the readonly COLLECT path.
        if path.suffix.lower() == ".zip" and _zip_has_root_patch_manifest(path):
            items.append(QueueItem(path.name, "PATCH", "manifest"))
            continue

        if path.suffix.lower() == ".zip":
            nonrunnable, reason = _zip_known_nonrunnable(path)
            if nonrunnable:
                warnings.append(
                    f"SKIPPED non-patch candidate: patchs/{path.name} ({reason})"
                )
                continue

        ok, detail = inspect_collect_zip(path)
        if ok:
            items.append(QueueItem(path.name, "COLLECT", detail))
            continue

        collect_invalid = (
            path.suffix.lower() == ".zip"
            and (
                low.startswith("code_collection_request")
                or detail == "invalid_request"
                or detail.startswith("invalid_request_json:")
                or detail.startswith("request_too_large=")
                or (
                    detail.startswith("request_json_count=")
                    and detail != "request_json_count=0"
                )
            )
        )
        if collect_invalid:
            items.append(QueueItem(path.name, "COLLECT INVALID", detail))
            continue

        supported = low.endswith((".zip", ".py", ".tar.gz", ".tgz"))
        if not supported:
            continue
        is_patch, patch_detail = inspect_patch_candidate(path)
        if is_patch:
            items.append(QueueItem(path.name, "PATCH", patch_detail))
        else:
            warnings.append(
                f"SKIPPED non-patch candidate: patchs/{path.name} ({patch_detail})"
            )

    items.sort(key=lambda x: natural_name_key(x.name))
    return items, warnings


def _read_key(fd):
    raw = os.read(fd, 1)
    if raw in {b"\r", b"\n"}:
        return "ENTER"
    if raw == b" ":
        return "SPACE"
    if raw == b"\x1b":
        seq = b""
        for _ in range(2):
            ready, _, _ = select.select([fd], [], [], 0.035)
            if ready:
                seq += os.read(fd, 1)
        return {b"[A": "UP", b"[B": "DOWN"}.get(seq, "ESC")
    return raw.decode(errors="ignore").lower()


def _selector_term_size() -> tuple[int, int]:
    """Return live terminal (columns, rows) for fullscreen selector redraw.

    A direct TTY query wins over COLUMNS/LINES because environment values can
    become stale after resize. Width protects against physical line wrapping;
    height protects against a frame scrolling beyond the visible screen, which
    would make cursor-up redraw accounting incorrect for long queues.
    """
    try:
        fd = sys.stdout.fileno()
        size = os.get_terminal_size(fd)
        if size.columns > 0 and size.lines > 0:
            return size.columns, size.lines
    except Exception:
        pass

    def _positive_env(name: str, fallback: int) -> int:
        raw = os.environ.get(name)
        if raw:
            try:
                value = int(raw)
                if value > 0:
                    return value
            except (TypeError, ValueError):
                pass
        return fallback

    return _positive_env("COLUMNS", 120), _positive_env("LINES", 24)


def _selector_term_width() -> int:
    return _selector_term_size()[0]


def _selector_term_height() -> int:
    return _selector_term_size()[1]


def _selector_viewport(count: int, cursor: int, capacity: int) -> tuple[int, int]:
    """Return a stable visible item slice that always contains cursor.

    Keeping the frame at most terminal-height minus one physical row prevents
    scrolling from invalidating the next ``CSI n F`` redraw.  The slice is
    centered when practical and remains stable at the beginning/end.
    """
    if count <= 0:
        return 0, 0
    capacity = max(1, min(int(capacity), count))
    cursor = min(max(int(cursor), 0), count - 1)
    start = cursor - capacity // 2
    start = max(0, min(start, count - capacity))
    return start, start + capacity


def _display_cell_width(text: str) -> int:
    width = 0
    for ch in _safe_display(text):
        if unicodedata.combining(ch):
            continue
        width += 2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1
    return width


def _clip_selector_line(text: str, terminal_width: int | None = None) -> str:
    """Clip one fullscreen row so cursor-up redraw accounting stays exact."""
    clean = _safe_display(text)
    cols = _selector_term_width() if terminal_width is None else int(terminal_width)
    limit = max(1, cols - 2)
    if _display_cell_width(clean) <= limit:
        return clean
    if limit == 1:
        return "…"
    out: list[str] = []
    used = 0
    target = limit - 1
    for ch in clean:
        w = 0 if unicodedata.combining(ch) else (2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1)
        if used + w > target:
            break
        out.append(ch)
        used += w
    return "".join(out) + "…"


def _selection_mark(index: int, selected: set[int], priorities: dict[int, int]) -> str:
    if index not in selected:
        return " "
    if index in priorities:
        return str(priorities[index])
    return "x"


def _ordered_selection(items: list[QueueItem], selected: set[int], priorities: dict[int, int]):
    """Return selected items in requested execution order.

    Explicit 0..9 priorities run first from low to high. Items selected with
    plain [x] have no explicit priority and run afterwards in the tool's
    existing natural queue order. Stable original indexes break ties so equal
    priorities never disturb the queue ordering already shown to the user.
    """
    indexes = sorted(
        selected,
        key=lambda i: (priorities.get(i, 10), i),
    )
    return [items[i] for i in indexes]


def _render(items, cursor, selected, priorities, msg, prev):
    terminal_width, terminal_height = _selector_term_size()

    # Keep one physical row free below the frame.  Writing a frame as tall as
    # the terminal can itself trigger a scroll on the final newline, after
    # which cursor-up no longer returns to the frame's true first row.
    frame_budget = max(1, terminal_height - 1)
    full_footer = [
        "",
        "Space: chọn/bỏ [x] | 0-9: gán ưu tiên | ↑/↓: di chuyển",
        "a: tất cả [x] | n: bỏ tất cả | d: xóa item tại con trỏ",
        "Enter: xác nhận | q/Esc: hủy | Số nhỏ chạy trước; cùng số giữ thứ tự hiện tại",
        _safe_display(msg) if msg else "",
    ]
    compact_help = "Space/[0-9]/↑↓ | Enter chạy | q hủy"

    # Scale the fixed UI down before sacrificing the cursor row. Even an
    # extremely short terminal must show the current item and must never write
    # more physical rows than terminal_height-1.
    if frame_budget >= 10:
        header_rows = 2
        footer = full_footer
    elif frame_budget >= 5:
        header_rows = 1
        footer = [compact_help, _safe_display(msg) if msg else ""]
    elif frame_budget == 4:
        header_rows = 1
        footer = [_safe_display(msg) if msg else compact_help]
    elif frame_budget >= 2:
        header_rows = 1
        footer = []
    else:
        header_rows = 0
        footer = []

    item_capacity = max(1, frame_budget - header_rows - len(footer))
    start, end = _selector_viewport(len(items), cursor, item_capacity)

    if header_rows == 2:
        if len(items) > item_capacity:
            header = [f"CHỌN CÔNG VIỆC SẼ CHẠY  [{start + 1}-{end}/{len(items)}]", ""]
        else:
            header = ["CHỌN CÔNG VIỆC SẼ CHẠY", ""]
    elif header_rows == 1:
        header = [f"CHỌN CÔNG VIỆC SẼ CHẠY [{start + 1}-{end}/{len(items)}]"]
    else:
        header = []

    lines = list(header)
    for i in range(start, end):
        item = items[i]
        detail = f"  [{_safe_display(item.detail)}]" if item.detail else ""
        lines.append(
            f"{'›' if i == cursor else ' '} "
            f"[{_selection_mark(i, selected, priorities)}] {i + 1:>3}. "
            f"[{_safe_display(item.kind)}] {_safe_display(item.name)}{detail}"
        )
    lines.extend(footer)

    # Clip horizontally after choosing the vertical viewport: every logical row
    # is exactly one physical row, and the full frame never exceeds the live
    # terminal height. Both dimensions are re-read on every redraw/resize.
    lines = [_clip_selector_line(line, terminal_width) for line in lines[:frame_budget]]
    frame_height = max(prev, len(lines))
    # If the terminal shrank vertically, never attempt to cursor-up more rows
    # than are now physically addressable. Clearing the visible budget is safer
    # than letting an old oversized frame corrupt the new viewport.
    cursor_up = min(prev, max(0, frame_budget))
    if cursor_up:
        sys.stdout.write(f"\x1b[{cursor_up}F")
    padded = lines + [""] * (max(len(lines), min(frame_height, frame_budget)) - len(lines))
    for line in padded:
        sys.stdout.write("\r\x1b[2K" + line + "\n")
    sys.stdout.flush()
    return len(padded)


def _readline_or_interrupt():
    try:
        return sys.stdin.readline(), False
    except KeyboardInterrupt:
        return "", True


def _parse_index_spec(spec: str, count: int):
    """Parse the documented line-selector grammar into zero-based indexes."""
    text = spec.strip().lower()
    if not text:
        return set()
    result: set[int] = set()
    for token in re.split(r"[\s,]+", text):
        if not token:
            continue
        if re.fullmatch(r"\d+", token):
            value = int(token)
            if not 1 <= value <= count:
                raise ValueError(f"index out of range: {value}")
            result.add(value - 1)
            continue
        match = re.fullmatch(r"(\d+)-(\d+)", token)
        if match:
            first, last = (int(match.group(1)), int(match.group(2)))
            if first > last:
                first, last = last, first
            if first < 1 or last > count:
                raise ValueError(f"range out of bounds: {token}")
            result.update(range(first - 1, last))
            continue
        raise ValueError(f"invalid selector token: {token}")
    return result


def _load_zero_argument_config(root: Path):
    """Read only the previously documented zero-argument selection settings.

    Invalid or stale config must never turn into implicit execution; callers
    receive safe prompt defaults plus warnings.
    """
    cfg = {
        "selection": "prompt",
        "non_interactive_confirmed": False,
        "initial_selection": "none",
        "selector_ui": "auto",
    }
    warnings: list[str] = []
    path = root / ".python_patch_tool.json"
    if not path.is_file():
        return cfg, warnings
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        node = data.get("automation", {}).get("zero_argument", {})
    except Exception as exc:
        warnings.append(f"invalid .python_patch_tool.json; using prompt defaults ({type(exc).__name__})")
        return cfg, warnings
    if not isinstance(node, dict):
        warnings.append("automation.zero_argument is not an object; using prompt defaults")
        return cfg, warnings

    selection = str(node.get("selection", cfg["selection"])).lower()
    if selection in {"prompt", "all", "first", "newest"}:
        cfg["selection"] = selection
    else:
        warnings.append(f"unsupported zero_argument.selection={selection!r}; using prompt")

    cfg["non_interactive_confirmed"] = node.get("non_interactive_confirmed") is True

    initial = str(node.get("initial_selection", cfg["initial_selection"])).lower()
    if initial in {"none", "all"}:
        cfg["initial_selection"] = initial
    else:
        warnings.append(f"unsupported initial_selection={initial!r}; using none")

    selector_ui = str(node.get("selector_ui", cfg["selector_ui"])).lower()
    if selector_ui in {"auto", "line"}:
        cfg["selector_ui"] = selector_ui
    else:
        warnings.append(f"unsupported selector_ui={selector_ui!r}; using auto")
    return cfg, warnings


def _initial_selected(items, initial_selection: str):
    if len(items) == 1:
        return {0}
    if initial_selection == "all":
        return set(range(len(items)))
    return set()


def _delete_indexes(
    root: Path,
    items: list[QueueItem],
    selected: set[int],
    indexes: set[int],
    priorities: dict[int, int] | None = None,
):
    """Delete queue files while preserving selection/priority index mapping."""
    priorities = dict(priorities or {})
    deleted: list[str] = []
    failures: list[str] = []
    for i in sorted(indexes, reverse=True):
        victim = items[i]
        target = root / "patchs" / victim.name
        try:
            # Queue discovery rejects symlinks, and unlink never follows one.
            target.unlink()
        except FileNotFoundError:
            # An external process already removed it: treat it as no longer queued.
            pass
        except OSError as exc:
            failures.append(f"{victim.name}: {type(exc).__name__}")
            continue
        deleted.append(victim.name)
        items.pop(i)
        selected = {j if j < i else j - 1 for j in selected if j != i}
        priorities = {
            (j if j < i else j - 1): value
            for j, value in priorities.items()
            if j != i
        }
    if len(items) == 1:
        selected = {0}
        # Auto-selected sole item follows normal tool order unless the surviving
        # item already had an explicit priority.
    priorities = {j: value for j, value in priorities.items() if j in selected}
    return selected, priorities, list(reversed(deleted)), list(reversed(failures))


def _select_items_line(root: Path, items: list[QueueItem], initial_selection: str):
    selected = _initial_selected(items, initial_selection)
    while items:
        print("CHỌN CÔNG VIỆC SẼ CHẠY")
        for i, item in enumerate(items, 1):
            mark = "x" if i - 1 in selected else " "
            print(f"  [{mark}] {i}. [{_safe_display(item.kind)}] {_safe_display(item.name)}")
        print("Nhập: 1,3-5 | a=all | n=none | d <số/range>=xóa | q=quit | Enter=xác nhận")
        raw_line, interrupted = _readline_or_interrupt()
        if interrupted:
            print("\nCancelled by Ctrl+C.")
            raise KeyboardInterrupt
        if raw_line == "":
            return None
        raw = raw_line.strip().lower()
        if raw in {"q", "quit"}:
            return None
        if raw in {"a", "all"}:
            return list(items)
        if raw in {"n", "none"}:
            selected.clear()
            continue
        if raw.startswith("d "):
            try:
                indexes = _parse_index_spec(raw[2:].strip(), len(items))
            except ValueError as exc:
                print(f"Lựa chọn xóa không hợp lệ: {_safe_display(str(exc))}")
                continue
            if not indexes:
                print("Chưa chọn item để xóa.")
                continue
            names = ", ".join(_safe_display(items[i].name) for i in sorted(indexes))
            sys.stdout.write(f"Xóa vĩnh viễn {names}? [y/N]: ")
            sys.stdout.flush()
            confirm, interrupted = _readline_or_interrupt()
            if interrupted:
                print("\nCancelled by Ctrl+C.")
                raise KeyboardInterrupt
            if confirm == "" or confirm.strip().lower() != "y":
                print("Xóa đã hủy.")
                continue
            selected, _priorities, deleted, failures = _delete_indexes(root, items, selected, indexes)
            for name in deleted:
                print(f"DELETED: patchs/{_safe_display(name)}")
            for detail in failures:
                print(f"DELETE FAILED: {_safe_display(detail)}", file=sys.stderr)
            continue
        if raw == "":
            if selected:
                return [items[i] for i in sorted(selected)]
            print("Chưa chọn item nào.")
            continue
        try:
            selected = _parse_index_spec(raw, len(items))
        except ValueError as exc:
            print(f"Lựa chọn không hợp lệ: {_safe_display(str(exc))}")
            continue
        if not selected:
            print("Chưa chọn item nào.")
            continue
        # Matching the historical line selector: a concrete number/range entry
        # is itself the confirmation for that selection.
        return [items[i] for i in sorted(selected)]
    return []


def select_items(root, items, *, initial_selection="none", selector_ui="auto"):
    if not items:
        return []
    # Full-screen controls are safe only when both sides are attached to a TTY.
    use_tty = (
        selector_ui != "line"
        and termios is not None
        and tty is not None
        and sys.stdin.isatty()
        and sys.stdout.isatty()
    )
    if not use_tty:
        return _select_items_line(root, items, initial_selection)

    fd = sys.stdin.fileno()
    old = termios.tcgetattr(fd)
    tty.setcbreak(fd)
    cursor = 0
    selected = _initial_selected(items, initial_selection)
    priorities: dict[int, int] = {}
    rendered = 0
    if len(items) == 1:
        msg = "Một item duy nhất đã chọn sẵn; Enter để chạy."
    elif selected:
        msg = "Các item đã được chọn theo cấu hình; Enter để chạy."
    else:
        msg = ""
    delete = False
    try:
        while items:
            rendered = _render(items, cursor, selected, priorities, msg, rendered)
            msg = ""
            key = _read_key(fd)
            if key == "\x03":
                raise KeyboardInterrupt
            if key in {"q", "ESC"}:
                return None
            if delete:
                delete = False
                if key != "y":
                    msg = "Xóa đã hủy."
                    continue
                victim = items[cursor]
                selected, priorities, deleted, failures = _delete_indexes(
                    root, items, selected, {cursor}, priorities
                )
                if failures:
                    msg = f"Xóa thất bại: {failures[0]}"
                elif deleted and len(items) == 1:
                    msg = "Còn một item; đã chọn sẵn. Enter để chạy."
                elif deleted and not items:
                    msg = "Queue đã trống sau khi xóa."
                elif deleted:
                    msg = f"Đã xóa {deleted[0]}."
                cursor = min(cursor, max(0, len(items) - 1))
                continue
            if key == "UP":
                cursor = (cursor - 1) % len(items)
            elif key == "DOWN":
                cursor = (cursor + 1) % len(items)
            elif key == "SPACE":
                if cursor in priorities:
                    # A numbered selection becomes a normal [x] selection first;
                    # a second Space can then deselect it. This makes Space the
                    # explicit way to return a row to default tool ordering.
                    priorities.pop(cursor, None)
                    selected.add(cursor)
                elif cursor in selected:
                    selected.remove(cursor)
                else:
                    selected.add(cursor)
            elif key in {str(i) for i in range(10)}:
                selected.add(cursor)
                priorities[cursor] = int(key)
                msg = f"Ưu tiên {key}: {_safe_display(items[cursor].name)}"
            elif key == "a":
                selected = set(range(len(items)))
                priorities.clear()
            elif key == "n":
                selected.clear()
                priorities.clear()
            elif key == "d":
                delete = True
                msg = f"Xóa {_safe_display(items[cursor].name)}? y để xác nhận"
            elif key == "ENTER":
                if selected:
                    return _ordered_selection(items, selected, priorities)
                msg = "Chưa chọn item nào."
    finally:
        try:
            termios.tcsetattr(fd, termios.TCSADRAIN, old)
        except Exception:
            pass
        sys.stdout.write("\r\x1b[2K")
        sys.stdout.flush()
    return []


def _configured_auto_selection(root: Path, items: list[QueueItem], cfg: dict):
    mode = cfg.get("selection", "prompt")
    if mode == "prompt":
        return None
    if not cfg.get("non_interactive_confirmed", False):
        print(
            f"[PTV v{VERSION} WARNING] zero_argument.selection={mode!r} is not "
            "confirmed; falling back to prompt"
        )
        return None
    # The documented automation predates unified COLLECT routing. Do not make
    # a pre-existing PATCH automation setting start COLLECT jobs implicitly.
    if any(item.kind != "PATCH" for item in items):
        print(
            f"[PTV v{VERSION} WARNING] automatic selection is limited to a "
            "PATCH-only queue; mixed PATCH/COLLECT queue requires confirmation"
        )
        return None
    if mode == "all":
        return list(items)
    if mode == "first":
        return [items[0]]
    if mode == "newest":
        try:
            newest = max(items, key=lambda item: (root / "patchs" / item.name).stat().st_mtime_ns)
        except OSError as exc:
            print(
                f"[PTV v{VERSION} WARNING] newest selection failed ({type(exc).__name__}); "
                "falling back to prompt"
            )
            return None
        return [newest]
    return None


def _normalize_subprocess_rc(rc: int) -> int:
    """Map signal-style negative subprocess return codes to shell convention."""
    value = int(rc)
    return 128 + abs(value) if value < 0 else value


def _collect_archive_postcondition(root: Path, item: QueueItem) -> tuple[bool, str]:
    """Verify the established COLLECT PASS queue lifecycle.

    A successful readonly collection must move its request ZIP from patchs/ to
    patchs/patched/.  Reporting PASS while the request remains runnable causes
    accidental repeated collections on the next zero-argument run.
    """
    source = root / "patchs" / item.name
    archived = root / "patchs" / "patched" / item.name
    if source.exists() or source.is_symlink():
        return False, f"COLLECT rc=0 but request is still queued: patchs/{item.name}"
    try:
        if archived.is_symlink() or not archived.is_file():
            return False, f"COLLECT rc=0 but archived request is missing: patchs/patched/{item.name}"
    except OSError as exc:
        return False, f"COLLECT archive verification failed: {type(exc).__name__}"
    return True, ""


def execute_items(root: Path, chosen: list[QueueItem]):
    """Execute in natural order, rechecking local duplicates immediately before PATCH launch."""
    executed: list[tuple[str, int]] = []
    late_duplicates: list[LocalDuplicate] = []
    duplicate_warnings: list[str] = []
    for index, item in enumerate(chosen):
        if item.kind == "PATCH":
            # Close the scan->execute race and catch duplicate packages selected
            # together: once an earlier identical PATCH is archived locally, a
            # later copy becomes a skip rather than a second execution.
            still_runnable, now_duplicates, now_warnings = _split_local_duplicate_patches(root, [item])
            for warning in now_warnings:
                if warning not in duplicate_warnings:
                    duplicate_warnings.append(warning)
            if now_duplicates:
                late_duplicates.extend(now_duplicates)
                continue
            if not still_runnable:
                # Defensive fail-safe: an item must never silently disappear.
                duplicate_warnings.append(
                    f"late duplicate check returned no decision for patchs/{item.name}; executing normally"
                )
            cmd = [str(root / "tools/run_python_patches.sh"), "--patch", f"patchs/{item.name}"]
        elif item.kind == "COLLECT":
            cmd = [str(root / "tools/run_python_patches.sh"), "collect", "request", f"patchs/{item.name}"]
        else:
            print(f"ERROR invalid collect package {_safe_display(item.name)}", file=sys.stderr)
            rc = 2
            executed.append((item.name, rc))
            remaining = chosen[index + 1 :]
            return rc, executed, remaining, late_duplicates, duplicate_warnings
        try:
            # Flush selector/status text before the child writes its own RUN
            # SUMMARY. This keeps captured/non-TTY consoles in chronological
            # order instead of letting Python buffering place the menu after
            # child output.
            try:
                sys.stdout.flush()
                sys.stderr.flush()
            except Exception:
                pass
            rc = _normalize_subprocess_rc(subprocess.run(cmd, cwd=root).returncode)
        except KeyboardInterrupt:
            # Ctrl+C is a normal operator stop, not an internal Python error.
            # The child receives the terminal signal too; report conventional
            # shell status 130 and stop the selected queue immediately.
            rc = 130
            print(f"[PTV v{VERSION}] INTERRUPTED by Ctrl+C", file=sys.stderr)
        if rc == 0 and item.kind == "COLLECT":
            ok, detail = _collect_archive_postcondition(root, item)
            if not ok:
                print(f"[PTV v{VERSION} ERROR] {_safe_display(detail)}", file=sys.stderr)
                rc = 3
        executed.append((item.name, rc))
        if rc:
            remaining = chosen[index + 1 :]
            return rc, executed, remaining, late_duplicates, duplicate_warnings
    return 0, executed, [], late_duplicates, duplicate_warnings

def _acquire_project_queue_lock(root: Path):
    """Acquire one exclusive zero-argument queue session per real project.

    The lock path itself is untrusted local filesystem state.  Never follow a
    symlinked ``patchs/`` directory or lock file, and never write/truncate the
    lock inode: a symlink/hardlink must not let queue startup modify a file
    outside the project.  ``flock`` ownership is kernel state, so PID text is
    unnecessary for correctness.
    """
    if fcntl is None:
        return None, "project queue locking is unavailable on this platform"

    queue_dir = root / "patchs"
    try:
        if queue_dir.exists() or queue_dir.is_symlink():
            if queue_dir.is_symlink():
                return None, "project queue lock refused: patchs/ is a symlink"
            if not queue_dir.is_dir():
                return None, "project queue lock refused: patchs/ is not a directory"
        else:
            queue_dir.mkdir(parents=True, exist_ok=True)

        dir_flags = os.O_RDONLY
        if hasattr(os, "O_DIRECTORY"):
            dir_flags |= os.O_DIRECTORY
        if hasattr(os, "O_CLOEXEC"):
            dir_flags |= os.O_CLOEXEC
        if hasattr(os, "O_NOFOLLOW"):
            dir_flags |= os.O_NOFOLLOW
        dir_fd = os.open(queue_dir, dir_flags)
    except OSError as exc:
        return None, f"project queue lock unavailable ({type(exc).__name__})"

    fd = None
    try:
        flags = os.O_RDONLY | os.O_CREAT
        if hasattr(os, "O_NONBLOCK"):
            flags |= os.O_NONBLOCK
        if hasattr(os, "O_CLOEXEC"):
            flags |= os.O_CLOEXEC
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        fd = os.open(".ptv_queue.lock", flags, 0o600, dir_fd=dir_fd)
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode):
            raise OSError("queue lock is not a regular file")
        # Multiple hardlinks would make the lock shared with another path and
        # violate project-local ownership even though no symlink is involved.
        if st.st_nlink != 1:
            raise OSError("queue lock has multiple hardlinks")
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        handle = os.fdopen(fd, "r", encoding="utf-8", closefd=True)
        fd = None
        return handle, None
    except BlockingIOError:
        return None, "another local Patch Tool queue session is already running"
    except OSError as exc:
        return None, f"project queue lock unavailable ({type(exc).__name__})"
    finally:
        if fd is not None:
            try:
                os.close(fd)
            except OSError:
                pass
        try:
            os.close(dir_fd)
        except OSError:
            pass

def _release_project_queue_lock(handle) -> None:
    if handle is None:
        return
    try:
        if fcntl is not None:
            fcntl.flock(handle.fileno(), fcntl.LOCK_UN)
    except Exception:
        pass
    try:
        handle.close()
    except Exception:
        pass


def _run_locked_queue(root: Path):
    items, warnings = discover_queue(root)
    items, local_duplicates, duplicate_warnings = _split_local_duplicate_patches(root, items)
    printed_warnings = set()
    for warning in [*warnings, *duplicate_warnings]:
        if warning in printed_warnings:
            continue
        printed_warnings.add(warning)
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    if not items:
        if local_duplicates:
            print("AUTO STATUS: IDLE — no new runnable package; local duplicate PATCHes were skipped.")
            _print_local_duplicate_skips(local_duplicates)
        else:
            print("AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.")
        return 0

    cfg, config_warnings = _load_zero_argument_config(root)
    for warning in config_warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")

    chosen = _configured_auto_selection(root, items, cfg)
    if chosen is None:
        try:
            chosen = select_items(
                root,
                list(items),
                initial_selection=cfg.get("initial_selection", "none"),
                selector_ui=cfg.get("selector_ui", "auto"),
            )
        except KeyboardInterrupt:
            print("\nCancelled by Ctrl+C.")
            return 130
    if chosen is None:
        print("Cancelled.")
        if local_duplicates:
            _print_local_duplicate_skips(local_duplicates)
        return 0
    if not chosen:
        print("AUTO STATUS: IDLE — queue is empty or no runnable item remains; nothing executed.")
        if local_duplicates:
            _print_local_duplicate_skips(local_duplicates)
        return 0

    rc, executed, remaining, late_duplicates, late_duplicate_warnings = execute_items(root, chosen)
    local_duplicates = [*local_duplicates, *late_duplicates]
    for warning in late_duplicate_warnings:
        if warning in printed_warnings:
            continue
        printed_warnings.add(warning)
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}", file=sys.stderr if rc else sys.stdout)
    if rc:
        failed_name = executed[-1][0]
        print(
            f"SUMMARY: FAIL | stopped after {_safe_display(failed_name)} rc={rc}",
            file=sys.stderr,
        )
        if remaining:
            print(f"SKIPPED / NOT EXECUTED: {len(remaining)} selected item(s)", file=sys.stderr)
            for item in remaining:
                print(f"  - {_safe_display(item.name)}", file=sys.stderr)
        if local_duplicates:
            _print_local_duplicate_skips(local_duplicates, stream=sys.stderr)
        return rc
    if local_duplicates:
        _print_local_duplicate_skips(local_duplicates)
    completed_count = len(executed)
    duplicate_count = len(local_duplicates)
    if duplicate_count:
        print(
            f"SUMMARY: PASS | {completed_count} item(s) completed | "
            f"{duplicate_count} item(s) skipped as local duplicate"
        )
    else:
        print(f"SUMMARY: PASS | {completed_count} item(s) completed")
    return 0

def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--project-root", required=True)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve()
    lock_handle, lock_error = _acquire_project_queue_lock(root)
    if lock_handle is None:
        detail = _safe_display(lock_error or "project queue lock unavailable")
        if detail.startswith("another local Patch Tool queue session"):
            print(
                f"[PTV v{VERSION} WARNING] BUSY: {detail}; nothing executed.",
                file=sys.stderr,
            )
        else:
            print(
                f"[PTV v{VERSION} ERROR] QUEUE LOCK: {detail}; nothing executed.",
                file=sys.stderr,
            )
        return getattr(os, "EX_TEMPFAIL", 75)
    try:
        return _run_locked_queue(root)
    finally:
        _release_project_queue_lock(lock_handle)


if __name__ == "__main__":
    raise SystemExit(main())
