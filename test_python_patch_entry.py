#!/usr/bin/env python3
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import python_patch_entry as entry


class RouteContractTests(unittest.TestCase):
    def setUp(self):
        self.base = Path("/opt/taskdeck/patchtool")
        self.root = "/workspace"

    def route(self, *args):
        argv, required = entry.build_exec_argv(self.root, list(args), self.base)
        return argv, [str(path) for path in required]

    def test_zero_argument_dispatches_queue(self):
        argv, required = self.route()
        self.assertEqual(argv, [
            "/opt/taskdeck/patchtool/_patch_lib/python_patch_queue_dispatcher.py",
            "--project-root", self.root,
        ])
        self.assertEqual(required, ["/opt/taskdeck/patchtool/_patch_lib/python_patch_queue_dispatcher.py"])

    def test_dispatch_commands_preserve_arguments(self):
        argv, _ = self.route("report", "--list")
        self.assertEqual(argv[-2:], ["report", "--list"])

    def test_collect_uses_progress_supervisor(self):
        argv, required = self.route("collect", "search", "needle")
        self.assertIn("python_patch_collect_progress_v6_7.py", argv[0])
        self.assertEqual(argv[1:5], ["--project-root", self.root, "--collector", "/opt/taskdeck/patchtool/_patch_lib/python_patch_collect_compat.py"])
        self.assertEqual(argv[5:], ["--", "search", "needle"])
        self.assertTrue(any(path.endswith("python_patch_collect_regex_worker.py") for path in required))

    def test_automation_routes_to_dispatcher_and_strips_legacy_transaction(self):
        argv, _ = self.route("--all", "--transaction", "required", "-y")
        self.assertEqual(argv, [
            "/opt/taskdeck/patchtool/_patch_lib/python_patch_queue_dispatcher.py",
            "--project-root", self.root,
            "run", "--all", "-y",
        ])

    def test_direct_patch_forces_inplace(self):
        argv, _ = self.route("--patch", "patchs/example.zip", "--transaction=required")
        self.assertEqual(argv[-4:], ["--patch", "patchs/example.zip", "--transaction", "off"])

    def test_utility_does_not_gain_execution_flags(self):
        argv, _ = self.route("health-search")
        self.assertEqual(argv, ["/opt/taskdeck/patchtool/_patch_lib/python_patch_runner.py", "health-search"])

    def test_transaction_only_fails_closed(self):
        with self.assertRaises(entry.EntryError):
            self.route("--transaction", "required")

    def test_malformed_transaction_does_not_swallow_following_automation_flag(self):
        argv, _ = self.route("--transaction", "--all")
        self.assertEqual(argv[-2:], ["run", "--all"])


if __name__ == "__main__":
    unittest.main()
