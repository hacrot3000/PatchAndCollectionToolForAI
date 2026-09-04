#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import fnmatch
import json
import os
import shutil
import tempfile
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath
from typing import Any

VERSION = "6.20.1"
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


def _normalize_diagnostic_rerun(raw: Any, *, profile_name: str) -> dict[str, Any] | None:
    if raw is None:
        return None
    if not isinstance(raw, dict):
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun must be an object", kind="validation_profile_invalid")
    allowed = {"enabled", "safe", "name", "append_args", "timeout_seconds", "on_timeout"}
    extra = sorted(set(raw) - allowed)
    if extra:
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun contains unsupported field(s): {', '.join(extra)}", kind="validation_profile_invalid")
    enabled = raw.get("enabled", True)
    safe = raw.get("safe", False)
    on_timeout = raw.get("on_timeout", False)
    if not isinstance(enabled, bool) or not isinstance(safe, bool) or not isinstance(on_timeout, bool):
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun enabled/safe/on_timeout must be boolean", kind="validation_profile_invalid")
    name = raw.get("name", f"{profile_name} diagnostic rerun")
    if not isinstance(name, str) or not name.strip() or len(name) > 200:
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun.name must be 1..200 characters", kind="validation_profile_invalid")
    append_args = raw.get("append_args", [])
    if not isinstance(append_args, list) or len(append_args) > 32 or any(not isinstance(x, str) or not x or len(x) > 1000 for x in append_args):
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun.append_args must be a bounded string array", kind="validation_profile_invalid")
    timeout = raw.get("timeout_seconds", 600)
    if not isinstance(timeout, int) or isinstance(timeout, bool) or not 1 <= timeout <= MAX_PROFILE_TIMEOUT:
        raise ProjectStateError(f"validation profile {profile_name}.diagnostic_rerun.timeout_seconds must be 1..{MAX_PROFILE_TIMEOUT}", kind="validation_profile_invalid")
    return {"enabled": enabled, "safe": safe, "name": name.strip(), "append_args": list(append_args), "timeout_seconds": timeout, "on_timeout": on_timeout}


def _resolve_named_profiles(root: Path, names: list[str]) -> list[dict[str, Any]]:
    table = _validation_profile_table(root)
    resolved: list[dict[str, Any]] = []
    seen: set[str] = set()
    for name in names:
        if name in seen:
            continue
        seen.add(name)
        raw = table.get(name)
        if not isinstance(raw, dict):
            raise ProjectStateError(f"validation profile is not configured locally: {name}", kind="validation_profile_missing")
        allowed = {"argv", "cwd", "timeout_seconds", "description", "diagnostic_rerun"}
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
        rerun = _normalize_diagnostic_rerun(raw.get("diagnostic_rerun"), profile_name=name)
        row = {"name": name, "argv": list(argv), "cwd": cwd, "timeout_seconds": timeout, "description": desc or ""}
        if rerun is not None:
            row["diagnostic_rerun"] = rerun
        resolved.append(row)
    return resolved


def resolve_validation_profile_names(root: Path, names: list[str]) -> list[dict[str, Any]]:
    return _resolve_named_profiles(root, names)


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
    return _resolve_named_profiles(root, names)


def _safe_glob(value: Any, *, field: str) -> str:
    if not isinstance(value, str) or not value.strip() or "\\" in value:
        raise ProjectStateError(f"{field} must be a non-empty POSIX project-relative glob", kind="validation_selection_invalid")
    text = value.strip()
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(part == ".." for part in rel.parts) or any(":" in part for part in rel.parts):
        raise ProjectStateError(f"unsafe {field}: {text}", kind="validation_selection_invalid")
    return text


def validation_selection_policy(root: Path) -> dict[str, Any]:
    data = load_project_config(root)
    validation = data.get("validation", {})
    if validation is None:
        validation = {}
    if not isinstance(validation, dict):
        raise ProjectStateError("validation in local config must be an object", kind="validation_selection_invalid")
    allowed_validation = {"selection", "diagnostic_rerun"}
    extra_validation = sorted(set(validation) - allowed_validation)
    if extra_validation:
        raise ProjectStateError(f"validation local config contains unsupported field(s): {', '.join(extra_validation)}", kind="validation_selection_invalid")

    global_rerun = validation.get("diagnostic_rerun", {})
    if global_rerun is None:
        global_rerun = {}
    if not isinstance(global_rerun, dict):
        raise ProjectStateError("validation.diagnostic_rerun must be an object", kind="validation_selection_invalid")
    extra_rerun = sorted(set(global_rerun) - {"max_commands", "on_timeout"})
    if extra_rerun:
        raise ProjectStateError(f"validation.diagnostic_rerun contains unsupported field(s): {', '.join(extra_rerun)}", kind="validation_selection_invalid")
    max_commands = global_rerun.get("max_commands", 1)
    on_timeout = global_rerun.get("on_timeout", False)
    if not isinstance(max_commands, int) or isinstance(max_commands, bool) or not 0 <= max_commands <= 10:
        raise ProjectStateError("validation.diagnostic_rerun.max_commands must be 0..10", kind="validation_selection_invalid")
    if not isinstance(on_timeout, bool):
        raise ProjectStateError("validation.diagnostic_rerun.on_timeout must be boolean", kind="validation_selection_invalid")

    selection = validation.get("selection", {})
    if selection is None:
        selection = {}
    if not isinstance(selection, dict):
        raise ProjectStateError("validation.selection must be an object", kind="validation_selection_invalid")
    extra = sorted(set(selection) - {"mode", "fallback_profiles", "rules"})
    if extra:
        raise ProjectStateError(f"validation.selection contains unsupported field(s): {', '.join(extra)}", kind="validation_selection_invalid")
    mode = selection.get("mode", "off")
    if mode not in {"off", "append", "replace"}:
        raise ProjectStateError("validation.selection.mode must be off/append/replace", kind="validation_selection_invalid")
    fallback = selection.get("fallback_profiles", [])
    if not isinstance(fallback, list) or any(not isinstance(x, str) or not x.strip() for x in fallback):
        raise ProjectStateError("validation.selection.fallback_profiles must be a string array", kind="validation_selection_invalid")
    fallback_names = list(dict.fromkeys(x.strip() for x in fallback))
    rules_raw = selection.get("rules", [])
    if not isinstance(rules_raw, list) or len(rules_raw) > 200:
        raise ProjectStateError("validation.selection.rules must be an array of at most 200 rules", kind="validation_selection_invalid")
    rules: list[dict[str, Any]] = []
    referenced = list(fallback_names)
    for i, raw in enumerate(rules_raw):
        if not isinstance(raw, dict):
            raise ProjectStateError(f"validation.selection.rules[{i}] must be an object", kind="validation_selection_invalid")
        extra_rule = sorted(set(raw) - {"name", "include", "exclude", "profiles"})
        if extra_rule:
            raise ProjectStateError(f"validation.selection.rules[{i}] unsupported field(s): {', '.join(extra_rule)}", kind="validation_selection_invalid")
        name = raw.get("name", f"rule-{i+1}")
        if not isinstance(name, str) or not name.strip() or len(name) > 200:
            raise ProjectStateError(f"validation.selection.rules[{i}].name must be 1..200 characters", kind="validation_selection_invalid")
        include = raw.get("include", [])
        exclude = raw.get("exclude", [])
        profiles = raw.get("profiles", [])
        if not isinstance(include, list) or not include:
            raise ProjectStateError(f"validation.selection.rules[{i}].include must be a non-empty array", kind="validation_selection_invalid")
        if not isinstance(exclude, list) or not isinstance(profiles, list) or not profiles:
            raise ProjectStateError(f"validation.selection.rules[{i}] exclude/profiles must be arrays and profiles non-empty", kind="validation_selection_invalid")
        includes = [_safe_glob(x, field=f"validation.selection.rules[{i}].include") for x in include]
        excludes = [_safe_glob(x, field=f"validation.selection.rules[{i}].exclude") for x in exclude]
        names: list[str] = []
        for raw_name in profiles:
            if not isinstance(raw_name, str) or not raw_name.strip():
                raise ProjectStateError(f"validation.selection.rules[{i}].profiles must contain non-empty strings", kind="validation_selection_invalid")
            n = raw_name.strip()
            if n not in names: names.append(n)
            if n not in referenced: referenced.append(n)
        rules.append({"name": name.strip(), "include": includes, "exclude": excludes, "profiles": names})
    if referenced:
        _resolve_named_profiles(root, referenced)  # validate references/config before mutation
    return {
        "mode": mode,
        "fallback_profiles": fallback_names,
        "rules": rules,
        "referenced_profiles": referenced,
        "diagnostic_rerun": {"max_commands": max_commands, "on_timeout": on_timeout},
    }


def _glob_match(path: str, pattern: str) -> bool:
    return fnmatch.fnmatchcase(path, pattern) or PurePosixPath(path).match(pattern)


def resolve_effective_validation_profiles(root: Path, manifest: dict[str, Any], changed_paths: list[str], *, disabled: bool = False) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    requested_rows = resolve_validation_profiles(root, manifest)
    requested = [str(x["name"]) for x in requested_rows]
    policy = validation_selection_policy(root)
    if disabled:
        return [], {"status": "DISABLED_BY_CLI", "mode": policy["mode"], "changed_paths": sorted(set(changed_paths)), "requested_profiles": requested, "auto_profiles": [], "final_profiles": [], "matched_rules": []}
    auto: list[str] = []
    matched_rules: list[dict[str, Any]] = []
    paths = sorted(set(str(x) for x in changed_paths if isinstance(x, str) and x))
    if policy["mode"] != "off":
        for rule in policy["rules"]:
            matched_paths = [p for p in paths if any(_glob_match(p, pat) for pat in rule["include"]) and not any(_glob_match(p, pat) for pat in rule["exclude"])]
            if matched_paths:
                matched_rules.append({"name": rule["name"], "paths": matched_paths, "profiles": list(rule["profiles"])})
                for name in rule["profiles"]:
                    if name not in auto: auto.append(name)
        if not auto:
            auto.extend(policy["fallback_profiles"])
    if policy["mode"] == "replace":
        final_names = list(auto)
    else:
        final_names = list(requested)
        for name in auto:
            if name not in final_names: final_names.append(name)
    final_rows = _resolve_named_profiles(root, final_names) if final_names else []
    report = {
        "status": "PASS",
        "mode": policy["mode"],
        "changed_paths": paths,
        "requested_profiles": requested,
        "auto_profiles": auto,
        "final_profiles": final_names,
        "matched_rules": matched_rules,
        "fallback_used": bool(policy["mode"] != "off" and not matched_rules and auto),
        "diagnostic_rerun": policy["diagnostic_rerun"],
    }
    return final_rows, report

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
