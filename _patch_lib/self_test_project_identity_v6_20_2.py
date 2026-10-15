#!/usr/bin/env python3
"""Offline regression for persistent project identity and anti-cross-project guards."""
from __future__ import annotations

import json
import tempfile
from pathlib import Path

from python_patch_project_state import (
    ProjectStateError, ensure_project_identity, enforce_project_identity, identity_ai_instructions,
)
from python_patch_collect_schema import CollectSchemaError, validate_request_data


with tempfile.TemporaryDirectory(prefix="taskdeck-identity-") as tmp:
    parent = Path(tmp)
    a = parent / "one" / "admin-go-tool"
    b = parent / "two" / "admin-go-tool"
    a.mkdir(parents=True)
    b.mkdir(parents=True)
    cfg = a / ".python_patch_tool.json"
    cfg.write_text(json.dumps({
        "validation_profiles": {"test": {"argv": ["python3", "-V"]}},
        "project": {"key": "admin-go-tool"},
    }), encoding="utf-8")

    first = ensure_project_identity(a)
    other = ensure_project_identity(b)
    assert first["key"] == other["key"] == "admin-go-tool"
    assert first["id"] != other["id"], "same-named workspaces must have distinct UUIDs"
    assert ensure_project_identity(a) == first, "UUID cannot rotate on restart"
    assert json.loads(cfg.read_text(encoding="utf-8"))["validation_profiles"]["test"]["argv"] == ["python3", "-V"]
    assert enforce_project_identity(a, {"project": first})["project_id"] == first["id"]

    try:
        enforce_project_identity(a, {"project": other})
    except ProjectStateError as e:
        assert e.kind == "project_mismatch"
    else:
        raise AssertionError("foreign project UUID accepted by PATCH")

    try:
        enforce_project_identity(a, {"project": {"key": first["key"]}})
    except ProjectStateError as e:
        assert e.kind == "project_identity_missing"
    else:
        raise AssertionError("key-only PATCH must not bypass UUID identity")

    for req, valid in [
        ({"actions": [{"type": "overview"}], "project": first}, True),
        ({"actions": [{"type": "overview"}], "project": {"key": "admin-go-tool"}}, False),
        ({"actions": [{"type": "overview"}], "project": {"key": "", "id": first["id"]}}, False),
    ]:
        try:
            normalized = validate_request_data(req)
        except CollectSchemaError:
            assert not valid
        else:
            assert valid and normalized["project"] == first
    prompt = identity_ai_instructions(first)
    assert first["id"] in prompt and "refuse" in prompt and "user" in prompt
    bad = {"project": {"key": "admin-go-tool", "id": "invalid-uuid"}}
    cfg.write_text(json.dumps(bad), encoding="utf-8")
    try:
        ensure_project_identity(a)
    except ProjectStateError:
        pass
    else:
        raise AssertionError("invalid identity must never be silently replaced")
    assert json.loads(cfg.read_text(encoding="utf-8")) == bad

print("PASS project identity: migration, stable UUID, cross-project, prompt, schema, corrupt config")
