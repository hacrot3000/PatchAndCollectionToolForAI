#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import zipfile
import time
from datetime import datetime, timezone

from python_patch_utils import PatchFailure, finish_failure, run_ops
from python_patch_package_schema import PatchSchemaError, run_preflight, sha256_file

VERSION = "6.13.0"


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _safe_archive_name(name: str) -> PurePosixPath:
    text = name.replace("\\", "/")
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise ValueError(f"unsafe archive member: {name}")
    return rel


def _safe_extract_zip(path: Path, dest: Path) -> None:
    with zipfile.ZipFile(path) as zf:
        for info in zf.infolist():
            if info.is_dir():
                continue
            rel = _safe_archive_name(info.filename)
            mode = (info.external_attr >> 16) & 0xFFFF
            if stat.S_ISLNK(mode):
                raise ValueError(f"archive symlink is not allowed: {info.filename}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            with zf.open(info) as src, target.open("wb") as out:
                shutil.copyfileobj(src, out)


def _safe_extract_tar(path: Path, dest: Path) -> None:
    with tarfile.open(path, "r:*") as tf:
        for member in tf.getmembers():
            if member.isdir():
                continue
            rel = _safe_archive_name(member.name)
            if not member.isfile():
                raise ValueError(f"archive non-regular member is not allowed: {member.name}")
            src = tf.extractfile(member)
            if src is None:
                raise ValueError(f"cannot read archive member: {member.name}")
            target = dest.joinpath(*rel.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            with src, target.open("wb") as out:
                shutil.copyfileobj(src, out)


def _read_json(path: Path, label: str) -> dict:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise ValueError(f"invalid {label}: {type(exc).__name__}: {exc}") from exc
    if not isinstance(data, dict):
        raise ValueError(f"{label} root must be an object")
    return data


def _find_payload(extracted: Path) -> tuple[dict, str, Path]:
    manifest_path = extracted / "PATCH_TOOL_MANIFEST.json"
    manifest = _read_json(manifest_path, "PATCH_TOOL_MANIFEST.json") if manifest_path.is_file() else {}
    ops = extracted / "PATCH_TOOL_OPS.json"
    py_files: list[Path] = []
    for path in extracted.rglob("*.py"):
        rel = path.relative_to(extracted)
        if not path.is_file() or "__MACOSX" in rel.parts or "resources" in rel.parts:
            continue
        py_files.append(path)
    if ops.is_file() and py_files:
        raise ValueError("package contains both PATCH_TOOL_OPS.json and Python patch entrypoint")
    if ops.is_file():
        return manifest, "ops", ops
    if len(py_files) != 1:
        raise ValueError(f"package must contain exactly one Python patch entrypoint when OPS is absent (found {len(py_files)})")
    return manifest, "python", py_files[0]


def _git_bytes(root: Path, args: list[str], *, timeout: int = 30) -> bytes:
    proc = subprocess.run(["git", *args], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
    if proc.returncode != 0:
        return b""
    return proc.stdout


def _git_worktree_fingerprint(root: Path) -> str | None:
    if not (root / ".git").exists():
        return None
    h = hashlib.sha256()
    h.update(_git_bytes(root, ["diff", "--binary", "HEAD", "--", "."], timeout=60))
    raw = _git_bytes(root, ["ls-files", "--others", "--exclude-standard", "-z"], timeout=30)
    for name in raw.split(b"\0"):
        if not name:
            continue
        try:
            rel = name.decode("utf-8", errors="surrogateescape")
            path = root / rel
            if path.is_file() and not path.is_symlink():
                h.update(name); h.update(b"\0"); h.update(_sha256(path).encode()); h.update(b"\0")
        except OSError:
            continue
    return h.hexdigest()


def _dirty_paths(root: Path) -> dict[str, str]:
    if not (root / ".git").exists():
        return {}
    proc = subprocess.run(
        ["git", "status", "--porcelain=v1", "-z", "--untracked-files=all"],
        cwd=root, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30,
    )
    if proc.returncode != 0:
        return {}
    out: dict[str, str] = {}
    parts = proc.stdout.split(b"\0")
    i = 0
    while i < len(parts):
        entry = parts[i]
        i += 1
        if not entry:
            continue
        text = entry.decode("utf-8", errors="surrogateescape")
        if len(text) < 4:
            continue
        status = text[:2]
        name = text[3:]
        if status[0] in {"R", "C"} and i < len(parts):
            name = parts[i].decode("utf-8", errors="surrogateescape"); i += 1
        path = root / name
        try:
            digest = _sha256(path) if path.is_file() and not path.is_symlink() else "<nonfile>"
        except OSError:
            digest = "<unreadable>"
        out[name] = f"{status}:{digest}"
    return out


def _touched_paths(before: dict[str, str], after: dict[str, str]) -> list[str]:
    return sorted(name for name in set(before) | set(after) if before.get(name) != after.get(name))


def _run_argv(root: Path, cmd: dict) -> int:
    argv = cmd.get("argv")
    if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
        raise ValueError("post_patch command argv must be a non-empty string array")
    cwd_raw = cmd.get("cwd", ".")
    if not isinstance(cwd_raw, str) or not cwd_raw:
        raise ValueError("post_patch cwd must be a string")
    rel = PurePosixPath(cwd_raw)
    if rel.is_absolute() or any(p == ".." for p in rel.parts):
        raise ValueError(f"unsafe post_patch cwd: {cwd_raw}")
    cwd = root if cwd_raw == "." else root.joinpath(*rel.parts)
    if not cwd.is_dir():
        raise ValueError(f"post_patch cwd not found: {cwd_raw}")
    timeout = int(cmd.get("timeout_seconds", 300))
    name = cmd.get("name") or " ".join(argv)
    print(f"POST PATCH: {name}", flush=True)
    try:
        proc = subprocess.run(argv, cwd=cwd, timeout=max(1, timeout))
    except subprocess.TimeoutExpired:
        print(f"ERROR: post_patch command timeout after {timeout}s", file=sys.stderr)
        return 124
    return proc.returncode if proc.returncode >= 0 else 128 + abs(proc.returncode)


def _run_post_patch(root: Path, manifest: dict, *, changed: bool) -> int:
    pp = manifest.get("post_patch")
    if not isinstance(pp, dict):
        return 0
    commands = pp.get("commands")
    if not isinstance(commands, list) or not commands:
        return 0
    if not changed and not bool(pp.get("run_when_no_changes", False)):
        print("POST PATCH: skipped because payload produced no detected project delta")
        return 0
    for cmd in commands:
        if not isinstance(cmd, dict):
            print("ERROR: invalid post_patch command object", file=sys.stderr)
            return 2
        try:
            rc = _run_argv(root, cmd)
        except Exception as exc:
            print(f"ERROR: post_patch command invalid: {exc}", file=sys.stderr)
            return 2
        if rc:
            return rc
    return 0


def _run_git_policy(root: Path, manifest: dict, before_dirty: dict[str, str], after_dirty: dict[str, str]) -> int:
    policy = manifest.get("git")
    if not isinstance(policy, dict) or not (root / ".git").exists():
        return 0
    touched = _touched_paths(before_dirty, after_dirty)
    fail_on_error = bool(policy.get("fail_on_error", True))
    try:
        if policy.get("add") not in {None, "off", False} and touched:
            proc = subprocess.run(["git", "add", "--", *touched], cwd=root)
            if proc.returncode:
                raise RuntimeError(f"git add failed rc={proc.returncode}")
        commit_mode = policy.get("commit")
        if commit_mode == "auto" and touched:
            message = policy.get("commit_message")
            if not isinstance(message, str) or not message.strip():
                raise RuntimeError("git.commit=auto requires commit_message")
            # --only confines the commit to paths touched by this patch run.
            proc = subprocess.run(["git", "commit", "-m", message, "--only", "--", *touched], cwd=root)
            if proc.returncode not in {0, 1}:
                raise RuntimeError(f"git commit failed rc={proc.returncode}")
        if policy.get("push") == "auto":
            proc = subprocess.run(["git", "push"], cwd=root)
            if proc.returncode:
                raise RuntimeError(f"git push failed rc={proc.returncode}")
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1 if fail_on_error else 0
    return 0


def _archive_success(root: Path, source: Path) -> Path:
    patchs = (root / "patchs").resolve(strict=False)
    src = source.resolve(strict=True)
    try:
        src.relative_to(patchs)
    except ValueError as exc:
        raise ValueError("PATCH input must be under project patchs/ for zero-argument lifecycle") from exc
    if src.parent != patchs:
        raise ValueError("PATCH input must be a direct file under project patchs/")
    out_dir = root / "patchs" / "patched"
    out_dir.mkdir(parents=True, exist_ok=True)
    dst = out_dir / source.name
    if dst.exists() or dst.is_symlink():
        if dst.is_symlink() or not dst.is_file():
            raise ValueError(f"unsafe archive destination: patchs/patched/{source.name}")
        if _sha256(dst) == _sha256(source):
            source.unlink()
            return dst
        raise ValueError(f"archive destination already exists with different content: patchs/patched/{source.name}")
    os.replace(source, dst)
    return dst


def _execute_python(script: Path, root: Path, timeout: int) -> int:
    env = os.environ.copy()
    lib = str(Path(__file__).resolve().parent)
    env["PYTHONPATH"] = lib + (os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else "")
    try:
        proc = subprocess.run([sys.executable, str(script)], cwd=root, env=env, timeout=max(1, timeout))
    except subprocess.TimeoutExpired:
        return 124
    return proc.returncode if proc.returncode >= 0 else 128 + abs(proc.returncode)


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _snapshot_declared_paths(root: Path, paths: list[str]) -> dict[str, dict[str, object]]:
    snap: dict[str, dict[str, object]] = {}
    for rel in sorted(set(paths)):
        path = root.joinpath(*PurePosixPath(rel).parts)
        try:
            if path.is_symlink():
                snap[rel] = {"kind": "symlink"}
            elif path.is_file():
                st = path.stat()
                snap[rel] = {"kind": "file", "size": st.st_size, "sha256": _sha256(path)}
            elif path.exists():
                snap[rel] = {"kind": "other"}
            else:
                snap[rel] = {"kind": "missing"}
        except OSError as exc:
            snap[rel] = {"kind": "unreadable", "error": type(exc).__name__}
    return snap


def _snapshot_changes(before: dict[str, dict[str, object]], after: dict[str, dict[str, object]]) -> list[str]:
    return sorted(path for path in set(before) | set(after) if before.get(path) != after.get(path))


def _write_patch_result(result: dict[str, object]) -> None:
    raw = os.environ.get("PTV_PATCH_RESULT_FILE", "").strip()
    if not raw:
        return
    path = Path(raw)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, temp_name = tempfile.mkstemp(prefix=".ptv-result-", suffix=".json", dir=path.parent)
        os.close(fd)
        temp = Path(temp_name)
        temp.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        os.replace(temp, path)
    except Exception as exc:
        print(f"[PTV v{VERSION} WARNING] could not write structured PATCH result: {type(exc).__name__}", file=sys.stderr)


def _failure_kind_from_patch_failure(exc: PatchFailure) -> str:
    text = str(exc).lower()
    if any(token in text for token in ("not found", "ambiguous", "expected block", "match failed", "anchor")):
        return "anchor_mismatch"
    if "does not exist" in text or "target" in text:
        return "source_drift"
    return "patch_operation_failed"


def _partial_state(
    root: Path,
    *,
    before_fp: str | None,
    before_dirty: dict[str, str],
    before_targets: dict[str, dict[str, object]],
    target_paths: list[str],
) -> dict[str, object]:
    after_fp = _git_worktree_fingerprint(root)
    after_dirty = _dirty_paths(root)
    after_targets = _snapshot_declared_paths(root, target_paths)
    changed = sorted(set(_touched_paths(before_dirty, after_dirty)) | set(_snapshot_changes(before_targets, after_targets)))
    if before_fp is not None and after_fp is not None:
        detected: bool | None = before_fp != after_fp
    elif target_paths:
        detected = bool(_snapshot_changes(before_targets, after_targets))
    else:
        detected = None
    return {
        "detected": detected,
        "changed_paths": changed,
        "evidence": "git_worktree+declared_targets" if before_fp is not None else ("declared_targets" if target_paths else "insufficient_non_git_target_declaration"),
    }


def _base_result(source: Path) -> dict[str, object]:
    return {
        "format": "python-patch-tool-patch-result",
        "format_version": 1,
        "tool_version": VERSION,
        "patch_file": source.name,
        "patch_sha256": _sha256(source) if source.is_file() and not source.is_symlink() else None,
        "started_at": _utc_now(),
        "finished_at": None,
        "status": "RUNNING",
        "rc": None,
        "stage": "start",
        "preflight": None,
        "diagnosis": None,
        "partial_modification": {"detected": False, "changed_paths": [], "evidence": "preflight_not_started"},
    }


def _finish_result(result: dict[str, object], *, status: str, rc: int, stage: str, diagnosis: dict[str, object] | None = None, partial: dict[str, object] | None = None) -> int:
    result["status"] = status
    result["rc"] = int(rc)
    result["stage"] = stage
    result["finished_at"] = _utc_now()
    if diagnosis is not None:
        result["diagnosis"] = diagnosis
    if partial is not None:
        result["partial_modification"] = partial
    _write_patch_result(result)
    return int(rc)


def _prepare_package(root: Path, source: Path):
    temp_dir: tempfile.TemporaryDirectory[str] | None = None
    manifest: dict = {}
    extracted: Path | None = None
    if source.suffix.lower() == ".py":
        kind = "python"
        payload = source
        preflight = {
            "status": "PASS",
            "tool_version": VERSION,
            "warnings": ["standalone legacy Python patch has no PATCH_TOOL_MANIFEST.json compatibility/schema metadata"],
            "checks": [{"kind": "legacy_standalone_python", "status": "PASS"}],
            "target_paths": [],
            "legacy_standalone": True,
        }
        return temp_dir, extracted, manifest, kind, payload, None, preflight
    if not (source.suffix.lower() == ".zip" or source.name.lower().endswith((".tar.gz", ".tgz"))):
        raise PatchSchemaError(f"unsupported PATCH extension: {source.name}", kind="package_invalid")
    temp_dir = tempfile.TemporaryDirectory(prefix="ptv-patch-")
    extracted = Path(temp_dir.name)
    if source.suffix.lower() == ".zip":
        _safe_extract_zip(source, extracted)
    else:
        _safe_extract_tar(source, extracted)
    manifest_path = extracted / "PATCH_TOOL_MANIFEST.json"
    if not manifest_path.is_file():
        raise PatchSchemaError("archive PATCH requires root PATCH_TOOL_MANIFEST.json", kind="manifest_missing")
    manifest, kind, payload = _find_payload(extracted)
    ops_data = _read_json(payload, "PATCH_TOOL_OPS.json") if kind == "ops" else None
    preflight = run_preflight(root, manifest, extracted=extracted, kind=kind, payload=payload, ops_data=ops_data)
    return temp_dir, extracted, manifest, kind, payload, ops_data, preflight


def _print_preflight_report(source: Path, manifest: dict, kind: str, preflight: dict[str, object], *, inspect_only: bool = False) -> None:
    patch = manifest.get("patch") if isinstance(manifest, dict) else None
    patch_id = patch.get("id") if isinstance(patch, dict) else source.stem
    targets = preflight.get("target_paths") or []
    prefix = "INSPECT" if inspect_only else "PREFLIGHT"
    print(f"{prefix}: PASS | {source.name} | id={patch_id} | payload={kind} | targets={len(targets)} | tool={VERSION}")
    compat = manifest.get("compatibility") if isinstance(manifest, dict) else None
    if isinstance(compat, dict) and compat:
        print(
            f"  Compatibility: min={compat.get('min_tool_version','-')} "
            f"max={compat.get('max_tool_version','-')} tested={compat.get('max_tested_version','-')}"
        )
    if targets:
        print("  Targets:")
        for rel in targets[:40]:
            print(f"    - {rel}")
        if len(targets) > 40:
            print(f"    ... {len(targets)-40} more")
    for warning in preflight.get("warnings") or []:
        print(f"  WARNING: {warning}")


def _inspect_patch(root: Path, source: Path) -> int:
    if source.is_symlink() or not source.is_file():
        print(f"INSPECT FAIL: PATCH input is not a regular non-symlink file: {source}", file=sys.stderr)
        return 2
    temp_dir = None
    try:
        temp_dir, _extracted, manifest, kind, _payload, _ops_data, preflight = _prepare_package(root, source)
        _print_preflight_report(source, manifest, kind, preflight, inspect_only=True)
        pp = manifest.get("post_patch") if isinstance(manifest, dict) else None
        if isinstance(pp, dict) and pp.get("commands"):
            print("  Post-patch commands:")
            for cmd in pp.get("commands"):
                name = cmd.get("name") or " ".join(cmd.get("argv") or [])
                print(f"    - {name}")
        git = manifest.get("git") if isinstance(manifest, dict) else None
        if isinstance(git, dict) and git:
            print(f"  Git policy: add={git.get('add','off')} commit={git.get('commit','off')} push={git.get('push','off')}")
        print("INSPECT RESULT: PASS — project unchanged")
        return 0
    except PatchSchemaError as exc:
        print(f"INSPECT RESULT: FAIL — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
        return 2
    except Exception as exc:
        print(f"INSPECT RESULT: FAIL — project unchanged | {type(exc).__name__}: {exc}", file=sys.stderr)
        return 2
    finally:
        if temp_dir is not None:
            temp_dir.cleanup()


def _execute_patch(root: Path, source: Path) -> int:
    result = _base_result(source)
    if source.is_symlink() or not source.is_file():
        print(f"ERROR: PATCH input is not a regular non-symlink file: {source}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage="input", diagnosis={"kind": "package_invalid", "message": "input is not a regular non-symlink file", "affected_paths": []})

    temp_dir: tempfile.TemporaryDirectory[str] | None = None
    try:
        try:
            temp_dir, _extracted, manifest, kind, payload, ops_data, preflight = _prepare_package(root, source)
            result["preflight"] = preflight
            result["manifest_patch"] = manifest.get("patch") if isinstance(manifest, dict) else None
            result["recovery"] = manifest.get("recovery") if isinstance(manifest, dict) else None
            _print_preflight_report(source, manifest, kind, preflight)
        except PatchSchemaError as exc:
            diagnosis = {"kind": exc.kind, "message": str(exc), "affected_paths": [exc.path] if exc.path else []}
            result["preflight"] = {"status": "FAIL", "kind": exc.kind, "message": str(exc), "affected_paths": diagnosis["affected_paths"]}
            print(f"PREFLIGHT FAIL — project unchanged | {exc.kind}: {exc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "preflight_failed_before_payload"})
        except Exception as exc:
            diagnosis = {"kind": "package_invalid", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []}
            result["preflight"] = {"status": "FAIL", "kind": "package_invalid", "message": diagnosis["message"]}
            print(f"PREFLIGHT FAIL — project unchanged | {diagnosis['message']}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=2, stage="preflight", diagnosis=diagnosis, partial={"detected": False, "changed_paths": [], "evidence": "preflight_failed_before_payload"})

        target_paths = list(preflight.get("target_paths") or [])
        before_fp = _git_worktree_fingerprint(root)
        before_dirty = _dirty_paths(root)
        before_targets = _snapshot_declared_paths(root, target_paths)

        timeout = 900
        if isinstance(manifest.get("execution"), dict):
            timeout = int(manifest["execution"].get("timeout_seconds", timeout))
        print(f"PATCH: {source.name}")
        print("Execution: IN-PLACE (SANDBOX/worktree disabled)")
        result["stage"] = "payload"
        if kind == "python":
            rc = _execute_python(payload, root, timeout)
            payload_diag = None
        else:
            try:
                if ops_data is None:
                    ops_data = _read_json(payload, "PATCH_TOOL_OPS.json")
                patch_name = str((manifest.get("patch") or {}).get("id") or ops_data.get("patch_name") or source.stem)
                state = run_ops(root, ops_data, patch_name=patch_name)
                print(f"PATCH OPS: patched={state.stats.patched} created={state.stats.created} unchanged={state.stats.unchanged}")
                rc = 0
                payload_diag = None
            except PatchFailure as exc:
                rc = finish_failure(exc)
                payload_diag = {"kind": _failure_kind_from_patch_failure(exc), "message": str(exc), "affected_paths": [exc.rel_path] if exc.rel_path else []}
            except Exception as exc:
                rc = finish_failure(exc)
                payload_diag = {"kind": "patch_operation_failed", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []}
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = payload_diag or {"kind": "patch_payload_failed", "message": f"payload returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: project changed before PATCH failed !!!", file=sys.stderr)
                for rel in partial.get("changed_paths") or []:
                    print(f"  changed: {rel}", file=sys.stderr)
            elif partial.get("detected") is None:
                print("[PTV WARNING] PATCH failed but partial-modification state is unknown outside Git because no targets were declared.", file=sys.stderr)
            print(f"RUN SUMMARY: FAIL | {source.name} rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="payload", diagnosis=diagnosis, partial=partial)

        after_payload_fp = _git_worktree_fingerprint(root)
        changed = before_fp is None or after_payload_fp is None or before_fp != after_payload_fp
        result["stage"] = "post_patch"
        rc = _run_post_patch(root, manifest, changed=changed)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "post_patch_failed", "message": f"post_patch returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: patch payload changed project before validation failed !!!", file=sys.stderr)
            print(f"RUN SUMMARY: FAIL | post_patch rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="post_patch", diagnosis=diagnosis, partial=partial)

        after_dirty = _dirty_paths(root)
        result["stage"] = "git"
        rc = _run_git_policy(root, manifest, before_dirty, after_dirty)
        if rc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "git_policy_failed", "message": f"git policy returned rc={rc}", "affected_paths": list(partial.get("changed_paths") or [])}
            if partial.get("detected") is True:
                print("!!! PARTIAL MODIFICATION DETECTED: project changed before Git policy failed !!!", file=sys.stderr)
            print(f"RUN SUMMARY: FAIL | git policy rc={rc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=rc, stage="git", diagnosis=diagnosis, partial=partial)

        result["stage"] = "archive"
        try:
            archived = _archive_success(root, source)
        except Exception as exc:
            partial = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
            diagnosis = {"kind": "archive_failed", "message": str(exc), "affected_paths": list(partial.get("changed_paths") or [])}
            print(f"ERROR: PATCH succeeded but queue archive failed: {exc}", file=sys.stderr)
            return _finish_result(result, status="FAIL", rc=3, stage="archive", diagnosis=diagnosis, partial=partial)
        print(f"[INFO] PATCH ARCHIVED: {archived.relative_to(root).as_posix()}")
        print(f"RUN SUMMARY: PASS | {source.name}")
        project_delta = _partial_state(root, before_fp=before_fp, before_dirty=before_dirty, before_targets=before_targets, target_paths=target_paths)
        result["project_delta"] = project_delta
        return _finish_result(
            result, status="PASS", rc=0, stage="complete",
            diagnosis={"kind": "none", "message": "PATCH completed", "affected_paths": []},
            partial={"detected": False, "changed_paths": [], "evidence": "not_applicable_on_success"},
        )
    except KeyboardInterrupt:
        print("INTERRUPTED by Ctrl+C", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=130, stage=str(result.get("stage") or "unknown"), diagnosis={"kind": "interrupted", "message": "Ctrl+C", "affected_paths": []})
    except Exception as exc:
        print(f"ERROR: PATCH execution failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        return _finish_result(result, status="FAIL", rc=2, stage=str(result.get("stage") or "unknown"), diagnosis={"kind": "internal_error", "message": f"{type(exc).__name__}: {exc}", "affected_paths": []})
    finally:
        if temp_dir is not None:
            temp_dir.cleanup()

def _paths(root: Path) -> int:
    print(f"Project root : {root}")
    print(f"Patch folder : {root / 'patchs'}")
    print(f"Patched files: {root / 'patchs' / 'patched'}")
    print(f"Tools folder : {root / 'tools'}")
    print(f"Patch helper : {Path(__file__).resolve().parent / 'python_patch_utils.py'}")
    return 0


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if not args:
        print("ERROR: normal interactive use is ./tools/run_python_patches.sh with no arguments", file=sys.stderr)
        return 2
    if args[0] in {"version", "--version"}:
        print(VERSION); return 0
    if args[0] == "paths":
        root = Path.cwd().resolve(); return _paths(root)
    if args[0] in {"help", "--help", "-h"}:
        print("Python Patch Tool self-contained core. Normal use: ./tools/run_python_patches.sh")
        print("Interactive selector supports inspect/dry-run with key i.")
        return 0

    inspect_mode = False
    if args and args[0] == "inspect":
        inspect_mode = True
        args = args[1:]

    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--patch")
    ap.add_argument("--transaction", default="off")
    ns, unknown = ap.parse_known_args(args)
    if unknown:
        print(f"ERROR: unsupported self-contained runner argument(s): {' '.join(unknown)}", file=sys.stderr)
        return 2
    if ns.transaction != "off":
        print("ERROR: SANDBOX/worktree transaction modes are permanently unsupported; use --transaction off", file=sys.stderr)
        return 2
    if not ns.patch:
        print("ERROR: --patch is required for PATCH execution/inspect", file=sys.stderr)
        return 2
    root = Path.cwd().resolve()
    raw = Path(ns.patch)
    source = raw if raw.is_absolute() else root / raw
    if inspect_mode:
        return _inspect_patch(root, source)
    return _execute_patch(root, source)


if __name__ == "__main__":
    raise SystemExit(main())
