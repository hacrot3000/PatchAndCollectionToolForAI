#!/usr/bin/env python3
from __future__ import annotations

import json
import os
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


class ProtocolContractTests(unittest.TestCase):
    def setUp(self):
        self.base = Path(__file__).resolve().parent
        entry._prepare_environment(self.base)

    def test_protocol_is_opt_in_and_never_uses_stdio_fds(self):
        old = os.environ.pop(entry.EVENT_FD_ENV, None)
        try:
            self.assertIsNone(entry._protocol_writer_from_env())
            os.environ[entry.EVENT_FD_ENV] = "1"
            with self.assertRaises(entry.EntryError):
                entry._protocol_writer_from_env()
        finally:
            if old is not None:
                os.environ[entry.EVENT_FD_ENV] = old
            else:
                os.environ.pop(entry.EVENT_FD_ENV, None)

    def test_command_fd_requires_separate_event_channel(self):
        read_fd, write_fd = os.pipe()
        old_event = os.environ.pop(entry.EVENT_FD_ENV, None)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        try:
            os.environ[entry.COMMAND_FD_ENV] = str(read_fd)
            with self.assertRaises(entry.EntryError):
                entry._command_fd_from_env(None)
            with self.assertRaises(entry.EntryError):
                entry._command_fd_from_env(read_fd)
            self.assertEqual(entry._command_fd_from_env(write_fd), read_fd)
        finally:
            if old_event is not None:
                os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is not None:
                os.environ[entry.COMMAND_FD_ENV] = old_command
            else:
                os.environ.pop(entry.COMMAND_FD_ENV, None)
            os.close(read_fd)
            os.close(write_fd)

    def test_command_reader_accepts_versioned_bounded_jsonl(self):
        from python_patch_protocol import CommandReader

        read_fd, write_fd = os.pipe()
        reader = CommandReader(read_fd)
        try:
            os.write(
                write_fd,
                b'{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"future_prompt_response","payload":{"value":"yes"}}\n',
            )
            command = reader.read()
            self.assertEqual(command["command"], "future_prompt_response")
            self.assertEqual(command["payload"]["value"], "yes")
        finally:
            reader.close()
            os.close(read_fd)
            os.close(write_fd)

    def test_event_writer_emits_versioned_jsonl(self):
        from python_patch_protocol import EventWriter

        read_fd, write_fd = os.pipe()
        try:
            writer = EventWriter(write_fd)
            self.assertTrue(writer.emit("hello", route="queue"))
            writer.close()
            os.close(write_fd)
            write_fd = -1
            raw = os.read(read_fd, 65536).decode("utf-8")
        finally:
            if write_fd >= 0:
                os.close(write_fd)
            os.close(read_fd)
        event = json.loads(raw.strip())
        self.assertEqual(event["protocol"], "taskdeck.patch")
        self.assertEqual(event["version"], 1)
        self.assertEqual(event["type"], "hello")
        self.assertEqual(event["seq"], 1)
        self.assertEqual(event["route"], "queue")

    def test_protocol_supervisor_emits_run_lifecycle_without_touching_stdio(self):
        from python_patch_protocol import EventWriter

        with tempfile.TemporaryDirectory() as tmp:
            child = Path(tmp) / "child.py"
            child.write_text("raise SystemExit(7)\n", encoding="utf-8")
            read_fd, write_fd = os.pipe()
            try:
                writer = EventWriter(write_fd)
                rc = entry._run_with_protocol(writer, [str(child)], "/workspace", ["plan"])
                os.close(write_fd)
                write_fd = -1
                chunks = []
                while True:
                    data = os.read(read_fd, 65536)
                    if not data:
                        break
                    chunks.append(data)
            finally:
                if write_fd >= 0:
                    os.close(write_fd)
                os.close(read_fd)

        events = [json.loads(line) for line in b"".join(chunks).decode("utf-8").splitlines()]
        self.assertEqual(rc, 7)
        self.assertEqual([event["type"] for event in events], ["hello", "run_started", "run_finished"])
        self.assertEqual(events[-1]["exit_code"], 7)
        self.assertEqual(events[-1]["status"], "failed")

    def test_route_classification_is_stable(self):
        cases = [
            ([], "queue"),
            (["resume"], "resume"),
            (["collect", "search"], "collect"),
            (["--all"], "run"),
            (["health-search"], "utility"),
            (["example.zip"], "direct"),
        ]
        for args, want in cases:
            self.assertEqual(entry.classify_route(args), want)


    def test_queue_snapshot_uses_dispatcher_discovery_contract(self):
        from python_patch_protocol import build_queue_snapshot

        with tempfile.TemporaryDirectory() as tmp:
            snapshot = build_queue_snapshot(tmp)
        self.assertEqual(snapshot["status"], "empty")
        self.assertEqual(snapshot["items"], [])
        self.assertEqual(snapshot["warnings"], [])
        self.assertEqual(snapshot["counts"], {})
        self.assertEqual(snapshot["total"], 0)

    def test_protocol_supervisor_emits_snapshot_before_run_started(self):
        with tempfile.TemporaryDirectory() as tmp:
            child = Path(tmp) / "child.py"
            child.write_text("raise SystemExit(0)\n", encoding="utf-8")
            read_fd, write_fd = os.pipe()
            try:
                from python_patch_protocol import EventWriter
                writer = EventWriter(write_fd)
                rc = entry._run_with_protocol(writer, [str(child)], tmp, [])
                os.close(write_fd)
                write_fd = -1
                chunks = []
                while True:
                    data = os.read(read_fd, 65536)
                    if not data:
                        break
                    chunks.append(data)
            finally:
                if write_fd >= 0:
                    os.close(write_fd)
                os.close(read_fd)

        events = [json.loads(line) for line in b"".join(chunks).decode("utf-8").splitlines()]
        self.assertEqual(rc, 0)
        self.assertEqual([event["type"] for event in events], ["hello", "queue_snapshot", "run_started", "run_finished"])
        self.assertEqual(events[1]["status"], "empty")
        self.assertEqual(events[1]["total"], 0)
