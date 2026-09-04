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

assert m.VERSION == "6.11.0"
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
    assert "!!! [PRIMARY - UPLOAD THIS FILE] !!!" in out, out
    assert ">>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<" in out, out
    assert out.count(str(result)) == 1, out
    assert "ZIP        :" not in out, out
    assert out.count(str(request)) == 1, out
    assert "[INFO] REQUEST ARCHIVED:" in out, out

# Narrow-terminal banner regression: decorative banner/rule rows must never
# impose a historical 24-column minimum wider than the live terminal. The full
# artifact path remains complete/copyable and may naturally wrap.
class _NarrowTTY(io.StringIO):
    def isatty(self): return True
with tempfile.TemporaryDirectory(prefix="ptprog_narrow_banner_v695_") as td:
    narrow_root = Path(td)
    narrow_result = narrow_root / "artifacts" / "patch_tool_code_collections" / "result.zip"
    narrow_request = narrow_root / "patchs" / "patched" / "request.zip"
    narrow_result.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(narrow_result, "w") as zf:
        zf.writestr("COLLECTION_MANIFEST.json", "{}")
    narrow_lines = [f"ZIP : {narrow_result}", f"REQUEST : {narrow_request}"]
    old_stdout = m.sys.stdout
    old_term_width = m._term_width
    try:
        narrow = _NarrowTTY(); m.sys.stdout = narrow; m._term_width = lambda: 18
        ok = m._print_collect_success(narrow_root, narrow_lines)
    finally:
        m.sys.stdout = old_stdout; m._term_width = old_term_width
    narrow_out = narrow.getvalue()
    assert ok is True, narrow_out
    for line in narrow_out.splitlines():
        clean = m._ANSI_RE.sub('', line)
        if clean and (set(clean) == {'='} or 'PRIMARY - UPLOAD' in clean or 'ACTION REQUIRED' in clean):
            assert m._cell_width(clean) <= 16, (m._cell_width(clean), clean)

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



# Completion metadata is tracked independently from the bounded diagnostic tail.
# A result ZIP followed by >MAX_TAIL_LINES ordinary lines must remain uploadable.
with tempfile.TemporaryDirectory(prefix="ptprog_result_before_long_tail_v691_") as td:
    root = Path(td)
    result = root / "artifacts" / "patch_tool_code_collections" / "before-long-tail.zip"
    collector = root / "long_tail_collector.py"
    collector.write_text(
        "from pathlib import Path\n"
        "import zipfile\n"
        f"out=Path({str(result)!r})\n"
        "out.parent.mkdir(parents=True,exist_ok=True)\n"
        "with zipfile.ZipFile(out,'w') as zf: zf.writestr('manifest.json','{}')\n"
        "print(f'ZIP : {out}', flush=True)\n"
        "for i in range(200): print(f'after-result {i}', flush=True)\n",
        encoding="utf-8",
    )
    cp = subprocess.run(
        [sys.executable, '-S', str(p), '--project-root', str(root), '--collector', str(collector), '--'],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=15,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout, cp.stdout
    assert cp.stdout.count(str(result)) == 1, cp.stdout

# The collector process may exit before the reader thread drains bytes already
# buffered in stdout. A bounded post-exit drain must preserve the final ZIP
# line instead of turning a real PASS into rc=3.
with tempfile.TemporaryDirectory(prefix="ptprog_postexit_drain_v691_") as td:
    root = Path(td)
    result = root / "artifacts" / "patch_tool_code_collections" / "late-final.zip"
    collector = root / "noisy_collector.py"
    collector.write_text(
        "from pathlib import Path\n"
        "import zipfile\n"
        "root=Path.cwd()\n"
        "for i in range(600): print(f'noise {i}', flush=True)\n"
        f"out=Path({str(result)!r})\n"
        "out.parent.mkdir(parents=True,exist_ok=True)\n"
        "with zipfile.ZipFile(out,'w') as zf: zf.writestr('manifest.json','{}')\n"
        "print(f'ZIP : {out}', flush=True)\n",
        encoding="utf-8",
    )
    original_reader = m._reader
    def slow_reader(stream, q, tail):
        try:
            for raw in iter(stream.readline, ""):
                time.sleep(0.004)
                line = raw.rstrip("\r\n")
                tail.append(line)
                q.put(line)
        finally:
            try:
                stream.close()
            except Exception:
                pass
    old_stdout = m.sys.stdout
    try:
        import io
        m._reader = slow_reader
        capture = io.StringIO(); m.sys.stdout = capture
        rc = m.main(["--project-root", str(root), "--collector", str(collector), "--"])
    finally:
        m._reader = original_reader
        m.sys.stdout = old_stdout
    out = capture.getvalue()
    assert rc == 0, out
    assert "[PRIMARY - UPLOAD THIS FILE]" in out, out
    assert out.count(str(result)) == 1, out


# Signal during the post-exit drain window: the collector parent can exit while
# a descendant still owns stdout. Signalling only the supervisor must still be
# forwarded to that process group; otherwise the descendant becomes orphaned.
if os.name == 'posix':
    with tempfile.TemporaryDirectory(prefix='ptprog_postexit_signal_v693_') as td:
        root=Path(td); (root/'artifacts').mkdir()
        childpid=root/'descendant.pid'
        collector=root/'parent_exits.py'
        collector.write_text(
            "import subprocess,sys\n"
            "from pathlib import Path\n"
            "root=Path(sys.argv[sys.argv.index('--project-root')+1])\n"
            "p=subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)'],stdout=sys.stdout,stderr=sys.stdout)\n"
            "(root/'descendant.pid').write_text(str(p.pid))\n"
            "print('parent exits now',flush=True)\n",
            encoding='utf-8',
        )
        sup=subprocess.Popen(
            [sys.executable,'-S',str(p),'--project-root',str(root),'--collector',str(collector),'--'],
            stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,
        )
        deadline=time.monotonic()+5
        while not childpid.exists() and time.monotonic()<deadline:
            time.sleep(0.03)
        assert childpid.exists(),'descendant did not start'
        descendant=int(childpid.read_text())
        # Give the parent collector time to exit so this signal lands during
        # supervisor post-exit drain rather than its main poll loop.
        time.sleep(0.35)
        os.kill(sup.pid,signal.SIGTERM)
        out,err=sup.communicate(timeout=7)
        assert sup.returncode==143,(sup.returncode,out,err)
        procdir=Path('/proc')/str(descendant)
        end=time.monotonic()+2
        running=procdir.exists()
        while running and time.monotonic()<end:
            try:
                status=(procdir/'status').read_text(errors='replace')
                state=next((x for x in status.splitlines() if x.startswith('State:')),'')
                if '\tZ' in state or ' Z ' in state:
                    running=False; break
            except FileNotFoundError:
                running=False; break
            time.sleep(0.04)
            running=procdir.exists()
        if running:
            try: os.kill(descendant,signal.SIGKILL)
            except ProcessLookupError: pass
        assert not running,f'descendant remained alive after drain-window SIGTERM: pid={descendant}'

print('PASS: Python Patch Tool v6.11.0 collect progress/artifact robustness self-test')
