#!/usr/bin/env python3
from __future__ import annotations

import csv
import io
import json
import math
import os
from pathlib import Path
import queue
import re
import shutil
import socket
import sqlite3
import stat
import subprocess
import tempfile
import threading
import time
from typing import Any, Iterable
from urllib.parse import quote

VERSION = "6.19.4"

PROFILE_ENV = "PTV_DB_PROFILES_FILE"
PROFILE_CANDIDATES = (
    "tools/db_profiles.local.json",
    ".python_patch_tool/db_profiles.local.json",
)

IDENT_PART_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_$]*$")
PROFILE_NAME_RE = re.compile(r"^[A-Za-z0-9_.-]{1,96}$")
SAFE_LOCAL_HOSTS = {"127.0.0.1", "localhost", "::1"}

COMMON_FUNCTIONS = {
    "COUNT", "SUM", "AVG", "MIN", "MAX",
    "COALESCE", "NULLIF", "IFNULL",
    "LOWER", "UPPER", "LENGTH", "TRIM", "LTRIM", "RTRIM",
    "ROUND", "ABS", "CEIL", "CEILING", "FLOOR", "SQRT", "POWER",
    "SUBSTR", "SUBSTRING", "REPLACE", "INSTR",
    "CONCAT", "CONCAT_WS",
    "DATE", "DATETIME", "TIME", "YEAR", "MONTH", "DAY",
    "DATE_FORMAT", "STRFTIME",
    "JSON_EXTRACT", "JSON_UNQUOTE",
    "ROW_NUMBER", "RANK", "DENSE_RANK", "LAG", "LEAD", "FIRST_VALUE", "LAST_VALUE",
}
MYSQL_FUNCTIONS = COMMON_FUNCTIONS | {"CHAR_LENGTH", "UTC_TIMESTAMP", "NOW"}
SQLITE_FUNCTIONS = COMMON_FUNCTIONS | {"TOTAL", "JULIANDAY", "UNIXEPOCH"}

BINARY_OPS = {"+", "-", "*", "/", "%"}
COMPARE_OPS = {
    "=", "!=", "<>", "<", "<=", ">", ">=",
    "LIKE", "NOT LIKE", "IN", "NOT IN", "BETWEEN", "NOT BETWEEN",
    "IS NULL", "IS NOT NULL",
}
JOIN_TYPES = {"INNER", "LEFT", "RIGHT", "CROSS"}
ORDER_DIRECTIONS = {"ASC", "DESC"}
CAST_TYPES = {
    "INTEGER", "INT", "REAL", "NUMERIC", "TEXT", "CHAR", "VARCHAR",
    "DATE", "DATETIME", "DECIMAL", "SIGNED", "UNSIGNED", "BINARY",
}
FRAME_UNITS = {"ROWS", "RANGE"}
FRAME_BOUNDARY_WORDS = {"UNBOUNDED PRECEDING", "CURRENT ROW", "UNBOUNDED FOLLOWING"}

DB_ACTION_ALLOWED_FIELDS = [
    "id", "title", "type", "profile",
    "select", "from", "joins", "where", "group_by", "having", "order_by",
    "distinct", "limit", "offset",
    "format", "max_rows", "max_bytes", "timeout_sec", "chunk_rows", "must_return_rows",
]
DB_ACTION_REQUIRED_FIELDS = ["type", "profile", "select", "from"]

MAX_AST_DEPTH = 8
MAX_AST_NODES = 5000
MAX_SELECT_ITEMS = 256
MAX_JOINS = 64
MAX_GROUP_ITEMS = 128
MAX_ORDER_ITEMS = 128
MAX_IN_ITEMS = 5000
MAX_ROWS_HARD = 1_000_000
MAX_BYTES_HARD = 256 * 1024 * 1024
MAX_TIMEOUT_HARD = 600


class DatabaseSelectError(ValueError):
    pass


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise DatabaseSelectError(f"duplicate JSON key in database profile file: {key}")
        out[key] = value
    return out


def _assert_keys(obj: dict[str, Any], allowed: Iterable[str], label: str) -> None:
    unknown = sorted(set(obj) - set(allowed))
    if unknown:
        raise DatabaseSelectError(f"{label}: unsupported field(s): {', '.join(unknown)}")


def _identifier(value: Any, label: str, *, dotted: bool = False, wildcard: bool = False) -> str:
    if not isinstance(value, str) or not value:
        raise DatabaseSelectError(f"{label} must be a non-empty identifier")
    if value == "*" and wildcard:
        return value
    parts = value.split(".") if dotted else [value]
    if wildcard and parts and parts[-1] == "*":
        check = parts[:-1]
        if not check:
            return "*"
    else:
        check = parts
    if not check or any(not IDENT_PART_RE.fullmatch(part) for part in check):
        raise DatabaseSelectError(f"{label} contains an unsafe identifier: {value}")
    return value


def _json_scalar(value: Any, label: str) -> Any:
    if value is None or isinstance(value, (str, bool, int)):
        return value
    if isinstance(value, float):
        if not math.isfinite(value):
            raise DatabaseSelectError(f"{label} must not be NaN/Infinity")
        return value
    raise DatabaseSelectError(f"{label} must be a JSON scalar (null/string/bool/number)")


class _AstBudget:
    def __init__(self):
        self.nodes = 0

    def touch(self, label: str) -> None:
        self.nodes += 1
        if self.nodes > MAX_AST_NODES:
            raise DatabaseSelectError(f"database_select AST exceeds {MAX_AST_NODES} nodes near {label}")


def _validate_order_item(item: Any, label: str, budget: _AstBudget, depth: int) -> dict[str, Any]:
    if not isinstance(item, dict):
        raise DatabaseSelectError(f"{label} must be an object")
    _assert_keys(item, {"expr", "direction"}, label)
    if "expr" not in item:
        raise DatabaseSelectError(f"{label}.expr is required")
    direction = str(item.get("direction", "ASC")).upper()
    if direction not in ORDER_DIRECTIONS:
        raise DatabaseSelectError(f"{label}.direction must be ASC or DESC")
    return {"expr": _validate_expr(item["expr"], f"{label}.expr", budget, depth + 1), "direction": direction}


def _validate_frame_boundary(value: Any, label: str) -> Any:
    if isinstance(value, str):
        word = " ".join(value.upper().split())
        if word not in FRAME_BOUNDARY_WORDS:
            raise DatabaseSelectError(f"{label} has unsupported frame boundary")
        return word
    if isinstance(value, dict):
        _assert_keys(value, {"preceding", "following"}, label)
        if len(value) != 1:
            raise DatabaseSelectError(f"{label} must contain exactly preceding or following")
        key = next(iter(value))
        amount = value[key]
        if not isinstance(amount, int) or isinstance(amount, bool) or not 0 <= amount <= 1_000_000:
            raise DatabaseSelectError(f"{label}.{key} must be an integer from 0 to 1000000")
        return {key: amount}
    raise DatabaseSelectError(f"{label} must be a fixed boundary string or bounded offset object")


def _validate_over(value: Any, label: str, budget: _AstBudget, depth: int) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise DatabaseSelectError(f"{label} must be an object")
    _assert_keys(value, {"partition_by", "order_by", "frame"}, label)
    out: dict[str, Any] = {"partition_by": [], "order_by": []}
    part = value.get("partition_by", [])
    if not isinstance(part, list) or len(part) > MAX_GROUP_ITEMS:
        raise DatabaseSelectError(f"{label}.partition_by must be an array up to {MAX_GROUP_ITEMS} items")
    out["partition_by"] = [_validate_expr(x, f"{label}.partition_by[{i}]", budget, depth + 1) for i, x in enumerate(part)]
    order = value.get("order_by", [])
    if not isinstance(order, list) or len(order) > MAX_ORDER_ITEMS:
        raise DatabaseSelectError(f"{label}.order_by must be an array up to {MAX_ORDER_ITEMS} items")
    out["order_by"] = [_validate_order_item(x, f"{label}.order_by[{i}]", budget, depth + 1) for i, x in enumerate(order)]
    if "frame" in value:
        frame = value["frame"]
        if not isinstance(frame, dict):
            raise DatabaseSelectError(f"{label}.frame must be an object")
        _assert_keys(frame, {"unit", "start", "end"}, f"{label}.frame")
        unit = str(frame.get("unit", "ROWS")).upper()
        if unit not in FRAME_UNITS:
            raise DatabaseSelectError(f"{label}.frame.unit must be ROWS or RANGE")
        if "start" not in frame:
            raise DatabaseSelectError(f"{label}.frame.start is required")
        out["frame"] = {
            "unit": unit,
            "start": _validate_frame_boundary(frame["start"], f"{label}.frame.start"),
            "end": _validate_frame_boundary(frame.get("end", "CURRENT ROW"), f"{label}.frame.end"),
        }
    return out


def _validate_expr(value: Any, label: str, budget: _AstBudget, depth: int) -> dict[str, Any]:
    budget.touch(label)
    if depth > MAX_AST_DEPTH:
        raise DatabaseSelectError(f"{label} exceeds maximum AST nesting depth {MAX_AST_DEPTH}")
    if not isinstance(value, dict) or len(value) != 1:
        raise DatabaseSelectError(
            f"{label} must be an expression object with exactly one of column/value/function/binary/case/subquery/cast"
        )
    key = next(iter(value))
    payload = value[key]
    if key == "column":
        return {"column": _identifier(payload, f"{label}.column", dotted=True, wildcard=True)}
    if key == "value":
        return {"value": _json_scalar(payload, f"{label}.value")}
    if key == "function":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.function must be an object")
        _assert_keys(payload, {"name", "args", "distinct", "over"}, f"{label}.function")
        name = str(payload.get("name", "")).upper()
        if name not in MYSQL_FUNCTIONS | SQLITE_FUNCTIONS:
            raise DatabaseSelectError(f"{label}.function.name is not allowlisted: {name or '<empty>'}")
        args = payload.get("args", [])
        if not isinstance(args, list) or len(args) > 64:
            raise DatabaseSelectError(f"{label}.function.args must be an array up to 64 items")
        out = {
            "name": name,
            "args": [_validate_expr(x, f"{label}.function.args[{i}]", budget, depth + 1) for i, x in enumerate(args)],
            "distinct": bool(payload.get("distinct", False)),
        }
        if "distinct" in payload and not isinstance(payload["distinct"], bool):
            raise DatabaseSelectError(f"{label}.function.distinct must be boolean")
        if "over" in payload:
            out["over"] = _validate_over(payload["over"], f"{label}.function.over", budget, depth + 1)
        return {"function": out}
    if key == "binary":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.binary must be an object")
        _assert_keys(payload, {"op", "left", "right"}, f"{label}.binary")
        op = str(payload.get("op", "")).upper()
        if op not in BINARY_OPS:
            raise DatabaseSelectError(f"{label}.binary.op unsupported: {op}")
        if "left" not in payload or "right" not in payload:
            raise DatabaseSelectError(f"{label}.binary requires left and right")
        return {"binary": {
            "op": op,
            "left": _validate_expr(payload["left"], f"{label}.binary.left", budget, depth + 1),
            "right": _validate_expr(payload["right"], f"{label}.binary.right", budget, depth + 1),
        }}
    if key == "case":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.case must be an object")
        _assert_keys(payload, {"when", "else"}, f"{label}.case")
        whens = payload.get("when")
        if not isinstance(whens, list) or not whens or len(whens) > 64:
            raise DatabaseSelectError(f"{label}.case.when must be a non-empty array up to 64 items")
        out_when = []
        for i, row in enumerate(whens):
            if not isinstance(row, dict):
                raise DatabaseSelectError(f"{label}.case.when[{i}] must be an object")
            _assert_keys(row, {"if", "then"}, f"{label}.case.when[{i}]")
            if "if" not in row or "then" not in row:
                raise DatabaseSelectError(f"{label}.case.when[{i}] requires if and then")
            out_when.append({
                "if": _validate_condition(row["if"], f"{label}.case.when[{i}].if", budget, depth + 1),
                "then": _validate_expr(row["then"], f"{label}.case.when[{i}].then", budget, depth + 1),
            })
        out = {"when": out_when}
        if "else" in payload:
            out["else"] = _validate_expr(payload["else"], f"{label}.case.else", budget, depth + 1)
        return {"case": out}
    if key == "subquery":
        return {"subquery": _validate_select_spec(payload, f"{label}.subquery", budget, depth + 1)}
    if key == "cast":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.cast must be an object")
        _assert_keys(payload, {"expr", "as"}, f"{label}.cast")
        cast_type = str(payload.get("as", "")).upper()
        if cast_type not in CAST_TYPES:
            raise DatabaseSelectError(f"{label}.cast.as unsupported: {cast_type or '<empty>'}")
        if "expr" not in payload:
            raise DatabaseSelectError(f"{label}.cast.expr is required")
        return {"cast": {"expr": _validate_expr(payload["expr"], f"{label}.cast.expr", budget, depth + 1), "as": cast_type}}
    raise DatabaseSelectError(f"{label}: unsupported expression kind: {key}")


def _validate_condition(value: Any, label: str, budget: _AstBudget, depth: int) -> dict[str, Any]:
    budget.touch(label)
    if depth > MAX_AST_DEPTH:
        raise DatabaseSelectError(f"{label} exceeds maximum AST nesting depth {MAX_AST_DEPTH}")
    if not isinstance(value, dict) or len(value) != 1:
        raise DatabaseSelectError(f"{label} must contain exactly one of and/or/not/compare/exists")
    key = next(iter(value))
    payload = value[key]
    if key in {"and", "or"}:
        if not isinstance(payload, list) or not payload or len(payload) > 256:
            raise DatabaseSelectError(f"{label}.{key} must be a non-empty array up to 256 conditions")
        return {key: [_validate_condition(x, f"{label}.{key}[{i}]", budget, depth + 1) for i, x in enumerate(payload)]}
    if key == "not":
        return {"not": _validate_condition(payload, f"{label}.not", budget, depth + 1)}
    if key == "exists":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.exists must be an object")
        _assert_keys(payload, {"query", "not"}, f"{label}.exists")
        if "query" not in payload:
            raise DatabaseSelectError(f"{label}.exists.query is required")
        negate = payload.get("not", False)
        if not isinstance(negate, bool):
            raise DatabaseSelectError(f"{label}.exists.not must be boolean")
        return {"exists": {"query": _validate_select_spec(payload["query"], f"{label}.exists.query", budget, depth + 1), "not": negate}}
    if key == "compare":
        if not isinstance(payload, dict):
            raise DatabaseSelectError(f"{label}.compare must be an object")
        _assert_keys(payload, {"left", "op", "right", "lower", "upper"}, f"{label}.compare")
        if "left" not in payload or "op" not in payload:
            raise DatabaseSelectError(f"{label}.compare requires left and op")
        op = " ".join(str(payload["op"]).upper().split())
        if op not in COMPARE_OPS:
            raise DatabaseSelectError(f"{label}.compare.op unsupported: {op}")
        out: dict[str, Any] = {
            "left": _validate_expr(payload["left"], f"{label}.compare.left", budget, depth + 1),
            "op": op,
        }
        if op in {"IS NULL", "IS NOT NULL"}:
            if any(k in payload for k in ("right", "lower", "upper")):
                raise DatabaseSelectError(f"{label}.compare {op} must not provide right/lower/upper")
        elif op in {"BETWEEN", "NOT BETWEEN"}:
            if "lower" not in payload or "upper" not in payload:
                raise DatabaseSelectError(f"{label}.compare {op} requires lower and upper")
            out["lower"] = _validate_expr(payload["lower"], f"{label}.compare.lower", budget, depth + 1)
            out["upper"] = _validate_expr(payload["upper"], f"{label}.compare.upper", budget, depth + 1)
        elif op in {"IN", "NOT IN"}:
            if "right" not in payload:
                raise DatabaseSelectError(f"{label}.compare {op} requires right")
            right = payload["right"]
            if isinstance(right, list):
                if not right or len(right) > MAX_IN_ITEMS:
                    raise DatabaseSelectError(f"{label}.compare.right list must contain 1..{MAX_IN_ITEMS} expressions")
                out["right"] = [_validate_expr(x, f"{label}.compare.right[{i}]", budget, depth + 1) for i, x in enumerate(right)]
            elif isinstance(right, dict) and set(right) == {"subquery"}:
                out["right"] = _validate_expr(right, f"{label}.compare.right", budget, depth + 1)
            else:
                raise DatabaseSelectError(f"{label}.compare.right for {op} must be an expression array or subquery")
        else:
            if "right" not in payload:
                raise DatabaseSelectError(f"{label}.compare {op} requires right")
            out["right"] = _validate_expr(payload["right"], f"{label}.compare.right", budget, depth + 1)
        return {"compare": out}
    raise DatabaseSelectError(f"{label}: unsupported condition kind: {key}")


def _validate_source(value: Any, label: str, budget: _AstBudget, depth: int) -> dict[str, Any]:
    budget.touch(label)
    if not isinstance(value, dict):
        raise DatabaseSelectError(f"{label} must be an object")
    _assert_keys(value, {"table", "subquery", "alias"}, label)
    has_table = "table" in value
    has_sub = "subquery" in value
    if has_table == has_sub:
        raise DatabaseSelectError(f"{label} must contain exactly one of table or subquery")
    out: dict[str, Any] = {}
    if has_table:
        out["table"] = _identifier(value["table"], f"{label}.table", dotted=True)
    else:
        out["subquery"] = _validate_select_spec(value["subquery"], f"{label}.subquery", budget, depth + 1)
    if "alias" in value:
        out["alias"] = _identifier(value["alias"], f"{label}.alias")
    elif has_sub:
        raise DatabaseSelectError(f"{label}.alias is required for subquery sources")
    return out


def _validate_select_spec(value: Any, label: str, budget: _AstBudget, depth: int = 0) -> dict[str, Any]:
    budget.touch(label)
    if depth > MAX_AST_DEPTH:
        raise DatabaseSelectError(f"{label} exceeds maximum AST nesting depth {MAX_AST_DEPTH}")
    if not isinstance(value, dict):
        raise DatabaseSelectError(f"{label} must be an object")
    allowed = {"select", "from", "joins", "where", "group_by", "having", "order_by", "distinct", "limit", "offset"}
    _assert_keys(value, allowed, label)
    if "select" not in value or "from" not in value:
        raise DatabaseSelectError(f"{label} requires select and from")
    select = value["select"]
    if not isinstance(select, list) or not select or len(select) > MAX_SELECT_ITEMS:
        raise DatabaseSelectError(f"{label}.select must be a non-empty array up to {MAX_SELECT_ITEMS} items")
    out_select = []
    for i, item in enumerate(select):
        ilabel = f"{label}.select[{i}]"
        if not isinstance(item, dict):
            raise DatabaseSelectError(f"{ilabel} must be an object")
        _assert_keys(item, {"expr", "alias"}, ilabel)
        if "expr" not in item:
            raise DatabaseSelectError(f"{ilabel}.expr is required")
        row = {"expr": _validate_expr(item["expr"], f"{ilabel}.expr", budget, depth + 1)}
        if "alias" in item:
            row["alias"] = _identifier(item["alias"], f"{ilabel}.alias")
        out_select.append(row)
    out: dict[str, Any] = {
        "select": out_select,
        "from": _validate_source(value["from"], f"{label}.from", budget, depth + 1),
        "joins": [], "group_by": [], "order_by": [],
        "distinct": bool(value.get("distinct", False)),
    }
    if "distinct" in value and not isinstance(value["distinct"], bool):
        raise DatabaseSelectError(f"{label}.distinct must be boolean")
    joins = value.get("joins", [])
    if not isinstance(joins, list) or len(joins) > MAX_JOINS:
        raise DatabaseSelectError(f"{label}.joins must be an array up to {MAX_JOINS} items")
    for i, join in enumerate(joins):
        jlabel = f"{label}.joins[{i}]"
        if not isinstance(join, dict):
            raise DatabaseSelectError(f"{jlabel} must be an object")
        _assert_keys(join, {"type", "source", "on"}, jlabel)
        jtype = str(join.get("type", "INNER")).upper().replace(" OUTER", "")
        if jtype not in JOIN_TYPES:
            raise DatabaseSelectError(f"{jlabel}.type must be INNER/LEFT/RIGHT/CROSS")
        if "source" not in join:
            raise DatabaseSelectError(f"{jlabel}.source is required")
        row = {"type": jtype, "source": _validate_source(join["source"], f"{jlabel}.source", budget, depth + 1)}
        if jtype == "CROSS":
            if "on" in join:
                raise DatabaseSelectError(f"{jlabel}: CROSS JOIN must not provide on")
        else:
            if "on" not in join:
                raise DatabaseSelectError(f"{jlabel}.on is required for {jtype} JOIN")
            row["on"] = _validate_condition(join["on"], f"{jlabel}.on", budget, depth + 1)
        out["joins"].append(row)
    if "where" in value:
        out["where"] = _validate_condition(value["where"], f"{label}.where", budget, depth + 1)
    group_by = value.get("group_by", [])
    if not isinstance(group_by, list) or len(group_by) > MAX_GROUP_ITEMS:
        raise DatabaseSelectError(f"{label}.group_by must be an array up to {MAX_GROUP_ITEMS} items")
    out["group_by"] = [_validate_expr(x, f"{label}.group_by[{i}]", budget, depth + 1) for i, x in enumerate(group_by)]
    if "having" in value:
        if not group_by:
            raise DatabaseSelectError(f"{label}.having requires group_by")
        out["having"] = _validate_condition(value["having"], f"{label}.having", budget, depth + 1)
    order_by = value.get("order_by", [])
    if not isinstance(order_by, list) or len(order_by) > MAX_ORDER_ITEMS:
        raise DatabaseSelectError(f"{label}.order_by must be an array up to {MAX_ORDER_ITEMS} items")
    out["order_by"] = [_validate_order_item(x, f"{label}.order_by[{i}]", budget, depth + 1) for i, x in enumerate(order_by)]
    if "limit" in value:
        lim = value["limit"]
        if not isinstance(lim, int) or isinstance(lim, bool) or not 1 <= lim <= MAX_ROWS_HARD:
            raise DatabaseSelectError(f"{label}.limit must be an integer from 1 to {MAX_ROWS_HARD}")
        out["limit"] = lim
    if "offset" in value:
        off = value["offset"]
        if not isinstance(off, int) or isinstance(off, bool) or not 0 <= off <= 2_147_483_647:
            raise DatabaseSelectError(f"{label}.offset must be an integer from 0 to 2147483647")
        if "limit" not in value:
            raise DatabaseSelectError(f"{label}.offset requires limit")
        out["offset"] = off
    return out


def validate_database_select_action(action: dict[str, Any], label: str = "database_select") -> dict[str, Any]:
    _assert_keys(action, DB_ACTION_ALLOWED_FIELDS, label)
    for field in DB_ACTION_REQUIRED_FIELDS:
        if field not in action:
            raise DatabaseSelectError(f"{label}: missing required field: {field}")
    profile = action.get("profile")
    if not isinstance(profile, str) or not PROFILE_NAME_RE.fullmatch(profile):
        raise DatabaseSelectError(f"{label}.profile must match {PROFILE_NAME_RE.pattern}")
    select_spec = {k: action[k] for k in ("select", "from", "joins", "where", "group_by", "having", "order_by", "distinct", "limit", "offset") if k in action}
    budget = _AstBudget()
    normalized = _validate_select_spec(select_spec, label, budget, 0)
    out: dict[str, Any] = {
        "type": "database_select",
        "profile": profile,
        **normalized,
        "format": str(action.get("format", "csv")).lower(),
        "max_rows": action.get("max_rows", 100_000),
        "max_bytes": action.get("max_bytes", 100 * 1024 * 1024),
        "timeout_sec": action.get("timeout_sec", 120),
        "chunk_rows": action.get("chunk_rows", 10_000),
        "must_return_rows": action.get("must_return_rows", False),
    }
    for optional in ("id", "title"):
        if optional in action:
            if not isinstance(action[optional], str):
                raise DatabaseSelectError(f"{label}.{optional} must be a string")
            out[optional] = action[optional]
    if out["format"] not in {"csv", "jsonl"}:
        raise DatabaseSelectError(f"{label}.format must be csv or jsonl")
    if not isinstance(out["max_rows"], int) or isinstance(out["max_rows"], bool) or not 1 <= out["max_rows"] <= MAX_ROWS_HARD:
        raise DatabaseSelectError(f"{label}.max_rows must be an integer from 1 to {MAX_ROWS_HARD}")
    if not isinstance(out["max_bytes"], int) or isinstance(out["max_bytes"], bool) or not 1024 <= out["max_bytes"] <= MAX_BYTES_HARD:
        raise DatabaseSelectError(f"{label}.max_bytes must be an integer from 1024 to {MAX_BYTES_HARD}")
    if not isinstance(out["timeout_sec"], int) or isinstance(out["timeout_sec"], bool) or not 1 <= out["timeout_sec"] <= MAX_TIMEOUT_HARD:
        raise DatabaseSelectError(f"{label}.timeout_sec must be an integer from 1 to {MAX_TIMEOUT_HARD}")
    if not isinstance(out["chunk_rows"], int) or isinstance(out["chunk_rows"], bool) or not 1 <= out["chunk_rows"] <= 100_000:
        raise DatabaseSelectError(f"{label}.chunk_rows must be an integer from 1 to 100000")
    if not isinstance(out["must_return_rows"], bool):
        raise DatabaseSelectError(f"{label}.must_return_rows must be boolean")
    return out


def _secure_profile_path(root: Path) -> Path:
    raw = os.environ.get(PROFILE_ENV)
    candidates: list[Path] = []
    if raw:
        candidates.append(Path(raw).expanduser())
    else:
        candidates.extend(root / rel for rel in PROFILE_CANDIDATES)
    for candidate in candidates:
        if not candidate.is_absolute():
            candidate = root / candidate
        try:
            st = candidate.lstat()
        except FileNotFoundError:
            continue
        attrs = int(getattr(st, "st_file_attributes", 0) or 0)
        reparse = int(getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or (os.name == "nt" and attrs & reparse) or not stat.S_ISREG(st.st_mode):
            raise DatabaseSelectError(f"database profile path must be a regular non-symlink file: {candidate}")
        return candidate.resolve(strict=True)
    searched = ", ".join(str(p) for p in candidates)
    raise DatabaseSelectError(
        f"database profile file not found; create tools/db_profiles.local.json or set {PROFILE_ENV}. searched: {searched}"
    )


def _safe_arg_text(value: Any, label: str, *, allow_at: bool = False) -> str:
    if not isinstance(value, str) or not value or any(ord(c) < 32 for c in value) or any(c.isspace() for c in value):
        raise DatabaseSelectError(f"{label} must be a non-empty whitespace/control-free string")
    if value.startswith("-"):
        raise DatabaseSelectError(f"{label} must not start with '-'")
    allowed = r"^[A-Za-z0-9_.:@%+\-\[\]]+$" if allow_at else r"^[A-Za-z0-9_.%+\-]+$"
    if not re.fullmatch(allowed, value):
        raise DatabaseSelectError(f"{label} contains unsupported characters")
    return value


def load_database_profile(root: Path, profile_name: str) -> tuple[dict[str, Any], Path]:
    path = _secure_profile_path(root)
    try:
        data = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
    except Exception as exc:
        if isinstance(exc, DatabaseSelectError):
            raise
        raise DatabaseSelectError(f"invalid database profile JSON: {type(exc).__name__}: {exc}") from exc
    if not isinstance(data, dict):
        raise DatabaseSelectError("database profile JSON root must be an object")
    _assert_keys(data, {"version", "profiles"}, "database profiles")
    if data.get("version", 1) != 1:
        raise DatabaseSelectError("database profile version must be 1")
    profiles = data.get("profiles")
    if not isinstance(profiles, dict):
        raise DatabaseSelectError("database profiles.profiles must be an object")
    profile = profiles.get(profile_name)
    if not isinstance(profile, dict):
        raise DatabaseSelectError(f"database profile not found: {profile_name}")
    engine = str(profile.get("engine", "")).lower()
    if engine == "sqlite":
        _assert_keys(profile, {"engine", "path"}, f"profile[{profile_name}]")
        raw_path = profile.get("path")
        if not isinstance(raw_path, str) or not raw_path.strip():
            raise DatabaseSelectError(f"profile[{profile_name}].path must be a non-empty string")
        p = Path(raw_path).expanduser()
        if not p.is_absolute():
            p = root / p
        try:
            st = p.lstat()
        except FileNotFoundError as exc:
            raise DatabaseSelectError(f"SQLite database does not exist: {p}") from exc
        attrs = int(getattr(st, "st_file_attributes", 0) or 0)
        reparse = int(getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
        if stat.S_ISLNK(st.st_mode) or (os.name == "nt" and attrs & reparse) or not stat.S_ISREG(st.st_mode):
            raise DatabaseSelectError(f"SQLite database must be a regular non-symlink file: {p}")
        return {"engine": "sqlite", "path": str(p.resolve(strict=True))}, path
    if engine == "mysql":
        _assert_keys(profile, {"engine", "transport", "database", "auth", "host", "port", "ssh"}, f"profile[{profile_name}]")
        database = _safe_arg_text(profile.get("database"), f"profile[{profile_name}].database")
        auth = profile.get("auth")
        if not isinstance(auth, dict):
            raise DatabaseSelectError(f"profile[{profile_name}].auth must be an object")
        _assert_keys(auth, {"type", "login_path"}, f"profile[{profile_name}].auth")
        if auth.get("type") != "login_path":
            raise DatabaseSelectError(f"profile[{profile_name}].auth.type must be login_path")
        login_path = _safe_arg_text(auth.get("login_path"), f"profile[{profile_name}].auth.login_path")
        transport = str(profile.get("transport", "local")).lower()
        if transport == "local":
            host = str(profile.get("host", "127.0.0.1"))
            if host not in SAFE_LOCAL_HOSTS:
                raise DatabaseSelectError(
                    f"profile[{profile_name}].host must be loopback for transport=local; use ssh_tunnel for remote MySQL"
                )
            port = profile.get("port", 3306)
            if not isinstance(port, int) or isinstance(port, bool) or not 1 <= port <= 65535:
                raise DatabaseSelectError(f"profile[{profile_name}].port must be 1..65535")
            return {
                "engine": "mysql", "transport": "local", "database": database,
                "auth": {"type": "login_path", "login_path": login_path},
                "host": host, "port": port,
            }, path
        if transport == "ssh_tunnel":
            ssh = profile.get("ssh")
            if not isinstance(ssh, dict):
                raise DatabaseSelectError(f"profile[{profile_name}].ssh must be an object")
            _assert_keys(ssh, {"target", "remote_host", "remote_port", "connect_timeout_sec"}, f"profile[{profile_name}].ssh")
            target = _safe_arg_text(ssh.get("target"), f"profile[{profile_name}].ssh.target", allow_at=True)
            remote_host = _safe_arg_text(ssh.get("remote_host", "127.0.0.1"), f"profile[{profile_name}].ssh.remote_host", allow_at=True)
            remote_port = ssh.get("remote_port", 3306)
            if not isinstance(remote_port, int) or isinstance(remote_port, bool) or not 1 <= remote_port <= 65535:
                raise DatabaseSelectError(f"profile[{profile_name}].ssh.remote_port must be 1..65535")
            connect_timeout = ssh.get("connect_timeout_sec", 10)
            if not isinstance(connect_timeout, int) or isinstance(connect_timeout, bool) or not 1 <= connect_timeout <= 60:
                raise DatabaseSelectError(f"profile[{profile_name}].ssh.connect_timeout_sec must be 1..60")
            return {
                "engine": "mysql", "transport": "ssh_tunnel", "database": database,
                "auth": {"type": "login_path", "login_path": login_path},
                "ssh": {
                    "target": target, "remote_host": remote_host, "remote_port": remote_port,
                    "connect_timeout_sec": connect_timeout,
                },
            }, path
        raise DatabaseSelectError(f"profile[{profile_name}].transport must be local or ssh_tunnel")
    raise DatabaseSelectError(f"profile[{profile_name}].engine must be sqlite or mysql")


class SQLCompiler:
    def __init__(self, dialect: str, *, display: bool = False):
        if dialect not in {"sqlite", "mysql"}:
            raise DatabaseSelectError(f"unsupported SQL dialect: {dialect}")
        self.dialect = dialect
        self.display = display
        self.params: list[Any] = []

    def qident(self, value: str, *, wildcard: bool = False) -> str:
        value = _identifier(value, "identifier", dotted=True, wildcard=wildcard)
        parts = value.split(".")
        quote_char = "`" if self.dialect == "mysql" else '"'
        out = []
        for part in parts:
            if part == "*" and wildcard:
                out.append("*")
            else:
                out.append(f"{quote_char}{part}{quote_char}")
        return ".".join(out)

    def value(self, value: Any) -> str:
        if self.display:
            self.params.append(value)
            return "?"
        if self.dialect == "sqlite":
            self.params.append(value)
            return "?"
        if value is None:
            return "NULL"
        if isinstance(value, bool):
            return "TRUE" if value else "FALSE"
        if isinstance(value, int):
            return str(value)
        if isinstance(value, float):
            if not math.isfinite(value):
                raise DatabaseSelectError("non-finite number reached compiler")
            return repr(value)
        if isinstance(value, str):
            # Hex + CONVERT avoids quote/backslash/sql_mode ambiguity. No user text is emitted as SQL syntax.
            return f"CONVERT(0x{value.encode('utf-8').hex()} USING utf8mb4)"
        raise DatabaseSelectError(f"unsupported bound value type: {type(value).__name__}")

    def expr(self, expr: dict[str, Any]) -> str:
        key = next(iter(expr))
        value = expr[key]
        if key == "column":
            return self.qident(value, wildcard=True)
        if key == "value":
            return self.value(value)
        if key == "function":
            name = value["name"]
            allowed = MYSQL_FUNCTIONS if self.dialect == "mysql" else SQLITE_FUNCTIONS
            if name not in allowed:
                raise DatabaseSelectError(f"function {name} is not supported for {self.dialect}")
            args = ", ".join(self.expr(x) for x in value["args"])
            if value.get("distinct"):
                args = "DISTINCT " + args
            sql = f"{name}({args})"
            if "over" in value:
                sql += " OVER " + self.over(value["over"])
            return sql
        if key == "binary":
            return f"({self.expr(value['left'])} {value['op']} {self.expr(value['right'])})"
        if key == "case":
            parts = ["CASE"]
            for row in value["when"]:
                parts.append(f"WHEN {self.condition(row['if'])} THEN {self.expr(row['then'])}")
            if "else" in value:
                parts.append(f"ELSE {self.expr(value['else'])}")
            parts.append("END")
            return " ".join(parts)
        if key == "subquery":
            return f"({self.select(value)})"
        if key == "cast":
            return f"CAST({self.expr(value['expr'])} AS {value['as']})"
        raise DatabaseSelectError(f"unsupported expression at compile time: {key}")

    def frame_boundary(self, value: Any) -> str:
        if isinstance(value, str):
            return value
        if "preceding" in value:
            return f"{value['preceding']} PRECEDING"
        return f"{value['following']} FOLLOWING"

    def over(self, over: dict[str, Any]) -> str:
        parts = []
        if over.get("partition_by"):
            parts.append("PARTITION BY " + ", ".join(self.expr(x) for x in over["partition_by"]))
        if over.get("order_by"):
            parts.append("ORDER BY " + ", ".join(f"{self.expr(x['expr'])} {x['direction']}" for x in over["order_by"]))
        if over.get("frame"):
            f = over["frame"]
            parts.append(f"{f['unit']} BETWEEN {self.frame_boundary(f['start'])} AND {self.frame_boundary(f['end'])}")
        return "(" + " ".join(parts) + ")"

    def condition(self, cond: dict[str, Any]) -> str:
        key = next(iter(cond))
        value = cond[key]
        if key in {"and", "or"}:
            joiner = f" {key.upper()} "
            return "(" + joiner.join(self.condition(x) for x in value) + ")"
        if key == "not":
            return f"(NOT {self.condition(value)})"
        if key == "exists":
            prefix = "NOT EXISTS" if value.get("not") else "EXISTS"
            return f"({prefix} ({self.select(value['query'])}))"
        if key == "compare":
            left = self.expr(value["left"])
            op = value["op"]
            if op in {"IS NULL", "IS NOT NULL"}:
                return f"({left} {op})"
            if op in {"BETWEEN", "NOT BETWEEN"}:
                return f"({left} {op} {self.expr(value['lower'])} AND {self.expr(value['upper'])})"
            if op in {"IN", "NOT IN"}:
                right = value["right"]
                if isinstance(right, list):
                    rhs = ", ".join(self.expr(x) for x in right)
                else:
                    rhs = self.expr(right)[1:-1]  # strip subquery expression parens; add canonical parens below
                return f"({left} {op} ({rhs}))"
            return f"({left} {op} {self.expr(value['right'])})"
        raise DatabaseSelectError(f"unsupported condition at compile time: {key}")

    def source(self, source: dict[str, Any]) -> str:
        if "table" in source:
            sql = self.qident(source["table"])
        else:
            sql = f"({self.select(source['subquery'])})"
        if source.get("alias"):
            sql += " AS " + self.qident(source["alias"])
        return sql

    def select(self, spec: dict[str, Any]) -> str:
        pieces = ["SELECT"]
        if spec.get("distinct"):
            pieces.append("DISTINCT")
        items = []
        for row in spec["select"]:
            part = self.expr(row["expr"])
            if row.get("alias"):
                part += " AS " + self.qident(row["alias"])
            items.append(part)
        pieces.append(", ".join(items))
        pieces.append("FROM " + self.source(spec["from"]))
        for join in spec.get("joins", []):
            part = f"{join['type']} JOIN {self.source(join['source'])}"
            if join["type"] != "CROSS":
                part += " ON " + self.condition(join["on"])
            pieces.append(part)
        if spec.get("where"):
            pieces.append("WHERE " + self.condition(spec["where"]))
        if spec.get("group_by"):
            pieces.append("GROUP BY " + ", ".join(self.expr(x) for x in spec["group_by"]))
        if spec.get("having"):
            pieces.append("HAVING " + self.condition(spec["having"]))
        if spec.get("order_by"):
            pieces.append("ORDER BY " + ", ".join(f"{self.expr(x['expr'])} {x['direction']}" for x in spec["order_by"]))
        if "limit" in spec:
            pieces.append(f"LIMIT {spec['limit']}")
            if "offset" in spec:
                pieces.append(f"OFFSET {spec['offset']}")
        sql = " ".join(pieces)
        self.assert_select_only(sql)
        return sql

    @staticmethod
    def assert_select_only(sql: str) -> None:
        stripped = sql.strip()
        if not stripped.upper().startswith("SELECT "):
            raise DatabaseSelectError("internal safety assertion: generated SQL is not SELECT")
        # Compiler never needs statement separators or SQL comments. Reject if future refactors introduce them.
        if ";" in stripped or "--" in stripped or "/*" in stripped or "*/" in stripped or "\x00" in stripped:
            raise DatabaseSelectError("internal safety assertion: generated SQL contains forbidden statement/comment syntax")


def compile_database_select(action: dict[str, Any], dialect: str, *, display: bool = False) -> tuple[str, list[Any]]:
    spec = {k: action[k] for k in ("select", "from", "joins", "where", "group_by", "having", "order_by", "distinct", "limit", "offset") if k in action}
    compiler = SQLCompiler(dialect, display=display)
    sql = compiler.select(spec)
    return sql, compiler.params


def _disambiguate_columns(columns: list[str]) -> list[str]:
    seen: dict[str, int] = {}
    out = []
    for i, raw in enumerate(columns, 1):
        base = raw or f"column_{i}"
        count = seen.get(base, 0) + 1
        seen[base] = count
        out.append(base if count == 1 else f"{base}_{count}")
    return out


class QueryResultWriter:
    def __init__(self, base: Path, fmt: str, columns: list[str], *, max_bytes: int, chunk_rows: int, max_chunk_bytes: int):
        self.base = base
        self.base.mkdir(parents=True, exist_ok=True)
        self.fmt = fmt
        self.columns = _disambiguate_columns(columns)
        self.original_columns = columns
        self.max_bytes = max_bytes
        self.chunk_rows = chunk_rows
        self.max_chunk_bytes = max(1024, max_chunk_bytes)
        self.total_bytes = 0
        self.rows = 0
        self.chunk_rows_written = 0
        self.chunk_bytes = 0
        self.chunk_index = 0
        self.files: list[Path] = []
        self._fh = None
        self._open_chunk()

    def _csv_line(self, row: list[Any]) -> bytes:
        sio = io.StringIO(newline="")
        writer = csv.writer(sio, lineterminator="\n")
        writer.writerow(["" if x is None else x for x in row])
        return sio.getvalue().encode("utf-8", errors="replace")

    def _header_bytes(self) -> bytes:
        return self._csv_line(self.columns) if self.fmt == "csv" and self.columns else b""

    def _open_chunk(self) -> None:
        self.chunk_index += 1
        suffix = "csv" if self.fmt == "csv" else "jsonl"
        path = self.base / f"result_{self.chunk_index:04d}.{suffix}"
        self._fh = path.open("wb")
        self.files.append(path)
        self.chunk_rows_written = 0
        self.chunk_bytes = 0
        header = self._header_bytes()
        if header:
            if self.total_bytes + len(header) > self.max_bytes or len(header) > self.max_chunk_bytes:
                raise DatabaseSelectError("database result header exceeds configured byte limit")
            self._fh.write(header)
            self.total_bytes += len(header)
            self.chunk_bytes += len(header)

    def _encode_row(self, row: list[Any]) -> bytes:
        if self.fmt == "csv":
            return self._csv_line(row)
        obj = {self.columns[i]: row[i] for i in range(len(self.columns))}
        return (json.dumps(obj, ensure_ascii=False, separators=(",", ":"), default=str) + "\n").encode("utf-8", errors="replace")

    def can_write(self, row: list[Any]) -> tuple[bool, str | None]:
        raw = self._encode_row(row)
        if len(raw) > self.max_chunk_bytes:
            return False, f"one result row exceeds max_file_bytes/chunk limit ({len(raw)} > {self.max_chunk_bytes})"
        next_needs_chunk = self.chunk_rows_written >= self.chunk_rows or self.chunk_bytes + len(raw) > self.max_chunk_bytes
        extra_header = len(self._header_bytes()) if next_needs_chunk and self.fmt == "csv" else 0
        if self.total_bytes + extra_header + len(raw) > self.max_bytes:
            return False, f"database result reached max_bytes={self.max_bytes}"
        return True, None

    def write_row(self, row: list[Any]) -> tuple[bool, str | None]:
        raw = self._encode_row(row)
        ok, reason = self.can_write(row)
        if not ok:
            return False, reason
        if self.chunk_rows_written >= self.chunk_rows or self.chunk_bytes + len(raw) > self.max_chunk_bytes:
            self._fh.close()
            self._open_chunk()
            raw = self._encode_row(row)
        self._fh.write(raw)
        self.rows += 1
        self.chunk_rows_written += 1
        self.total_bytes += len(raw)
        self.chunk_bytes += len(raw)
        return True, None

    def close(self) -> None:
        if self._fh is not None and not self._fh.closed:
            self._fh.flush()
            self._fh.close()
        # Do not keep an empty data chunk when JSONL returned zero rows.
        if self.rows == 0 and self.fmt == "jsonl" and self.files:
            try:
                if self.files[0].stat().st_size == 0:
                    self.files[0].unlink()
                    self.files.clear()
            except OSError:
                pass


def _profile_summary(profile: dict[str, Any]) -> dict[str, Any]:
    if profile["engine"] == "sqlite":
        return {"engine": "sqlite", "transport": "local_file", "path": profile["path"]}
    out = {"engine": "mysql", "transport": profile["transport"], "database": profile["database"]}
    if profile["transport"] == "local":
        out["host"] = profile["host"]
        out["port"] = profile["port"]
    else:
        out["ssh_target"] = profile["ssh"]["target"]
        out["remote_host"] = profile["ssh"]["remote_host"]
        out["remote_port"] = profile["ssh"]["remote_port"]
    out["auth"] = "mysql_login_path"
    out["login_path"] = profile["auth"]["login_path"]
    return out


def _sqlite_authorizer(action_code, _p1, _p2, _db_name, _trigger):
    allowed = {sqlite3.SQLITE_SELECT, sqlite3.SQLITE_READ, sqlite3.SQLITE_FUNCTION}
    recursive = getattr(sqlite3, "SQLITE_RECURSIVE", None)
    if recursive is not None:
        allowed.add(recursive)
    return sqlite3.SQLITE_OK if action_code in allowed else sqlite3.SQLITE_DENY


def _stream_sqlite(profile: dict[str, Any], sql: str, params: list[Any], writer: QueryResultWriter, *, max_rows: int, timeout_sec: int) -> tuple[bool, list[str]]:
    deadline = time.monotonic() + timeout_sec
    uri = "file:" + quote(profile["path"], safe="/:") + "?mode=ro"
    con = sqlite3.connect(uri, uri=True, timeout=min(timeout_sec, 30))
    con.set_authorizer(_sqlite_authorizer)
    con.set_progress_handler(lambda: 1 if time.monotonic() >= deadline else 0, 1000)
    reasons: list[str] = []
    complete = True
    try:
        cur = con.execute(sql, params)
        if cur.description is None:
            raise DatabaseSelectError("internal safety assertion: SELECT produced no result columns")
        columns = [str(col[0]) for col in cur.description]
        if columns != writer.original_columns:
            # Writer is created after execute in the SQLite path; this is a defensive assertion.
            raise DatabaseSelectError("internal SQLite column metadata mismatch")
        while True:
            if time.monotonic() >= deadline:
                complete = False; reasons.append(f"database_select timeout after {timeout_sec}s"); break
            row = cur.fetchone()
            if row is None:
                break
            if writer.rows >= max_rows:
                complete = False; reasons.append(f"database_select reached max_rows={max_rows}; additional rows exist"); break
            values = [x for x in row]
            ok, reason = writer.write_row(values)
            if not ok:
                complete = False; reasons.append(reason or "database_select output byte limit reached"); break
    except sqlite3.OperationalError as exc:
        if "interrupted" in str(exc).lower() and time.monotonic() >= deadline:
            complete = False; reasons.append(f"database_select timeout after {timeout_sec}s")
        else:
            raise DatabaseSelectError(f"SQLite SELECT failed: {exc}") from exc
    finally:
        con.close()
    return complete, reasons


def _mysql_unescape(value: str) -> Any:
    if value == "NULL":
        return None
    out = []
    i = 0
    mapping = {"0": "\x00", "n": "\n", "t": "\t", "r": "\r", "b": "\b", "Z": "\x1a", "\\": "\\"}
    while i < len(value):
        if value[i] == "\\" and i + 1 < len(value) and value[i + 1] in mapping:
            out.append(mapping[value[i + 1]])
            i += 2
        else:
            out.append(value[i])
            i += 1
    return "".join(out)


def _free_local_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def _wait_port(port: int, proc: subprocess.Popen, timeout: int) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            raise DatabaseSelectError(f"SSH tunnel exited before forwarding became ready (rc={proc.returncode})")
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.2):
                return
        except OSError:
            time.sleep(0.1)
    raise DatabaseSelectError(f"SSH tunnel did not become ready within {timeout}s")


def _terminate_process(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        return
    try:
        if os.name == "nt":
            proc.terminate()
        else:
            os.killpg(proc.pid, 15)
    except Exception:
        try: proc.terminate()
        except Exception: pass
    try:
        proc.wait(timeout=2)
        return
    except Exception:
        pass
    try:
        if os.name == "nt": proc.kill()
        else: os.killpg(proc.pid, 9)
    except Exception:
        try: proc.kill()
        except Exception: pass
    try: proc.wait(timeout=2)
    except Exception: pass


class _SshTunnel:
    def __init__(self, profile: dict[str, Any]):
        self.profile = profile
        self.proc: subprocess.Popen | None = None
        self.local_port: int | None = None

    def __enter__(self) -> int:
        ssh_bin = shutil.which("ssh")
        if not ssh_bin:
            raise DatabaseSelectError("ssh executable not found for MySQL ssh_tunnel profile")
        self.local_port = _free_local_port()
        ssh = self.profile["ssh"]
        forward = f"127.0.0.1:{self.local_port}:{ssh['remote_host']}:{ssh['remote_port']}"
        cmd = [
            ssh_bin,
            "-o", "BatchMode=yes",
            "-o", "ExitOnForwardFailure=yes",
            "-o", f"ConnectTimeout={ssh['connect_timeout_sec']}",
            "-o", "ServerAliveInterval=15",
            "-o", "ServerAliveCountMax=2",
            "-N", "-L", forward, ssh["target"],
        ]
        kwargs: dict[str, Any] = {"stdin": subprocess.DEVNULL, "stdout": subprocess.DEVNULL, "stderr": subprocess.PIPE, "text": True}
        if os.name == "nt":
            kwargs["creationflags"] = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
        else:
            kwargs["start_new_session"] = True
        self.proc = subprocess.Popen(cmd, **kwargs)
        try:
            _wait_port(self.local_port, self.proc, ssh["connect_timeout_sec"])
        except Exception:
            stderr = ""
            if self.proc.stderr:
                try: stderr = self.proc.stderr.read(4096)
                except Exception: pass
            _terminate_process(self.proc)
            if stderr:
                raise DatabaseSelectError("SSH tunnel failed: " + stderr.strip()[:1000])
            raise
        return self.local_port

    def __exit__(self, _typ, _value, _tb):
        if self.proc is not None:
            _terminate_process(self.proc)


def _reader_thread(stream, outq: queue.Queue, tag: str) -> None:
    try:
        for line in iter(stream.readline, ""):
            outq.put((tag, line))
    finally:
        outq.put((tag + "_EOF", None))


def _stream_mysql(profile: dict[str, Any], sql: str, writer_factory, *, max_rows: int, timeout_sec: int) -> tuple[QueryResultWriter, bool, list[str], str]:
    mysql_bin = shutil.which("mysql")
    if not mysql_bin:
        raise DatabaseSelectError("mysql client executable not found")
    tunnel = _SshTunnel(profile) if profile["transport"] == "ssh_tunnel" else None
    port = None
    try:
        if tunnel:
            port = tunnel.__enter__()
            host = "127.0.0.1"
        else:
            host = profile["host"]
            port = profile["port"]
        cmd = [
            mysql_bin,
            "--no-defaults",
            f"--login-path={profile['auth']['login_path']}",
            "--protocol=TCP",
            f"--host={host}",
            f"--port={port}",
            f"--database={profile['database']}",
            "--batch",
            "--quick",
            "--default-character-set=utf8mb4",
            "--connect-timeout=10",
            "--skip-auto-rehash",
            "--binary-mode",
        ]
        kwargs: dict[str, Any] = {
            "stdin": subprocess.PIPE, "stdout": subprocess.PIPE, "stderr": subprocess.PIPE,
            "text": True, "encoding": "utf-8", "errors": "replace", "bufsize": 1,
        }
        if os.name == "nt": kwargs["creationflags"] = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
        else: kwargs["start_new_session"] = True
        proc = subprocess.Popen(cmd, **kwargs)
        assert proc.stdin and proc.stdout and proc.stderr
        proc.stdin.write(sql + ";\n")
        proc.stdin.close()
        q: queue.Queue = queue.Queue()
        tout = threading.Thread(target=_reader_thread, args=(proc.stdout, q, "OUT"), daemon=True)
        terr = threading.Thread(target=_reader_thread, args=(proc.stderr, q, "ERR"), daemon=True)
        tout.start(); terr.start()
        deadline = time.monotonic() + timeout_sec
        stderr_parts: list[str] = []
        stdout_eof = stderr_eof = False
        writer: QueryResultWriter | None = None
        complete = True
        reasons: list[str] = []
        while not (stdout_eof and stderr_eof and proc.poll() is not None):
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                complete = False; reasons.append(f"database_select timeout after {timeout_sec}s")
                _terminate_process(proc)
                break
            try:
                tag, line = q.get(timeout=min(0.25, remaining))
            except queue.Empty:
                continue
            if tag == "ERR":
                stderr_parts.append(line)
                continue
            if tag == "ERR_EOF":
                stderr_eof = True; continue
            if tag == "OUT_EOF":
                stdout_eof = True; continue
            if tag != "OUT":
                continue
            text = line.rstrip("\r\n")
            fields = text.split("\t")
            if writer is None:
                columns = [str(_mysql_unescape(x) if _mysql_unescape(x) is not None else "") for x in fields]
                writer = writer_factory(columns)
                continue
            row = [_mysql_unescape(x) for x in fields]
            if len(row) != len(writer.original_columns):
                _terminate_process(proc)
                raise DatabaseSelectError(
                    f"MySQL client returned {len(row)} fields but header has {len(writer.original_columns)}; cannot safely package result"
                )
            if writer.rows >= max_rows:
                complete = False; reasons.append(f"database_select reached max_rows={max_rows}; additional rows exist")
                _terminate_process(proc); break
            ok, reason = writer.write_row(row)
            if not ok:
                complete = False; reasons.append(reason or "database_select output byte limit reached")
                _terminate_process(proc); break
        if proc.poll() is None:
            _terminate_process(proc)
        rc = proc.returncode
        stderr = "".join(stderr_parts).strip()
        if writer is None:
            if complete and rc == 0:
                raise DatabaseSelectError("MySQL SELECT returned no column header")
            if not complete:
                writer = writer_factory([])
            else:
                raise DatabaseSelectError(f"MySQL SELECT failed rc={rc}: {stderr[:2000] or 'no diagnostic'}")
        if complete and rc != 0:
            raise DatabaseSelectError(f"MySQL SELECT failed rc={rc}: {stderr[:2000] or 'no diagnostic'}")
        return writer, complete, reasons, stderr
    finally:
        if tunnel:
            tunnel.__exit__(None, None, None)


def _structure_only(value: Any) -> Any:
    if isinstance(value, dict):
        if set(value) == {"value"}:
            val = value["value"]
            return {"value": {"bound_type": "null" if val is None else type(val).__name__}}
        return {k: _structure_only(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_structure_only(x) for x in value]
    return value


def execute_database_select(root: Path, action: dict[str, Any], global_limits: dict[str, int], work_dir: Path) -> dict[str, Any]:
    profile, profile_path = load_database_profile(root, action["profile"])
    engine = profile["engine"]
    display_sql, display_params = compile_database_select(action, engine, display=True)
    exec_sql, exec_params = compile_database_select(action, engine, display=False)
    SQLCompiler.assert_select_only(display_sql)
    SQLCompiler.assert_select_only(exec_sql)
    max_bytes = min(int(action["max_bytes"]), int(global_limits["max_total_bytes"]))
    max_chunk_bytes = min(int(global_limits["max_file_bytes"]), max_bytes)
    action_dir = work_dir / "database_select"
    result_dir = action_dir / "result"
    action_dir.mkdir(parents=True, exist_ok=True)
    started = time.time()
    writer: QueryResultWriter | None = None
    complete = True
    reasons: list[str] = []
    stderr = ""
    if engine == "sqlite":
        uri = "file:" + quote(profile["path"], safe="/:") + "?mode=ro"
        con = sqlite3.connect(uri, uri=True, timeout=min(action["timeout_sec"], 30))
        con.set_authorizer(_sqlite_authorizer)
        deadline = time.monotonic() + action["timeout_sec"]
        con.set_progress_handler(lambda: 1 if time.monotonic() >= deadline else 0, 1000)
        try:
            cur = con.execute(exec_sql, exec_params)
            if cur.description is None:
                raise DatabaseSelectError("internal safety assertion: SELECT produced no result columns")
            columns = [str(col[0]) for col in cur.description]
            writer = QueryResultWriter(result_dir, action["format"], columns, max_bytes=max_bytes, chunk_rows=action["chunk_rows"], max_chunk_bytes=max_chunk_bytes)
            while True:
                if time.monotonic() >= deadline:
                    complete = False; reasons.append(f"database_select timeout after {action['timeout_sec']}s"); break
                row = cur.fetchone()
                if row is None:
                    break
                if writer.rows >= action["max_rows"]:
                    complete = False; reasons.append(f"database_select reached max_rows={action['max_rows']}; additional rows exist"); break
                ok, reason = writer.write_row(list(row))
                if not ok:
                    complete = False; reasons.append(reason or "database_select output byte limit reached"); break
        except sqlite3.OperationalError as exc:
            if "interrupted" in str(exc).lower() and time.monotonic() >= deadline:
                complete = False; reasons.append(f"database_select timeout after {action['timeout_sec']}s")
            else:
                raise DatabaseSelectError(f"SQLite SELECT failed: {exc}") from exc
        finally:
            con.close()
    else:
        def factory(columns: list[str]) -> QueryResultWriter:
            return QueryResultWriter(result_dir, action["format"], columns, max_bytes=max_bytes, chunk_rows=action["chunk_rows"], max_chunk_bytes=max_chunk_bytes)
        writer, complete, reasons, stderr = _stream_mysql(
            profile, exec_sql, factory, max_rows=action["max_rows"], timeout_sec=action["timeout_sec"]
        )
    assert writer is not None
    writer.close()
    if action.get("must_return_rows") and writer.rows == 0:
        complete = False
        reasons.append("must_return_rows=true but SELECT returned zero rows")
    elapsed = time.time() - started
    meta = {
        "format": "python-patch-tool-database-select",
        "format_version": 1,
        "tool_version": VERSION,
        "status": "COMPLETED" if complete else "PARTIAL",
        "profile": action["profile"],
        "profile_source": "local-only",
        "engine": engine,
        "transport": _profile_summary(profile).get("transport"),
        "rows_collected": writer.rows,
        "bytes_collected": writer.total_bytes,
        "chunks": len(writer.files),
        "columns": writer.columns,
        "original_columns": writer.original_columns,
        "limits": {
            "max_rows": action["max_rows"], "max_bytes": max_bytes,
            "timeout_sec": action["timeout_sec"], "chunk_rows": action["chunk_rows"],
            "max_chunk_bytes": max_chunk_bytes,
        },
        "reasons": reasons,
        "elapsed_seconds": round(elapsed, 3),
        "safety": {
            "raw_sql_accepted": False,
            "generated_statement": "SELECT",
            "profile_credentials_embedded": False,
            "sqlite_open_mode": "read-only" if engine == "sqlite" else None,
            "mysql_auth": "login_path" if engine == "mysql" else None,
        },
    }
    (action_dir / "META.json").write_text(json.dumps(meta, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (action_dir / "BUILT_QUERY.sql").write_text(display_sql + "\n", encoding="utf-8")
    (action_dir / "BOUND_VALUES_META.json").write_text(
        json.dumps({"count": len(display_params), "types": ["null" if x is None else type(x).__name__ for x in display_params]}, indent=2) + "\n",
        encoding="utf-8",
    )
    (action_dir / "ACTIVE_QUERY_STRUCTURE.json").write_text(
        json.dumps(_structure_only({k: action[k] for k in ("select", "from", "joins", "where", "group_by", "having", "order_by", "distinct", "limit", "offset") if k in action}), ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    if stderr:
        (action_dir / "CLIENT_STDERR.log").write_text(stderr[:1024 * 1024] + ("\n[TRUNCATED]\n" if len(stderr) > 1024 * 1024 else ""), encoding="utf-8")
    profile_info = _profile_summary(profile)
    lines = [
        "# Database SELECT", "",
        f"- Profile: `{action['profile']}`",
        f"- Engine: `{engine}`",
        f"- Transport: `{profile_info.get('transport')}`",
        "- SQL source: **Patch Tool active builder only (raw SQL is not accepted)**",
        "- Executed statement class: **SELECT only**",
        f"- Output format: `{action['format']}`",
        f"- Rows collected: **{writer.rows}**",
        f"- Bytes collected: **{writer.total_bytes}**",
        f"- Result chunks: **{len(writer.files)}**",
        f"- Execution status: **{'COMPLETED' if complete else 'PARTIAL'}**",
        f"- Coverage status: **{'VERIFIED' if complete else 'PARTIAL'}**",
        f"- Elapsed: `{elapsed:.3f}s`",
        "",
        "## Built SELECT template", "", "```sql", display_sql, "```", "",
    ]
    if reasons:
        lines += ["## Partial/incomplete reasons", ""] + [f"- {x}" for x in reasons] + [""]
    lines += [
        "## Safety", "",
        "- No `query`/`raw_sql` field exists in the action contract.",
        "- Identifiers/operators/functions are validated against active-builder allowlists.",
        "- Values are bound by SQLite or encoded as non-syntax MySQL literals by the tool.",
        "- SQLite is opened with `mode=ro` and an authorizer denies non-read operations.",
        "- MySQL profiles require `mysql_config_editor` login-path auth; remote access uses an SSH tunnel.",
        "- Use a DB account with SELECT-only grants as the independent server-side safety boundary.",
    ]
    artifacts = [p for p in action_dir.rglob("*") if p.is_file()]
    return {
        "report": "\n".join(lines) + "\n",
        "artifacts": artifacts,
        "artifact_root": action_dir,
        "incomplete": not complete,
        "reasons": reasons,
        "meta": meta,
    }
