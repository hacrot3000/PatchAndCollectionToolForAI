#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
LAUNCHER = HERE.parent / "run_python_patches.sh"
PROGRESS = HERE / "python_patch_collect_progress_v6_7.py"
COMPAT = HERE / "python_patch_collect_compat.py"


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def make_request(path: Path, data: dict) -> None:
    member = path.name[:-4] + ".json"
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr(member, json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def make_project(root: Path) -> Path:
    tools = root / "tools"
    lib = tools / "_patch_lib"
    lib.mkdir(parents=True)
    (tools / "run_python_patches.sh").write_bytes(LAUNCHER.read_bytes())
    (tools / "run_python_patches.sh").chmod(0o755)
    (lib / "python_patch_collect_progress_v6_7.py").write_bytes(PROGRESS.read_bytes())
    (lib / "python_patch_collect_compat.py").write_bytes(COMPAT.read_bytes())
    (root / "patchs").mkdir()
    bindir = root / "bin"
    bindir.mkdir()
    py = bindir / "python3"
    py.write_text("#!/usr/bin/env bash\nexec " + shlex.quote(sys.executable) + " -S \"$@\"\n", encoding="utf-8")
    py.chmod(0o755)
    return tools / "run_python_patches.sh"


def run_collect(root: Path, launcher: Path, request_name: str) -> subprocess.CompletedProcess[str]:
    env = dict(os.environ)
    env["PATH"] = str(root / "bin") + os.pathsep + env.get("PATH", "")
    return subprocess.run(
        [str(launcher), "collect", "request", f"patchs/{request_name}"],
        cwd=root,
        env=env,
        text=True,
        capture_output=True,
        timeout=15,
    )


# Exact shape of CODE_COLLECTION_REQUEST_ota_v1091_transport_rebase_20260808_2204.zip:
# one pack action with four project-relative OTA transport files.
with tempfile.TemporaryDirectory(prefix="ptv6110_pack_") as td:
    root = Path(td)
    launcher = make_project(root)
    payloads = {
        "main-esp32c3/main/ota/app_ota_ble_transport.c": b"/* transport c */\nint transport_v1091(void){return 1091;}\n",
        "main-esp32c3/main/ota/app_ota_ble_transport.h": b"#pragma once\nint transport_v1091(void);\n",
        "main-esp32c3/main/ota/app_gateway_ota_client.h": b"#pragma once\n/* gateway client */\n",
        "main-esp32c3/main/ota/app_gateway_ota_ble_stream.inc": b"/* stream include */\n",
    }
    for rel, content in payloads.items():
        p = root / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(content)

    request_name = "CODE_COLLECTION_REQUEST_ota_v1091_transport_rebase_20260808_2204.zip"
    request = root / "patchs" / request_name
    request_data = {
        "id": "ota-v1091-transport-rebase",
        "title": "Exact current OTA transport source for v10B.26.109.1 rebase after BEGIN precheck anchor failure",
        "actions": [{"type": "pack", "paths": list(payloads)}],
    }
    make_request(request, request_data)
    cp = run_collect(root, launcher, request_name)
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert "[PRIMARY - UPLOAD THIS FILE]" in cp.stdout, cp.stdout
    assert "ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER" in cp.stdout, cp.stdout
    assert not request.exists(), "successful pack request remained queued"
    archived = root / "patchs" / "patched" / request_name
    assert archived.is_file(), archived
    results = list((root / "artifacts" / "patch_tool_code_collections").glob("CODE_COLLECTION_RESULT_ota-v1091-transport-rebase_*.zip"))
    assert len(results) == 1, results
    result = results[0]
    assert cp.stdout.count(str(result)) == 1, cp.stdout
    with zipfile.ZipFile(result) as zf:
        assert zf.testzip() is None
        manifest = json.loads(zf.read("COLLECTION_MANIFEST.json"))
        assert manifest["tool_version"] == "6.11.0", manifest
        assert manifest["request_id"] == request_data["id"]
        assert manifest["action"] == "pack"
        assert manifest["file_count"] == 4
        by_path = {entry["path"]: entry for entry in manifest["files"]}
        assert set(by_path) == set(payloads)
        for rel, content in payloads.items():
            entry = by_path[rel]
            assert entry["archive_path"] == f"files/{rel}"
            assert entry["size"] == len(content)
            assert entry["sha256"] == sha256(content)
            assert zf.read(entry["archive_path"]) == content


# Pack path safety is fail-closed: no traversal, absolute path, symlink,
# directory, or missing source may become a collection artifact.
with tempfile.TemporaryDirectory(prefix="ptv6110_pack_bad_") as td:
    root = Path(td)
    launcher = make_project(root)
    (root / "safe.txt").write_text("safe", encoding="utf-8")
    (root / "link.txt").symlink_to(root / "safe.txt")
    cases = [
        "../outside.txt",
        "/etc/passwd",
        "link.txt",
        "missing.txt",
    ]
    for i, bad in enumerate(cases, 1):
        request_name = f"CODE_COLLECTION_REQUEST_bad_pack_{i}.zip"
        request = root / "patchs" / request_name
        make_request(request, {"id": f"bad-{i}", "actions": [{"type": "pack", "paths": [bad]}]})
        cp = run_collect(root, launcher, request_name)
        assert cp.returncode == 2, (bad, cp.returncode, cp.stdout, cp.stderr)
        assert "COLLECT FAILED" in cp.stdout, (bad, cp.stdout)
        assert request.exists(), (bad, "failed request was archived")
        assert not list((root / "artifacts" / "patch_tool_code_collections").glob(f"CODE_COLLECTION_RESULT_bad-{i}_*.zip")), bad
        request.unlink()


# Non-pack requests are never partially interpreted by the overlay. They are
# delegated intact to the installed private collector, preserving old behavior.
with tempfile.TemporaryDirectory(prefix="ptv6110_pack_delegate_") as td:
    root = Path(td)
    (root / "patchs").mkdir()
    request = root / "patchs" / "CODE_COLLECTION_REQUEST_delegate.zip"
    make_request(request, {"id": "delegate", "actions": [{"type": "overview"}]})
    delegate = root / "delegate.py"
    marker = root / "delegated.json"
    delegate.write_text(
        "import json,sys\nfrom pathlib import Path\n"
        f"Path({str(marker)!r}).write_text(json.dumps(sys.argv[1:]))\n"
        "raise SystemExit(7)\n",
        encoding="utf-8",
    )
    env = dict(os.environ)
    env["PTV_PRIVATE_COLLECTOR"] = str(delegate)
    cp = subprocess.run(
        [sys.executable, "-S", str(COMPAT), "--project-root", str(root), "request", "patchs/CODE_COLLECTION_REQUEST_delegate.zip"],
        cwd=root,
        env=env,
        text=True,
        capture_output=True,
        timeout=10,
    )
    assert cp.returncode == 7, (cp.returncode, cp.stdout, cp.stderr)
    argv = json.loads(marker.read_text())
    assert argv[-2:] == ["request", "patchs/CODE_COLLECTION_REQUEST_delegate.zip"], argv
    assert request.exists(), "delegated request must remain under delegate control"

print("PASS: v6.11.0 overlay natively supports exact-file pack COLLECT requests and safely delegates all other actions")
