#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, shutil, signal, subprocess, sys, tempfile, time, zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
TOOLS = HERE.parent
sys.path.insert(0, str(HERE))
import python_patch_runner as runner
from python_patch_batch import load_patch_meta, transaction_compatibility
from python_patch_package_schema import PatchSchemaError, resolve_project_path, validate_manifest


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def install(root: Path) -> None:
    shutil.copytree(TOOLS, root / "tools")
    (root / "tools" / "run_python_patches.sh").chmod(0o755)
    (root / "patchs").mkdir()


def package(path: Path, manifest: dict, script: str) -> None:
    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr("PATCH_TOOL_MANIFEST.json", json.dumps(manifest))
        zf.writestr("patch_apply.py", script)


def run_patch(root: Path, name: str, timeout: int = 30):
    result = root / "result.json"
    env = dict(os.environ)
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    env["PTV_PATCH_RESULT_FILE"] = str(result)
    cp = subprocess.run(
        [str(root / "tools" / "run_python_patches.sh"), "--patch", f"patchs/{name}"],
        cwd=root, text=True, capture_output=True, env=env, timeout=timeout,
    )
    return cp, json.loads(result.read_text())


# Schema accepts the new failure-only command and bounded Git timeout contract,
# and rejects an out-of-range timeout before execution.
schema_manifest={"schema_version":1,"patch":{"id":"schema-on-failure"},"on_failure":{"commands":[{"name":"f","argv":[sys.executable,"-c","pass"],"cwd":".","timeout_seconds":3}]},"git":{"timeout_seconds":300}}
validate_manifest(schema_manifest)
bad_manifest=json.loads(json.dumps(schema_manifest)); bad_manifest["git"]["timeout_seconds"]=1801
try:
    validate_manifest(bad_manifest)
    raise AssertionError("git.timeout_seconds > 1800 was accepted")
except PatchSchemaError:
    pass

# on_failure commands run only after execution failure, after rollback, while
# preserving the original PATCH rc/result as the authoritative failure.
with tempfile.TemporaryDirectory(prefix="ptv6178_on_failure_") as td:
    root = Path(td); install(root)
    target = root / "a.txt"; target.write_text("BASE\n")
    manifest = {
        "schema_version": 1,
        "patch": {"id": "on-failure-order"},
        "targets": ["a.txt"],
        "preflight": {"files": [{"path": "a.txt", "exists": True, "sha256": sha(target)}]},
        "recovery": {"rollback": {"targets": ["a.txt"], "on": ["payload_failure"]}},
        "on_failure": {"commands": [{
            "name": "record restored state",
            "argv": [sys.executable, "-c", "from pathlib import Path; Path('failure_marker.txt').write_text(Path('a.txt').read_text())"],
            "cwd": ".", "timeout_seconds": 10,
        }]},
    }
    package(root / "patchs" / "f.zip", manifest, "from pathlib import Path\nPath('a.txt').write_text('MUTATED\\n')\nraise SystemExit(7)\n")
    cp, data = run_patch(root, "f.zip")
    assert cp.returncode == 7, (cp.stdout, cp.stderr, data)
    assert target.read_text() == "BASE\n"
    assert (root / "failure_marker.txt").read_text() == "BASE\n"
    assert data["rollback"]["status"] == "PASS", data
    assert data["on_failure"]["status"] == "PASS" and data["on_failure"]["rc"] == 0, data
    assert data["rc"] == 7 and data["diagnosis"]["kind"] != "on_failure_failed", data

# A failing failure-command is secondary: it is reported but cannot replace the
# primary PATCH failure rc.
with tempfile.TemporaryDirectory(prefix="ptv6178_on_failure_secondary_") as td:
    root = Path(td); install(root); (root / "a.txt").write_text("BASE\n")
    manifest = {
        "schema_version": 1, "patch": {"id": "on-failure-secondary"}, "targets": ["a.txt"],
        "on_failure": {"commands": [{"name": "secondary failure", "argv": [sys.executable, "-c", "raise SystemExit(9)"], "timeout_seconds": 10}]},
    }
    package(root / "patchs" / "f.zip", manifest, "raise SystemExit(7)\n")
    cp, data = run_patch(root, "f.zip")
    assert cp.returncode == 7 and data["rc"] == 7, (cp.stdout, cp.stderr, data)
    assert data["on_failure"]["status"] == "FAIL" and data["on_failure"]["rc"] == 9, data

# Real exit 124 is not a tool timeout; an actual enforced timeout is marked by
# its own bit. Managed commands are non-interactive (stdin=DEVNULL).
with tempfile.TemporaryDirectory(prefix="ptv6178_outcomes_") as td:
    root = Path(td)
    r1 = runner._run_command_sequence(root, [{"name":"exit124", "argv":[sys.executable,"-c","raise SystemExit(124)"], "timeout_seconds":2}], label="TEST")
    row1 = r1["commands"][0]
    assert row1["rc"] == 124 and row1["timed_out"] is False, r1
    r2 = runner._run_command_sequence(root, [{"name":"timeout", "argv":[sys.executable,"-c","import time; time.sleep(5)"], "timeout_seconds":1}], label="TEST")
    row2 = r2["commands"][0]
    assert row2["rc"] == 124 and row2["timed_out"] is True, r2
    started = time.monotonic()
    r3 = runner._run_command_sequence(root, [{"name":"stdin", "argv":[sys.executable,"-c","input()"], "timeout_seconds":5}], label="TEST")
    assert time.monotonic() - started < 3 and r3["commands"][0]["timed_out"] is False, r3

# A leader cannot return PASS while a same-process-group background child keeps
# running and mutating the project after result publication.
if os.name != "nt":
    with tempfile.TemporaryDirectory(prefix="ptv6178_orphan_") as td:
        root = Path(td)
        code = "import subprocess,sys; subprocess.Popen([sys.executable,'-c',\"import time,pathlib; time.sleep(1); pathlib.Path('orphan.txt').write_text('bad')\"]); raise SystemExit(0)"
        outcome = runner._run_managed_process([sys.executable, "-c", code], cwd=root, env=runner._external_command_env(), timeout=5)
        assert outcome.lingering_descendants and outcome.effective_rc == 125, outcome
        time.sleep(1.3)
        assert not (root / "orphan.txt").exists(), "background descendant escaped command completion"

# Patch Tool's own result/lock control channels are not inherited by untrusted
# payload/project commands.
with tempfile.TemporaryDirectory(prefix="ptv6178_env_") as td:
    root = Path(td)
    old_result = os.environ.get("PTV_PATCH_RESULT_FILE")
    old_token = os.environ.get("PTV_PARENT_MUTATION_LOCK_TOKEN")
    try:
        os.environ["PTV_PATCH_RESULT_FILE"] = "/tmp/internal-result"
        os.environ["PTV_PARENT_MUTATION_LOCK_TOKEN"] = "secret-control-token"
        report = runner._run_command_sequence(root, [{
            "name":"env-boundary",
            "argv":[sys.executable,"-c","import os,pathlib; pathlib.Path('seen.txt').write_text(str(os.getenv('PTV_PATCH_RESULT_FILE'))+'|'+str(os.getenv('PTV_PARENT_MUTATION_LOCK_TOKEN')))"],
            "timeout_seconds":5,
        }], label="TEST")
        assert report["rc"] == 0 and (root / "seen.txt").read_text() == "None|None", report
    finally:
        if old_result is None: os.environ.pop("PTV_PATCH_RESULT_FILE", None)
        else: os.environ["PTV_PATCH_RESULT_FILE"] = old_result
        if old_token is None: os.environ.pop("PTV_PARENT_MUTATION_LOCK_TOKEN", None)
        else: os.environ["PTV_PARENT_MUTATION_LOCK_TOKEN"] = old_token

# Ctrl+C/SIGTERM inside post/on-failure command execution is control flow and
# must propagate; it must not be converted to an ordinary rc=130 command FAIL.
original_run_argv = runner._run_argv
try:
    def interrupt(*_a, **_kw):
        raise KeyboardInterrupt
    runner._run_argv = interrupt
    try:
        runner._run_command_sequence(Path.cwd(), [{"argv":[sys.executable,"-c","pass"]}], label="TEST")
        raise AssertionError("KeyboardInterrupt was swallowed")
    except KeyboardInterrupt:
        pass
finally:
    runner._run_argv = original_run_argv

# Git auto policy hooks are bounded and descendants are contained. This is a
# real hook reproducer on POSIX; Windows coverage is static until native tests.
if os.name != "nt" and shutil.which("git"):
    with tempfile.TemporaryDirectory(prefix="ptv6178_git_hook_") as td:
        root = Path(td)
        subprocess.run(["git","init","-q"],cwd=root,check=True)
        subprocess.run(["git","config","user.email","ptv@example.invalid"],cwd=root,check=True)
        subprocess.run(["git","config","user.name","PTV"],cwd=root,check=True)
        (root/"a.txt").write_text("BASE\n")
        subprocess.run(["git","add","a.txt"],cwd=root,check=True)
        subprocess.run(["git","commit","-qm","base"],cwd=root,check=True)
        before = runner._dirty_paths(root)
        (root/"a.txt").write_text("PATCH\n")
        after = runner._dirty_paths(root)
        hook = root/".git"/"hooks"/"pre-commit"
        hook.write_text("#!/bin/sh\n( sleep 2; printf bad > hook_orphan.txt ) &\nsleep 20\n")
        hook.chmod(0o755)
        manifest={"git":{"add":"changed","commit":"auto","commit_message":"x","timeout_seconds":1,"fail_on_error":True}}
        started=time.monotonic(); rc=runner._run_git_policy(root,manifest,before,after); elapsed=time.monotonic()-started
        assert rc != 0 and elapsed < 8, elapsed
        time.sleep(2.2)
        assert not (root/"hook_orphan.txt").exists(), "Git hook descendant escaped timeout containment"
        staged=subprocess.run(["git","diff","--cached","--name-only"],cwd=root,text=True,capture_output=True,check=True).stdout.strip()
        assert staged == "", staged

# Windows/ADS-style project-relative paths are rejected cross-platform.
with tempfile.TemporaryDirectory(prefix="ptv6178_path_") as td:
    root=Path(td); (root/"ok").write_text("x")
    for bad in ("C:evil.txt", "dir/foo:bar"):
        try:
            resolve_project_path(root,bad,allow_missing=True)
            raise AssertionError(f"unsafe Windows/ADS path accepted: {bad}")
        except PatchSchemaError:
            pass

# Atomic whole-batch rollback cannot safely include failure-only command side
# effects because those side effects are not target-bounded.
with tempfile.TemporaryDirectory(prefix="ptv6178_batch_") as td:
    root=Path(td); (root/"patchs").mkdir()
    m={"schema_version":1,"patch":{"id":"batch-failure-command"},"targets":[],"on_failure":{"commands":[{"name":"x","argv":[sys.executable,"-c","pass"],"timeout_seconds":2}]}}
    package(root/"patchs"/"x.zip",m,"pass\n")
    meta=load_patch_meta(root,"x.zip")
    issues=transaction_compatibility([meta],"batch")
    assert any("on_failure.commands" in x for x in issues), issues

# Failure-only cleanup itself must PASS before independent continuation is
# permitted; a failed cleanup cannot be treated as safely contained merely
# because the tracked project delta looks clean.
import python_patch_queue_dispatcher as dispatcher

# Dispatcher foreground actions (inspect/preview/validate/COLLECT supervisor)
# must not use a bare subprocess.run lifecycle. Timeout/termination contains
# the whole child process group, including a background child.
if os.name != "nt":
    with tempfile.TemporaryDirectory(prefix="ptv6178_foreground_tree_") as td:
        root = Path(td)
        code = "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c',\"import time,pathlib; time.sleep(2); pathlib.Path('escaped.txt').write_text('bad')\"]); time.sleep(30)"
        started = time.monotonic()
        rc = dispatcher._run_foreground_child(root, [sys.executable, "-c", code], timeout=1, label="TEST FOREGROUND")
        assert rc == 124 and time.monotonic() - started < 8, rc
        time.sleep(2.2)
        assert not (root / "escaped.txt").exists(), "dispatcher foreground child escaped timeout containment"

# An unexpected internal exception after payload execution still follows the
# failure contract: configured rollback first, then on_failure; the original
# internal-error rc remains authoritative.
with tempfile.TemporaryDirectory(prefix="ptv6178_internal_failure_") as td:
    root = Path(td); install(root)
    target = root / "a.txt"; target.write_text("BASE\n")
    manifest = {
        "schema_version":1, "patch":{"id":"internal-failure"}, "targets":["a.txt"],
        "preflight":{"files":[{"path":"a.txt","exists":True,"sha256":sha(target)}]},
        "recovery":{"rollback":{"targets":["a.txt"],"on":["post_patch_failure"]}},
        "on_failure":{"commands":[{
            "name":"record rollback state",
            "argv":[sys.executable,"-c","from pathlib import Path; Path('internal_marker.txt').write_text(Path('a.txt').read_text())"],
            "timeout_seconds":5,
        }]},
    }
    package(root / "patchs" / "internal.zip", manifest, "from pathlib import Path\nPath('a.txt').write_text('MUTATED\\n')\n")
    original_post = runner._run_post_patch
    try:
        def explode_post(*_a, **_kw):
            raise RuntimeError("synthetic internal post error")
        runner._run_post_patch = explode_post
        result_file = root / "internal-result.json"
        old_result = os.environ.get("PTV_PATCH_RESULT_FILE")
        os.environ["PTV_PATCH_RESULT_FILE"] = str(result_file)
        try:
            rc = runner._execute_patch(root, root / "patchs" / "internal.zip")
        finally:
            if old_result is None: os.environ.pop("PTV_PATCH_RESULT_FILE", None)
            else: os.environ["PTV_PATCH_RESULT_FILE"] = old_result
    finally:
        runner._run_post_patch = original_post
    data = json.loads(result_file.read_text())
    assert rc == 2 and data["diagnosis"]["kind"] == "internal_error", data
    assert data["rollback"]["status"] == "PASS" and target.read_text() == "BASE\n", data
    assert data["on_failure"]["status"] == "PASS" and (root / "internal_marker.txt").read_text() == "BASE\n", data

base_detail={"rc":7,"patch_result":{"diagnosis":{"kind":"patch_payload_failed"},"partial_modification":{"detected":False,"changed_paths":[],"evidence":"test"}}}
failed_detail=json.loads(json.dumps(base_detail)); failed_detail["patch_result"]["on_failure"]={"status":"FAIL","rc":9}
assert dispatcher._safe_to_continue_after_failure(failed_detail)==(False,"on_failure_commands_failed_or_incomplete")
pass_detail=json.loads(json.dumps(base_detail)); pass_detail["patch_result"]["on_failure"]={"status":"PASS","rc":0}
assert dispatcher._safe_to_continue_after_failure(pass_detail)==(True,"on_failure_commands_passed_project_unchanged")

# Static Windows contract: managed children and dispatcher/collector execution
# use process groups/tree termination. Native verification is separate.
launcher_text=(TOOLS/"run_python_patches.sh").read_text()
assert "sys.version_info >= (3, 10)" in launcher_text and "Python 3.10+" in launcher_text
runner_text=(HERE/"python_patch_runner.py").read_text()
dispatcher_text=(HERE/"python_patch_queue_dispatcher.py").read_text()
collector_text=(HERE/"python_patch_collect_progress_v6_7.py").read_text()
assert "CREATE_NEW_PROCESS_GROUP" in runner_text and "SIGBREAK" in runner_text and "taskkill" in runner_text.lower()
assert "CREATE_NEW_PROCESS_GROUP" in dispatcher_text and "CTRL_BREAK_EVENT" in dispatcher_text and "taskkill" in dispatcher_text.lower()
assert "_run_foreground_child(root, cmd" in dispatcher_text and 'label="COLLECT"' in dispatcher_text
assert "CREATE_NEW_PROCESS_GROUP" in collector_text and "taskkill" in collector_text.lower()

print("PASS: v6.18.6 failure-only commands and script-execution audit regressions")
