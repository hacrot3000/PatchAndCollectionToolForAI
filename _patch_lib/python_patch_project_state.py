#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import os
import shutil
import tempfile
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any

VERSION = "6.17.10"
CONFIG_NAME = ".python_patch_tool.json"
MAX_CONFIG_BYTES = 1024 * 1024
MAX_PROFILE_TIMEOUT = 1800

class ProjectStateError(ValueError):
    def __init__(self, message: str, *, kind: str = "project_config_invalid"):
        super().__init__(message)
        self.kind = kind


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _safe_rel_dir(value: str) -> str:
    if not isinstance(value, str) or "\\" in value:
        raise ProjectStateError("validation profile cwd must be a POSIX project-relative directory")
    value = value.strip() or "."
    if value == ".":
        return "."
    rel = PurePosixPath(value)
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise ProjectStateError(f"unsafe validation profile cwd: {value}")
    if any(":" in part for part in rel.parts):
        raise ProjectStateError(f"Windows drive/ADS syntax is not allowed in validation profile cwd: {value}")
    return rel.as_posix()


def load_project_config(root: Path, *, strict: bool = True) -> dict[str, Any]:
    path = root / CONFIG_NAME
    if not path.exists():
        return {}
    try:
        if path.is_symlink() or not path.is_file():
            raise ProjectStateError(f"{CONFIG_NAME} must be a regular non-symlink file")
        if path.stat().st_size > MAX_CONFIG_BYTES:
            raise ProjectStateError(f"{CONFIG_NAME} is too large")
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        if not isinstance(data, dict):
            raise ProjectStateError(f"{CONFIG_NAME} root must be an object")
        return data
    except ProjectStateError:
        if strict:
            raise
    except Exception as exc:
        if strict:
            raise ProjectStateError(f"invalid {CONFIG_NAME}: {type(exc).__name__}: {exc}") from exc
    return {}


def local_project_key(root: Path) -> str | None:
    data = load_project_config(root)
    node = data.get("project")
    if node is None:
        return None
    if not isinstance(node, dict):
        raise ProjectStateError("project config must be an object")
    value = node.get("key")
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise ProjectStateError("project.key in local config must be a non-empty string")
    return value.strip()


def enforce_project_identity(root: Path, manifest: dict[str, Any]) -> dict[str, Any] | None:
    project = manifest.get("project")
    if not isinstance(project, dict):
        return None
    requested = project.get("key")
    if requested is None:
        return None
    if not isinstance(requested, str) or not requested.strip():
        raise ProjectStateError("manifest.project.key must be a non-empty string", kind="project_identity_invalid")
    requested = requested.strip()
    actual = local_project_key(root)
    if actual is None:
        raise ProjectStateError(
            f"PATCH requires project.key={requested!r}, but {CONFIG_NAME} does not configure project.key",
            kind="project_identity_unconfigured",
        )
    if actual != requested:
        raise ProjectStateError(
            f"PATCH project mismatch: manifest.project.key={requested!r}, local project.key={actual!r}",
            kind="project_mismatch",
        )
    return {"kind": "project_identity", "status": "PASS", "project_key": actual}


def _validation_profile_table(root: Path) -> dict[str, Any]:
    data = load_project_config(root)
    table = data.get("validation_profiles", {})
    if table is None:
        table = {}
    if not isinstance(table, dict):
        raise ProjectStateError("validation_profiles in local config must be an object")
    return table


def resolve_validation_profiles(root: Path, manifest: dict[str, Any]) -> list[dict[str, Any]]:
    validation = manifest.get("validation")
    requested = validation.get("profiles") if isinstance(validation, dict) else None
    if requested in (None, []):
        return []
    if not isinstance(requested, list):
        raise ProjectStateError("manifest.validation.profiles must be an array", kind="validation_profile_invalid")
    names: list[str] = []
    for raw in requested:
        if not isinstance(raw, str) or not raw.strip():
            raise ProjectStateError("manifest.validation.profiles entries must be non-empty strings", kind="validation_profile_invalid")
        name = raw.strip()
        if name in names:
            raise ProjectStateError(f"duplicate validation profile requested: {name}", kind="validation_profile_invalid")
        names.append(name)
    table = _validation_profile_table(root)
    resolved: list[dict[str, Any]] = []
    for name in names:
        raw = table.get(name)
        if not isinstance(raw, dict):
            raise ProjectStateError(f"validation profile is not configured locally: {name}", kind="validation_profile_missing")
        allowed = {"argv", "cwd", "timeout_seconds", "description"}
        extra = sorted(set(raw) - allowed)
        if extra:
            raise ProjectStateError(f"validation profile {name} contains unsupported field(s): {', '.join(extra)}", kind="validation_profile_invalid")
        argv = raw.get("argv")
        if not isinstance(argv, list) or not argv or any(not isinstance(x, str) or not x for x in argv):
            raise ProjectStateError(f"validation profile {name}.argv must be a non-empty string array", kind="validation_profile_invalid")
        timeout = raw.get("timeout_seconds", 900)
        if not isinstance(timeout, int) or isinstance(timeout, bool) or timeout < 1 or timeout > MAX_PROFILE_TIMEOUT:
            raise ProjectStateError(f"validation profile {name}.timeout_seconds must be 1..{MAX_PROFILE_TIMEOUT}", kind="validation_profile_invalid")
        cwd = _safe_rel_dir(str(raw.get("cwd", ".")))
        desc = raw.get("description")
        if desc is not None and not isinstance(desc, str):
            raise ProjectStateError(f"validation profile {name}.description must be a string", kind="validation_profile_invalid")
        resolved.append({"name": name, "argv": list(argv), "cwd": cwd, "timeout_seconds": timeout, "description": desc or ""})
    return resolved


def artifact_state_file(root: Path, name: str) -> Path:
    return root / "artifacts" / "patch_tool" / name


def _atomic_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as fh:
            json.dump(data, fh, ensure_ascii=False, indent=2, sort_keys=True)
            fh.write("\n")
            fh.flush()
            try: os.fsync(fh.fileno())
            except OSError: pass
        os.replace(tmp, path)
    finally:
        try: tmp.unlink()
        except FileNotFoundError: pass


def load_json_state(path: Path, default: dict[str, Any]) -> dict[str, Any]:
    try:
        if path.is_symlink() or not path.is_file():
            return dict(default)
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        return data if isinstance(data, dict) else dict(default)
    except Exception:
        return dict(default)


def update_patch_ledger(root: Path, report: dict[str, Any]) -> dict[str, Any]:
    path = artifact_state_file(root, "PATCH_LEDGER.json")
    ledger = load_json_state(path, {"format": "python-patch-tool-ledger", "format_version": 1, "entries": {}})
    entries = ledger.get("entries")
    if not isinstance(entries, dict): entries = {}; ledger["entries"] = entries
    run_id = str(report.get("run_id") or "")
    now = datetime.now(timezone.utc).isoformat()
    for row in report.get("results") or []:
        if not isinstance(row, dict) or str(row.get("kind") or "PATCH") != "PATCH":
            continue
        result = row.get("patch_result") if isinstance(row.get("patch_result"), dict) else {}
        mp = result.get("manifest_patch") if isinstance(result.get("manifest_patch"), dict) else {}
        patch_id = row.get("patch_id") or mp.get("id")
        sha = result.get("patch_sha256")
        if not isinstance(patch_id, str) or not patch_id or not isinstance(sha, str) or len(sha) != 64:
            continue
        key = f"{patch_id}::{sha.lower()}"
        old = entries.get(key) if isinstance(entries.get(key), dict) else {}
        entries[key] = {
            "patch_id": patch_id,
            "sha256": sha.lower(),
            "name": str(row.get("name") or old.get("name") or ""),
            "first_seen": old.get("first_seen") or now,
            "last_seen": now,
            "last_run_id": run_id,
            "last_status": str(row.get("status") or "UNKNOWN"),
            "run_count": int(old.get("run_count") or 0) + 1,
        }
    ledger["updated_at"] = now
    _atomic_json(path, ledger)
    return ledger


def ledger_id_reuse(root: Path, patch_id: str, sha256: str) -> list[dict[str, Any]]:
    ledger = load_json_state(artifact_state_file(root, "PATCH_LEDGER.json"), {"entries": {}})
    entries = ledger.get("entries") if isinstance(ledger.get("entries"), dict) else {}
    return [dict(v) for v in entries.values() if isinstance(v, dict) and v.get("patch_id") == patch_id and str(v.get("sha256") or "").lower() != sha256.lower()]


def disk_preflight(root: Path, package_paths: list[Path], target_paths: list[str]) -> dict[str, Any]:
    package_bytes = 0
    target_bytes = 0
    for path in package_paths:
        try:
            if path.is_file() and not path.is_symlink(): package_bytes += path.stat().st_size
        except OSError: pass
    seen: set[str] = set()
    for rel in target_paths:
        if rel in seen: continue
        seen.add(rel)
        try:
            path = root.joinpath(*PurePosixPath(rel).parts)
            if path.is_file() and not path.is_symlink(): target_bytes += path.stat().st_size
        except OSError: pass
    # Enough for one exact package snapshot + before/rollback copy + ordinary
    # reports. FAIL_HANDOFF/COLLECT have independent quotas and are not reserved
    # here because doing so would reject small but otherwise valid projects.
    required_project = max(32 * 1024 * 1024, package_bytes + target_bytes * 2 + 8 * 1024 * 1024)
    project_usage = shutil.disk_usage(root)
    temp_usage = shutil.disk_usage(tempfile.gettempdir())
    required_temp = max(16 * 1024 * 1024, package_bytes * 2 + min(target_bytes, 256 * 1024 * 1024))
    status = "PASS" if project_usage.free >= required_project and temp_usage.free >= required_temp else "FAIL"
    return {
        "status": status,
        "package_bytes": package_bytes,
        "target_bytes": target_bytes,
        "required_project_free_bytes": required_project,
        "actual_project_free_bytes": project_usage.free,
        "required_temp_free_bytes": required_temp,
        "actual_temp_free_bytes": temp_usage.free,
    }
