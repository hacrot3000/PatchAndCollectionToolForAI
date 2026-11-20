#!/usr/bin/env python3
"""Behavioral regression: a PATCH may run old scripts, never new ones it creates."""
from __future__ import annotations

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
TOOLS = HERE.parent
sys.path.insert(0, str(HERE))
from python_patch_script_guard import ScriptOriginGuard, NewScriptExecutionError, command_script_path

if os.name == 'nt':
    print('SKIP: POSIX-only executable script integration; Windows guard uses same runtime logic')
    raise SystemExit(0)

ENV = dict(os.environ)
ENV["PYTHONDONTWRITEBYTECODE"] = "1"
ENV["PTV_DISABLE_LIVE_STATUS"] = "1"


def install(root: Path) -> None:
    shutil.copytree(TOOLS, root / "tools")
    (root / "tools" / "run_python_patches.sh").chmod(0o755)
    (root / "patchs").mkdir()


def pack(root: Path, name: str, payload: str | None, *, commands=None, on_failure=None,
         targets=None) -> None:
    manifest = {
        "schema_version": 1,
        "patch": {"id": name},
        "execution": {"timeout_seconds": 25},
    }
    if targets:
        manifest["targets"] = targets
    if commands is not None:
        manifest["post_patch"] = {
            "commands": [{"name": "test", "argv": args, "cwd": ".",
                          "timeout_seconds": 10} for args in commands],
            "run_when_no_changes": payload is None,
        }
        if payload is None:
            manifest["post_patch"]["no_change_reason"] = (
                "Run an already existing script from a separate, completed PATCH."
            )
    if on_failure is not None:
        manifest["on_failure"] = {
            "commands": [{"name": "failure test", "argv": args, "cwd": ".",
                          "timeout_seconds": 10} for args in on_failure],
        }
    with zipfile.ZipFile(root / "patchs" / (name + ".zip"), "w") as z:
        z.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
        if payload is not None:
            z.writestr("patch_apply.py", payload)


def run(root: Path, name: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        [str(root / "tools" / "run_python_patches.sh"), "--patch",
         "patchs/" + name + ".zip"],
        cwd=root, capture_output=True, text=True, env=ENV, timeout=65,
    )


def created_script_payload(ext: str = ".sh") -> str:
    return (
        "from pathlib import Path\n"
        "p = Path('scripts/new" + ext + "')\n"
        "p.parent.mkdir(parents=True, exist_ok=True)\n"
        "p.write_text('#!/bin/sh\\nprintf BAD > ran-created-script.txt\\n')\n"
        "p.chmod(0o755)\n"
    )


# Missing direct script references must be rejected before payload mutation,
# whether or not the author declared the new script in manifest.targets.
for argv in (
    ["bash", "scripts/new.sh"],
    ["sh", "./scripts/new.sh"],
    ["env", "bash", "scripts/new.sh"],
    ["./scripts/new.sh"],
    ["python3", "-u", "scripts/new.py"],
):
    with tempfile.TemporaryDirectory(prefix="ptv_new_script_rejected_") as td:
        root = Path(td)
        install(root)
        script_ext = ".py" if "python3" in argv else ".sh"
        pack(root, "reject", created_script_payload(script_ext),
             commands=[argv], targets=["scripts/new" + script_ext])
        cp = run(root, "reject")
        assert cp.returncode != 0, (argv, cp.stdout, cp.stderr)
        # Direct file execution is already blocked by the existing executable
        # availability preflight before the new origin guard is reached.
        expected = ("command_missing" if argv[0].startswith("./")
                    else "new_script_execution_forbidden")
        assert expected in cp.stdout + cp.stderr, (
            argv, cp.stdout, cp.stderr
        )
        assert not (root / "scripts" / ("new" + script_ext)).exists(), argv
        assert not (root / "ran-created-script.txt").exists(), argv


# on_failure.commands is subject to the same preflight, even though it would
# only execute on a failed payload.
with tempfile.TemporaryDirectory(prefix="ptv_new_script_failure_") as td:
    root = Path(td)
    install(root)
    pack(root, "failure", created_script_payload(),
         on_failure=[["bash", "scripts/new.sh"]])
    cp = run(root, "failure")
    assert cp.returncode != 0
    assert "new_script_execution_forbidden" in cp.stdout + cp.stderr
    assert not (root / "scripts" / "new.sh").exists()


# An old script may run after a source-changing PATCH.
with tempfile.TemporaryDirectory(prefix="ptv_preexisting_script_") as td:
    root = Path(td)
    install(root)
    (root / "scripts").mkdir()
    (root / "scripts" / "check.sh").write_text(
        "#!/bin/sh\nprintf PASS > script-marker.txt\n"
    )
    pack(root, "existing", "from pathlib import Path\nPath('source.txt').write_text('new')\n",
         commands=[["bash", "scripts/check.sh"]], targets=["source.txt"])
    cp = run(root, "existing")
    assert cp.returncode == 0, (cp.stdout, cp.stderr)
    assert (root / "script-marker.txt").read_text() == "PASS"


# Separate completed PATCHes: first creates the script, second runs it.
with tempfile.TemporaryDirectory(prefix="ptv_two_patch_script_") as td:
    root = Path(td)
    install(root)
    pack(root, "create", created_script_payload(), targets=["scripts/new.sh"])
    first = run(root, "create")
    assert first.returncode == 0, (first.stdout, first.stderr)
    assert (root / "scripts" / "new.sh").is_file()
    pack(root, "execute", None, commands=[["bash", "scripts/new.sh"]])
    second = run(root, "execute")
    assert second.returncode == 0, (second.stdout, second.stderr)
    assert (root / "ran-created-script.txt").read_text() == "BAD"


# Internal pure helper: pre-existing scripts are permitted, new ones refused
# by the runtime guard even when files appear after the baseline snapshot.
with tempfile.TemporaryDirectory(prefix="ptv_script_runtime_") as td:
    root = Path(td)
    old = root / "old.sh"
    old.write_text("#!/bin/sh\nexit 0\n")
    commands = [
        {"argv": ["bash", "old.sh"], "cwd": "."},
        {"argv": ["bash", "new.sh"], "cwd": "."},
    ]
    guard = ScriptOriginGuard(root, commands)
    assert guard.missing_paths() == [root / "new.sh"]
    guard.check(["bash", "old.sh"], root)
    (root / "new.sh").write_text("#!/bin/sh\nexit 0\n")
    try:
        guard.check(["bash", "new.sh"], root)
    except NewScriptExecutionError as exc:
        assert exc.kind == "new_script_execution_forbidden"
    else:
        raise AssertionError("a script generated after the snapshot was executed")

print("PASS: preflight/runtime script-origin guard and two-PATCH workflow")
