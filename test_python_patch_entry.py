#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import tempfile
import threading
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

    def test_prompt_response_is_bound_to_active_prompt_id(self):
        from python_patch_protocol import CommandReader, EventWriter, request_prompt

        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        writer = EventWriter(event_write)
        reader = CommandReader(command_read)
        seen = {}

        def respond():
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
            seen.update(prompt)
            response = {
                "protocol": "taskdeck.patch",
                "version": 1,
                "type": "command",
                "seq": 1,
                "command": "prompt_response",
                "payload": {
                    "prompt_id": prompt["prompt_id"],
                    "action": "select",
                    "indexes": [1],
                },
            }
            os.write(command_write, (json.dumps(response) + "\n").encode("utf-8"))

        worker = threading.Thread(target=respond)
        worker.start()
        try:
            response = request_prompt(writer, reader, "queue_selection", items=[])
        finally:
            worker.join(timeout=2)
            reader.close()
            writer.close()
            for fd in (event_read, event_write, command_read, command_write):
                try:
                    os.close(fd)
                except OSError:
                    pass
        self.assertEqual(seen["type"], "prompt")
        self.assertEqual(seen["prompt_kind"], "queue_selection")
        self.assertEqual(response["action"], "select")
        self.assertEqual(response["indexes"], [1])

    def test_dispatcher_protocol_queue_selection_uses_existing_contract(self):
        from python_patch_queue_dispatcher import QueueItem, _protocol_queue_selection

        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)

        def respond():
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
            response = {
                "protocol": "taskdeck.patch",
                "version": 1,
                "type": "command",
                "seq": 1,
                "command": "prompt_response",
                "payload": {
                    "prompt_id": prompt["prompt_id"],
                    "action": "select",
                    "indexes": [1],
                },
            }
            os.write(command_write, (json.dumps(response) + "\n").encode("utf-8"))

        worker = threading.Thread(target=respond)
        worker.start()
        items = [
            QueueItem("one.zip", "PATCH", "manifest"),
            QueueItem("two.zip", "COLLECT", "request"),
        ]
        try:
            handled, chosen = _protocol_queue_selection(items, "none", set())
        finally:
            worker.join(timeout=2)
            if old_event is None:
                os.environ.pop(entry.EVENT_FD_ENV, None)
            else:
                os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None:
                os.environ.pop(entry.COMMAND_FD_ENV, None)
            else:
                os.environ[entry.COMMAND_FD_ENV] = old_command
            for fd in (event_read, event_write, command_read, command_write):
                try:
                    os.close(fd)
                except OSError:
                    pass
        self.assertTrue(handled)
        self.assertEqual([item.name for item in chosen], ["one.zip"])

    def test_dispatcher_protocol_selector_absent_channels_falls_back(self):
        from python_patch_queue_dispatcher import QueueItem, _protocol_queue_selection

        old_event = os.environ.pop(entry.EVENT_FD_ENV, None)
        old_command = os.environ.pop(entry.COMMAND_FD_ENV, None)
        try:
            handled, chosen = _protocol_queue_selection([QueueItem("one.zip", "PATCH")], "none", set())
        finally:
            if old_event is not None:
                os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is not None:
                os.environ[entry.COMMAND_FD_ENV] = old_command
        self.assertFalse(handled)
        self.assertIsNone(chosen)

    def test_runtime_event_helper_is_opt_in_and_jsonl(self):
        from python_patch_protocol import EVENT_FD_ENV, emit_runtime_event

        old = os.environ.get(EVENT_FD_ENV)
        read_fd, write_fd = os.pipe()
        try:
            os.environ[EVENT_FD_ENV] = str(write_fd)
            self.assertTrue(emit_runtime_event("item_started", name="demo.zip", kind="PATCH"))
            os.close(write_fd)
            write_fd = -1
            raw = os.read(read_fd, 65536).decode("utf-8")
        finally:
            if write_fd >= 0:
                os.close(write_fd)
            os.close(read_fd)
            if old is None:
                os.environ.pop(EVENT_FD_ENV, None)
            else:
                os.environ[EVENT_FD_ENV] = old
        event = json.loads(raw.strip())
        self.assertEqual(event["type"], "item_started")
        self.assertEqual(event["name"], "demo.zip")
        self.assertEqual(event["kind"], "PATCH")

    def test_dispatcher_item_events_wrap_only_payload_execution(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        started = dispatcher.index('_emit_protocol_event(\n            "item_started"')
        running = dispatcher.index('live_status.set_status(item.name, "RUNNING")')
        child = dispatcher.index('rc, console_log, patch_result = _run_patch_child(', running)
        finished = dispatcher.index('_emit_protocol_event(\n            "item_finished"')
        details = dispatcher.index('_LAST_EXECUTION_DETAILS.append(detail)', child)
        self.assertGreater(started, running)
        self.assertLess(started, child)
        self.assertGreater(finished, details)
        self.assertNotIn('"item_started"', dispatcher[dispatcher.index('if item.kind == "PATCH" and preflight_detail is not None:'):running])

    def test_dispatcher_artifact_rows_only_publish_real_project_artifacts(self):
        from python_patch_queue_dispatcher import _detail_artifact_rows

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifacts = root / "artifacts"
            artifacts.mkdir()
            fail_zip = artifacts / "FAIL_HANDOFF_demo.zip"
            fail_zip.write_bytes(b"zip")
            fail_txt = artifacts / "FAIL_HANDOFF_demo.txt"
            fail_txt.write_text("text", encoding="utf-8")
            collect_zip = artifacts / "collect.zip"
            collect_zip.write_bytes(b"collect")
            outside = root / "outside.zip"
            outside.write_bytes(b"outside")

            rows = _detail_artifact_rows(root, {
                "fail_handoff": str(fail_zip),
                "fail_handoff_text": "artifacts/FAIL_HANDOFF_demo.txt",
                "ai_sync_result": str(outside),
                "log_path": "artifacts/internal.log",
                "collect_result": {
                    "result_zip": str(collect_zip),
                    "result_text": str(root / "missing.txt"),
                    "request_archive": "patchs/patched/request.zip",
                },
            })

        self.assertEqual(rows, [
            {"artifact_kind": "fail_handoff_zip", "path": "artifacts/FAIL_HANDOFF_demo.zip", "primary": True},
            {"artifact_kind": "fail_handoff_text", "path": "artifacts/FAIL_HANDOFF_demo.txt", "primary": False},
            {"artifact_kind": "collect_result_zip", "path": "artifacts/collect.zip", "primary": True},
        ])

    def test_dispatcher_artifact_events_precede_item_finished(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        details = dispatcher.index("_LAST_EXECUTION_DETAILS.append(detail)")
        artifacts = dispatcher.index("_emit_detail_artifacts(root, item, detail, index=index + 1, total=len(chosen))", details)
        finished = dispatcher.index('"item_finished"', artifacts)
        self.assertLess(details, artifacts)
        self.assertLess(artifacts, finished)
        self.assertIn("_emit_detail_artifacts(root, item, detail)", dispatcher)

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
                rc = entry._run_with_protocol(writer, [str(child)], tmp, ["health-search"])
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

    def test_child_events_are_relayed_with_entrypoint_sequence(self):
        with tempfile.TemporaryDirectory() as tmp:
            child = Path(tmp) / "child.py"
            child.write_text(
                "import json, os\n"
                "fd=int(os.environ['TASKDECK_PATCH_EVENT_FD'])\n"
                "event={'protocol':'taskdeck.patch','version':1,'type':'child_probe','seq':1,'value':'ok'}\n"
                "os.write(fd,(json.dumps(event)+'\\n').encode())\n",
                encoding="utf-8",
            )
            read_fd, write_fd = os.pipe()
            try:
                from python_patch_protocol import EventWriter
                writer = EventWriter(write_fd)
                rc = entry._run_with_protocol(writer, [str(child)], tmp, ["health-search"])
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
        self.assertEqual([event["type"] for event in events], ["hello", "run_started", "child_probe", "run_finished"])
        self.assertEqual([event["seq"] for event in events], [1, 2, 3, 4])
        self.assertEqual(events[2]["value"], "ok")

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


if __name__ == "__main__":
    unittest.main()
