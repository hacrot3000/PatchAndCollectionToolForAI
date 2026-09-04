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

from python_patch_utils import PatchFailure, finish_failure, run_ops

VERSION = "6.12.1"


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


def _execute_patch(root: Path, source: Path) -> int:
    if source.is_symlink() or not source.is_file():
        print(f"ERROR: PATCH input is not a regular non-symlink file: {source}", file=sys.stderr)
        return 2
    before_fp = _git_worktree_fingerprint(root)
    before_dirty = _dirty_paths(root)
    manifest: dict = {}
    temp_dir: tempfile.TemporaryDirectory[str] | None = None
    try:
        if source.suffix.lower() == ".py":
            kind = "python"
            payload = source
        elif source.suffix.lower() == ".zip" or source.name.lower().endswith((".tar.gz", ".tgz")):
            temp_dir = tempfile.TemporaryDirectory(prefix="ptv-patch-")
            extracted = Path(temp_dir.name)
            if source.suffix.lower() == ".zip":
                _safe_extract_zip(source, extracted)
            else:
                _safe_extract_tar(source, extracted)
            manifest, kind, payload = _find_payload(extracted)
        else:
            print(f"ERROR: unsupported PATCH extension: {source.name}", file=sys.stderr)
            return 2

        timeout = 900
        if isinstance(manifest.get("execution"), dict):
            try:
                timeout = int(manifest["execution"].get("timeout_seconds", timeout))
            except Exception:
                return 2
        print(f"PATCH: {source.name}")
        print("Execution: IN-PLACE (SANDBOX/worktree disabled)")
        if kind == "python":
            rc = _execute_python(payload, root, timeout)
        else:
            try:
                ops_data = _read_json(payload, "PATCH_TOOL_OPS.json")
                patch_name = str((manifest.get("patch") or {}).get("id") or ops_data.get("patch_name") or source.stem)
                state = run_ops(root, ops_data, patch_name=patch_name)
                print(f"PATCH OPS: patched={state.stats.patched} created={state.stats.created} unchanged={state.stats.unchanged}")
                rc = 0
            except Exception as exc:
                rc = finish_failure(exc)
        if rc:
            print(f"RUN SUMMARY: FAIL | {source.name} rc={rc}", file=sys.stderr)
            return rc

        after_payload_fp = _git_worktree_fingerprint(root)
        changed = before_fp is None or after_payload_fp is None or before_fp != after_payload_fp
        rc = _run_post_patch(root, manifest, changed=changed)
        if rc:
            print(f"RUN SUMMARY: FAIL | post_patch rc={rc}", file=sys.stderr)
            return rc
        after_dirty = _dirty_paths(root)
        rc = _run_git_policy(root, manifest, before_dirty, after_dirty)
        if rc:
            print(f"RUN SUMMARY: FAIL | git policy rc={rc}", file=sys.stderr)
            return rc
        try:
            archived = _archive_success(root, source)
        except Exception as exc:
            print(f"ERROR: PATCH succeeded but queue archive failed: {exc}", file=sys.stderr)
            return 3
        print(f"[INFO] PATCH ARCHIVED: {archived.relative_to(root).as_posix()}")
        print(f"RUN SUMMARY: PASS | {source.name}")
        return 0
    except KeyboardInterrupt:
        print("INTERRUPTED by Ctrl+C", file=sys.stderr)
        return 130
    except Exception as exc:
        print(f"ERROR: PATCH execution failed: {type(exc).__name__}: {exc}", file=sys.stderr)
        return 2
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
        return 0

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
        print("ERROR: --patch is required for PATCH execution", file=sys.stderr)
        return 2
    root = Path.cwd().resolve()
    raw = Path(ns.patch)
    source = raw if raw.is_absolute() else root / raw
    return _execute_patch(root, source)


if __name__ == "__main__":
    raise SystemExit(main())
