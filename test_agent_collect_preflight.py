#!/usr/bin/env python3
"""Native Python validator regression for Agent-created COLLECT requests.

The schema validator is authoritative; no synthetic Go action parser and no
PATCH/COLLECT execution or ZIP publication is performed by these tests.
"""
from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import python_patch_entry as entry


ROOT = Path(__file__).resolve().parent
IDENTITY = {"key": "game_automation_client", "id": "6159c0dc-b25e-4cf8-bfa3-1539965d3b71"}


class AgentNativeCollectSchemaPreflight(unittest.TestCase):
    def test_router_reuses_canonical_native_schema(self):
        argv, required = entry.build_exec_argv("/workspace", ["validate-collect"], ROOT)
        self.assertEqual(argv, [
            str(ROOT / "_patch_lib" / "python_patch_collect_schema.py"),
            "--project-root", "/workspace", "--validate-stdin",
        ])
        self.assertEqual(required, [ROOT / "_patch_lib" / "python_patch_collect_schema.py"])
        self.assertEqual(entry.classify_route(["validate-collect"]), "utility")

    def test_invalid_and_valid_requests_do_not_create_queue_entries(self):
        scenarios = [
            ("read unsupported", [{"type": "read", "path": "README.md"}],
             "unsupported action type: read", 2),
            ("git operations empty", [{"type": "git", "operations": []}],
             "git.operations must be a non-empty array", 2),
            ("pack paths empty", [{"type": "pack", "paths": []}],
             "paths must be a non-empty array", 2),
            ("valid overview", [{"type": "overview", "path": "."}],
             "COLLECT_SCHEMA_VALID", 0),
        ]
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            cfg = root / ".python_patch_tool.json"
            cfg.write_text(json.dumps({"project": IDENTITY}), encoding="utf-8")
            for label, actions, expected, rc in scenarios:
                with self.subTest(label=label):
                    source = json.dumps({"project": IDENTITY, "actions": actions})
                    proc = subprocess.run([
                        sys.executable, str(ROOT / "python_patch_entry.py"),
                        "--project-root", str(root), "--", "validate-collect",
                    ], input=source, text=True, capture_output=True, timeout=15)
                    self.assertEqual(proc.returncode, rc, proc.stderr)
                    self.assertIn(expected, proc.stdout + proc.stderr)
                    self.assertEqual(list(root.glob("patchs/*.zip")), [])
                    self.assertEqual(json.loads(cfg.read_text(encoding="utf-8")),
                                     {"project": IDENTITY})

    def test_foreign_identity_rejected_without_repair(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            (root / ".python_patch_tool.json").write_text(
                json.dumps({"project": IDENTITY}), encoding="utf-8")
            request = {"project": {"key": "other", "id": IDENTITY["id"]},
                       "actions": [{"type": "overview"}]}
            proc = subprocess.run([
                sys.executable, str(ROOT / "python_patch_entry.py"),
                "--project-root", str(root), "--", "validate-collect",
            ], input=json.dumps(request), text=True, capture_output=True, timeout=15)
            self.assertEqual(proc.returncode, 2)
            self.assertIn("project.key/project.id do not match", proc.stderr)
            self.assertFalse((root / "patchs").exists())


if __name__ == "__main__":
    unittest.main()
