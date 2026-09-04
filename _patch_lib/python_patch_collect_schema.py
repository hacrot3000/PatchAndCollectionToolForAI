#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path
from typing import Any

VERSION = "6.17.3"
SCHEMA_PATH = Path(__file__).resolve().parent / "docs" / "COLLECT_ACTION_SCHEMA.json"
DEFAULT_LIMITS = {
    "max_file_bytes": 8 * 1024 * 1024,
    "max_total_bytes": 256 * 1024 * 1024,
    "max_files": 5000,
    "max_report_bytes": 16 * 1024 * 1024,
}
HARD_LIMITS = {
    "max_file_bytes": 64 * 1024 * 1024,
    "max_total_bytes": 512 * 1024 * 1024,
    "max_files": 20000,
    "max_report_bytes": 64 * 1024 * 1024,
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

        if action_type == "pack":
            norm["paths"] = _path_list(norm.get("paths"), f"{label}.paths")
        elif action_type == "overview":
            path = norm.get("path", ".")
            if not isinstance(path, str) or not path.strip():
                raise CollectSchemaError(f"{label}.path must be a non-empty string")
            norm["path"] = path.strip()
            depth = norm.get("tree_depth", 2)
            if not isinstance(depth, int) or isinstance(depth, bool) or not 0 <= depth <= 8:
                raise CollectSchemaError(f"{label}.tree_depth must be an integer from 0 to 8")
            norm["tree_depth"] = depth
        elif action_type == "find":
            norm["paths"] = _path_list(norm.get("paths", ["."]), f"{label}.paths")
            patterns = norm.get("patterns")
            if not isinstance(patterns, list) or not patterns:
                raise CollectSchemaError(f"{label}.patterns must be a non-empty array")
            for i, pattern in enumerate(patterns, 1):
                if not isinstance(pattern, str) or not pattern.strip():
                    raise CollectSchemaError(f"{label}.patterns[{i}] must be a non-empty string")
            norm["patterns"] = [p.strip() for p in patterns]
            if not isinstance(norm.get("collect", False), bool):
                raise CollectSchemaError(f"{label}.collect must be boolean")
            norm["collect"] = bool(norm.get("collect", False))
            mr = norm.get("max_results", 1000)
            if not isinstance(mr, int) or isinstance(mr, bool) or not 1 <= mr <= 20000:
                raise CollectSchemaError(f"{label}.max_results must be an integer from 1 to 20000")
            norm["max_results"] = mr
        elif action_type == "search":
            query = norm.get("query")
            if not isinstance(query, str) or not query:
                raise CollectSchemaError(f"{label}.query must be a non-empty string")
            norm["paths"] = _path_list(norm.get("paths", ["."]), f"{label}.paths")
            if not isinstance(norm.get("regex", False), bool):
                raise CollectSchemaError(f"{label}.regex must be boolean")
            norm["regex"] = bool(norm.get("regex", False))
            cl = norm.get("context_lines", 4)
            if not isinstance(cl, int) or isinstance(cl, bool) or not 0 <= cl <= 100:
                raise CollectSchemaError(f"{label}.context_lines must be an integer from 0 to 100")
            norm["context_lines"] = cl
            mm = norm.get("max_matches", 500)
            if not isinstance(mm, int) or isinstance(mm, bool) or not 1 <= mm <= 20000:
                raise CollectSchemaError(f"{label}.max_matches must be an integer from 1 to 20000")
            norm["max_matches"] = mm
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
            le = norm.get("log_entries", 20)
            if not isinstance(le, int) or isinstance(le, bool) or not 1 <= le <= 200:
                raise CollectSchemaError(f"{label}.log_entries must be an integer from 1 to 200")
            norm["log_entries"] = le
        normalized_actions.append(norm)

    return {
        "id": data.get("id"),
        "title": data.get("title"),
        "actions": normalized_actions,
        "limits": limits,
    }
