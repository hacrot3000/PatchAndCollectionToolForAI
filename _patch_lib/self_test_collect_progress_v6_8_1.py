#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import os
import pty
import fcntl
import struct
import subprocess
import signal
import sys
import time
import tempfile
import termios
from pathlib import Path

HERE = Path(__file__).resolve().parent
p = HERE / "python_patch_collect_progress_v6_7.py"
spec = importlib.util.spec_from_file_location("ptv678_progress", p)
assert spec and spec.loader
m = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = m
spec.loader.exec_module(m)

assert m.VERSION == "6.8.1"
assert m._cell_width("abc") == 3
assert m._cell_width("测试") == 4
for width in [0, 1, 2, 12, 20, 40, 80, 120]:
    s = m._clip_cells("x" * 200, width)
    assert m._cell_width(s) <= width, (width, m._cell_width(s))

raw = "\x1b[31mRED\x1b[0m\rnext\tfield\x07"
clean = m._sanitize_terminal_text(raw)
assert "\x1b" not in clean and "\r" not in clean and "\t" not in clean and "\x07" not in clean
assert "RED" in clean and "next" in clean
assert m._infer_phase("search candidates with ripgrep", "start") == "search"
assert m._infer_phase("writing SHA256 manifest", "start") == "hash"
assert m._infer_phase("compressing ZIP archive", "start") == "zip"

# Live PTY size must override stale COLUMNS and reflect resize.
master, slave = pty.openpty()
old_stdout = m.sys.stdout
class TtyProxy:
    def __init__(self, fd): self.fd = fd
    def fileno(self): return self.fd
    def isatty(self): return True
try:
    os.environ["COLUMNS"] = "200"
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 24, 0, 0))
    m.sys.stdout = TtyProxy(slave)
    assert m._term_width() == 24, m._term_width()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 18, 0, 0))
    assert m._term_width() == 18, m._term_width()
finally:
    m.sys.stdout = old_stdout
    os.environ.pop("COLUMNS", None)
    os.close(master)
    os.close(slave)

# Invalid heartbeat configuration and invalid UTF-8 collector bytes must not
# crash the supervisor or silently discard otherwise useful PASS output.
with tempfile.TemporaryDirectory(prefix="ptprog_v6710_") as td:
    root = Path(td)
    result = root / "artifacts" / "patch_tool_code_collections" / "bad-bytes.zip"
    collector = root / "bad_bytes_collector.py"
    collector.write_text(
        "import os,zipfile\n"
        "from pathlib import Path\n"
        f"out=Path({str(result)!r})\n"
        "out.parent.mkdir(parents=True,exist_ok=True)\n"
        "with zipfile.ZipFile(out,'w') as zf: zf.writestr('manifest.json','{}')\n"
        "os.write(1, b\'begin\\n\\xffbad\\n\')\n"
        "print(f'ZIP : {out}', flush=True)\n",
        encoding="utf-8",
    )
    env = dict(os.environ)
    env["PTV_COLLECT_HEARTBEAT_SECONDS"] = "not-a-number"
    cp = subprocess.run(
        [
            sys.executable,
            str(p),
            "--project-root", str(root),
            "--collector", str(collector),
            "--", "request", "dummy.zip",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
        env=env,
        timeout=10,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert "Traceback" not in cp.stderr, cp.stderr
    assert "[PRIMARY - UPLOAD THIS FILE]" in cp.stdout, cp.stdout
    assert str(result) in cp.stdout, cp.stdout
    assert "output=3 lines" in cp.stdout, cp.stdout


# If an external task runner signals only the supervisor PID, the collector
# process group must be terminated too instead of becoming an orphan.
if os.name == "posix":
    with tempfile.TemporaryDirectory(prefix="ptprog_signal_v678_") as td:
        root = Path(td)
        pidfile = root / "collector.pid"
        collector = root / "sleep_collector.py"
        collector.write_text(
            "import os,time\n"
            "from pathlib import Path\n"
            f"Path({str(pidfile)!r}).write_text(str(os.getpid()))\n"
            "print('collector started', flush=True)\n"
            "time.sleep(30)\n",
            encoding="utf-8",
        )
        sup = subprocess.Popen(
            [
                sys.executable, str(p),
                "--project-root", str(root),
                "--collector", str(collector),
                "--", "request", "dummy.zip",
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        deadline = time.monotonic() + 5.0
        while not pidfile.exists() and time.monotonic() < deadline:
            time.sleep(0.05)
        assert pidfile.exists(), "collector did not start"
        child_pid = int(pidfile.read_text())
        os.kill(sup.pid, signal.SIGTERM)
        out, err = sup.communicate(timeout=6)
        assert sup.returncode == 143, (sup.returncode, out, err)
        # /proc may briefly retain a zombie, but the child must no longer be a
        # running process after its supervisor has completed.
        child_proc = Path('/proc') / str(child_pid)
        end = time.monotonic() + 2.0
        running = child_proc.exists()
        while running and time.monotonic() < end:
            try:
                status = (child_proc / 'status').read_text(errors='replace')
                state_line = next((x for x in status.splitlines() if x.startswith('State:')), '')
                if '\tZ' in state_line or ' Z ' in state_line:
                    running = False
                    break
            except FileNotFoundError:
                running = False
                break
            time.sleep(0.05)
            running = child_proc.exists()
        assert not running, f"collector child still running after supervisor SIGTERM: pid={child_pid}"


# Deterministic result-summary tests use the supervisor functions directly.
# They do not need to launch another Python interpreter merely to print three
# lines; keeping subprocess coverage only for decoding/signal behavior avoids
# process-churn flakes in the release regression suite.
import io
import zipfile

with tempfile.TemporaryDirectory(prefix="ptprog_result_v6711_") as td:
    root = Path(td)
    result = root / "artifacts" / "patch_tool_code_collections" / "m3_client_cjk_to_english_inventory_v1.zip"
    request = root / "patchs" / "patched" / "m3_collect_cjk_to_english_inventory_v1.zip"
    result.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(result, "w") as zf:
        zf.writestr("COLLECTION_MANIFEST.json", "{}")
    lines = [f"ZIP        : {result}", str(result), f"REQUEST     : {request}"]
    old_stdout = m.sys.stdout
    try:
        capture = io.StringIO(); m.sys.stdout = capture
        ok = m._print_collect_success(root, lines)
    finally:
        m.sys.stdout = old_stdout
    out = capture.getvalue()
    assert ok is True, out
    assert "[PRIMARY - UPLOAD THIS FILE]" in out, out
    assert "Destination: ChatGPT / AI server" in out, out
    assert out.count(str(result)) == 1, out
    assert "ZIP        :" not in out, out
    assert out.count(str(request)) == 1, out
    assert "[INFO] REQUEST ARCHIVED:" in out, out

# A stale earlier candidate must not mask a later valid result.
with tempfile.TemporaryDirectory(prefix="ptprog_fallback_v6711_") as td:
    root = Path(td)
    stale = root / "artifacts" / "patch_tool_code_collections" / "stale.zip"
    valid = root / "artifacts" / "patch_tool_code_collections" / "final.zip"
    valid.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(valid, "w") as zf:
        zf.writestr("COLLECTION_MANIFEST.json", "{}")
    old_stdout = m.sys.stdout
    try:
        capture = io.StringIO(); m.sys.stdout = capture
        ok = m._print_collect_success(root, [f"ZIP : {stale}", str(valid)])
    finally:
        m.sys.stdout = old_stdout
    out = capture.getvalue()
    assert ok is True, out
    assert str(valid) in out and str(stale) not in out, out
    assert out.count(str(valid)) == 1, out

# A reported ZIP outside artifacts/, missing, or corrupt must not be advertised.
with tempfile.TemporaryDirectory(prefix="ptprog_invalid_result_v6711_") as td:
    root = Path(td)
    missing = root / "artifacts" / "patch_tool_code_collections" / "missing.zip"
    old_stdout = m.sys.stdout
    try:
        capture = io.StringIO(); m.sys.stdout = capture
        ok = m._print_collect_success(root, [f"ZIP : {missing}"])
    finally:
        m.sys.stdout = old_stdout
    out = capture.getvalue()
    assert ok is False, out
    assert "[PRIMARY - UPLOAD THIS FILE]" not in out, out
    assert "no valid upload ZIP was verified" in out, out

print('PASS: Python Patch Tool v6.8.1 collect progress/artifact robustness self-test')
