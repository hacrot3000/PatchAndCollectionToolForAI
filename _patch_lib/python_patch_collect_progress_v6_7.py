#!/usr/bin/env python3
from __future__ import annotations

import argparse
from collections import deque
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
import zipfile
from typing import Iterable

VERSION = "6.7.11"
DEFAULT_HEARTBEAT = 0.8
DEFAULT_MARGIN = 2
MAX_TAIL_LINES = 120

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
    r"^\s*(?:ZIP|RESULT(?:\s+ZIP)?|OUTPUT(?:\s+ZIP)?|ARTIFACT(?:\s+ZIP)?|FILE)\s*:\s*([\"']?)(.+?\.zip)\1\s*$",
    re.I,
)
_REQUEST_ZIP_RE = re.compile(r"^\s*REQUEST\s*:\s*([\"']?)(.+?\.zip)\1\s*$", re.I)
_BARE_ZIP_RE = re.compile(r"^\s*([\"']?)((?:/|\.{1,2}/|~[/\\]|[A-Za-z]:[/\\]|artifacts[/\\]).+?\.zip)\1\s*$", re.I)


def _clean_reported_path(value: str) -> str:
    value = _sanitize_terminal_text(value).strip().strip('\"\'')
    return value


def _is_request_archive_path(value: str) -> bool:
    low = value.replace('\\', '/').lower()
    basename = low.rsplit('/', 1)[-1]
    return '/patchs/patched/' in f'/{low.lstrip("/")}' or basename.startswith('code_collection_request')


def _resolve_reported_zip(root: Path, value: str) -> Path | None:
    """Resolve a collector-reported path when it is local to this platform.

    The collector normally reports an absolute or project-relative local path.
    Cross-platform-looking paths are left unverifiable rather than being joined
    to the current project incorrectly.
    """
    value = _clean_reported_path(value)
    if not value:
        return None
    if os.name != 'nt' and re.match(r'^[A-Za-z]:[/\\]', value):
        return None
    if os.name == 'nt' and value.startswith('\\\\'):
        return Path(value)
    expanded = Path(os.path.expanduser(value))
    return expanded if expanded.is_absolute() else (root / expanded)


def _validate_result_zip(root: Path, value: str) -> tuple[bool, str]:
    resolved = _resolve_reported_zip(root, value)
    if resolved is None:
        # A path native to another platform cannot be verified here; this case
        # is kept detectable for portability but should not occur for a local
        # collector invocation.
        return False, f'cannot verify result ZIP path on this platform: {value}'
    try:
        if not resolved.is_file():
            return False, f'result ZIP does not exist: {value}'
        if not zipfile.is_zipfile(resolved):
            return False, f'result artifact is not a valid ZIP: {value}'
    except OSError as exc:
        return False, f'result ZIP validation failed ({type(exc).__name__}): {value}'
    return True, str(resolved)


def _extract_collect_completion(lines: Iterable[str]) -> tuple[str | None, str | None, list[str]]:
    result_explicit: list[str] = []
    result_bare: list[str] = []
    request_paths: list[str] = []
    extras: list[str] = []
    for raw in lines:
        line = _sanitize_terminal_text(raw).strip()
        if not line:
            continue
        m = _REQUEST_ZIP_RE.match(line)
        if m:
            request_paths.append(_clean_reported_path(m.group(2)))
            continue
        m = _RESULT_ZIP_RE.match(line)
        if m:
            candidate = _clean_reported_path(m.group(2))
            if candidate and not _is_request_archive_path(candidate):
                result_explicit.append(candidate)
            continue
        m = _BARE_ZIP_RE.match(line)
        if m:
            candidate = _clean_reported_path(m.group(2))
            if candidate and not _is_request_archive_path(candidate):
                result_bare.append(candidate)
            continue
        # On PASS the supervisor already prints the final rc/duration row. Keep
        # only meaningful warnings/notes instead of replaying the collector log.
        if re.search(r"(^|\b)(WARNING|WARN|ERROR|NOTICE)(\b|:)", line, re.I):
            extras.append(line)
    result = result_explicit[-1] if result_explicit else (result_bare[-1] if result_bare else None)
    request = request_paths[-1] if request_paths else None
    return result, request, extras


def _completion_progress_detail(line: str) -> str:
    result_zip, request_zip, _ = _extract_collect_completion([line])
    if result_zip:
        return 'result collection ZIP ready'
    if request_zip:
        return 'request archived'
    return _sanitize_terminal_text(line.strip())


def _print_collect_success(root: Path, lines: Iterable[str]) -> bool:
    material = list(lines)
    result_zip, request_zip, extras = _extract_collect_completion(material)
    valid_result = False
    validation_error = None
    if result_zip:
        valid_result, normalized = _validate_result_zip(root, result_zip)
        if valid_result:
            result_zip = normalized
        else:
            validation_error = normalized
    if result_zip and valid_result:
        title = '[PRIMARY - UPLOAD THIS FILE]'
        is_tty = bool(getattr(sys.stdout, 'isatty', lambda: False)())
        print('')
        print('================ COLLECT RESULT ================')
        if is_tty:
            print(f'\x1b[1;7m{title}\x1b[0m')
            print(f'\x1b[1m{result_zip}\x1b[0m')
        else:
            print(title)
            print(result_zip)
        print('Destination: ChatGPT / AI server')
        print('================================================')
    else:
        if validation_error:
            print(f'[PTV v{VERSION} ERROR] COLLECT reported an unusable result artifact: {validation_error}')
        else:
            print(f'[PTV v{VERSION} ERROR] COLLECT returned rc=0 but the result ZIP path was not detected.')
        print('Recent collector output follows so the artifact can be recovered:')
        for line in material[-8:]:
            print(_sanitize_terminal_text(line))
    if request_zip:
        print(f'[INFO] REQUEST ARCHIVED: {request_zip}')
    for line in extras[-4:]:
        print(line)
    return bool(result_zip and valid_result)


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
    ap = argparse.ArgumentParser(description="Python Patch Tool v6.7.11 COLLECT one-line progress supervisor")
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

    # Install stop handlers *before* spawning the collector.  Earlier releases
    # had a small race between Popen() and signal.signal(): a task runner could
    # terminate the supervisor in that window and leave the newly-created
    # collector session orphaned.
    received_signal: list[int] = []
    signal_deadline: list[float | None] = [None]
    old_handlers: dict[int, object] = {}
    proc_holder: list[subprocess.Popen | None] = [None]

    def _signal_collector(active_proc: subprocess.Popen, signum: int) -> None:
        if active_proc.poll() is not None:
            return
        try:
            if os.name == "posix":
                os.killpg(active_proc.pid, signum)
            else:
                active_proc.send_signal(signum)
        except (ProcessLookupError, PermissionError, OSError):
            pass

    def _forward_stop(signum, _frame):
        if not received_signal:
            received_signal.append(int(signum))
            signal_deadline[0] = time.monotonic() + 2.0
        active_proc = proc_holder[0]
        if active_proc is not None:
            # SIGQUIT commonly triggers a core dump in Python/native helpers,
            # which can make task cancellation appear hung while a large core
            # is written/processed. Preserve the supervisor's original 131
            # status, but terminate the readonly collector tree with SIGTERM.
            child_signum = (
                int(signal.SIGTERM)
                if getattr(signal, "SIGQUIT", None) is not None and int(signum) == int(signal.SIGQUIT)
                else int(signum)
            )
            _signal_collector(active_proc, child_signum)

    stop_signals = (
        getattr(signal, "SIGINT", None),
        getattr(signal, "SIGTERM", None),
        getattr(signal, "SIGHUP", None),
        getattr(signal, "SIGQUIT", None),
    )
    for sig in stop_signals:
        if sig is None:
            continue
        try:
            old_handlers[int(sig)] = signal.getsignal(sig)
            signal.signal(sig, _forward_stop)
        except (ValueError, OSError):
            pass

    try:
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
            # Put the collector in its own process group. If the supervisor is
            # stopped by an IDE/task runner that signals only the parent PID,
            # terminate the complete collector tree instead of leaving a
            # readonly collection process running in the background.
            start_new_session=(os.name == "posix"),
        )
    except BaseException:
        for sig_num, old_handler in old_handlers.items():
            try:
                signal.signal(sig_num, old_handler)
            except (ValueError, OSError):
                pass
        raise
    proc_holder[0] = proc
    assert proc.stdout is not None

    # A stop signal may have arrived during Popen(). The pre-installed handler
    # recorded it while proc_holder was empty; forward it now that the child
    # session exists.
    if received_signal:
        pending = received_signal[0]
        child_signum = (
            int(signal.SIGTERM)
            if getattr(signal, "SIGQUIT", None) is not None and pending == int(signal.SIGQUIT)
            else pending
        )
        _signal_collector(proc, child_signum)

    q: queue.Queue[str] = queue.Queue()
    tail: deque[str] = deque(maxlen=MAX_TAIL_LINES)
    # Result/request paths are completion metadata, not failure-tail detail.
    # Keep them independently so a verbose collector cannot push the upload ZIP
    # path out of the bounded tail before rc=0 is evaluated.
    completion_evidence: deque[str] = deque(maxlen=16)

    def remember_completion(line: str) -> None:
        result_zip, request_zip, _ = _extract_collect_completion([line])
        if result_zip or request_zip:
            completion_evidence.append(line)

    thread = threading.Thread(target=_reader, args=(proc.stdout, q, tail), daemon=True)
    thread.start()

    started = time.monotonic()
    last_render = 0.0
    output_lines = 0
    phase = "start"
    last_detail = "starting collector"
    is_tty = bool(getattr(sys.stdout, "isatty", lambda: False)())

    while True:
        while True:
            try:
                line = q.get_nowait()
            except queue.Empty:
                break
            output_lines += 1
            phase = _infer_phase(line, phase)
            remember_completion(line)
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

    for sig_num, old_handler in old_handlers.items():
        try:
            signal.signal(sig_num, old_handler)
        except (ValueError, OSError):
            pass

    thread.join(timeout=1.0)
    while True:
        try:
            line = q.get_nowait()
        except queue.Empty:
            break
        output_lines += 1
        phase = _infer_phase(line, phase)
        remember_completion(line)
        if line.strip():
            last_detail = _completion_progress_detail(line)

    elapsed = time.monotonic() - started
    raw_rc = int(rc)
    if received_signal:
        final_rc = 128 + received_signal[0]
    elif raw_rc < 0:
        final_rc = 128 + abs(raw_rc)
    else:
        final_rc = raw_rc

    # A zero collector exit code is only a PASS when there is a usable result
    # ZIP to upload. Decide that before rendering the final status so the
    # console can never say `✓ rc=0` and then fail afterward. Completion paths
    # are combined with the tail because they may have appeared much earlier
    # than MAX_TAIL_LINES before process exit.
    completion_material = [*completion_evidence, *tail]
    completion_contract_failed = False
    if final_rc == 0:
        result_zip, _request_zip, _extras = _extract_collect_completion(completion_material)
        if not result_zip:
            completion_contract_failed = True
        else:
            valid_result, _validation_detail = _validate_result_zip(root, result_zip)
            completion_contract_failed = not valid_result
        if completion_contract_failed:
            final_rc = 2

    _render_one_line(
        f"{'✓' if final_rc == 0 else '✗'} COLLECT | rc={final_rc} | {elapsed:.1f}s | phase={phase} | output={output_lines} lines",
        final=True,
    )

    # Keep the terminal compact on PASS. On a collector failure, surface a
    # bounded tail. A completion-contract failure gets its canonical artifact
    # diagnostic instead of a contradictory PASS block.
    if completion_contract_failed:
        _print_collect_success(root, completion_material)
        print(f'[PTV v{VERSION} ERROR] COLLECT completion contract failed; returning rc=2.', file=sys.stderr)
    elif final_rc != 0:
        print("COLLECT FAILED — recent collector output:")
        for line in list(tail)[-30:]:
            print(_sanitize_terminal_text(line))
    else:
        # Canonicalize collector completion output. In particular, legacy
        # collectors may print the result archive twice (a ``ZIP:`` line plus
        # the bare path). The user should see exactly one highlighted upload
        # target and must not confuse it with the archived request ZIP.
        _print_collect_success(root, completion_material)
    return final_rc


if __name__ == "__main__":
    raise SystemExit(main())
