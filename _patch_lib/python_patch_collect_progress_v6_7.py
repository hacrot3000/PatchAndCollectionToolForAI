#!/usr/bin/env python3
from __future__ import annotations

import argparse
from collections import deque
import json
import os
import math
from pathlib import Path
import queue
import re
import signal
import shutil
import subprocess
import sys
import threading
import time
import unicodedata
from typing import Iterable

VERSION = "6.17.7"
DEFAULT_HEARTBEAT = 0.8
DEFAULT_MARGIN = 2
MAX_TAIL_LINES = 120
POST_EXIT_DRAIN_SECONDS = 3.0
POST_EXIT_KILL_GRACE_SECONDS = 0.5
MAX_TRACKED_RESULT_CANDIDATES = 16

# Collector output is untrusted terminal text. Strip common ANSI escape/control
# sequences before using it in the one-line status or bounded completion tail.
_ANSI_RE = re.compile(
    r"(?:\x1B\][^\x07]*(?:\x07|\x1B\\))"
    r"|(?:\x1B\[[0-?]*[ -/]*[@-~])"
    r"|(?:\x1B[@-_])"
)


def _sanitize_terminal_text(text: str) -> str:
    text = _ANSI_RE.sub("", text)
    out: list[str] = []
    for ch in text:
        if ch in "\r\n\t":
            out.append(" ")
            continue
        # C0/C1 controls (including ESC/DEL) can move the cursor, erase rows,
        # ring the bell, etc. Drop them so a collector cannot break one-row UI.
        if unicodedata.category(ch) == "Cc":
            continue
        out.append(ch)
    return "".join(out)

# Completion-output parsing. The readonly collector historically printed the
# same result ZIP both as a labelled line (``ZIP: ...``) and as a bare path.
# The supervisor owns the user-facing PASS summary, so it canonicalizes those
# variants into one highlighted upload target.
_RESULT_ZIP_RE = re.compile(
    r"^\s*(?:ZIP|RESULT(?:\s+ZIP)?|OUTPUT(?:\s+ZIP)?|ARTIFACT(?:\s+ZIP)?|FILE)\s*:\s*(.+?\.zip)\s*$",
    re.I,
)
_REQUEST_ZIP_RE = re.compile(r"^\s*REQUEST\s*:\s*(.+?\.zip)\s*$", re.I)
_BARE_ZIP_RE = re.compile(r"^\s*((?:/|\.{1,2}/|~[/\\]|[A-Za-z]:[/\\]|artifacts[/\\]).+?\.zip)\s*$", re.I)


def _clean_reported_path(value: str) -> str:
    value = _sanitize_terminal_text(value).strip().strip('\"\'')
    return value


def _is_request_archive_path(value: str) -> bool:
    low = value.replace('\\', '/').lower()
    return '/patchs/patched/' in f'/{low.lstrip("/")}' or Path(value).name.lower().startswith('code_collection_request')


def _extract_collect_candidates(lines: Iterable[str]) -> tuple[list[str], str | None, list[str]]:
    """Return result candidates in observed order, plus request/extras.

    Some legacy collectors may mention an older/stale ZIP and then emit the
    actual final ZIP later.  Keep all distinct candidates so PASS validation
    can walk newest-to-oldest instead of trusting one textual variant.
    """
    result_candidates: list[str] = []
    request_paths: list[str] = []
    extras: list[str] = []
    for raw in lines:
        line = _sanitize_terminal_text(raw).strip()
        if not line:
            continue
        m = _REQUEST_ZIP_RE.match(line)
        if m:
            request_paths.append(_clean_reported_path(m.group(1)))
            continue
        candidate = None
        m = _RESULT_ZIP_RE.match(line)
        if m:
            candidate = _clean_reported_path(m.group(1))
        else:
            m = _BARE_ZIP_RE.match(line)
            if m:
                candidate = _clean_reported_path(m.group(1))
        if candidate:
            if not _is_request_archive_path(candidate):
                # Move a repeated candidate to the end so ordering reflects the
                # most recent observation while remaining deduplicated.
                result_candidates = [x for x in result_candidates if x != candidate]
                result_candidates.append(candidate)
            continue
        if re.search(r"(^|\b)(WARNING|WARN|ERROR|NOTICE)(\b|:)", line, re.I):
            extras.append(line)
    request = request_paths[-1] if request_paths else None
    return result_candidates, request, extras


def _extract_collect_completion(lines: Iterable[str]) -> tuple[str | None, str | None, list[str]]:
    candidates, request, extras = _extract_collect_candidates(lines)
    return (candidates[-1] if candidates else None), request, extras

def _completion_progress_detail(line: str) -> str:
    result_zip, request_zip, _ = _extract_collect_completion([line])
    if result_zip:
        return 'result collection ZIP ready'
    if request_zip:
        return 'request archived'
    return _sanitize_terminal_text(line.strip())


def _validated_result_zip(root: Path, reported: str) -> tuple[str | None, str | None]:
    try:
        raw = Path(reported).expanduser()
        candidate = (raw if raw.is_absolute() else (root / raw)).resolve(strict=True)
        artifacts = (root / "artifacts").resolve(strict=False)
        candidate.relative_to(artifacts)
        if not candidate.is_file():
            return None, "result path is not a regular file"
        if candidate.suffix.lower() != ".zip":
            return None, "result artifact is not a .zip file"
        import zipfile
        with zipfile.ZipFile(candidate) as zf:
            bad = zf.testzip()
            if bad is not None:
                return None, f"result ZIP CRC failed at {bad}"
        return str(candidate), None
    except FileNotFoundError:
        return None, "reported result ZIP does not exist"
    except ValueError:
        return None, "reported result ZIP is outside project artifacts/"
    except Exception as exc:
        return None, f"reported result ZIP is invalid ({type(exc).__name__})"


_LAST_COLLECT_SUCCESS_META: dict[str, object] = {}


def _fmt_quality_bytes(value: int) -> str:
    n = float(max(0, int(value)))
    for unit in ("B", "KiB", "MiB", "GiB"):
        if n < 1024.0 or unit == "GiB":
            return f"{int(n)}B" if unit == "B" else f"{n:.1f}{unit}"
        n /= 1024.0
    return str(value)


def _collect_quality(result_zip: str) -> dict[str, object]:
    quality: dict[str, object] = {
        "files": 0, "source_bytes": 0, "reports": 0, "report_bytes": 0,
        "zip_bytes": 0, "truncated_reports": 0, "missing": 0,
    }
    try:
        path = Path(result_zip)
        quality["zip_bytes"] = path.stat().st_size
        import zipfile
        with zipfile.ZipFile(path) as zf:
            if "COLLECTION_MANIFEST.json" not in zf.namelist():
                quality["manifest"] = "missing"
                quality["missing"] = 1
                return quality
            manifest = json.loads(zf.read("COLLECTION_MANIFEST.json").decode("utf-8"))
            if not isinstance(manifest, dict):
                quality["manifest"] = "invalid"
                quality["missing"] = 1
                return quality
            quality["files"] = int(manifest.get("file_count", len(manifest.get("files") or [])) or 0)
            quality["source_bytes"] = int(manifest.get("total_file_bytes", sum(int(x.get("size",0)) for x in (manifest.get("files") or []) if isinstance(x,dict))) or 0)
            reports = [x for x in (manifest.get("reports") or []) if isinstance(x, dict)]
            quality["reports"] = len(reports)
            quality["report_bytes"] = int(manifest.get("report_bytes", 0) or 0)
            truncated = sum(1 for x in reports if x.get("truncated"))
            # Compatibility with older result manifests: inspect report text for
            # explicit bounded-result markers when the boolean was not recorded.
            if truncated == 0:
                for report in reports:
                    arc = report.get("archive_path")
                    if isinstance(arc, str) and arc in zf.namelist():
                        text = zf.read(arc).decode("utf-8", errors="replace")
                        if "[TRUNCATED" in text:
                            truncated += 1
            quality["truncated_reports"] = truncated
            quality["manifest"] = "ok"
    except Exception as exc:
        quality["manifest"] = f"error:{type(exc).__name__}"
        quality["missing"] = 1
    return quality


def _print_collect_quality(result_zip: str) -> dict[str, object]:
    q = _collect_quality(result_zip)
    print(
        "COLLECT QUALITY: "
        f"files={q.get('files',0)} | source={_fmt_quality_bytes(int(q.get('source_bytes',0) or 0))} | "
        f"reports={q.get('reports',0)} | zip={_fmt_quality_bytes(int(q.get('zip_bytes',0) or 0))} | "
        f"truncated={q.get('truncated_reports',0)} | missing={q.get('missing',0)}"
    )
    if int(q.get("truncated_reports", 0) or 0) > 0:
        print("[PTV WARNING] COLLECT evidence is bounded/truncated; tell AI that some report limits were reached.")
    if int(q.get("missing", 0) or 0) > 0:
        print("[PTV WARNING] COLLECT quality metadata is incomplete.")
    return q


def _write_collect_run_result(data: dict[str, object]) -> None:
    raw = os.environ.get("PTV_COLLECT_RESULT_FILE", "").strip()
    if not raw:
        return
    path = Path(raw)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_name(f".{path.name}.{os.getpid()}.tmp")
        tmp.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        os.replace(tmp, path)
    except Exception:
        pass


def _print_collect_success(root: Path, lines: Iterable[str]) -> bool:
    global _LAST_COLLECT_SUCCESS_META
    _LAST_COLLECT_SUCCESS_META = {}
    material = list(lines)
    candidates, request_zip, extras = _extract_collect_candidates(material)
    result_zip = None
    validation_errors: list[str] = []
    for reported in reversed(candidates):
        validated, error = _validated_result_zip(root, reported)
        if validated:
            result_zip = validated
            break
        validation_errors.append(f"{reported}: {error or 'invalid result ZIP'}")
    if result_zip:
        is_tty = bool(getattr(sys.stdout, 'isatty', lambda: False)())
        banner = '!!! [PRIMARY - UPLOAD THIS FILE] !!!'
        destination = '>>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<'
        # Never force a decorative minimum wider than the live terminal.
        # The final artifact path may naturally wrap because it must remain
        # complete/copyable, but banner/rule rows themselves must not create
        # avoidable physical-line wrapping on very narrow terminals.
        width = max(1, min(72, _term_width() - 2))
        rule = '=' * width
        print('')
        print(rule)
        if is_tty:
            # High-contrast yellow action banner; keep ANSI outside the path so
            # the artifact itself is still easy to select/copy from a terminal.
            print(f'\x1b[1;30;103m{_clip_cells(banner, width)}\x1b[0m')
            print(f'\x1b[1;7m{_clip_cells(destination, width)}\x1b[0m')
            print(f'\x1b[1;4m{result_zip}\x1b[0m')
        else:
            print(banner)
            print(destination)
            print(result_zip)
        print(rule)
    else:
        reason = f" ({validation_errors[0]})" if validation_errors else ''
        print(f'[PTV v{VERSION} ERROR] COLLECT process returned success but no valid upload ZIP was verified{reason}.')
        print('Recent collector output follows so the artifact can be recovered:')
        for line in material[-8:]:
            print(_sanitize_terminal_text(line))
    quality = None
    if result_zip:
        quality = _print_collect_quality(result_zip)
    if request_zip:
        print(f'[INFO] REQUEST ARCHIVED: {request_zip}')
    for line in extras[-4:]:
        print(line)
    if result_zip:
        _LAST_COLLECT_SUCCESS_META = {"result_zip": result_zip, "request_archive": request_zip, "quality": quality}
    return result_zip is not None

def _cell_width(text: str) -> int:
    width = 0
    for ch in text:
        if ch in "\r\n\t":
            width += 1 if ch != "\t" else 4
            continue
        if unicodedata.combining(ch):
            continue
        width += 2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1
    return width


def _clip_cells(text: str, limit: int) -> str:
    text = _sanitize_terminal_text(text)
    if limit <= 1:
        return "…"[:max(0, limit)]
    if _cell_width(text) <= limit:
        return text
    out = []
    used = 0
    target = max(1, limit - 1)
    for ch in text:
        w = 0 if unicodedata.combining(ch) else (2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1)
        if used + w > target:
            break
        out.append(ch)
        used += w
    return "".join(out) + "…"


def _term_width() -> int:
    # A live TTY query must win over COLUMNS. Environment values may be stale
    # after a terminal resize, and a minimum such as 40 columns can itself
    # force wrapping in genuinely narrow terminals.
    try:
        fd = sys.stdout.fileno()
        cols = os.get_terminal_size(fd).columns
        if cols > 0:
            return cols
    except Exception:
        pass

    # Fallbacks are mainly for unusual streams/tests. _render_one_line() only
    # emits heartbeat rows on a TTY, so a stale environment value cannot affect
    # normal interactive resize handling once the direct query succeeds.
    env = os.environ.get("COLUMNS")
    if env:
        try:
            cols = int(env)
            if cols > 0:
                return cols
        except ValueError:
            pass
    try:
        cols = shutil.get_terminal_size((120, 24)).columns
        return cols if cols > 0 else 120
    except Exception:
        return 120


def _render_one_line(text: str, *, final: bool = False) -> None:
    is_tty = bool(getattr(sys.stdout, "isatty", lambda: False)())
    if not is_tty:
        if final:
            print(text)
        return
    width = max(0, _term_width() - DEFAULT_MARGIN)
    clipped = _clip_cells(text, width)
    sys.stdout.write("\r\x1b[2K" + clipped)
    if final:
        sys.stdout.write("\n")
    sys.stdout.flush()


def _proc_snapshot(pid: int) -> tuple[str, str, int | None, int | None]:
    state = "?"
    rss = "?"
    read_b = None
    write_b = None
    proc = Path("/proc") / str(pid)
    try:
        for line in (proc / "status").read_text(errors="replace").splitlines():
            if line.startswith("State:"):
                state = line.split(":", 1)[1].strip().split()[0]
            elif line.startswith("VmRSS:"):
                rss = line.split(":", 1)[1].strip().replace(" ", "")
    except Exception:
        pass
    try:
        vals = {}
        for line in (proc / "io").read_text(errors="replace").splitlines():
            if ":" in line:
                k, v = line.split(":", 1)
                if k in {"read_bytes", "write_bytes"}:
                    vals[k] = int(v.strip())
        read_b = vals.get("read_bytes")
        write_b = vals.get("write_bytes")
    except Exception:
        pass
    return state, rss, read_b, write_b


def _fmt_bytes(value: int | None) -> str:
    if value is None:
        return "?"
    n = float(value)
    for unit in ("B", "K", "M", "G", "T"):
        if n < 1024.0 or unit == "T":
            return f"{n:.0f}{unit}" if unit == "B" else f"{n:.1f}{unit}"
        n /= 1024.0
    return str(value)


_PHASE_PATTERNS: list[tuple[re.Pattern[str], str]] = [
    (re.compile(r"request|normalized", re.I), "request"),
    # Explicit search/ripgrep output is more specific than generic candidate enumeration.
    (re.compile(r"ripgrep|\brg\b|search|match", re.I), "search"),
    (re.compile(r"candidate|enumerat|scan|walking|discover", re.I), "scan"),
    (re.compile(r"symbol|definition|reference|caller|callee", re.I), "symbols"),
    (re.compile(r"depend|import|include|closure", re.I), "deps"),
    (re.compile(r"snippet|context", re.I), "snippets"),
    (re.compile(r"copy|collect.*file", re.I), "copy"),
    (re.compile(r"sha|hash|manifest", re.I), "hash"),
    (re.compile(r"zip|archive|compress", re.I), "zip"),
]


def _infer_phase(line: str, current: str) -> str:
    for rx, phase in _PHASE_PATTERNS:
        if rx.search(line):
            return phase
    return current


def _reader(stream, q: queue.Queue[str], tail: deque[str]) -> None:
    try:
        for raw in iter(stream.readline, ""):
            line = raw.rstrip("\r\n")
            tail.append(line)
            q.put(line)
    finally:
        try:
            stream.close()
        except Exception:
            pass


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="Python Patch Tool v6.17.6 COLLECT one-line progress supervisor")
    ap.add_argument("--project-root", required=True)
    ap.add_argument("--collector", required=True)
    ap.add_argument("rest", nargs=argparse.REMAINDER)
    ns = ap.parse_args(argv)
    rest = list(ns.rest)
    if rest and rest[0] == "--":
        rest = rest[1:]
    root = Path(ns.project_root).resolve()
    collector = Path(ns.collector).resolve()
    raw_heartbeat = os.environ.get("PTV_COLLECT_HEARTBEAT_SECONDS")
    try:
        heartbeat = float(raw_heartbeat) if raw_heartbeat is not None else DEFAULT_HEARTBEAT
        if not math.isfinite(heartbeat):
            raise ValueError("heartbeat must be finite")
    except (TypeError, ValueError):
        heartbeat = DEFAULT_HEARTBEAT
    heartbeat = max(0.2, heartbeat)

    cmd = [sys.executable, str(collector), "--project-root", str(root), *rest]
    env = dict(os.environ)
    env["PYTHONUNBUFFERED"] = "1"
    proc = subprocess.Popen(
        cmd,
        cwd=str(root),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        encoding="utf-8",
        errors="replace",
        bufsize=1,
        # Put the collector in its own process group.  If the supervisor is
        # stopped by an IDE/task runner that signals only the parent PID, we
        # can terminate the complete collector tree instead of leaving a
        # readonly collection process running in the background.
        start_new_session=(os.name == "posix"),
    )
    assert proc.stdout is not None

    received_signal: list[int] = []
    signal_deadline: list[float | None] = [None]
    old_handlers: dict[int, object] = {}

    def _forward_stop(signum, _frame):
        if not received_signal:
            received_signal.append(int(signum))
            signal_deadline[0] = time.monotonic() + 2.0
        try:
            if os.name == "posix":
                # The process-group leader may already have exited while one of
                # its descendants still owns stdout.  killpg(pgid) remains the
                # correct cleanup target even after proc.poll() is non-None.
                os.killpg(proc.pid, signum)
            elif proc.poll() is None:
                proc.send_signal(signum)
        except (ProcessLookupError, PermissionError, OSError):
            pass

    for sig in (getattr(signal, "SIGINT", None), getattr(signal, "SIGTERM", None)):
        if sig is None:
            continue
        try:
            old_handlers[int(sig)] = signal.getsignal(sig)
            signal.signal(sig, _forward_stop)
        except (ValueError, OSError):
            pass

    q: queue.Queue[str] = queue.Queue()
    tail: deque[str] = deque(maxlen=MAX_TAIL_LINES)
    thread = threading.Thread(target=_reader, args=(proc.stdout, q, tail), daemon=True)
    thread.start()

    started = time.monotonic()
    last_render = 0.0
    output_lines = 0
    phase = "start"
    last_detail = "starting collector"
    is_tty = bool(getattr(sys.stdout, "isatty", lambda: False)())

    # Completion metadata must outlive the bounded diagnostic tail. A valid ZIP
    # may be reported and followed by hundreds of ordinary log lines; using
    # only tail[-120:] would then lose the upload target and create a false FAIL.
    tracked_result_candidates: list[str] = []
    tracked_request_zip: str | None = None
    tracked_extras: deque[str] = deque(maxlen=4)

    def observe_completion_metadata(line: str) -> None:
        nonlocal tracked_request_zip
        candidates, request_zip, extras = _extract_collect_candidates([line])
        for candidate in candidates:
            tracked_result_candidates[:] = [x for x in tracked_result_candidates if x != candidate]
            tracked_result_candidates.append(candidate)
            if len(tracked_result_candidates) > MAX_TRACKED_RESULT_CANDIDATES:
                del tracked_result_candidates[:-MAX_TRACKED_RESULT_CANDIDATES]
        if request_zip:
            tracked_request_zip = request_zip
        for extra in extras:
            tracked_extras.append(extra)

    while True:
        while True:
            try:
                line = q.get_nowait()
            except queue.Empty:
                break
            output_lines += 1
            phase = _infer_phase(line, phase)
            observe_completion_metadata(line)
            if line.strip():
                last_detail = _completion_progress_detail(line)
        rc = proc.poll()
        now = time.monotonic()
        if rc is not None:
            break
        if is_tty and now - last_render >= heartbeat:
            last_render = now
            state, rss, read_b, write_b = _proc_snapshot(proc.pid)
            elapsed = now - started
            status = (
                f"⏳ COLLECT | {elapsed:6.1f}s | {phase:<8} | pid={proc.pid} {state} rss={rss} "
                f"r={_fmt_bytes(read_b)} w={_fmt_bytes(write_b)} | {last_detail}"
            )
            _render_one_line(status)
        if received_signal and signal_deadline[0] is not None and now >= signal_deadline[0]:
            try:
                if os.name == "posix":
                    os.killpg(proc.pid, signal.SIGKILL)
                else:
                    proc.kill()
            except (ProcessLookupError, PermissionError, OSError):
                pass
            signal_deadline[0] = None
        time.sleep(0.08)

    def consume_pending_output() -> None:
        nonlocal output_lines, phase, last_detail
        while True:
            try:
                line = q.get_nowait()
            except queue.Empty:
                break
            output_lines += 1
            phase = _infer_phase(line, phase)
            observe_completion_metadata(line)
            if line.strip():
                last_detail = _completion_progress_detail(line)

    # The child can exit before the reader thread has drained the final bytes
    # already buffered in the stdout pipe.  Waiting only one second can lose
    # the final ZIP line on large/noisy collections and turn a real PASS into
    # a false rc=3.  Drain until EOF for a bounded grace period.
    drain_deadline = time.monotonic() + POST_EXIT_DRAIN_SECONDS
    while thread.is_alive() and time.monotonic() < drain_deadline:
        consume_pending_output()
        now = time.monotonic()
        if received_signal and signal_deadline[0] is not None and now >= signal_deadline[0]:
            try:
                if os.name == "posix":
                    os.killpg(proc.pid, signal.SIGKILL)
                elif proc.poll() is None:
                    proc.kill()
            except (ProcessLookupError, PermissionError, OSError):
                pass
            signal_deadline[0] = None
        thread.join(timeout=0.05)
    consume_pending_output()

    lingering_output_tree = thread.is_alive()
    if lingering_output_tree:
        # A normal exited collector closes stdout.  If the pipe is still held
        # open, a descendant most likely inherited it.  Clean that process
        # group so a finished COLLECT cannot leave a writer/process behind.
        try:
            if os.name == "posix":
                os.killpg(proc.pid, signal.SIGTERM)
        except (ProcessLookupError, PermissionError, OSError):
            pass
        thread.join(timeout=POST_EXIT_KILL_GRACE_SECONDS)
        if thread.is_alive():
            try:
                if os.name == "posix":
                    os.killpg(proc.pid, signal.SIGKILL)
            except (ProcessLookupError, PermissionError, OSError):
                pass
            thread.join(timeout=POST_EXIT_KILL_GRACE_SECONDS)
        consume_pending_output()

    # Keep our forwarding handlers installed through the complete post-exit
    # drain/descendant cleanup window.  Restoring them immediately when the
    # parent collector exits can let an IDE SIGTERM kill only the supervisor
    # and leave a stdout-holding descendant orphaned.
    for sig_num, old_handler in old_handlers.items():
        try:
            signal.signal(sig_num, old_handler)
        except (ValueError, OSError):
            pass

    elapsed = time.monotonic() - started
    raw_rc = int(rc)
    if received_signal:
        final_rc = 128 + received_signal[0]
    elif raw_rc < 0:
        final_rc = 128 + abs(raw_rc)
    else:
        final_rc = raw_rc
    _render_one_line(
        f"{'✓' if final_rc == 0 else '✗'} COLLECT | rc={final_rc} | {elapsed:.1f}s | phase={phase} | output={output_lines} lines",
        final=True,
    )

    # Keep the terminal compact on PASS. On FAIL, surface a bounded tail so the user has useful evidence.
    if lingering_output_tree:
        print(f"[PTV v{VERSION} WARNING] collector stdout remained open after parent exit; lingering process-group output was cleaned up")

    if final_rc != 0:
        print("COLLECT FAILED — recent collector output:")
        for line in list(tail)[-30:]:
            print(_sanitize_terminal_text(line))
    else:
        # Canonicalize collector completion output. In particular, legacy
        # collectors may print the result archive twice (a ``ZIP:`` line plus
        # the bare path). The user should see exactly one highlighted upload
        # target and must not confuse it with the archived request ZIP.
        completion_material = [f"ZIP : {x}" for x in tracked_result_candidates]
        if tracked_request_zip:
            completion_material.append(f"REQUEST : {tracked_request_zip}")
        completion_material.extend(tracked_extras)
        if not _print_collect_success(root, completion_material):
            final_rc = 3
    result_meta = dict(_LAST_COLLECT_SUCCESS_META)
    result_meta.update({
        "format": "python-patch-tool-collect-result",
        "format_version": 1,
        "tool_version": VERSION,
        "status": "PASS" if final_rc == 0 else "FAIL",
        "rc": final_rc,
        "phase": phase,
        "elapsed_seconds": round(elapsed, 3),
        "output_lines": output_lines,
    })
    _write_collect_run_result(result_meta)
    return final_rc


if __name__ == "__main__":
    raise SystemExit(main())
