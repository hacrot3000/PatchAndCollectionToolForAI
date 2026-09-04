#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from python_patch_database_select import DatabaseSelectError, validate_database_select_action

VERSION = "6.19.1"
SCHEMA_PATH = Path(__file__).resolve().parent / "docs" / "COLLECT_ACTION_SCHEMA.json"
DEFAULT_LIMITS = {
    "max_file_bytes": 8 * 1024 * 1024,
    "max_total_bytes": 256 * 1024 * 1024,
    "max_files": 5000,
    "max_report_bytes": 16 * 1024 * 1024,
    # Search/discovery budgets are intentionally separate from collection quotas.
    "max_search_files": 250000,
    "max_search_file_bytes": 64 * 1024 * 1024,
    # Large IDA/Ghidra text dumps historically exceeded normal per-file budgets.
    "max_decompile_file_bytes": 512 * 1024 * 1024,
}
HARD_LIMITS = {
    "max_file_bytes": 64 * 1024 * 1024,
    "max_total_bytes": 512 * 1024 * 1024,
    "max_files": 20000,
    "max_report_bytes": 64 * 1024 * 1024,
    "max_search_files": 1000000,
    "max_search_file_bytes": 256 * 1024 * 1024,
    "max_decompile_file_bytes": 2 * 1024 * 1024 * 1024,
}


class CollectSchemaError(ValueError):
    pass


def load_schema() -> dict[str, Any]:
    with SCHEMA_PATH.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict) or data.get("schema_version") != 1:
        raise CollectSchemaError("invalid bundled COLLECT_ACTION_SCHEMA.json")
    return data


def _assert_allowed_fields(obj: dict[str, Any], allowed: list[str], label: str) -> None:
    unknown = sorted(set(obj) - set(allowed))
    if unknown:
        raise CollectSchemaError(f"{label}: unsupported field(s): {', '.join(unknown)}")


def _path_list(value: Any, label: str, *, default_dot: bool = False) -> list[str]:
    if value is None and default_dot:
        return ["."]
    if not isinstance(value, list) or not value:
        raise CollectSchemaError(f"{label} must be a non-empty array of project-relative paths")
    out: list[str] = []
    for i, item in enumerate(value, 1):
        if not isinstance(item, str) or not item.strip():
            raise CollectSchemaError(f"{label}[{i}] must be a non-empty string")
        out.append(item.strip())
    return out


def _single_path(value: Any, label: str, *, default: str | None = None) -> str:
    if value is None:
        if default is None:
            raise CollectSchemaError(f"{label} must be a non-empty project-relative path")
        value = default
    if not isinstance(value, str) or not value.strip():
        raise CollectSchemaError(f"{label} must be a non-empty project-relative path")
    return value.strip()


def _bool(norm: dict[str, Any], field: str, label: str, default: bool) -> bool:
    value = norm.get(field, default)
    if not isinstance(value, bool):
        raise CollectSchemaError(f"{label}.{field} must be boolean")
    norm[field] = value
    return value


def _bounded_int(
    norm: dict[str, Any],
    field: str,
    label: str,
    default: int,
    minimum: int,
    maximum: int,
) -> int:
    value = norm.get(field, default)
    if not isinstance(value, int) or isinstance(value, bool) or not minimum <= value <= maximum:
        raise CollectSchemaError(
            f"{label}.{field} must be an integer from {minimum} to {maximum}"
        )
    norm[field] = value
    return value


def _string_list(
    value: Any,
    label: str,
    *,
    default: list[str] | None = None,
    allow_empty: bool = True,
) -> list[str]:
    if value is None:
        return list(default or [])
    if not isinstance(value, list):
        raise CollectSchemaError(f"{label} must be an array of strings")
    if not value and not allow_empty:
        raise CollectSchemaError(f"{label} must be a non-empty array of strings")
    out: list[str] = []
    for i, item in enumerate(value, 1):
        if not isinstance(item, str) or not item.strip():
            raise CollectSchemaError(f"{label}[{i}] must be a non-empty string")
        out.append(item.strip())
    return out


def _normalize_search_fields(norm: dict[str, Any], label: str) -> None:
    query = norm.get("query")
    if not isinstance(query, str) or not query:
        raise CollectSchemaError(f"{label}.query must be a non-empty string")
    norm["paths"] = _path_list(norm.get("paths", ["."]), f"{label}.paths")
    _bool(norm, "regex", label, False)
    _bounded_int(norm, "context_lines", label, 4, 0, 100)
    _bounded_int(norm, "max_matches", label, 500, 1, 20000)
    backend = norm.get("backend", "auto")
    if backend not in {"auto", "rg", "python"}:
        raise CollectSchemaError(f"{label}.backend must be auto, rg, or python")
    norm["backend"] = backend
    source_scope = norm.get("source_scope")
    filesystem_alias = norm.get("filesystem")
    if filesystem_alias is not None and not isinstance(filesystem_alias, bool):
        raise CollectSchemaError(f"{label}.filesystem must be boolean")
    if source_scope is None:
        source_scope = "filesystem" if filesystem_alias is not False else "git_tracked"
    if source_scope not in {"filesystem", "git_tracked"}:
        raise CollectSchemaError(f"{label}.source_scope must be filesystem or git_tracked")
    if filesystem_alias is not None and (
        (filesystem_alias and source_scope != "filesystem")
        or (not filesystem_alias and source_scope != "git_tracked")
    ):
        raise CollectSchemaError(f"{label}.filesystem conflicts with source_scope")
    norm["source_scope"] = source_scope
    norm["filesystem"] = source_scope == "filesystem"
    for flag, default in (
        ("respect_gitignore", False),
        ("follow_symlinks", False),
        ("must_find", False),
        ("diagnose_on_zero", True),
        ("fallback_search", True),
        ("verify_nonzero_with_fallback", False),
        ("report_coverage", True),
        ("report_skipped_dirs", True),
        ("module_discovery", True),
    ):
        _bool(norm, flag, label, default)
    for field in ("anchor_paths", "expected_files"):
        value = norm.get(field, [])
        norm[field] = [] if value == [] else _path_list(value, f"{label}.{field}")


def _normalize_research(norm: dict[str, Any], label: str) -> None:
    # Historical requests used both path and paths.  Canonicalize to paths while
    # retaining path in the normalized object for report provenance.
    path = norm.get("path")
    paths = norm.get("paths")
    if path is not None:
        path = _single_path(path, f"{label}.path")
        norm["path"] = path
        if paths is None:
            norm["paths"] = [path]
    _normalize_search_fields(norm, label)
    _bounded_int(norm, "tree_depth", label, 2, 0, 8)


def _normalize_line_reader(norm: dict[str, Any], label: str, action_type: str) -> None:
    norm["path"] = _single_path(norm.get("path"), f"{label}.path")
    if action_type in {"head", "tail"}:
        _bounded_int(norm, "lines", label, 100, 1, 20000)
        return
    start = norm.get("start_line")
    end = norm.get("end_line")
    if action_type == "range" and (start is None or end is None):
        raise CollectSchemaError(f"{label}.range requires start_line and end_line")
    if start is not None:
        if not isinstance(start, int) or isinstance(start, bool) or start < 1:
            raise CollectSchemaError(f"{label}.start_line must be a positive integer")
        norm["start_line"] = start
    if end is not None:
        if not isinstance(end, int) or isinstance(end, bool) or end < 1:
            raise CollectSchemaError(f"{label}.end_line must be a positive integer")
        norm["end_line"] = end
    if start is not None and end is not None and start > end:
        raise CollectSchemaError(f"{label}.start_line must not exceed end_line")


def _normalize_decompile(norm: dict[str, Any], label: str) -> None:
    source = norm.get("source", norm.get("path"))
    source = _single_path(source, f"{label}.source")
    norm["source"] = source
    if norm.get("path") is not None:
        norm["path"] = _single_path(norm.get("path"), f"{label}.path")
    address = norm.get("address")
    name = norm.get("name", norm.get("symbol"))
    if address is None and name is None:
        raise CollectSchemaError(f"{label} requires address or name/symbol")
    if address is not None and not isinstance(address, (str, int)):
        raise CollectSchemaError(f"{label}.address must be a hexadecimal string or integer")
    if name is not None:
        if not isinstance(name, str) or not name.strip():
            raise CollectSchemaError(f"{label}.name/symbol must be a non-empty string")
        norm["name"] = name.strip()
    match = norm.get("match", "contains")
    if match not in {"exact", "contains", "regex"}:
        raise CollectSchemaError(f"{label}.match must be exact, contains, or regex")
    norm["match"] = match
    _bool(norm, "case_sensitive", label, True)
    _bounded_int(norm, "neighbors_before", label, 0, 0, 20)
    _bounded_int(norm, "neighbors_after", label, 0, 0, 20)
    _bounded_int(norm, "max_matches", label, 20, 1, 1000)
    include = norm.get("include_references", norm.get("references", False))
    if not isinstance(include, bool):
        raise CollectSchemaError(f"{label}.include_references/references must be boolean")
    norm["include_references"] = include
    if norm.get("references") is not None:
        norm["references"] = include
    ref = norm.get("reference_term")
    if ref is not None and (not isinstance(ref, str) or not ref):
        raise CollectSchemaError(f"{label}.reference_term must be a non-empty string")
    _bounded_int(norm, "reference_context_lines", label, 8, 0, 100)
    _bounded_int(norm, "max_reference_hits", label, 80, 1, 5000)


def validate_request_data(data: Any) -> dict[str, Any]:
    schema = load_schema()
    if not isinstance(data, dict):
        raise CollectSchemaError("request JSON root must be an object")
    req_schema = schema["request"]
    _assert_allowed_fields(data, req_schema["allowed_fields"], "request")
    actions = data.get("actions")
    if not isinstance(actions, list) or not actions:
        raise CollectSchemaError("request.actions must be a non-empty array")
    if data.get("id") is not None and not isinstance(data.get("id"), str):
        raise CollectSchemaError("request.id must be a string when present")
    if data.get("title") is not None and not isinstance(data.get("title"), str):
        raise CollectSchemaError("request.title must be a string when present")

    raw_limits = data.get("limits", {})
    if raw_limits is None:
        raw_limits = {}
    if not isinstance(raw_limits, dict):
        raise CollectSchemaError("request.limits must be an object when present")
    _assert_allowed_fields(raw_limits, schema["limits"]["allowed_fields"], "request.limits")
    limits = dict(DEFAULT_LIMITS)
    for key, value in raw_limits.items():
        if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
            raise CollectSchemaError(f"request.limits.{key} must be a positive integer")
        hard = HARD_LIMITS[key]
        if value > hard:
            raise CollectSchemaError(f"request.limits.{key} exceeds tool hard ceiling ({hard})")
        limits[key] = value
    if limits["max_file_bytes"] > limits["max_total_bytes"]:
        raise CollectSchemaError("max_file_bytes must not exceed max_total_bytes")

    normalized_actions: list[dict[str, Any]] = []
    supported = schema["actions"]
    for index, action in enumerate(actions, 1):
        label = f"action[{index}]"
        if not isinstance(action, dict):
            raise CollectSchemaError(f"{label} must be an object")
        action_type = action.get("type")
        if not isinstance(action_type, str) or not action_type.strip():
            raise CollectSchemaError(f"{label}.type must be a non-empty string")
        action_type = action_type.strip().lower()
        if action_type not in supported:
            raise CollectSchemaError(
                f"{label}: unsupported action type: {action_type}; "
                f"supported={','.join(sorted(supported))}"
            )
        spec = supported[action_type]
        _assert_allowed_fields(action, spec["allowed_fields"], label)
        for field in spec.get("required", []):
            if field not in action:
                raise CollectSchemaError(f"{label}: missing required field: {field}")
        norm = dict(action)
        norm["type"] = action_type
        if norm.get("id") is not None and not isinstance(norm.get("id"), str):
            raise CollectSchemaError(f"{label}.id must be a string")
        if norm.get("title") is not None and not isinstance(norm.get("title"), str):
            raise CollectSchemaError(f"{label}.title must be a string")

        if action_type in {"pack", "zip"}:
            norm["paths"] = _path_list(norm.get("paths"), f"{label}.paths")
        elif action_type == "overview":
            norm["path"] = _single_path(norm.get("path"), f"{label}.path", default=".")
            _bounded_int(norm, "tree_depth", label, 2, 0, 8)
        elif action_type == "ls":
            norm["path"] = _single_path(norm.get("path"), f"{label}.path", default=".")
            _bounded_int(norm, "max_entries", label, 1000, 1, 20000)
        elif action_type == "tree":
            norm["path"] = _single_path(norm.get("path"), f"{label}.path", default=".")
            _bounded_int(norm, "max_depth", label, 4, 0, 16)
            _bounded_int(norm, "max_entries", label, 5000, 1, 50000)
        elif action_type == "find":
            norm["paths"] = _path_list(norm.get("paths", ["."]), f"{label}.paths")
            patterns = _string_list(norm.get("patterns"), f"{label}.patterns", allow_empty=False)
            norm["patterns"] = patterns
            _bool(norm, "collect", label, False)
            _bounded_int(norm, "max_results", label, 1000, 1, 20000)
        elif action_type in {"search", "search_files", "content"}:
            _normalize_search_fields(norm, label)
        elif action_type == "research":
            _normalize_research(norm, label)
        elif action_type in {"file", "range", "head", "tail"}:
            _normalize_line_reader(norm, label, action_type)
        elif action_type == "symbol":
            norm["path"] = _single_path(norm.get("path"), f"{label}.path")
            symbol = norm.get("symbol")
            if not isinstance(symbol, str) or not symbol.strip():
                raise CollectSchemaError(f"{label}.symbol must be a non-empty string")
            norm["symbol"] = symbol.strip()
            _bounded_int(norm, "context_lines", label, 8, 0, 100)
            _bounded_int(norm, "max_blocks", label, 20, 1, 200)
        elif action_type == "references":
            symbol = norm.get("symbol")
            if not isinstance(symbol, str) or not symbol.strip():
                raise CollectSchemaError(f"{label}.symbol must be a non-empty string")
            norm["symbol"] = symbol.strip()
            norm["query"] = symbol.strip()
            # query is synthetic and not part of the external references schema.
            _normalize_search_fields(norm, label)
            norm.pop("query", None)
        elif action_type == "callgraph":
            symbol = norm.get("symbol")
            if not isinstance(symbol, str) or not symbol.strip():
                raise CollectSchemaError(f"{label}.symbol must be a non-empty string")
            norm["symbol"] = symbol.strip()
            norm["paths"] = _path_list(norm.get("paths", [norm.get("path", ".")]), f"{label}.paths")
            if norm.get("path") is not None:
                norm["path"] = _single_path(norm.get("path"), f"{label}.path")
            _bounded_int(norm, "context_lines", label, 6, 0, 100)
            _bounded_int(norm, "max_callers", label, 200, 1, 5000)
            _bounded_int(norm, "max_callees", label, 100, 1, 2000)
            _bounded_int(norm, "max_occurrences", label, 1000, 1, 20000)
        elif action_type == "dependencies":
            norm["paths"] = _path_list(norm.get("paths", [norm.get("path", ".")]), f"{label}.paths")
            if norm.get("path") is not None:
                norm["path"] = _single_path(norm.get("path"), f"{label}.path")
            _bounded_int(norm, "max_results", label, 2000, 1, 50000)
            _bounded_int(norm, "dependency_depth", label, 1, 0, 8)
        elif action_type == "directory":
            norm["path"] = _single_path(norm.get("path"), f"{label}.path")
            norm["include"] = _string_list(norm.get("include"), f"{label}.include", default=["*"])
            norm["exclude"] = _string_list(norm.get("exclude"), f"{label}.exclude", default=[])
            _bounded_int(norm, "max_results", label, 5000, 1, 20000)
        elif action_type == "symbol_graph":
            norm["paths"] = _path_list(norm.get("paths", ["."]), f"{label}.paths")
            norm["symbols"] = _string_list(norm.get("symbols"), f"{label}.symbols", allow_empty=False)
            _bounded_int(norm, "context_lines", label, 8, 0, 100)
            for field, default in (
                ("include_references", True),
                ("include_callers", True),
                ("include_callees", True),
                ("include_dependencies", True),
            ):
                _bool(norm, field, label, default)
            _bounded_int(norm, "dependency_depth", label, 1, 0, 8)
            _bounded_int(norm, "max_occurrences", label, 1200, 1, 20000)
            _bounded_int(norm, "max_callers", label, 300, 1, 5000)
            _bounded_int(norm, "max_callees", label, 100, 1, 2000)
            _bounded_int(norm, "max_dependency_files", label, 400, 1, 10000)
        elif action_type in {"decompile", "ida", "ghidra"}:
            _normalize_decompile(norm, label)
        elif action_type == "database_select":
            try:
                norm = validate_database_select_action(norm, label)
            except DatabaseSelectError as exc:
                raise CollectSchemaError(str(exc)) from exc
        elif action_type == "git":
            sections = norm.get("sections")
            allowed_sections = set(schema["git_sections"])
            if not isinstance(sections, list) or not sections:
                raise CollectSchemaError(f"{label}.sections must be a non-empty array")
            cleaned: list[str] = []
            for i, section in enumerate(sections, 1):
                if not isinstance(section, str) or section not in allowed_sections:
                    raise CollectSchemaError(
                        f"{label}.sections[{i}] unsupported; allowed={','.join(sorted(allowed_sections))}"
                    )
                if section not in cleaned:
                    cleaned.append(section)
            norm["sections"] = cleaned
            _bounded_int(norm, "log_entries", label, 20, 1, 200)
        normalized_actions.append(norm)

    return {
        "id": data.get("id"),
        "title": data.get("title"),
        "actions": normalized_actions,
        "limits": limits,
    }
