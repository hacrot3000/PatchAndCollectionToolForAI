#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
from typing import Any

VERSION = "6.14.2"
SCHEMA_PATH = Path(__file__).resolve().parent / "docs" / "PATCH_PACKAGE_SCHEMA.json"
_HEX64 = re.compile(r"^[0-9a-fA-F]{64}$")
_SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")


class PatchSchemaError(ValueError):
    def __init__(self, message: str, *, kind: str = "schema_invalid", path: str | None = None):
        super().__init__(message)
        self.kind = kind
        self.path = path


def _load_schema() -> dict[str, Any]:
    data = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise PatchSchemaError("PATCH_PACKAGE_SCHEMA.json root must be an object")
    return data


def _type_ok(value: Any, expected: Any) -> bool:
    names = expected if isinstance(expected, list) else [expected]
    for name in names:
        if name == "object" and isinstance(value, dict): return True
        if name == "array" and isinstance(value, list): return True
        if name == "string" and isinstance(value, str): return True
        if name == "integer" and isinstance(value, int) and not isinstance(value, bool): return True
        if name == "boolean" and isinstance(value, bool): return True
    return False


def _safe_rel(value: str, *, label: str) -> str:
    if not isinstance(value, str) or not value.strip() or "\\" in value:
        raise PatchSchemaError(f"{label} must be a non-empty POSIX project-relative path", path=value if isinstance(value, str) else None)
    rel = PurePosixPath(value.strip())
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise PatchSchemaError(f"unsafe {label}: {value}", path=value)
    return rel.as_posix()


def _validate_format(value: Any, fmt: str, label: str) -> None:
    if fmt in {"project_relative_path", "archive_relative_path", "project_relative_dir"}:
        _safe_rel(value, label=label)
    elif fmt == "sha256":
        if not isinstance(value, str) or not _HEX64.fullmatch(value):
            raise PatchSchemaError(f"{label} must be a 64-character SHA-256 hex string")
    elif fmt == "semver":
        if not isinstance(value, str) or not _SEMVER.fullmatch(value):
            raise PatchSchemaError(f"{label} must be semantic version X.Y.Z")


def _validate_node(value: Any, spec: dict[str, Any], label: str) -> None:
    expected = spec.get("type")
    if expected is not None and not _type_ok(value, expected):
        raise PatchSchemaError(f"{label} has invalid type; expected {expected}")
    if "const" in spec and value != spec["const"]:
        raise PatchSchemaError(f"{label} must equal {spec['const']!r}")
    if "enum" in spec and value not in spec["enum"]:
        raise PatchSchemaError(f"{label} has unsupported value {value!r}")
    if isinstance(value, int) and not isinstance(value, bool):
        if "minimum" in spec and value < int(spec["minimum"]):
            raise PatchSchemaError(f"{label} must be >= {spec['minimum']}")
        if "maximum" in spec and value > int(spec["maximum"]):
            raise PatchSchemaError(f"{label} must be <= {spec['maximum']}")
    fmt = spec.get("format")
    if fmt:
        _validate_format(value, str(fmt), label)
    if isinstance(value, dict):
        required = spec.get("required", [])
        for field in required:
            if field not in value:
                raise PatchSchemaError(f"{label} missing required field: {field}")
        allowed = spec.get("allowed_fields")
        if isinstance(allowed, list):
            extra = sorted(set(value) - set(allowed))
            if extra:
                raise PatchSchemaError(f"{label} contains unsupported field(s): {', '.join(extra)}")
        fields = spec.get("fields", {})
        if isinstance(fields, dict):
            for key, child in fields.items():
                if key in value and isinstance(child, dict):
                    _validate_node(value[key], child, f"{label}.{key}")
    if isinstance(value, list) and isinstance(spec.get("items"), dict):
        for i, item in enumerate(value):
            _validate_node(item, spec["items"], f"{label}[{i}]")


def validate_manifest(manifest: dict[str, Any]) -> dict[str, Any]:
    schema = _load_schema()
    spec = schema.get("manifest")
    if not isinstance(spec, dict):
        raise PatchSchemaError("PATCH package schema is missing manifest definition")
    _validate_node(manifest, spec, "manifest")
    return schema


def parse_semver(value: str) -> tuple[int, int, int]:
    m = _SEMVER.fullmatch(str(value))
    if not m:
        raise PatchSchemaError(f"invalid semantic version: {value}", kind="compatibility_invalid")
    return tuple(int(x) for x in m.groups())  # type: ignore[return-value]


def check_compatibility(manifest: dict[str, Any], current_version: str = VERSION) -> list[str]:
    compat = manifest.get("compatibility")
    if not isinstance(compat, dict):
        return []
    cur = parse_semver(current_version)
    warnings: list[str] = []
    minimum = compat.get("min_tool_version")
    maximum = compat.get("max_tool_version")
    tested = compat.get("max_tested_version")
    if isinstance(minimum, str) and cur < parse_semver(minimum):
        raise PatchSchemaError(
            f"PATCH requires Python Patch Tool >= {minimum}; current={current_version}",
            kind="tool_version_incompatible",
        )
    if isinstance(maximum, str) and cur > parse_semver(maximum):
        raise PatchSchemaError(
            f"PATCH requires Python Patch Tool <= {maximum}; current={current_version}",
            kind="tool_version_incompatible",
        )
    if isinstance(tested, str) and cur > parse_semver(tested):
        warnings.append(f"current tool {current_version} is newer than patch max_tested_version {tested}")
    return warnings


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def resolve_project_path(root: Path, rel_text: str, *, allow_missing: bool = False) -> Path:
    canonical = _safe_rel(rel_text, label="project path")
    path = root.joinpath(*PurePosixPath(canonical).parts)
    if path.is_symlink():
        raise PatchSchemaError(f"project path must not be a symlink: {canonical}", path=canonical)
    if path.exists():
        try:
            path.resolve(strict=True).relative_to(root.resolve())
        except Exception as exc:
            raise PatchSchemaError(f"project path escapes project root: {canonical}", path=canonical) from exc
    elif not allow_missing:
        raise PatchSchemaError(f"project path does not exist: {canonical}", path=canonical)
    return path


def _check_command(root: Path, command: dict[str, Any], index: int) -> None:
    argv = command.get("argv")
    if not isinstance(argv, list) or not argv or not all(isinstance(x, str) and x for x in argv):
        raise PatchSchemaError(f"post_patch.commands[{index}].argv must be a non-empty string array")
    cwd_raw = str(command.get("cwd", "."))
    if cwd_raw == ".":
        cwd = root
    else:
        cwd = resolve_project_path(root, cwd_raw)
        if not cwd.is_dir():
            raise PatchSchemaError(f"post_patch.commands[{index}].cwd is not a directory: {cwd_raw}", path=cwd_raw)
    exe = argv[0]
    if "/" in exe or "\\" in exe:
        exe_path = Path(exe)
        if not exe_path.is_absolute():
            exe_path = cwd / exe_path
        if not exe_path.is_file():
            raise PatchSchemaError(f"post_patch.commands[{index}] executable not found: {exe}", kind="command_missing")
        if os.name != "nt" and not os.access(exe_path, os.X_OK):
            raise PatchSchemaError(f"post_patch.commands[{index}] executable is not executable: {exe}", kind="command_missing")
    elif shutil.which(exe) is None:
        raise PatchSchemaError(f"post_patch.commands[{index}] executable not found in PATH: {exe}", kind="command_missing")


def _ops_target_paths(ops: Any) -> set[str]:
    result: set[str] = set()
    if not isinstance(ops, list):
        return result
    for op in ops:
        if not isinstance(op, dict):
            continue
        raw = op.get("file")
        if isinstance(raw, str) and raw.strip():
            try: result.add(_safe_rel(raw, label="OPS file"))
            except PatchSchemaError: pass
        for key in ("then", "else", "alternatives"):
            nested = op.get(key)
            if isinstance(nested, list):
                for child in nested:
                    if isinstance(child, list): result.update(_ops_target_paths(child))
                    elif isinstance(child, dict): result.update(_ops_target_paths([child]))
    return result




def _validate_rollback_baseline_path(root: Path, rel: str, baseline: dict[str, Any]) -> None:
    """Require an exact, non-symlink parent chain for rollback targets.

    A missing rollback target is safe only when its parent directory already
    exists before payload execution. Otherwise removing the created file could
    leave newly-created directories behind while falsely reporting a complete
    rollback. Every ancestor is checked with lstat so a symlinked component
    cannot redirect rollback outside the project.
    """
    root_resolved = root.resolve(strict=True)
    pure = PurePosixPath(rel)
    current = root_resolved
    for part in pure.parts[:-1]:
        current = current / part
        try:
            st = current.lstat()
        except FileNotFoundError as exc:
            raise PatchSchemaError(
                f"rollback target parent must already exist before payload: {rel}",
                kind="rollback_parent_missing", path=rel,
            ) from exc
        if stat.S_ISLNK(st.st_mode):
            raise PatchSchemaError(
                f"rollback target ancestor must not be a symlink: {rel}",
                kind="rollback_path_unsafe", path=rel,
            )
        if not stat.S_ISDIR(st.st_mode):
            raise PatchSchemaError(
                f"rollback target ancestor is not a directory: {rel}",
                kind="rollback_path_unsafe", path=rel,
            )
        try:
            current.resolve(strict=True).relative_to(root_resolved)
        except Exception as exc:
            raise PatchSchemaError(
                f"rollback target ancestor escapes project root: {rel}",
                kind="rollback_path_unsafe", path=rel,
            ) from exc

    leaf = current / pure.name
    expected_exists = baseline.get("exists") is True
    try:
        st = leaf.lstat()
    except FileNotFoundError:
        if expected_exists:
            raise PatchSchemaError(
                f"rollback baseline changed before snapshot: {rel} is missing",
                kind="rollback_snapshot_race", path=rel,
            )
        return
    if not expected_exists:
        raise PatchSchemaError(
            f"rollback baseline changed before snapshot: {rel} now exists",
            kind="rollback_snapshot_race", path=rel,
        )
    if stat.S_ISLNK(st.st_mode) or not stat.S_ISREG(st.st_mode):
        raise PatchSchemaError(
            f"rollback target baseline must be a regular non-symlink file: {rel}",
            kind="rollback_path_unsafe", path=rel,
        )


def _rollback_contract(manifest: dict[str, Any], targets: set[str]) -> dict[str, Any] | None:
    recovery = manifest.get("recovery")
    if not isinstance(recovery, dict):
        return None
    rollback = recovery.get("rollback")
    if rollback is None:
        return None
    if not isinstance(rollback, dict):
        raise PatchSchemaError("recovery.rollback must be an object", kind="rollback_contract_invalid")
    raw_targets = rollback.get("targets")
    if not isinstance(raw_targets, list) or not raw_targets:
        raise PatchSchemaError("recovery.rollback.targets must be a non-empty array", kind="rollback_contract_invalid")
    rollback_targets = [_safe_rel(x, label="rollback target") for x in raw_targets]
    if len(set(rollback_targets)) != len(rollback_targets):
        raise PatchSchemaError("recovery.rollback.targets contains duplicate paths", kind="rollback_contract_invalid")
    if set(rollback_targets) != set(targets):
        missing = sorted(set(targets) - set(rollback_targets))
        extra = sorted(set(rollback_targets) - set(targets))
        raise PatchSchemaError(
            f"rollback targets must exactly cover PATCH targets; missing={missing} extra={extra}",
            kind="rollback_contract_invalid",
        )
    pre = manifest.get("preflight")
    specs = pre.get("files") if isinstance(pre, dict) else None
    if not isinstance(specs, list):
        raise PatchSchemaError(
            "recovery.rollback requires preflight.files baseline metadata for every target",
            kind="rollback_contract_invalid",
        )
    by_path: dict[str, dict[str, Any]] = {}
    for spec in specs:
        if not isinstance(spec, dict) or not isinstance(spec.get("path"), str):
            continue
        rel = _safe_rel(spec["path"], label="preflight file")
        if rel in by_path:
            raise PatchSchemaError(f"duplicate preflight baseline for rollback target: {rel}", kind="rollback_contract_invalid", path=rel)
        by_path[rel] = spec
    baselines: dict[str, dict[str, Any]] = {}
    for rel in rollback_targets:
        spec = by_path.get(rel)
        if spec is None or "exists" not in spec:
            raise PatchSchemaError(
                f"rollback target requires explicit preflight exists baseline: {rel}",
                kind="rollback_contract_invalid", path=rel,
            )
        exists = spec.get("exists")
        if exists is True and not isinstance(spec.get("sha256"), str):
            raise PatchSchemaError(
                f"existing rollback target requires preflight sha256 baseline: {rel}",
                kind="rollback_contract_invalid", path=rel,
            )
        baselines[rel] = {"exists": bool(exists)}
        if exists is True:
            baselines[rel]["sha256"] = str(spec.get("sha256")).lower()
    on = rollback.get("on", ["payload_failure", "post_patch_failure"])
    if not isinstance(on, list) or not on or not all(isinstance(x, str) for x in on):
        raise PatchSchemaError("recovery.rollback.on must be a non-empty array", kind="rollback_contract_invalid")
    allowed = {"payload_failure", "post_patch_failure"}
    if any(x not in allowed for x in on) or len(set(on)) != len(on):
        raise PatchSchemaError("recovery.rollback.on contains unsupported/duplicate stage", kind="rollback_contract_invalid")
    return {
        "targets": sorted(rollback_targets),
        "on": list(on),
        "max_total_bytes": int(rollback.get("max_total_bytes", 268435456)),
        "baselines": baselines,
    }

def run_preflight(
    root: Path,
    manifest: dict[str, Any],
    *,
    extracted: Path | None,
    kind: str,
    payload: Path,
    ops_data: dict[str, Any] | None = None,
) -> dict[str, Any]:
    validate_manifest(manifest)
    warnings = check_compatibility(manifest, VERSION)
    report: dict[str, Any] = {
        "status": "PASS",
        "tool_version": VERSION,
        "warnings": warnings,
        "checks": [],
        "target_paths": [],
    }
    checks: list[dict[str, Any]] = report["checks"]

    resources = manifest.get("resources") or []
    if resources and extracted is None:
        raise PatchSchemaError("manifest.resources requires an archive package")
    for raw in resources:
        rel = _safe_rel(raw, label="resource")
        path = extracted.joinpath(*PurePosixPath(rel).parts) if extracted is not None else None
        if path is None or path.is_symlink() or not path.is_file():
            raise PatchSchemaError(f"required package resource missing/non-regular: {rel}", kind="resource_missing", path=rel)
        checks.append({"kind": "resource", "path": rel, "status": "PASS"})

    targets: set[str] = set()
    for raw in manifest.get("targets") or []:
        targets.add(_safe_rel(raw, label="target"))
    if kind == "ops" and isinstance(ops_data, dict):
        operations = ops_data.get("ops")
        if not isinstance(operations, list):
            raise PatchSchemaError("PATCH_TOOL_OPS.json requires ops[]", kind="ops_invalid")
        targets.update(_ops_target_paths(operations))

    pre = manifest.get("preflight") or {}
    if isinstance(pre, dict) and bool(pre.get("require_clean_worktree", False)):
        git = root / ".git"
        if not git.exists():
            raise PatchSchemaError("preflight.require_clean_worktree requires a Git worktree", kind="worktree_requirement")
        import subprocess
        cp = subprocess.run(["git", "status", "--porcelain=v1"], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
        if cp.returncode != 0 or cp.stdout.strip():
            raise PatchSchemaError("preflight requires a clean Git worktree", kind="worktree_dirty")
        checks.append({"kind": "clean_worktree", "status": "PASS"})

    for spec in pre.get("files", []) if isinstance(pre, dict) else []:
        rel = _safe_rel(spec["path"], label="preflight file")
        expected_exists = bool(spec.get("exists", True))
        path = resolve_project_path(root, rel, allow_missing=True)
        exists = path.exists()
        if exists != expected_exists:
            raise PatchSchemaError(
                f"preflight existence mismatch: {rel} expected exists={expected_exists}, actual={exists}",
                kind="source_drift",
                path=rel,
            )
        if not exists:
            checks.append({"kind": "file", "path": rel, "status": "PASS", "exists": False})
            targets.add(rel)
            continue
        if path.is_symlink() or not path.is_file():
            raise PatchSchemaError(f"preflight path must be a regular file: {rel}", kind="source_drift", path=rel)
        actual_sha = sha256_file(path)
        expected_sha = spec.get("sha256")
        if isinstance(expected_sha, str) and actual_sha.lower() != expected_sha.lower():
            raise PatchSchemaError(
                f"preflight SHA-256 mismatch: {rel} expected={expected_sha.lower()} actual={actual_sha}",
                kind="source_drift",
                path=rel,
            )
        anchors = spec.get("anchors") or []
        if anchors:
            try: text = path.read_text(encoding="utf-8")
            except UnicodeDecodeError as exc:
                raise PatchSchemaError(f"preflight anchors require UTF-8 text file: {rel}", kind="anchor_mismatch", path=rel) from exc
            for anchor in anchors:
                if anchor not in text:
                    raise PatchSchemaError(f"preflight anchor missing: {rel}", kind="anchor_mismatch", path=rel)
        checks.append({"kind": "file", "path": rel, "status": "PASS", "sha256": actual_sha})
        targets.add(rel)

    rollback = _rollback_contract(manifest, targets)
    if rollback is not None:
        for rel in rollback["targets"]:
            _validate_rollback_baseline_path(root, rel, rollback["baselines"][rel])
        report["rollback"] = rollback
        checks.append({"kind": "rollback_contract", "status": "PASS", "targets": len(rollback["targets"]), "on": rollback["on"]})

    pp = manifest.get("post_patch")
    if isinstance(pp, dict):
        for i, cmd in enumerate(pp.get("commands") or []):
            _check_command(root, cmd, i)
        if pp.get("commands"):
            checks.append({"kind": "post_patch_commands", "status": "PASS", "count": len(pp.get("commands") or [])})

    git_policy = manifest.get("git")
    if isinstance(git_policy, dict) and any(git_policy.get(k) not in {None, "off", False} for k in ("add", "commit", "push")):
        if shutil.which("git") is None:
            raise PatchSchemaError("Git policy requested but git executable is not available", kind="command_missing")
        if not (root / ".git").exists():
            raise PatchSchemaError("Git policy requested but project is not a Git worktree", kind="worktree_requirement")
        checks.append({"kind": "git", "status": "PASS"})

    report["target_paths"] = sorted(targets)
    return report
