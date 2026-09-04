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

VERSION = "6.16.0"
SCHEMA_PATH = Path(__file__).resolve().parent / "docs" / "PATCH_PACKAGE_SCHEMA.json"
_HEX64 = re.compile(r"^[0-9a-fA-F]{64}$")
_SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")


class PatchSchemaError(ValueError):
    def __init__(
        self,
        message: str,
        *,
        kind: str = "schema_invalid",
        path: str | None = None,
        issues: list[dict[str, Any]] | None = None,
        report: dict[str, Any] | None = None,
    ):
        super().__init__(message)
        self.kind = kind
        self.path = path
        self.issues = list(issues or [])
        self.report = report


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


_FIELD_SUGGESTIONS = {
    "manifest.source_baseline": (
        "Unsupported legacy/custom field. Move baseline assumptions to "
        "manifest.preflight.files using path/exists/sha256/anchors."
    ),
    "manifest.execution.timeout_seconds": (
        "Remove timeout_seconds to use the default, or set an integer from 1 to 1800."
    ),
    "manifest.post_patch.commands[].timeout_seconds": (
        "Set an integer from 1 to 1800."
    ),
    "manifest.git.push": "Use exactly 'auto' or 'off'.",
}


def _suggestion_for(field: str, message: str) -> str | None:
    direct = _FIELD_SUGGESTIONS.get(field)
    if direct:
        return direct
    if ".post_patch.commands[" in field and field.endswith("].timeout_seconds"):
        return _FIELD_SUGGESTIONS["manifest.post_patch.commands[].timeout_seconds"]
    if "unsupported field" in message:
        return "Remove the field or migrate it to an allowed field defined by PATCH_PACKAGE_SCHEMA.json."
    return None


def _schema_issue(field: str, message: str, *, value: Any = None) -> dict[str, Any]:
    issue: dict[str, Any] = {"kind": "schema_invalid", "field": field, "message": message}
    suggestion = _suggestion_for(field, message)
    if suggestion:
        issue["suggestion"] = suggestion
    if value is not None and isinstance(value, (str, int, bool, float)):
        issue["actual"] = value
    return issue


def _validate_format_collect(value: Any, fmt: str, label: str, issues: list[dict[str, Any]]) -> None:
    try:
        _validate_format(value, fmt, label)
    except PatchSchemaError as exc:
        issues.append(_schema_issue(label, str(exc), value=value))


def _validate_node_collect(value: Any, spec: dict[str, Any], label: str, issues: list[dict[str, Any]]) -> None:
    expected = spec.get("type")
    if expected is not None and not _type_ok(value, expected):
        issues.append(_schema_issue(label, f"{label} has invalid type; expected {expected}", value=value))
        return
    if "const" in spec and value != spec["const"]:
        issues.append(_schema_issue(label, f"{label} must equal {spec['const']!r}", value=value))
    if "enum" in spec and value not in spec["enum"]:
        issues.append(_schema_issue(label, f"{label} has unsupported value {value!r}", value=value))
    if isinstance(value, int) and not isinstance(value, bool):
        if "minimum" in spec and value < int(spec["minimum"]):
            issues.append(_schema_issue(label, f"{label} must be >= {spec['minimum']}", value=value))
        if "maximum" in spec and value > int(spec["maximum"]):
            issues.append(_schema_issue(label, f"{label} must be <= {spec['maximum']}", value=value))
    fmt = spec.get("format")
    if fmt:
        _validate_format_collect(value, str(fmt), label, issues)
    if isinstance(value, dict):
        for field in spec.get("required", []):
            if field not in value:
                field_path = f"{label}.{field}"
                issues.append(_schema_issue(field_path, f"{label} missing required field: {field}"))
        allowed = spec.get("allowed_fields")
        if isinstance(allowed, list):
            for extra in sorted(set(value) - set(allowed)):
                field_path = f"{label}.{extra}"
                issues.append(_schema_issue(field_path, f"{label} contains unsupported field: {extra}", value=value.get(extra)))
        fields = spec.get("fields", {})
        if isinstance(fields, dict):
            for key, child in fields.items():
                if key in value and isinstance(child, dict):
                    _validate_node_collect(value[key], child, f"{label}.{key}", issues)
    if isinstance(value, list) and isinstance(spec.get("items"), dict):
        for i, item in enumerate(value):
            _validate_node_collect(item, spec["items"], f"{label}[{i}]", issues)


def collect_manifest_issues(manifest: dict[str, Any]) -> list[dict[str, Any]]:
    schema = _load_schema()
    spec = schema.get("manifest")
    if not isinstance(spec, dict):
        return [_schema_issue("manifest", "PATCH package schema is missing manifest definition")]
    issues: list[dict[str, Any]] = []
    _validate_node_collect(manifest, spec, "manifest", issues)
    return issues


def validate_manifest(manifest: dict[str, Any]) -> dict[str, Any]:
    schema = _load_schema()
    issues = collect_manifest_issues(manifest)
    if issues:
        summary = "; ".join(f"{x.get('field')}: {x.get('message')}" for x in issues[:8])
        if len(issues) > 8:
            summary += f"; ... {len(issues)-8} more"
        raise PatchSchemaError(
            f"manifest schema validation found {len(issues)} issue(s): {summary}",
            kind="schema_invalid",
            issues=issues,
        )
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


def _stat_is_link_or_reparse(st: os.stat_result) -> bool:
    if stat.S_ISLNK(st.st_mode):
        return True
    attrs = int(getattr(st, "st_file_attributes", 0) or 0)
    reparse = int(getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
    return bool(os.name == "nt" and attrs & reparse)


def path_is_link_or_reparse(path: Path) -> bool:
    try:
        return _stat_is_link_or_reparse(path.lstat())
    except FileNotFoundError:
        return False


def resolve_project_path(root: Path, rel_text: str, *, allow_missing: bool = False) -> Path:
    canonical = _safe_rel(rel_text, label="project path")
    path = root.joinpath(*PurePosixPath(canonical).parts)
    if path_is_link_or_reparse(path):
        raise PatchSchemaError(f"project path must not be a symlink/reparse point: {canonical}", path=canonical)
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
        if _stat_is_link_or_reparse(st):
            raise PatchSchemaError(
                f"rollback target ancestor must not be a symlink/reparse point: {rel}",
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
    if _stat_is_link_or_reparse(st) or not stat.S_ISREG(st.st_mode):
        raise PatchSchemaError(
            f"rollback target baseline must be a regular non-link/reparse file: {rel}",
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

    source_issues: list[dict[str, Any]] = []
    for file_index, spec in enumerate(pre.get("files", []) if isinstance(pre, dict) else []):
        rel = _safe_rel(spec["path"], label="preflight file")
        targets.add(rel)
        expected_exists = bool(spec.get("exists", True))
        path = resolve_project_path(root, rel, allow_missing=True)
        exists = path.exists()
        check: dict[str, Any] = {
            "kind": "file", "path": rel, "status": "PASS",
            "expected_exists": expected_exists, "actual_exists": exists,
        }
        if exists != expected_exists:
            check["status"] = "MISMATCH"
            issue = {
                "kind": "source_drift", "path": rel,
                "field": f"manifest.preflight.files[{file_index}].exists",
                "message": f"preflight existence mismatch: {rel} expected exists={expected_exists}, actual={exists}",
                "expected": expected_exists, "actual": exists,
            }
            source_issues.append(issue); checks.append(check)
            continue
        if not exists:
            checks.append(check)
            continue
        if path_is_link_or_reparse(path) or not path.is_file():
            check["status"] = "MISMATCH"
            source_issues.append({
                "kind": "source_drift", "path": rel,
                "field": f"manifest.preflight.files[{file_index}].path",
                "message": f"preflight path must be a regular non-symlink file: {rel}",
                "expected": "regular_file", "actual": "non_regular_or_symlink",
            })
            checks.append(check)
            continue
        actual_sha = sha256_file(path)
        check["actual_sha256"] = actual_sha
        expected_sha = spec.get("sha256")
        if isinstance(expected_sha, str):
            check["expected_sha256"] = expected_sha.lower()
            if actual_sha.lower() != expected_sha.lower():
                check["status"] = "MISMATCH"
                source_issues.append({
                    "kind": "source_drift", "path": rel,
                    "field": f"manifest.preflight.files[{file_index}].sha256",
                    "message": f"preflight SHA-256 mismatch: {rel} expected={expected_sha.lower()} actual={actual_sha}",
                    "expected": expected_sha.lower(), "actual": actual_sha,
                })
        anchors = spec.get("anchors") or []
        if anchors:
            try:
                text = path.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                check["status"] = "MISMATCH"
                source_issues.append({
                    "kind": "anchor_mismatch", "path": rel,
                    "field": f"manifest.preflight.files[{file_index}].anchors",
                    "message": f"preflight anchors require UTF-8 text file: {rel}",
                })
                checks.append(check)
                continue
            anchor_results: list[dict[str, Any]] = []
            for anchor_index, anchor in enumerate(anchors):
                present = anchor in text
                anchor_results.append({"index": anchor_index, "status": "PASS" if present else "MISSING", "anchor": anchor})
                if not present:
                    check["status"] = "MISMATCH"
                    source_issues.append({
                        "kind": "anchor_mismatch", "path": rel,
                        "field": f"manifest.preflight.files[{file_index}].anchors[{anchor_index}]",
                        "message": f"preflight anchor missing: {rel}",
                        "expected_anchor": anchor,
                    })
            check["anchors"] = anchor_results
        checks.append(check)

    report["target_paths"] = sorted(targets)
    if source_issues:
        report["status"] = "FAIL"
        report["issues"] = source_issues
        kinds = {str(x.get("kind")) for x in source_issues}
        primary = "source_drift" if "source_drift" in kinds else "anchor_mismatch"
        affected = sorted({str(x.get("path")) for x in source_issues if x.get("path")})
        raise PatchSchemaError(
            f"preflight source assumptions found {len(source_issues)} mismatch(es) across {len(affected)} file(s)",
            kind=primary,
            path=affected[0] if len(affected) == 1 else None,
            issues=source_issues,
            report=report,
        )

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
