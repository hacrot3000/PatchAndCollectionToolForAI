#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

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

    def test_dispatcher_protocol_item_action_emits_result_and_keeps_prompt_active(self):
        from python_patch_queue_dispatcher import QueueItem, _protocol_queue_selection
        import python_patch_queue_dispatcher as dispatcher

        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        seen = {}

        def respond():
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
                seen["prompt"] = prompt
                action = {
                    "protocol": "taskdeck.patch",
                    "version": 1,
                    "type": "command",
                    "seq": 1,
                    "command": "item_action",
                    "payload": {
                        "prompt_id": prompt["prompt_id"],
                        "action_id": "action-1",
                        "action": "preview",
                        "index": 1,
                    },
                }
                os.write(command_write, (json.dumps(action) + "\n").encode("utf-8"))
                result = json.loads(stream.readline())
                seen["result"] = result
                response = {
                    "protocol": "taskdeck.patch",
                    "version": 1,
                    "type": "command",
                    "seq": 2,
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
        items = [QueueItem("one.zip", "PATCH", "manifest")]
        try:
            with mock.patch.object(dispatcher, "_run_runner_captured", return_value=(0, "preview output\n", False)):
                handled, chosen = _protocol_queue_selection(items, "none", set(), root=Path("/workspace"))
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
        self.assertEqual(seen["prompt"]["item_actions"], ["inspect", "preview", "validate"])
        result = seen["result"]
        self.assertEqual(result["type"], "action_result")
        self.assertEqual(result["action_id"], "action-1")
        self.assertEqual(result["action"], "preview")
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["rc"], 0)
        self.assertEqual(result["output"], "preview output\n")

    def test_protocol_item_action_rejects_collect_without_running_runner(self):
        import python_patch_queue_dispatcher as dispatcher

        class Writer:
            def __init__(self): self.events = []
            def emit(self, event_type, **payload): self.events.append((event_type, payload)); return True

        writer = Writer()
        command = {
            "payload": {
                "prompt_id": "prompt-1",
                "action_id": "action-collect",
                "action": "inspect",
                "index": 1,
            }
        }
        with mock.patch.object(dispatcher, "_run_runner_captured") as runner:
            dispatcher._protocol_item_action(
                Path("/workspace"),
                writer,
                [dispatcher.QueueItem("request.zip", "COLLECT", "request")],
                "prompt-1",
                command,
            )
        runner.assert_not_called()
        self.assertEqual(writer.events[0][0], "action_result")
        self.assertEqual(writer.events[0][1]["status"], "UNSUPPORTED")
        self.assertEqual(writer.events[0][1]["rc"], 2)

    def test_protocol_action_output_is_bounded_and_preserves_tail(self):
        import python_patch_queue_dispatcher as dispatcher
        text = "A" * (dispatcher._PROTOCOL_ACTION_OUTPUT_BYTES + 1000) + "TAIL"
        bounded, truncated = dispatcher._protocol_action_output(text)
        self.assertTrue(truncated)
        self.assertLessEqual(len(bounded.encode("utf-8")), dispatcher._PROTOCOL_ACTION_OUTPUT_BYTES + 8)
        self.assertTrue(bounded.endswith("TAIL"))

    def test_normal_queue_has_no_second_terminal_confirmation_after_selection(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        start = dispatcher.index('    if chosen is None:\n        print("Cancelled.")')
        end = dispatcher.index('        rc, executed, remaining, late_duplicates, late_duplicate_warnings = execute_items(', start)
        execution_path = dispatcher[start:end]
        for forbidden in (
            "input(",
            "sys.stdin",
            "_read_key(",
            "_read_key_windows(",
            "_interactive_choice_menu(",
            "_select_failed_rows(",
        ):
            self.assertNotIn(forbidden, execution_path)
        self.assertIn("resource_preflight_report = disk_preflight(", execution_path)
        self.assertIn("_batch_preflight(", execution_path)

    def test_noninteractive_auto_selection_remains_explicitly_confirmed_and_patch_only(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        start = dispatcher.index("def _configured_auto_selection(")
        end = dispatcher.index("\ndef _normalize_subprocess_rc(", start)
        block = dispatcher[start:end]
        self.assertIn('cfg.get("non_interactive_confirmed", False)', block)
        self.assertIn('any(item.kind != "PATCH" for item in items)', block)
        self.assertIn('return None', block)

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

    @unittest.skipUnless(os.name == "posix", "protocol event FD inheritance requires POSIX pass_fds")
    def test_collect_progress_emits_structured_protocol_events(self):
        progress = self.base / "_patch_lib" / "python_patch_collect_progress_v6_7.py"
        with tempfile.TemporaryDirectory(prefix="taskdeck-progress-") as td:
            root = Path(td)
            collector = root / "collector.py"
            collector.write_text(
                "import time\n"
                "print('search candidates', flush=True)\n"
                "time.sleep(0.35)\n",
                encoding="utf-8",
            )
            read_fd, write_fd = os.pipe()
            env = dict(os.environ)
            env[entry.EVENT_FD_ENV] = str(write_fd)
            env["PTV_COLLECT_HEARTBEAT_SECONDS"] = "0.1"
            env["TASKDECK_PATCH_PROGRESS_RUN_ID"] = "run-test"
            env["TASKDECK_PATCH_PROGRESS_INDEX"] = "2"
            env["TASKDECK_PATCH_PROGRESS_TOTAL"] = "3"
            env["TASKDECK_PATCH_PROGRESS_ITEM_NAME"] = "collect.zip"
            env["TASKDECK_PATCH_PROGRESS_ITEM_KIND"] = "COLLECT"
            try:
                cp = subprocess.run(
                    [
                        sys.executable, str(progress),
                        "--project-root", str(root),
                        "--collector", str(collector),
                        "--",
                    ],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    env=env,
                    pass_fds=(write_fd,),
                    timeout=10,
                )
                os.close(write_fd)
                write_fd = -1
                raw = os.read(read_fd, 1 << 20).decode("utf-8")
            finally:
                if write_fd >= 0:
                    os.close(write_fd)
                os.close(read_fd)
            self.assertNotEqual(raw.strip(), "")
            events = [json.loads(line) for line in raw.splitlines() if line.strip()]
            progress_events = [event for event in events if event.get("type") == "progress"]
            self.assertTrue(progress_events, (cp.returncode, cp.stdout, cp.stderr, events))
            self.assertTrue(any(event.get("status") == "RUNNING" for event in progress_events), progress_events)
            final = progress_events[-1]
            self.assertEqual(final.get("run_id"), "run-test")
            self.assertEqual(final.get("index"), 2)
            self.assertEqual(final.get("total"), 3)
            self.assertEqual(final.get("item_name"), "collect.zip")
            self.assertEqual(final.get("item_kind"), "COLLECT")
            self.assertIn(final.get("status"), {"PASS", "FAIL", "INCOMPLETE"})
            self.assertIsInstance(final.get("output_lines"), int)
            self.assertLessEqual(len(str(final.get("detail") or "")), 512)

    def test_dispatcher_passes_protocol_fd_only_to_collect_supervisor(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        self.assertIn('if label == "COLLECT":', dispatcher)
        self.assertIn('kwargs["pass_fds"] = (event_fd,)', dispatcher)
        self.assertIn('env["TASKDECK_PATCH_PROGRESS_INDEX"] = str(index + 1)', dispatcher)
        self.assertIn('env["TASKDECK_PATCH_PROGRESS_ITEM_NAME"] = item.name', dispatcher)

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


    def test_queue_snapshot_projects_new_and_failed_groups_from_python_policy(self):
        import python_patch_queue_dispatcher as dispatcher
        from python_patch_protocol import build_queue_snapshot

        items = [
            dispatcher.QueueItem("new.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("failed.zip", "PATCH", "manifest"),
        ]
        failed_row = {
            "name": "failed.zip",
            "status": "FAIL",
            "rc": 2,
            "patch_result": {
                "diagnosis": {
                    "kind": "build_failed",
                    "message": "compiler rejected source\nsecond line",
                }
            },
        }
        with mock.patch.object(dispatcher, "discover_queue", return_value=(items, ["warning"])), \
             mock.patch.object(dispatcher, "_load_previous_run", return_value={"selected": ["failed.zip"], "results": []}), \
             mock.patch.object(dispatcher, "_is_meaningful_run", return_value=True), \
             mock.patch.object(dispatcher, "_persistent_failed_queue_rows", return_value=[failed_row]), \
             mock.patch.object(dispatcher, "_failure_row_matches_queue_item", side_effect=lambda _root, row, item: row.get("name") == item.name):
            snapshot = build_queue_snapshot("/workspace")

        self.assertEqual(snapshot["group_counts"], {"new": 1, "failed": 1})
        rows = {row["name"]: row for row in snapshot["items"]}
        self.assertEqual(rows["new.zip"]["group"], "new")
        self.assertNotIn("failure", rows["new.zip"])
        self.assertEqual(rows["failed.zip"]["group"], "failed")
        self.assertEqual(rows["failed.zip"]["failure"]["status"], "FAIL")
        self.assertEqual(rows["failed.zip"]["failure"]["rc"], 2)
        self.assertEqual(rows["failed.zip"]["failure"]["diagnosis_kind"], "build_failed")
        self.assertNotIn("\n", rows["failed.zip"]["failure"]["message"])

    def test_failed_group_names_share_protocol_projection_mapping(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [dispatcher.QueueItem("failed.zip", "PATCH", "manifest")]
        row = {"name": "failed.zip", "status": "FAIL"}
        with mock.patch.object(dispatcher, "_persistent_failed_queue_rows", return_value=[row]), \
             mock.patch.object(dispatcher, "_failure_row_matches_queue_item", return_value=True):
            mapping = dispatcher._failed_queue_rows_by_name(Path("/workspace"), items, None)
            names = dispatcher._last_failed_queue_names(Path("/workspace"), items, None)
        self.assertEqual(set(mapping), names)
        self.assertEqual(names, {"failed.zip"})

    def test_queue_snapshot_uses_dispatcher_discovery_contract(self):
        from python_patch_protocol import build_queue_snapshot

        with tempfile.TemporaryDirectory() as tmp:
            snapshot = build_queue_snapshot(tmp)
        self.assertEqual(snapshot["status"], "empty")
        self.assertEqual(snapshot["items"], [])
        self.assertEqual(snapshot["warnings"], [])
        self.assertEqual(snapshot["counts"], {})
        self.assertEqual(snapshot["group_counts"], {"new": 0, "failed": 0})
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
