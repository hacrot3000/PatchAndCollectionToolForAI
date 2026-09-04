#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import select
import subprocess
import sys
import tarfile
import unicodedata
import zipfile
from dataclasses import dataclass
from pathlib import Path

try:
    import termios
    import tty
except Exception:
    termios = tty = None

VERSION = "6.7.13"
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
    # Snapshot captured during discovery.  Selection can stay open for an
    # arbitrary amount of time; refuse to execute a same-named file that was
    # replaced or modified after the user saw/selected it.
    identity: tuple[int, int, int, int, int] | None = None


def _queue_identity(path: Path) -> tuple[int, int, int, int, int] | None:
    try:
        st = path.stat()
    except OSError:
        return None
    return (
        int(st.st_dev),
        int(st.st_ino),
        int(st.st_size),
        int(st.st_mtime_ns),
        int(st.st_ctime_ns),
    )


def _queue_item(path: Path, kind: str, detail: str = "") -> QueueItem:
    return QueueItem(path.name, kind, detail, _queue_identity(path))


def _safe_display(value: str) -> str:
    value = _ANSI_RE.sub("", str(value))
    out: list[str] = []
    for ch in value:
        if ch in "\r\n\t":
            out.append(" ")
            continue
        category = unicodedata.category(ch)
        if category in {"Cc", "Cf"}:
            continue
        if category in {"Zl", "Zp"}:
            out.append(" ")
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


def _archive_support_views(names: list[str]) -> list[list[str]]:
    """Return archive-name views used only for support-bundle signatures.

    Downloaders/repackagers often wrap an otherwise identical archive in one
    top-level directory.  Recognize tool/HANDOFF signatures through that single
    wrapper before scanning Python helper markers, so support bundles cannot
    become runnable PATCH items merely because they contain self-tests.
    """
    clean: list[str] = []
    for raw in names:
        n = raw.replace("\\", "/")
        while n.startswith("./"):
            n = n[2:]
        n = n.strip("/")
        if n:
            clean.append(n)
    views = [clean]
    first_parts = {n.split("/", 1)[0] for n in clean}
    if len(first_parts) == 1 and clean and all("/" in n for n in clean):
        prefix = next(iter(first_parts)) + "/"
        if prefix not in {"./", "../"}:
            stripped = [n[len(prefix):] for n in clean if n.startswith(prefix)]
            if stripped:
                views.append(stripped)
    return views


def _support_bundle_filename_kind(name: str) -> str | None:
    """Recognize well-known generated support artifact filenames.

    Root PATCH manifests still take precedence.  These filename signatures are
    a fallback for generated HANDOFF/DETAIL/tool archives whose internal layout
    may vary and may legitimately contain runnable-looking source evidence.
    """
    low = name.lower()
    if low.startswith("python_patch_tool_"):
        return "tool_distribution"
    if low.startswith("ptv_") and any(tag in low for tag in ("_handoff", "_detail", "_summary", "_report", "_code")):
        return "ptv_support_archive"
    # Project handoffs are not always PTV-generated (for example
    # DATBIKE_BLE_OTA_HANDOFF_....zip), and browsers commonly append copy
    # suffixes such as ``(1)``.  A root PATCH manifest is checked before this
    # filename fallback.  Preserve the historical ``patch_*`` namespace for
    # genuine legacy patch archives even when the patch description itself
    # contains the word ``handoff``.
    if not low.startswith("patch_"):
        stem = re.sub(r"(?i)\.(?:zip|tgz|tar\.gz)$", "", low)
        stem = re.sub(r"\s*\(\d+\)$", "", stem)
        if re.search(r"(?:^|[_. -])handoff(?:[_. -]|$)", stem):
            return "handoff_archive"
    return None


def _is_support_bundle_names(names: list[str]) -> str | None:
    for view in _archive_support_views(names):
        if (
            "tools/run_python_patches.sh" in view
            and any(n.startswith("tools/_patch_lib/") for n in view)
        ):
            return "tool_distribution"
        root_names = {n for n in view if "/" not in n}
        if {"HANDOFF_README.md", "CURRENT_STATE.md"} <= root_names:
            return "handoff_archive"
    return None


def _zip_support_kind(path: Path) -> str | None:
    """Recognize non-runnable support bundles before COLLECT inspection.

    A HANDOFF may legitimately embed a prior CODE_COLLECTION_REQUEST JSON.
    If COLLECT inspection runs first, that unrelated nested request can turn the
    complete HANDOFF into a runnable queue item. Support-bundle identity is
    stronger than nested request content, while a root PATCH manifest remains
    stronger than both and is checked by the caller first.
    """
    if path.suffix.lower() != ".zip" or not path.is_file():
        return None
    filename_kind = _support_bundle_filename_kind(path.name)
    if filename_kind:
        return filename_kind
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
        return _is_support_bundle_names(names)
    except Exception:
        return None


def _zip_is_patch(path: Path):
    try:
        with zipfile.ZipFile(path) as zf:
            names = [n for n in zf.namelist() if not n.endswith("/")]
            # v5+ standard package: manifest must be at package root. Requiring
            # the root prevents a HANDOFF that merely embeds a patch tree from
            # being mistaken for the patch itself.
            if "PATCH_TOOL_MANIFEST.json" in names:
                return True, "manifest"
            filename_kind = _support_bundle_filename_kind(path.name)
            if filename_kind:
                return False, filename_kind
            # Known distribution/support bundles are not runnable patches.
            # Check both the archive root and a common single wrapper folder
            # before scanning Python text for legacy helper markers.
            support_kind = _is_support_bundle_names(names)
            if support_kind:
                return False, support_kind

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
            filename_kind = _support_bundle_filename_kind(path.name)
            if filename_kind:
                return False, filename_kind
            support_kind = _is_support_bundle_names(names)
            if support_kind:
                return False, support_kind
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
            items.append(_queue_item(path, "PATCH", "manifest"))
            continue

        # A tool distribution or HANDOFF can embed a previous valid COLLECT
        # request as evidence.  Never let that nested JSON override the archive's
        # stronger support-bundle identity.
        support_kind = _zip_support_kind(path)
        if support_kind:
            warnings.append(
                f"SKIPPED non-patch candidate: patchs/{path.name} ({support_kind})"
            )
            continue

        ok, detail = inspect_collect_zip(path)
        if ok:
            items.append(_queue_item(path, "COLLECT", detail))
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
            items.append(_queue_item(path, "COLLECT INVALID", detail))
            continue

        supported = low.endswith((".zip", ".py", ".tar.gz", ".tgz"))
        if not supported:
            continue
        is_patch, patch_detail = inspect_patch_candidate(path)
        if is_patch:
            items.append(_queue_item(path, "PATCH", patch_detail))
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


def _render(items, cursor, selected, msg, prev):
    lines = ["CHỌN CÔNG VIỆC SẼ CHẠY", ""]
    for i, item in enumerate(items):
        detail = f"  [{_safe_display(item.detail)}]" if item.detail else ""
        lines.append(
            f"{'›' if i == cursor else ' '} "
            f"[{'x' if i in selected else ' '}] {i + 1:>3}. "
            f"[{_safe_display(item.kind)}] {_safe_display(item.name)}{detail}"
        )
    lines += [
        "",
        "Space: chọn/bỏ | ↑/↓: di chuyển | a: tất cả | n: bỏ tất cả",
        "d: xóa item tại con trỏ | Enter: xác nhận | q/Esc: hủy",
    ]
    if msg:
        lines.append(_safe_display(msg))
    frame_height = max(prev, len(lines))
    if prev:
        sys.stdout.write(f"\x1b[{prev}F")
    padded = lines + [""] * (frame_height - len(lines))
    for line in padded:
        sys.stdout.write("\r\x1b[2K" + line + "\n")
    sys.stdout.flush()
    return frame_height


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


def _delete_indexes(root: Path, items: list[QueueItem], selected: set[int], indexes: set[int]):
    """Delete only queue files selected by index, preserving selection mapping."""
    deleted: list[str] = []
    failures: list[str] = []
    for i in sorted(indexes, reverse=True):
        victim = items[i]
        target = root / "patchs" / victim.name
        try:
            # Deletion is a destructive action and needs the same TOCTOU guard
            # as execution.  Never unlink a same-named file that appeared after
            # the selector was rendered.
            if target.is_symlink():
                failures.append(f"{victim.name}: queue entry changed into a symlink")
                continue
            if victim.identity is None:
                failures.append(f"{victim.name}: missing queue identity; refusing destructive delete")
                continue
            current_identity = _queue_identity(target)
            if current_identity is None:
                # An external process already removed it: drop the stale menu
                # entry, but do not claim that this invocation deleted it.
                items.pop(i)
                selected = {j if j < i else j - 1 for j in selected if j != i}
                continue
            if current_identity != victim.identity:
                failures.append(f"{victim.name}: queue entry was replaced or modified after selection")
                continue
            target.unlink()
        except FileNotFoundError:
            # It disappeared between identity validation and unlink. Treat the
            # selector entry as stale without deleting any replacement.
            items.pop(i)
            selected = {j if j < i else j - 1 for j in selected if j != i}
            continue
        except OSError as exc:
            failures.append(f"{victim.name}: {type(exc).__name__}")
            continue
        deleted.append(victim.name)
        items.pop(i)
        selected = {j if j < i else j - 1 for j in selected if j != i}
    if len(items) == 1:
        selected = {0}
    return selected, list(reversed(deleted)), list(reversed(failures))


def _select_items_line(root: Path, items: list[QueueItem], initial_selection: str):
    selected = _initial_selected(items, initial_selection)
    while items:
        print("CHỌN CÔNG VIỆC SẼ CHẠY")
        for i, item in enumerate(items, 1):
            mark = "x" if i - 1 in selected else " "
            print(f"  [{mark}] {i}. [{_safe_display(item.kind)}] {_safe_display(item.name)}")
        print("Nhập: 1,3-5 | a=all | n=none | d <số/range>=xóa | q=quit | Enter=xác nhận")
        raw_line = sys.stdin.readline()
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
            confirm = sys.stdin.readline()
            if confirm == "" or confirm.strip().lower() != "y":
                print("Xóa đã hủy.")
                continue
            selected, deleted, failures = _delete_indexes(root, items, selected, indexes)
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
            rendered = _render(items, cursor, selected, msg, rendered)
            msg = ""
            key = _read_key(fd)
            if key in {"q", "ESC", "\x03"}:
                return None
            if delete:
                delete = False
                if key != "y":
                    msg = "Xóa đã hủy."
                    continue
                victim = items[cursor]
                selected, deleted, failures = _delete_indexes(root, items, selected, {cursor})
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
                selected.remove(cursor) if cursor in selected else selected.add(cursor)
            elif key == "a":
                selected = set(range(len(items)))
            elif key == "n":
                selected.clear()
            elif key == "d":
                delete = True
                msg = f"Xóa {_safe_display(items[cursor].name)}? y để xác nhận"
            elif key == "ENTER":
                if selected:
                    return [items[i] for i in sorted(selected)]
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



def _revalidate_selected_item(root: Path, item: QueueItem) -> tuple[bool, str]:
    """Fail closed if a queued file changed after selection.

    Selection and execution are separate user-visible phases.  A file may be
    deleted/replaced by another task between them; do not execute a stale
    QueueItem classification in that case.
    """
    path = root / "patchs" / item.name
    try:
        if path.is_symlink():
            return False, "queue entry became a symlink after selection"
        if not path.is_file():
            return False, "queue entry no longer exists as a regular file"
    except OSError as exc:
        return False, f"queue entry cannot be inspected ({type(exc).__name__})"

    if item.identity is not None:
        current_identity = _queue_identity(path)
        if current_identity != item.identity:
            return False, "queue entry was replaced or modified after selection"

    if item.kind == "PATCH":
        # A root patch manifest has precedence over any nested COLLECT resource.
        if path.suffix.lower() == ".zip" and _zip_has_root_patch_manifest(path):
            return True, "manifest"
        is_patch, detail = inspect_patch_candidate(path)
        return (True, detail) if is_patch else (False, f"PATCH signature changed: {detail}")

    if item.kind == "COLLECT":
        if path.suffix.lower() == ".zip" and _zip_has_root_patch_manifest(path):
            return False, "COLLECT entry changed into a PATCH package"
        support_kind = _zip_support_kind(path)
        if support_kind:
            return False, f"COLLECT entry changed into a support bundle: {support_kind}"
        ok, detail = inspect_collect_zip(path)
        return (True, detail) if ok else (False, f"COLLECT request changed: {detail}")

    return False, f"non-runnable queue kind: {item.kind}"

def execute_items(root: Path, chosen: list[QueueItem]):
    """Execute in natural selected order and stop immediately on first failure."""
    executed: list[tuple[str, int]] = []
    for index, item in enumerate(chosen):
        valid, reason = _revalidate_selected_item(root, item)
        if not valid:
            print(
                f"ERROR queue item changed after selection: {_safe_display(item.name)} ({_safe_display(reason)})",
                file=sys.stderr,
            )
            rc = 2
            executed.append((item.name, rc))
            remaining = chosen[index + 1 :]
            return rc, executed, remaining
        if item.kind == "PATCH":
            cmd = [str(root / "tools/run_python_patches.sh"), "--patch", f"patchs/{item.name}"]
        elif item.kind == "COLLECT":
            # COLLECT subcommands are intentionally not part of the public
            # launcher contract.  Zero-argument queue routing invokes the
            # internal supervisor directly so users/AI have exactly one public
            # command: ./tools/run_python_patches.sh
            lib = root / "tools" / "_patch_lib"
            progress = lib / "python_patch_collect_progress_v6_7.py"
            collector = lib / "python_patch_readonly_collector.py"
            if not progress.is_file() or not collector.is_file():
                missing = progress if not progress.is_file() else collector
                print(f"ERROR missing COLLECT core: {_safe_display(str(missing))}", file=sys.stderr)
                rc = 2
                executed.append((item.name, rc))
                remaining = chosen[index + 1 :]
                return rc, executed, remaining
            cmd = [
                sys.executable, str(progress),
                "--project-root", str(root),
                "--collector", str(collector),
                "--", "request", f"patchs/{item.name}",
            ]
        else:
            print(f"ERROR invalid collect package {_safe_display(item.name)}", file=sys.stderr)
            rc = 2
            executed.append((item.name, rc))
            remaining = chosen[index + 1 :]
            return rc, executed, remaining
        rc = subprocess.run(cmd, cwd=root).returncode
        executed.append((item.name, rc))
        if rc:
            remaining = chosen[index + 1 :]
            return rc, executed, remaining
    return 0, executed, []

def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--project-root", required=True)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve()
    items, warnings = discover_queue(root)
    for warning in warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")
    if not items:
        print("AUTO STATUS: IDLE — no runnable patch/collect package is waiting in patchs/.")
        return 0

    cfg, config_warnings = _load_zero_argument_config(root)
    for warning in config_warnings:
        print(f"[PTV v{VERSION} WARNING] {_safe_display(warning)}")

    chosen = _configured_auto_selection(root, items, cfg)
    if chosen is None:
        chosen = select_items(
            root,
            list(items),
            initial_selection=cfg.get("initial_selection", "none"),
            selector_ui=cfg.get("selector_ui", "auto"),
        )
    if chosen is None:
        print("Cancelled.")
        return 0
    if not chosen:
        print("AUTO STATUS: IDLE — queue is empty or no runnable item remains; nothing executed.")
        return 0

    rc, executed, remaining = execute_items(root, chosen)
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
        return rc
    print(f"SUMMARY: PASS | {len(executed)} selected item(s) completed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
