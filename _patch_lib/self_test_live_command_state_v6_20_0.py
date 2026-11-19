#!/usr/bin/env python3
"""Live PATCH command integration: assertions MUST run while child still runs.

Unlike the end-of-run History tests, this checks that TaskDeck's machine
channel exposes QUEUE/RUNNING before the process completes.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import select
import shutil
import subprocess
import tempfile
import time
import zipfile

HERE = Path(__file__).resolve().parent
if os.name == "nt":
    print("SKIP: POSIX live FD relay integration")
    raise SystemExit(0)

with tempfile.TemporaryDirectory(prefix="ptv_command_live_") as td:
    root = Path(td)
    shutil.copytree(HERE.parent, root / "tools")
    (root / "tools" / "run_python_patches.sh").chmod(0o755)
    (root / "patchs").mkdir()
    pipe = root / "live_pipe.txt"
    os.mkfifo(pipe)
    manifest = {
        "schema_version": 1,
        "patch": {"id": "live-child-command"},
        "post_patch": {
            "run_when_no_changes": True,
            "no_change_reason": "Verify live command status events before run completion.",
            "commands": [
                {"name": "Read from slow FIFO", "argv": ["head", "-n", "1", "live_pipe.txt"],
                 "cwd": ".", "timeout_seconds": 15},
            ],
        },
    }
    with zipfile.ZipFile(root / "patchs" / "live.zip", "w") as zf:
        zf.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
    read_fd, write_fd = os.pipe()
    env = dict(os.environ)
    env.update({"TASKDECK_PATCH_EVENT_FD": str(write_fd),
                "PTV_DISABLE_LIVE_STATUS": "1", "PYTHONDONTWRITEBYTECODE": "1"})
    proc = subprocess.Popen(
        [str(root / "tools" / "run_python_patches.sh"),
         "run", "--patch", "patchs/live.zip", "-y"],
        cwd=root, env=env, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, pass_fds=(write_fd,),
    )
    os.close(write_fd)
    pending = b""
    seen = []
    saw_concurrent = False
    deadline = time.monotonic() + 25
    try:
        while time.monotonic() < deadline:
            if not select.select([read_fd], [], [], 0.25)[0]:
                if proc.poll() is not None:
                    break
                continue
            chunk = os.read(read_fd, 65536)
            if not chunk:
                break
            pending += chunk
            while b"\n" in pending:
                line, pending = pending.split(b"\n", 1)
                if not line.strip():
                    continue
                event = json.loads(line)
                if event.get("type") != "command_state":
                    continue
                seen.append(event)
                statuses = [item["command"]["status"] for item in seen]
                if statuses[:2] == ["QUEUE", "RUNNING"]:
                    if proc.poll() is None:
                        saw_concurrent = True
                    break
            if saw_concurrent:
                break
        assert saw_concurrent, (
            "No QUEUE + RUNNING states while PATCH subprocess was still active. "
            f"seen={seen!r}; returncode={proc.poll()!r}"
        )
        live_journals = list((root / "artifacts" / "patch_tool" / "runs").glob(
            "*/command_items/*/command_logs/command_states.json"
        ))
        assert len(live_journals) == 1, live_journals
        current = json.loads(live_journals[0].read_text(encoding="utf-8"))
        assert current["format"] == "taskdeck.patch.command-states.v1", current
        assert current["commands"][0]["status"] == "RUNNING", current
        assert proc.poll() is None, "command journal was written only after run finished"
        # Release head only AFTER observing RUNNING in the frontend event pipe.
        with pipe.open("w", encoding="utf-8") as stream:
            stream.write("alpha\n")
    finally:
        os.close(read_fd)
        if not saw_concurrent and proc.poll() is None:
            proc.kill()
        try:
            output, _ = proc.communicate(timeout=30)
        except subprocess.TimeoutExpired:
            proc.kill()
            output, _ = proc.communicate(timeout=5)
            raise
    assert proc.returncode == 0, output.decode("utf-8", "replace")[-6000:]
    state = json.loads((root / "artifacts" / "patch_tool" / "LAST_RUN.json").read_text())
    own = next(x for x in (state.get("details") or state.get("results") or [])
               if x.get("name") == "live.zip")
    commands = own["patch_result"]["commands"]
    assert [x["status"] for x in commands] == ["PASS"], commands
    assert all((root / x["log_path"]).is_file() for x in commands), commands
print("PASS: native command-state events were visible while a slow PATCH was executing")
