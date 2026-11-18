#!/usr/bin/env python3
"""PATCH remote SSH package integration with a non-network TaskDeck bridge stub.

The real SSH allowlist and saved-profile resolution have Go unit tests.
This test proves the Python PATCH layer's dispatch/preflight/format boundaries.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
if os.name == "nt":
    print("SKIP: POSIX TaskDeck stub integration; Go SSH whitelist tests run on Windows")
    raise SystemExit(0)


def pack(root: Path, label: str, manifest: dict, extra: dict[str, str] | None = None) -> Path:
    file = root / "patchs" / (label + ".zip")
    with zipfile.ZipFile(file, "w") as z:
        z.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
        for path, contents in (extra or {}).items():
            z.writestr(path, contents)
    return file


def make_manifest(root: Path, command: list[str] | None = None) -> dict:
    return {
        "schema_version": 1,
        "patch": {"id": "remote-ssh-test"},
        "project": {"key": root.name, "id": "00000000-0000-4000-8000-000000000001"},
        "remote_ssh": {
            "profile_name": "Test SSH profile",
            "argv": command or ["ls", "-lah", "."],
            "timeout_seconds": 10,
        },
    }


def execute(root: Path, zipfile_path: Path, stub: Path, log: Path) -> subprocess.CompletedProcess:
    env = dict(os.environ)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    env["PTV_DISABLE_LIVE_STATUS"] = "1"
    env["PTV_TASKDECK_EXECUTABLE"] = str(stub)
    env["PATCH_SSH_STUB_LOG"] = str(log)
    return subprocess.run(
        [sys.executable, "-S", str(HERE / "python_patch_runner.py"),
         "--patch", str(zipfile_path)],
        cwd=root, env=env, capture_output=True, text=True, timeout=45,
    )


with tempfile.TemporaryDirectory(prefix="ptv-ssh-remote-") as td:
    parent = Path(td)
    root = parent / "workspace"
    root.mkdir()
    (root / "patchs").mkdir()
    (root / ".python_patch_tool.json").write_text(json.dumps({
        "project": {"key": root.name, "id": "00000000-0000-4000-8000-000000000001"}
    }), encoding="utf-8")
    log = parent / "remote_calls.jsonl"
    stub = parent / "taskdeck-stub"
    stub.write_text(
        "#!" + sys.executable + "\n"
        "import json,os,sys\n"
        "if sys.argv[1:3] != ['patch-ssh-command','--workspace']:\n"
        "    sys.exit(50)\n"
        "req=json.load(sys.stdin)\n"
        "with open(os.environ['PATCH_SSH_STUB_LOG'],'a',encoding='utf-8') as fh:\n"
        "    fh.write(json.dumps(req)+'\\n')\n"
        "if req.get('profile_name') != 'Test SSH profile' or req.get('argv') != ['ls','-lah','.']:\n"
        "    sys.exit(51)\n"
        "print('remote-dry-run-ok' if req['dry_run'] else 'remote-command-output')\n",
        encoding="utf-8"
    )
    stub.chmod(0o700)

    good = pack(root, "good", make_manifest(root))
    result = execute(root, good, stub, log)
    assert result.returncode == 0, (result.stdout, result.stderr)
    calls = [json.loads(row) for row in log.read_text().splitlines()]
    assert [row["dry_run"] for row in calls] == [True, False], calls
    assert "remote-command-output" in result.stdout

    # No source mutation or helper scripts may accompany the remote command.
    for i, extra in enumerate((
        {"patch_apply.py": "print('unauthorized source payload')"},
        {"scripts/hidden.sh": "echo unauthorized"},
        {"PATCH_TOOL_OPS.json": '{"ops":[]}'},
    )):
        before = len(log.read_text().splitlines())
        package = pack(root, "reject-extra-" + str(i), make_manifest(root), extra)
        cp = execute(root, package, stub, log)
        assert cp.returncode != 0, (cp.stdout, cp.stderr)
        assert len(log.read_text().splitlines()) == before, extra

    # Remote command cannot be smuggled into a normal source-changing PATCH.
    forbidden = make_manifest(root)
    forbidden["post_patch"] = {"commands": [{"argv": ["pwd"]}]}
    before = len(log.read_text().splitlines())
    bad = pack(root, "mixed", forbidden)
    cp = execute(root, bad, stub, log)
    assert cp.returncode != 0 and "remote_ssh_contract_invalid" in cp.stdout + cp.stderr
    assert len(log.read_text().splitlines()) == before

print("PASS: PATCH remote SSH manifest-only dispatch, preflight and isolation")
