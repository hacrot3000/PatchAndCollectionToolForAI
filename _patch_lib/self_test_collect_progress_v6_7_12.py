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

assert m.VERSION == "6.7.12"
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

quoted_result = '/tmp/result with spaces.zip'
quoted_request = '/tmp/patchs/patched/request with spaces.zip'
parsed = m._extract_collect_completion([
    f'ZIP: "{quoted_result}"',
    f'REQUEST: "{quoted_request}"',
])
assert parsed[0] == quoted_result, parsed
assert parsed[1] == quoted_request, parsed
parsed_single = m._extract_collect_completion([f"ZIP: '{quoted_result}'"])
assert parsed_single[0] == quoted_result, parsed_single

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
with tempfile.TemporaryDirectory(prefix="ptprog_v678_") as td:
    root = Path(td)
    collector = root / "bad_bytes_collector.py"
    result_zip = root / "bad_bytes_result.zip"
    collector.write_text(
        "import os,zipfile\n"
        f"with zipfile.ZipFile({str(result_zip)!r}, 'w') as z: z.writestr('ok.txt', 'ok')\n"
        f"os.write(1, b'begin\\n\\xffbad\\nPASS done\\nZIP: {str(result_zip)}\\n')\n",
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
    assert "output=4 lines" in cp.stdout, cp.stdout
    assert "[PRIMARY - UPLOAD THIS FILE]" in cp.stdout, cp.stdout


# If an external task runner signals only the supervisor PID, the collector
# process group must be terminated too instead of becoming an orphan. Cover
# terminal/task-close signals in addition to Ctrl+C/SIGTERM.
if os.name == "posix":
    # Use SIGTERM for the live integration test. Firing SIGHUP/SIGQUIT at
    # nested supervisors can destabilize some CI/task harnesses themselves.
    # Static ordering assertions below lock the additional handlers/mapping.
    stop_signals = [signal.SIGTERM]
    for stop_signal in stop_signals:
        with tempfile.TemporaryDirectory(prefix=f"ptprog_signal_v6710_{stop_signal}_") as td:
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
            os.kill(sup.pid, stop_signal)
            out, err = sup.communicate(timeout=6)
            assert sup.returncode == 128 + int(stop_signal), (stop_signal, sup.returncode, out, err)
            child_proc = Path('/proc') / str(child_pid)
            end_wait = time.monotonic() + 2.0
            running = child_proc.exists()
            while running and time.monotonic() < end_wait:
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
            assert not running, f"collector child still running after supervisor signal {stop_signal}: pid={child_pid}"


progress_source = p.read_text(encoding='utf-8')
assert 'getattr(signal, "SIGHUP", None)' in progress_source
assert 'getattr(signal, "SIGQUIT", None)' in progress_source
assert 'SIGQUIT commonly triggers a core dump' in progress_source
assert progress_source.index('signal.signal(sig, _forward_stop)') < progress_source.index('proc = subprocess.Popen('), 'stop handlers must be installed before collector spawn'

# Regression: legacy collectors may emit the result archive twice: once with a
# ZIP label and once as a bare path. The supervisor must expose exactly one
# highlighted upload path and keep the archived request clearly separate.
with tempfile.TemporaryDirectory(prefix="ptprog_result_v6710_") as td:
    root = Path(td)
    result = root / "artifacts" / "patch_tool_code_collections" / "m3_client_cjk_to_english_inventory_v1.zip"
    request = root / "patchs" / "patched" / "m3_collect_cjk_to_english_inventory_v1.zip"
    result.parent.mkdir(parents=True, exist_ok=True)
    request.parent.mkdir(parents=True, exist_ok=True)
    import zipfile
    with zipfile.ZipFile(result, 'w') as z:
        z.writestr('evidence.txt', 'ok')
    with zipfile.ZipFile(request, 'w') as z:
        z.writestr('CODE_COLLECTION_REQUEST_test.json', '{"actions":[{"type":"overview"}]}')
    collector = root / "duplicate_result_collector.py"
    collector.write_text(
        "import sys\n"
        f"print('ZIP        : {result}')\n"
        f"print(r'{result}')\n"
        f"print('REQUEST     : {request}')\n",
        encoding="utf-8",
    )
    cp = subprocess.run(
        [
            sys.executable, str(p),
            "--project-root", str(root),
            "--collector", str(collector),
            "--", "request", "dummy.zip",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=10,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert "[PRIMARY - UPLOAD THIS FILE]" in cp.stdout, cp.stdout
    assert "Destination: ChatGPT / AI server" in cp.stdout, cp.stdout
    assert cp.stdout.count(str(result)) == 1, cp.stdout
    assert "ZIP        :" not in cp.stdout, cp.stdout
    assert cp.stdout.count(str(request)) == 1, cp.stdout
    assert "[INFO] REQUEST ARCHIVED:" in cp.stdout, cp.stdout



# Quoted paths with spaces are common in shell-oriented collectors and must be
# normalized without losing the one-path upload contract.
with tempfile.TemporaryDirectory(prefix="ptprog_quoted_v6710_") as td:
    root = Path(td)
    result = root / "artifacts" / "result with spaces.zip"
    request = root / "patchs" / "patched" / "request with spaces.zip"
    result.parent.mkdir(parents=True)
    request.parent.mkdir(parents=True)
    import zipfile
    with zipfile.ZipFile(result, 'w') as z:
        z.writestr('ok.txt', 'ok')
    with zipfile.ZipFile(request, 'w') as z:
        z.writestr('CODE_COLLECTION_REQUEST_x.json', '{"actions":[{"type":"overview"}]}')
    collector = root / "quoted_collector.py"
    collector.write_text(
        f'print(\'ZIP: "{result}"\')\n'
        f'print(\'REQUEST: "{request}"\')\n',
        encoding='utf-8',
    )
    cp = subprocess.run(
        [sys.executable, str(p), '--project-root', str(root), '--collector', str(collector), '--', 'request', 'dummy.zip'],
        text=True, capture_output=True, timeout=10,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert cp.stdout.count(str(result)) == 1, cp.stdout
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout, cp.stdout

# rc=0 from the private collector is not enough: without a real valid result ZIP
# there is nothing the user can upload, so the supervisor must fail closed.
with tempfile.TemporaryDirectory(prefix="ptprog_missing_artifact_v6710_") as td:
    root = Path(td)
    missing = root / 'artifacts' / 'missing.zip'
    collector = root / 'missing_collector.py'
    collector.write_text(f"print('ZIP: {missing}')\n", encoding='utf-8')
    cp = subprocess.run(
        [sys.executable, str(p), '--project-root', str(root), '--collector', str(collector), '--', 'request', 'dummy.zip'],
        text=True, capture_output=True, timeout=10,
    )
    assert cp.returncode == 2, (cp.returncode, cp.stdout, cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' not in cp.stdout, cp.stdout
    assert 'does not exist' in cp.stdout, cp.stdout
    assert 'completion contract failed' in cp.stderr, cp.stderr
    assert '✗ COLLECT | rc=2' in cp.stdout, cp.stdout
    assert '✓ COLLECT | rc=0' not in cp.stdout, cp.stdout


# Result metadata must not depend on the bounded failure tail. A verbose
# collector may report the ZIP and then print hundreds of later diagnostic
# lines; rc=0 still has a valid upload artifact and must remain PASS.
with tempfile.TemporaryDirectory(prefix="ptprog_long_tail_v6711_") as td:
    root = Path(td)
    result = root / 'artifacts' / 'early-result.zip'
    result.parent.mkdir(parents=True)
    import zipfile
    with zipfile.ZipFile(result, 'w') as z:
        z.writestr('ok.txt', 'ok')
    collector = root / 'long_tail_collector.py'
    collector.write_text(
        f"print('ZIP: {result}')\n"
        "for i in range(220): print(f'diagnostic line {i}')\n",
        encoding='utf-8',
    )
    cp = subprocess.run(
        [sys.executable, str(p), '--project-root', str(root), '--collector', str(collector), '--', 'request', 'dummy.zip'],
        text=True, capture_output=True, timeout=10,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout, cp.stdout
    assert cp.stdout.count(str(result)) == 1, cp.stdout
    assert 'completion contract failed' not in cp.stderr, cp.stderr


# Result and request completion metadata must not share a bounded FIFO.  A
# result can be reported first, followed by many request/archive metadata lines
# and then a long diagnostic tail; the valid result must remain authoritative.
with tempfile.TemporaryDirectory(prefix="ptprog_completion_eviction_v6712_") as td:
    root = Path(td)
    result = root / 'artifacts' / 'early-persistent-result.zip'
    result.parent.mkdir(parents=True)
    import zipfile
    with zipfile.ZipFile(result, 'w') as z:
        z.writestr('ok.txt', 'ok')
    patched = root / 'patchs' / 'patched'
    patched.mkdir(parents=True)
    request_paths=[]
    for i in range(24):
        req=patched / f'request_{i}.zip'
        with zipfile.ZipFile(req,'w') as z:
            z.writestr('CODE_COLLECTION_REQUEST_x.json','{"actions":[{"type":"overview"}]}')
        request_paths.append(req)
    collector = root / 'completion_eviction_collector.py'
    lines=[f"print('ZIP: {result}')"]
    lines.extend(f"print('REQUEST: {req}')" for req in request_paths)
    lines.append("[print(f'diagnostic {i}') for i in range(220)]")
    collector.write_text('\n'.join(lines)+'\n',encoding='utf-8')
    cp=subprocess.run(
        [sys.executable,str(p),'--project-root',str(root),'--collector',str(collector),'--','request','dummy.zip'],
        text=True,capture_output=True,timeout=10,
    )
    assert cp.returncode==0,(cp.returncode,cp.stdout,cp.stderr)
    assert '[PRIMARY - UPLOAD THIS FILE]' in cp.stdout,cp.stdout
    assert cp.stdout.count(str(result))==1,cp.stdout
    assert str(request_paths[-1]) in cp.stdout,cp.stdout

print('PASS: Python Patch Tool v6.7.12 collect progress robustness self-test')
