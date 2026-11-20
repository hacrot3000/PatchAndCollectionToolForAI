#!/usr/bin/env python3
"""Read-only, bounded diagnostics for malformed COLLECT database_select actions.

This module NEVER repairs JSON, writes an output ZIP, resolves a DB profile or
executes the request. It describes schema shapes solely from the input bytes.
"""
from __future__ import annotations

import json
import re
from typing import Any

_SAFE_IDENTIFIER = re.compile(r"^(?:[A-Za-z_][A-Za-z0-9_$]*|\*)(?:\.(?:[A-Za-z_][A-Za-z0-9_$]*|\*))*$")
_SELECT_FIELDS = {"expr", "alias"}
_ORDER_FIELDS = {"expr", "direction"}


def _shape(value: Any) -> str:
    """Give an exact, single-line example only for safe validated names."""
    if isinstance(value, str) and len(value) <= 128 and _SAFE_IDENTIFIER.fullmatch(value):
        return json.dumps(value, ensure_ascii=True)
    return '"<existing validated column name>"'


def inspect_database_select_items(action: dict[str, Any], index: int) -> tuple[list[str], list[str], dict[str, int]]:
    """Find common nested field-shape mistakes missed by action-level scans.

    Error paths are 1-based for actions and 0-based for nested arrays, matching
    the Python validator's action[1].select[0] convention. The reported changes
    are instructions, never edits applied to the source request.
    """
    errors: list[str] = []
    fixes: list[str] = []
    counts = {"select": 0, "order_by": 0}
    if action.get("type") != "database_select":
        return errors, fixes, counts

    for field, allowed in (("select", _SELECT_FIELDS), ("order_by", _ORDER_FIELDS)):
        items = action.get(field)
        if not isinstance(items, list):
            continue
        for item_index, item in enumerate(items):
            if not isinstance(item, dict):
                continue
            unexpected = sorted(set(item) - allowed)
            if not unexpected:
                continue
            location = f"action[{index}].{field}[{item_index}]"
            listed = ", ".join(str(key)[:64] for key in unexpected[:5])
            errors.append(f"{location}: unsupported field(s): {listed}")
            if "column" in unexpected and "expr" not in item and set(item) <= (allowed | {"column"}):
                column = _shape(item["column"])
                if field == "select":
                    before_item = {"column": item["column"]}
                    after_item = {"expr": {"column": item["column"]}}
                    if "alias" in item:
                        before_item["alias"] = item["alias"]
                        after_item["alias"] = item["alias"]
                    kept = "Keep the original column name and alias (if present)."
                else:
                    direction = item.get("direction", "ASC")
                    if not isinstance(direction, str) or direction.upper() not in {"ASC", "DESC"}:
                        fixes.append(
                            f"{location}: direction is not valid ASC/DESC; retain the original "
                            "source value for review rather than guessing a sort direction."
                        )
                        continue
                    before_item = {"column": item["column"], "direction": direction}
                    after_item = {"expr": {"column": item["column"]}, "direction": direction.upper()}
                    kept = "Keep original sort priority and direction (case normalized only)."
                if column == '"<existing validated column name>"' or (
                    field == "select" and "alias" in item and (
                        not isinstance(item["alias"], str) or
                        not _SAFE_IDENTIFIER.fullmatch(item["alias"])
                    )
                ):
                    fixes.append(
                        f"{location}: expected "
                        + ('{"expr": {"column": "<existing validated column name>"}, "alias": "<existing alias if any>"}'
                           if field == "select" else
                           '{"expr": {"column": "<existing validated column name>"}, "direction": "ASC or DESC"}')
                        + "; existing column/alias must be validated against schema, never guessed."
                    )
                    continue
                before = json.dumps(before_item, ensure_ascii=True)
                after = json.dumps(after_item, ensure_ascii=True)
                fixes.append(
                    f"{location}: move existing column under expr: {before} -> {after}. {kept}"
                )
                counts[field] += 1
            else:
                fixes.append(
                    f"{location}: expected {field} item fields are "
                    + ("expr and optional alias" if field == "select" else "expr and optional direction")
                    + "; inspect this original item against DATABASE_SELECT_ACTIVE_BUILDER.md. "
                    "Do not discard unexpected fields or invent their meaning."
                )
    return errors, fixes, counts


def summarize_database_select(actions: Any) -> list[str]:
    """Include enough verified source metadata that an AI need not invent it."""
    if not isinstance(actions, list):
        return []
    results = []
    for index, action in enumerate(actions, 1):
        if not isinstance(action, dict) or action.get("type") != "database_select":
            continue
        def display(key: str) -> str:
            value = action.get(key)
            if isinstance(value, str) and len(value) <= 128 and not any(c in value for c in ("\r", "\n", "\x00", chr(96))):
                return json.dumps(value, ensure_ascii=True)
            if type(value) in (int, bool) and key in {"limit", "max_rows", "max_bytes", "timeout_sec"}:
                return str(value)
            return "(unavailable/invalid; inspect original JSON)"
        source = action.get("from")
        table = source.get("table") if isinstance(source, dict) else None
        if not (isinstance(table, str) and 0 < len(table) <= 128 and _SAFE_IDENTIFIER.fullmatch(table)):
            table = "(inspect original JSON)"
        counts = [len(action.get(k)) if isinstance(action.get(k), list) else "invalid" for k in ("select", "order_by")]
        results.append(
            f"- action[{index}]: id={display('id')}, type=database_select, "
            f"profile={display('profile')}, from.table={json.dumps(table)}, "
            f"select items={counts[0]}, order_by items={counts[1]}, "
            f"limit={display('limit')}, max_rows={display('max_rows')}, "
            f"max_bytes={display('max_bytes')}, timeout_sec={display('timeout_sec')}, "
            f"format={display('format')}"
        )
    return results
