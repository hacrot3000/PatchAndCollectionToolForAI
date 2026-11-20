#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path


def read_tool_version(base: Path | None = None) -> str:
    """Read the authoritative tool version without importing other runtime modules."""
    here = (base or Path(__file__).resolve().parent).resolve()
    version_file = here / "VERSION"
    try:
        value = version_file.read_text(encoding="utf-8").strip()
        if value:
            return value
    except OSError:
        pass

    # Historical/minimal COLLECT fixtures may copy a runtime module set without
    # VERSION. Their authoritative schema still carries the release identity,
    # so standalone compatibility can degrade gracefully instead of crashing.
    schema_file = here / "docs" / "COLLECT_ACTION_SCHEMA.json"
    try:
        data = json.loads(schema_file.read_text(encoding="utf-8"))
        value = str(data.get("tool_version") or "").strip()
        if value:
            return value
    except (OSError, ValueError, TypeError):
        pass
    return "unknown"


VERSION = read_tool_version()
