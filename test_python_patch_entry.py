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

    def test_health_routes_to_read_only_dispatcher(self):
        argv, _ = self.route("health")
        self.assertEqual(argv, [
            "/opt/taskdeck/patchtool/_patch_lib/python_patch_queue_dispatcher.py",
            "--project-root", self.root,
            "health",
        ])

    def test_utility_does_not_gain_execution_flags(self):
        argv, _ = self.route("health-search")
        self.assertEqual(argv, ["/opt/taskdeck/patchtool/_patch_lib/python_patch_runner.py", "health-search"])

    def test_transaction_only_fails_closed(self):
        with self.assertRaises(entry.EntryError):
            self.route("--transaction", "required")

    def test_malformed_transaction_does_not_swallow_following_automation_flag(self):
        argv, _ = self.route("--transaction", "--all")
        self.assertEqual(argv[-2:], ["run", "--all"])


class HistorySupportProtocolTests(unittest.TestCase):
    def setUp(self):
        self.base = Path(__file__).resolve().parent
        entry._prepare_environment(self.base)

    def test_history_projection_advertises_item_support_only_at_item_level(self):
        import python_patch_queue_dispatcher as dispatcher

        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            row = {"name": "demo.zip", "kind": "PATCH", "status": "FAIL"}
            item = dispatcher._protocol_history_item(root, row, 1)
        self.assertEqual(item["actions"], ["support"])

        fake_view = {
            "runs": [{"run_id": "run-1", "pinned": False}],
            "default_run_id": "run-1",
        }
        with mock.patch.object(dispatcher, "protocol_history_view", return_value=fake_view), \
             mock.patch.object(dispatcher, "_protocol_history_cleanup_summary", return_value={
                 "eligible": 0, "idle_eligible": 0, "overflow_eligible": 0,
                 "pinned": 0, "meaningful": 1, "limit": dispatcher.RUN_HISTORY_LIMIT,
                 "policy": "remove_unpinned_idle_then_oldest_unpinned_over_limit",
             }):
            prompt = dispatcher.protocol_history_prompt_contract(Path("/workspace"))
        self.assertIn("support", prompt["constraints"]["item_actions"])
        self.assertNotIn("support", prompt["actions"])
        self.assertNotIn("support", prompt["runs"][0]["actions"])

    def test_history_support_uses_existing_bundle_helper_and_emits_verified_artifact(self):
        import python_patch_queue_dispatcher as dispatcher

        class Writer:
            def __init__(self):
                self.events = []
            def emit(self, event_type, **payload):
                self.events.append((event_type, payload))
                return True

        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            (root / "artifacts" / "support").mkdir(parents=True)
            report = {
                "run_id": "run-1",
                "selected": ["demo.zip"],
                "results": [{"name": "demo.zip", "kind": "PATCH", "status": "FAIL"}],
            }
            history_path = root / "history.json"
            history_path.write_text("{}", encoding="utf-8")
            final = root / "artifacts" / "support" / "PTV_SUPPORT_run-1_001_demo.zip"

            def create_bundle(_root, got_report, row_index):
                self.assertIs(got_report, report)
                self.assertEqual(row_index, 0)
                final.write_bytes(b"zip")
                return final

            writer = Writer()
            command = {
                "command": "history_support",
                "payload": {
                    "prompt_id": "prompt-1",
                    "support_id": "support-1",
                    "run_id": "run-1",
                    "item_index": 1,
                },
            }
            with mock.patch.object(dispatcher, "_find_history_entry", return_value=(history_path, report)), \
                 mock.patch.object(dispatcher, "_is_meaningful_run", return_value=True), \
                 mock.patch.object(dispatcher, "_create_report_support_bundle", side_effect=create_bundle):
                dispatcher._protocol_history_support(root, writer, "prompt-1", {"run-1"}, command)

            self.assertEqual(len(writer.events), 1)
            event_type, payload = writer.events[0]
            self.assertEqual(event_type, "history_support_result")
            self.assertEqual(payload["status"], "PASS")
            self.assertEqual(payload["support_id"], "support-1")
            self.assertEqual(payload["run_id"], "run-1")
            self.assertEqual(payload["item_index"], 1)
            self.assertEqual(payload["artifact"]["path"], "artifacts/support/PTV_SUPPORT_run-1_001_demo.zip")

    def test_history_support_rejects_stale_prompt_run_or_hidden_index(self):
        import python_patch_queue_dispatcher as dispatcher

        class Writer:
            def emit(self, *_args, **_kwargs):
                raise AssertionError("invalid support request must not emit a result")

        report = {
            "run_id": "run-1",
            "selected": ["demo.zip"],
            "results": [{"name": "demo.zip", "kind": "PATCH", "status": "FAIL"}],
        }
        base = {
            "command": "history_support",
            "payload": {
                "prompt_id": "prompt-1",
                "support_id": "support-1",
                "run_id": "run-1",
                "item_index": 1,
            },
        }
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            history_path = root / "history.json"
            history_path.write_text("{}", encoding="utf-8")
            with mock.patch.object(dispatcher, "_find_history_entry", return_value=(history_path, report)), \
                 mock.patch.object(dispatcher, "_is_meaningful_run", return_value=True):
                stale = json.loads(json.dumps(base)); stale["payload"]["prompt_id"] = "other"
                with self.assertRaises(ValueError):
                    dispatcher._protocol_history_support(root, Writer(), "prompt-1", {"run-1"}, stale)
                wrong_run = json.loads(json.dumps(base)); wrong_run["payload"]["run_id"] = "run-2"
                with self.assertRaises(ValueError):
                    dispatcher._protocol_history_support(root, Writer(), "prompt-1", {"run-1"}, wrong_run)
                hidden = json.loads(json.dumps(base)); hidden["payload"]["item_index"] = 2
                with self.assertRaises(ValueError):
                    dispatcher._protocol_history_support(root, Writer(), "prompt-1", {"run-1"}, hidden)


class HistoryCleanupProtocolTests(unittest.TestCase):
    def setUp(self):
        self.base = Path(__file__).resolve().parent
        entry._prepare_environment(self.base)

    def test_history_cleanup_plan_preserves_pins_and_shares_terminal_helper(self):
        import python_patch_queue_dispatcher as dispatcher

        with tempfile.TemporaryDirectory(prefix="taskdeck-history-cleanup-plan-") as td:
            root = Path(td)
            entries = []
            for index in range(dispatcher.RUN_HISTORY_LIMIT + 2):
                path = root / f"run-{index}.json"
                path.write_text("{}", encoding="utf-8")
                entries.append((path, {"run_id": f"run-{index}", "selected": ["demo.zip"], "results": [{"name": "demo.zip"}]}))
            idle = root / "idle.json"
            idle.write_text("{}", encoding="utf-8")
            entries.append((idle, {"run_id": "idle-1", "selected": [], "results": []}))
            pinned = {"run-0"}
            with mock.patch.object(dispatcher, "_history_entries", return_value=list(reversed(entries))), \
                 mock.patch.object(dispatcher, "_load_pinned_runs", return_value=pinned), \
                 mock.patch.object(dispatcher, "_batch_run_dir", side_effect=lambda _root, rid: root / "runs" / rid), \
                 mock.patch.object(dispatcher, "_visible_history_entries", return_value=[]):
                plan = dispatcher._history_cleanup_plan(root)
                self.assertEqual(plan["idle_eligible"], 1)
                self.assertEqual(plan["overflow_eligible"], 2)
                self.assertEqual(plan["eligible"], 3)
                self.assertFalse(any(str(data.get("run_id")) == "run-0" for _path, data, _reason in plan["candidates"]))
                result = dispatcher._cleanup_history(root)
            self.assertEqual(result["removed"], 3)
            self.assertEqual(result["pinned"], 1)
            self.assertTrue((root / "run-0.json").exists())

    def test_history_prompt_advertises_python_owned_cleanup_policy(self):
        import python_patch_queue_dispatcher as dispatcher

        fake_view = {"status": "available", "runs": [{"run_id": "run-1", "pinned": False}], "default_run_id": "run-1"}
        fake_cleanup = {
            "eligible": 4, "idle_eligible": 1, "overflow_eligible": 3,
            "pinned": 2, "meaningful": 33, "limit": 30,
            "policy": "remove_unpinned_idle_then_oldest_unpinned_over_limit",
        }
        with mock.patch.object(dispatcher, "protocol_history_view", return_value=fake_view), \
             mock.patch.object(dispatcher, "_protocol_history_cleanup_summary", return_value=fake_cleanup):
            prompt = dispatcher.protocol_history_prompt_contract(Path("/workspace"))
        self.assertIn("cleanup", prompt["actions"])
        self.assertIn("cleanup", prompt["constraints"]["destructive_actions"])
        self.assertEqual(prompt["constraints"]["cleanup"], fake_cleanup)
        self.assertNotIn("cleanup", prompt["runs"][0]["actions"])

    def test_history_cleanup_command_accepts_no_web_candidate_list_and_emits_counts(self):
        import python_patch_queue_dispatcher as dispatcher

        class Writer:
            def __init__(self):
                self.events = []
            def emit(self, event_type, **payload):
                self.events.append((event_type, payload))
                return True

        writer = Writer()
        command = {
            "command": "history_cleanup",
            "payload": {"prompt_id": "prompt-1", "cleanup_id": "cleanup-1", "confirmed": True},
        }
        with mock.patch.object(dispatcher, "_protocol_history_cleanup_summary", return_value={
            "eligible": 3, "idle_eligible": 1, "overflow_eligible": 2, "pinned": 1,
            "meaningful": 32, "limit": 30,
            "policy": "remove_unpinned_idle_then_oldest_unpinned_over_limit",
        }), mock.patch.object(dispatcher, "_cleanup_history", return_value={"removed": 3, "pinned": 1, "remaining": 30}):
            changed = dispatcher._protocol_history_cleanup(Path("/workspace"), writer, "prompt-1", True, command)
        self.assertTrue(changed)
        self.assertEqual(len(writer.events), 1)
        event_type, payload = writer.events[0]
        self.assertEqual(event_type, "history_cleanup_result")
        self.assertEqual(payload["cleanup_id"], "cleanup-1")
        self.assertEqual(payload["removed"], 3)
        self.assertEqual(payload["pinned"], 1)
        self.assertEqual(payload["remaining"], 30)
        self.assertTrue(payload["history_changed"])

        bad = json.loads(json.dumps(command))
        bad["payload"]["run_ids"] = ["run-1"]
        with self.assertRaises(ValueError):
            dispatcher._protocol_history_cleanup(Path("/workspace"), Writer(), "prompt-1", True, bad)
        stale = json.loads(json.dumps(command))
        stale["payload"]["prompt_id"] = "stale"
        with self.assertRaises(ValueError):
            dispatcher._protocol_history_cleanup(Path("/workspace"), Writer(), "prompt-1", True, stale)
        unconfirmed = json.loads(json.dumps(command))
        unconfirmed["payload"]["confirmed"] = False
        with self.assertRaises(ValueError):
            dispatcher._protocol_history_cleanup(Path("/workspace"), Writer(), "prompt-1", True, unconfirmed)


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

    def test_dispatcher_protocol_queue_delete_refreshes_snapshot_and_prompt_indexes(self):
        from python_patch_queue_dispatcher import QueueItem, _protocol_queue_selection
        import python_patch_queue_dispatcher as dispatcher

        with tempfile.TemporaryDirectory(prefix="taskdeck-queue-delete-") as tmp:
            root = Path(tmp)
            patchs = root / "patchs"
            patchs.mkdir()
            (patchs / "one.zip").write_bytes(b"one")
            (patchs / "two.zip").write_bytes(b"two")

            event_read, event_write = os.pipe()
            command_read, command_write = os.pipe()
            old_event = os.environ.get(entry.EVENT_FD_ENV)
            old_command = os.environ.get(entry.COMMAND_FD_ENV)
            os.environ[entry.EVENT_FD_ENV] = str(event_write)
            os.environ[entry.COMMAND_FD_ENV] = str(command_read)
            seen = {}

            def respond():
                with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                    first_prompt = json.loads(stream.readline())
                    seen["first_prompt"] = first_prompt
                    delete = {
                        "protocol": "taskdeck.patch",
                        "version": 1,
                        "type": "command",
                        "seq": 1,
                        "command": "queue_delete",
                        "payload": {
                            "prompt_id": first_prompt["prompt_id"],
                            "mutation_id": "delete-1",
                            "index": 1,
                        },
                    }
                    os.write(command_write, (json.dumps(delete) + "\n").encode("utf-8"))
                    seen["mutation"] = json.loads(stream.readline())
                    seen["snapshot"] = json.loads(stream.readline())
                    second_prompt = json.loads(stream.readline())
                    seen["second_prompt"] = second_prompt
                    response = {
                        "protocol": "taskdeck.patch",
                        "version": 1,
                        "type": "command",
                        "seq": 2,
                        "command": "prompt_response",
                        "payload": {
                            "prompt_id": second_prompt["prompt_id"],
                            "action": "select",
                            "indexes": [1],
                        },
                    }
                    os.write(command_write, (json.dumps(response) + "\n").encode("utf-8"))

            worker = threading.Thread(target=respond)
            worker.start()
            items = [
                QueueItem("one.zip", "PATCH", "manifest"),
                QueueItem("two.zip", "PATCH", "manifest"),
            ]
            refreshed = {
                "status": "runnable",
                "items": [{"name": "two.zip", "kind": "PATCH", "detail": "manifest", "group": "new"}],
                "warnings": [],
                "counts": {"PATCH": 1},
                "group_counts": {"new": 1, "failed": 0},
                "total": 1,
            }
            try:
                with mock.patch.object(dispatcher, "_unresolved_failure_rows", return_value=[]), \
                     mock.patch.object(dispatcher, "protocol_queue_view", return_value=refreshed):
                    handled, chosen = _protocol_queue_selection(items, "none", {"one.zip"}, root=root)
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
            self.assertEqual([item.name for item in chosen], ["two.zip"])
            self.assertFalse((patchs / "one.zip").exists())
            self.assertTrue((patchs / "two.zip").exists())
            self.assertEqual(seen["first_prompt"]["queue_actions"], ["delete"])
            self.assertNotEqual(seen["first_prompt"]["prompt_id"], seen["second_prompt"]["prompt_id"])
            self.assertEqual(seen["mutation"]["type"], "queue_mutation_result")
            self.assertEqual(seen["mutation"]["mutation_id"], "delete-1")
            self.assertEqual(seen["mutation"]["status"], "PASS")
            self.assertEqual(seen["snapshot"]["type"], "queue_snapshot")
            self.assertEqual(seen["second_prompt"]["items"][0]["index"], 1)
            self.assertEqual(seen["second_prompt"]["items"][0]["name"], "two.zip")
            self.assertEqual(seen["second_prompt"]["initial_selected"], [1])

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
        self.assertEqual(seen["prompt"]["queue_actions"], ["delete"])
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

    def test_protocol_queue_priority_uses_terminal_ordering_semantics(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [
            dispatcher.QueueItem("a.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("b.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("c.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("d.zip", "PATCH", "manifest"),
        ]
        selected = {0, 1, 2, 3}
        priorities = dispatcher._protocol_selection_priorities(
            items,
            selected,
            [
                {"index": 3, "priority": 5},
                {"index": 2, "priority": 0},
                {"index": 1, "priority": 5},
            ],
        )
        chosen = dispatcher._ordered_selection(items, selected, priorities)
        self.assertEqual(priorities, {2: 5, 1: 0, 0: 5})
        self.assertEqual([item.name for item in chosen], ["b.zip", "a.zip", "c.zip", "d.zip"])

    def test_protocol_queue_priority_ties_keep_original_queue_order(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [
            dispatcher.QueueItem("a.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("b.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("c.zip", "PATCH", "manifest"),
        ]
        selected = {0, 1, 2}
        priorities = dispatcher._protocol_selection_priorities(
            items,
            selected,
            [{"index": 2, "priority": 3}, {"index": 1, "priority": 3}],
        )
        self.assertEqual(
            [item.name for item in dispatcher._ordered_selection(items, selected, priorities)],
            ["a.zip", "b.zip", "c.zip"],
        )

    def test_protocol_queue_priority_rejects_invalid_or_nonpatch_rows(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [
            dispatcher.QueueItem("a.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("collect.zip", "COLLECT", "request"),
        ]
        invalid = [
            ({0}, [{"index": 2, "priority": 0}]),
            ({0}, [{"index": 1, "priority": 10}]),
            ({0}, [{"index": 1, "priority": True}]),
            ({0}, [{"index": 1, "priority": 0}, {"index": 1, "priority": 1}]),
            ({0}, [{"index": 1, "priority": 0, "extra": "x"}]),
            ({1}, [{"index": 2, "priority": 0}]),
        ]
        for selected, priorities in invalid:
            with self.subTest(selected=selected, priorities=priorities):
                with self.assertRaises(ValueError):
                    dispatcher._protocol_selection_priorities(items, selected, priorities)

    def test_protocol_queue_no_priority_uses_canonical_queue_order(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [
            dispatcher.QueueItem("a.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("b.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("c.zip", "PATCH", "manifest"),
        ]
        selected = {0, 2}
        priorities = dispatcher._protocol_selection_priorities(items, selected, None)
        self.assertEqual(priorities, {})
        self.assertEqual(
            [item.name for item in dispatcher._ordered_selection(items, selected, priorities)],
            ["a.zip", "c.zip"],
        )

    def test_protocol_queue_prompt_advertises_patch_priority_contract(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        start = dispatcher.index('def _protocol_queue_selection(')
        end = dispatcher.index('\ndef select_items(', start)
        block = dispatcher[start:end]
        for want in (
            '"patch_priority": {',
            '"min": 0',
            '"max": 9',
            '"unprioritized_order": 10',
            '"response_field": "priorities"',
            'response.get("priorities")',
            '_protocol_selection_priorities(items, selected_indexes',
            '_ordered_selection(items, selected_indexes, priorities)',
        ):
            self.assertIn(want, block)

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
            (["report"], "report"),
            (["collect", "search"], "collect"),
            (["--all"], "run"),
            (["health"], "health"),
            (["health-search"], "utility"),
            (["example.zip"], "direct"),
        ]
        for args, want in cases:
            self.assertEqual(entry.classify_route(args), want)


    def test_health_snapshot_projection_is_bounded_and_stable(self):
        from python_patch_health import protocol_health_snapshot

        report = {
            "status": "WARN",
            "tool_version": "6.20.2",
            "checks": [
                {"name": "version", "status": "PASS", "detail": "6.20.2", "private": "hide"},
                {"name": "sha256sums", "status": "FAIL", "entries": 12, "failures": 1, "secret": "hide"},
                {"name": "python_cache_hygiene", "status": "WARN", "files": 2, "dirs": 1},
            ],
            "warnings": ["cache files present"],
            "errors": ["checksum mismatch"],
            "internal": {"never": "expose"},
        }
        snapshot = protocol_health_snapshot(report)
        self.assertEqual(snapshot["status"], "WARN")
        self.assertEqual(snapshot["tool_version"], "6.20.2")
        self.assertEqual(snapshot["summary"], {"pass": 1, "warn": 1, "fail": 1, "total": 3})
        self.assertEqual(snapshot["checks"][0], {"name": "version", "status": "PASS", "detail": "6.20.2"})
        self.assertNotIn("private", snapshot["checks"][0])
        self.assertNotIn("secret", snapshot["checks"][1])
        self.assertNotIn("internal", snapshot)

    def test_health_runtime_path_mapping_preserves_historical_manifest_names(self):
        from python_patch_health import _runtime_rel

        self.assertEqual(_runtime_rel("tools/run_python_patches.sh"), "run_python_patches.sh")
        self.assertEqual(_runtime_rel("tools/_patch_lib/VERSION"), "_patch_lib/VERSION")
        for invalid in ("", "run_python_patches.sh", "../tools/run_python_patches.sh"):
            with self.assertRaises(ValueError):
                _runtime_rel(invalid)

    def test_plan_snapshot_projects_only_stable_bounded_fields(self):
        import python_patch_queue_dispatcher as dispatcher

        root = Path("/workspace")
        item = dispatcher.QueueItem("demo.zip", "PATCH", "manifest")
        meta = mock.Mock()
        meta.patch_id = "patch.demo"
        meta.package_sha256 = "a" * 64
        meta.effective_targets = ["src/a.ts", "src/b.ts"]
        meta.depends_on = ["patch.base"]
        preview_result = {
            "status": "PASS",
            "rc": 0,
            "stage": "preview",
            "preflight": {"target_paths": ["src/a.ts", "src/b.ts"], "_private": "hide"},
            "diagnosis": {
                "kind": "ready_to_apply",
                "message": "project unchanged; predicted_changed=2",
                "private": "hide",
            },
            "raw_internal": {"secret": True},
        }
        events = []

        with mock.patch.object(dispatcher, "_load_previous_run", return_value=None), \
             mock.patch.object(dispatcher, "_load_zero_argument_config", return_value=({"failure_policy":"continue_independent","transaction_policy":"patch"}, [])), \
             mock.patch.object(dispatcher, "discover_queue", return_value=([item], [])), \
             mock.patch.object(dispatcher, "_planning_previous", return_value=None), \
             mock.patch.object(dispatcher, "_build_batch_plan", return_value=([item], {"demo.zip": meta}, None)), \
             mock.patch.object(dispatcher, "transaction_compatibility", return_value=[]), \
             mock.patch.object(dispatcher, "analyze_static_conflicts", return_value=[{
                 "left":"demo.zip","left_patch_id":"patch.demo","right":"other.zip","right_patch_id":"patch.other",
                 "overlap":["src/a.ts"],"relation":"order_dependent_overlap","dependency_ordered":False,
             }]), \
             mock.patch.object(dispatcher, "ledger_id_reuse", return_value=[{"sha256":"old"}]), \
             mock.patch.object(dispatcher, "disk_preflight", return_value={
                 "status":"PASS",
                 "actual_project_free_bytes":1000,
                 "required_project_free_bytes":100,
                 "actual_temp_free_bytes":900,
                 "required_temp_free_bytes":50,
                 "private_internal":"hide",
             }), \
             mock.patch.object(dispatcher, "_plan_preview_item", return_value=(0, preview_result)), \
             mock.patch.object(dispatcher, "_emit_protocol_event", side_effect=lambda event_type, **payload: events.append((event_type,payload)) or True):
            rc = dispatcher._plan_queue(root)

        self.assertEqual(rc, 0)
        self.assertEqual(len(events), 1)
        event_type, snapshot = events[0]
        self.assertEqual(event_type, "plan_snapshot")
        self.assertEqual(snapshot["status"], "ready")
        self.assertEqual(snapshot["items"][0], {
            "index":1,"name":"demo.zip","patch_id":"patch.demo","package_sha256":"a"*64,
            "target_count":2,"depends_on":["patch.base"],"id_reuse_count":1,
        })
        self.assertEqual(snapshot["resources"], {
            "status":"PASS",
            "actual_project_free_bytes":1000,
            "required_project_free_bytes":100,
            "actual_temp_free_bytes":900,
            "required_temp_free_bytes":50,
        })
        self.assertEqual(snapshot["previews"][0]["status"], "PASS")
        self.assertEqual(snapshot["previews"][0]["target_count"], 2)
        serialized = json.dumps(snapshot, sort_keys=True)
        self.assertNotIn("raw_internal", serialized)
        self.assertNotIn("private_internal", serialized)
        self.assertNotIn('"_private"', serialized)

    def test_plan_snapshot_emits_blocked_resource_gate_without_preview(self):
        import python_patch_queue_dispatcher as dispatcher

        item = dispatcher.QueueItem("demo.zip", "PATCH", "manifest")
        meta = mock.Mock()
        meta.patch_id = "patch.demo"
        meta.package_sha256 = "b" * 64
        meta.effective_targets = ["src/a.ts"]
        meta.depends_on = []
        events = []
        with mock.patch.object(dispatcher, "_load_previous_run", return_value=None), \
             mock.patch.object(dispatcher, "_load_zero_argument_config", return_value=({}, [])), \
             mock.patch.object(dispatcher, "discover_queue", return_value=([item], [])), \
             mock.patch.object(dispatcher, "_planning_previous", return_value=None), \
             mock.patch.object(dispatcher, "_build_batch_plan", return_value=([item], {"demo.zip": meta}, None)), \
             mock.patch.object(dispatcher, "transaction_compatibility", return_value=[]), \
             mock.patch.object(dispatcher, "analyze_static_conflicts", return_value=[]), \
             mock.patch.object(dispatcher, "ledger_id_reuse", return_value=[]), \
             mock.patch.object(dispatcher, "disk_preflight", return_value={"status":"FAIL","actual_project_free_bytes":1,"required_project_free_bytes":2}), \
             mock.patch.object(dispatcher, "_plan_preview_item") as preview, \
             mock.patch.object(dispatcher, "_emit_protocol_event", side_effect=lambda event_type, **payload: events.append((event_type,payload)) or True):
            rc = dispatcher._plan_queue(Path("/workspace"))

        self.assertEqual(rc, 2)
        preview.assert_not_called()
        self.assertEqual(events[-1][0], "plan_snapshot")
        self.assertEqual(events[-1][1]["status"], "blocked")
        self.assertEqual(events[-1][1]["error"]["kind"], "insufficient_disk_space")

    def test_queue_search_projection_is_bounded_and_manifest_free(self):
        import python_patch_queue_dispatcher as dispatcher

        patch = dispatcher.QueueItem("Feature_DEMO.zip", "PATCH", "manifest")
        collect = dispatcher.QueueItem("collect_demo.zip", "COLLECT", "VIN lookup request")
        meta = mock.Mock()
        meta.patch_id = "PATCH-ID-ABC"
        meta.manifest = {
            "patch": {
                "summary": "Fix Hero Formation",
                "private_internal_field": "must not leak",
            },
            "other": {"secret": "must not leak"},
        }
        meta.effective_targets = ["Client/Hero.ts", "Server/HeroService.java"]

        cache: dict[str, object] = {}
        with mock.patch.object(dispatcher, "load_patch_meta", return_value=meta):
            patch_search = dispatcher._selector_search_projection(Path("/workspace"), patch, cache)
            collect_search = dispatcher._selector_search_projection(Path("/workspace"), collect, cache)

        self.assertEqual(
            set(patch_search),
            {"name", "id", "summary", "targets", "text"},
        )
        self.assertEqual(patch_search["name"], "feature_demo.zip")
        self.assertEqual(patch_search["id"], "patch-id-abc")
        self.assertEqual(patch_search["summary"], "fix hero formation")
        self.assertEqual(
            patch_search["targets"],
            ["client/hero.ts", "server/heroservice.java"],
        )
        self.assertNotIn("private_internal_field", json.dumps(patch_search))
        self.assertNotIn("must not leak", json.dumps(patch_search))
        self.assertEqual(collect_search["summary"], "vin lookup request")
        self.assertEqual(collect_search["id"], "")
        self.assertEqual(collect_search["targets"], [])

    def test_native_search_projection_matches_terminal_for_cutover_fields(self):
        import python_patch_queue_dispatcher as dispatcher

        patch = dispatcher.QueueItem("Feature_DEMO.zip", "PATCH", "manifest")
        collect = dispatcher.QueueItem("collect_demo.zip", "COLLECT", "VIN lookup request")
        items = [patch, collect]
        meta = mock.Mock()
        meta.patch_id = "PATCH-ID-ABC"
        meta.manifest = {"patch": {"summary": "Fix Hero Formation"}}
        meta.effective_targets = ["Client/Hero.ts", "Server/HeroService.java"]

        with mock.patch.object(dispatcher, "load_patch_meta", return_value=meta):
            for query in [
                "feature_demo",
                "patch-id-abc",
                "hero formation",
                "client/hero.ts",
                "vin lookup",
            ]:
                terminal = [
                    item.name
                    for item in dispatcher._filter_selector_items(
                        Path("/workspace"), items, query, {}
                    )
                ]
                projection_cache: dict[str, object] = {}
                native = [
                    item.name
                    for item in items
                    if query.strip().casefold()
                    in str(
                        dispatcher._selector_search_projection(
                            Path("/workspace"), item, projection_cache
                        )["text"]
                    )
                ]
                self.assertEqual(native, terminal, query)

    def test_queue_protocol_view_contains_only_bounded_search_projection(self):
        import python_patch_queue_dispatcher as dispatcher

        item = dispatcher.QueueItem("demo.zip", "PATCH", "manifest")
        meta = mock.Mock()
        meta.patch_id = "ID-1"
        meta.manifest = {"patch": {"summary": "Summary"}}
        meta.effective_targets = ["src/main.ts"]
        with mock.patch.object(dispatcher, "discover_queue", return_value=([item], [])), \
             mock.patch.object(dispatcher, "_load_previous_run", return_value=None), \
             mock.patch.object(dispatcher, "_failed_queue_rows_by_name", return_value={}), \
             mock.patch.object(dispatcher, "load_patch_meta", return_value=meta):
            view = dispatcher.protocol_queue_view(Path("/workspace"))

        self.assertEqual(view["items"][0]["search"], {
            "name": "demo.zip",
            "id": "id-1",
            "summary": "summary",
            "targets": ["src/main.ts"],
            "text": "demo.zip\nid-1\nsummary\nsrc/main.ts",
        })

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

    def test_resume_protocol_view_is_python_owned_and_bounded(self):
        import python_patch_queue_dispatcher as dispatcher

        items = [
            dispatcher.QueueItem("replay.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("failed.zip", "PATCH", "manifest"),
            dispatcher.QueueItem("remaining.zip", "PATCH", "manifest"),
        ]
        previous = {
            "status": "FAIL",
            "failed_item": "failed.zip",
            "results": [
                {"name": "replay.zip", "kind": "PATCH", "status": "PASS", "batch_rolled_back": True},
                {"name": "failed.zip", "kind": "PATCH", "status": "FAIL", "rc": 2, "patch_result": {"diagnosis": {"kind": "build_failed", "message": "bad\nsource"}}},
                {"name": "remaining.zip", "kind": "PATCH", "status": "BLOCKED"},
            ],
        }
        failed_row = previous["results"][1]
        bound = dict(failed_row)
        bound["_recovery_queue_name"] = "failed.zip"
        with mock.patch.object(dispatcher, "_merged_failed_recovery_rows", return_value=[failed_row]), \
             mock.patch.object(dispatcher, "_queued_failed_rows", return_value=[bound]), \
             mock.patch.object(dispatcher, "_bind_recovery_queue_row", return_value=bound), \
             mock.patch.object(dispatcher, "_visible_history_entries", return_value=[(Path("history.json"), previous)]):
            view = dispatcher.protocol_resume_view(Path("/workspace"), items, previous)
            prompt = dispatcher.protocol_resume_prompt_contract(Path("/workspace"), items, previous)

        self.assertEqual(view["status"], "available")
        self.assertEqual(view["summary"]["all"], 3)
        self.assertEqual(view["summary"]["failed"], 1)
        self.assertEqual(view["summary"]["remaining"], 1)
        self.assertEqual([row["group"] for row in view["items"]], ["replay", "failed", "remaining"])
        self.assertEqual(view["failed_items"][0]["queue_name"], "failed.zip")
        self.assertTrue(view["failed_items"][0]["can_retry"])
        self.assertTrue(view["failed_items"][0]["can_collect"])
        self.assertTrue(view["failed_items"][0]["can_delete"])
        self.assertNotIn("\n", view["failed_items"][0]["failure"]["message"])
        self.assertEqual(prompt["prompt_kind"], "resume_action")
        self.assertIn("failed", prompt["actions"])
        self.assertIn("collect_failed", prompt["actions"])
        self.assertEqual(prompt["constraints"]["failed_index_base"], 1)

    def test_native_resume_is_strictly_opt_in(self):
        import python_patch_queue_dispatcher as dispatcher
        old = os.environ.pop("TASKDECK_PATCH_NATIVE_RESUME", None)
        try:
            handled, decision = dispatcher._protocol_resume_selection(Path("/workspace"), [], None)
        finally:
            if old is not None:
                os.environ["TASKDECK_PATCH_NATIVE_RESUME"] = old
        self.assertFalse(handled)
        self.assertIsNone(decision)

    def test_native_resume_action_uses_prompt_bound_failed_indexes(self):
        import python_patch_queue_dispatcher as dispatcher

        previous = {"status": "FAIL", "results": [{"name": "failed.zip", "kind": "PATCH", "status": "FAIL"}]}
        items = [dispatcher.QueueItem("failed.zip", "PATCH", "manifest")]
        failed_row = {"name": "failed.zip", "kind": "PATCH", "status": "FAIL", "_recovery_queue_name": "failed.zip"}

        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_RESUME")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_RESUME"] = "1"
        seen = {}

        def respond():
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
                seen["prompt"] = prompt
            command = {
                "protocol": "taskdeck.patch",
                "version": 1,
                "type": "command",
                "seq": 1,
                "command": "resume_action",
                "payload": {
                    "prompt_id": prompt["prompt_id"],
                    "action": "failed",
                    "failed_indexes": [1],
                },
            }
            os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))

        worker = threading.Thread(target=respond)
        worker.start()
        try:
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-resume-") as td, \
                 mock.patch.object(dispatcher, "_merged_failed_recovery_rows", return_value=[failed_row]), \
                 mock.patch.object(dispatcher, "_queued_failed_rows", return_value=[failed_row]), \
                 mock.patch.object(dispatcher, "_bind_recovery_queue_row", return_value=failed_row), \
                 mock.patch.object(dispatcher, "_visible_history_entries", return_value=[]):
                root = Path(td)
                (root / "patchs").mkdir()
                (root / "patchs" / "failed.zip").write_bytes(b"fixture")
                handled, decision = dispatcher._protocol_resume_selection(root, items, previous)
        finally:
            worker.join(timeout=2)
            if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
            else: os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
            else: os.environ[entry.COMMAND_FD_ENV] = old_command
            if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_RESUME", None)
            else: os.environ["TASKDECK_PATCH_NATIVE_RESUME"] = old_native
            for fd in (event_read, event_write, command_read, command_write):
                try: os.close(fd)
                except OSError: pass

        self.assertTrue(handled)
        self.assertEqual(seen["prompt"]["prompt_kind"], "resume_action")
        self.assertIn("failed", seen["prompt"]["actions"])
        self.assertEqual([item.name for item in decision["items"]], ["failed.zip"])

    def test_native_resume_invalid_response_falls_back_without_execution(self):
        import python_patch_queue_dispatcher as dispatcher

        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_RESUME")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_RESUME"] = "1"

        def respond():
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
            command = {
                "protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": 1,
                "command": "resume_action",
                "payload": {"prompt_id": prompt["prompt_id"], "action": "failed", "failed_indexes": [999]},
            }
            os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))

        worker = threading.Thread(target=respond)
        worker.start()
        try:
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-resume-invalid-") as td, \
                 mock.patch.object(dispatcher, "_merged_failed_recovery_rows", return_value=[]), \
                 mock.patch.object(dispatcher, "_queued_failed_rows", return_value=[]), \
                 mock.patch.object(dispatcher, "_visible_history_entries", return_value=[]):
                root = Path(td)
                (root / "patchs").mkdir()
                handled, decision = dispatcher._protocol_resume_selection(root, [], {"status": "FAIL", "results": []})
        finally:
            worker.join(timeout=2)
            if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
            else: os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
            else: os.environ[entry.COMMAND_FD_ENV] = old_command
            if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_RESUME", None)
            else: os.environ["TASKDECK_PATCH_NATIVE_RESUME"] = old_native
            for fd in (event_read, event_write, command_read, command_write):
                try: os.close(fd)
                except OSError: pass
        self.assertFalse(handled)
        self.assertIsNone(decision)

    def test_resume_snapshot_emission_is_additive_before_terminal_resume_selection(self):
        dispatcher = (self.base / "_patch_lib" / "python_patch_queue_dispatcher.py").read_text(encoding="utf-8")
        emit = dispatcher.index("_emit_protocol_resume_snapshot(root, items, meaningful_previous)")
        select = dispatcher.index("_resume_selection(", emit)
        self.assertLess(emit, select)
        self.assertIn("def protocol_resume_prompt_contract(", dispatcher)
        self.assertIn('os.environ.get("TASKDECK_PATCH_NATIVE_RESUME", "").strip() != "1"', dispatcher)
        native = dispatcher.index("_protocol_resume_selection(root, items, meaningful_previous)", emit)
        terminal = dispatcher.index("_resume_selection(", native)
        self.assertLess(emit, native)
        self.assertLess(native, terminal)

    def test_native_history_is_strictly_opt_in(self):
        import python_patch_queue_dispatcher as dispatcher
        old = os.environ.pop("TASKDECK_PATCH_NATIVE_HISTORY", None)
        try:
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-history-off-") as td:
                self.assertFalse(dispatcher._protocol_history_detail_session(Path(td)))
        finally:
            if old is not None:
                os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = old

    @unittest.skipUnless(os.name == "posix", "native History command channel requires POSIX FDs")
    def test_native_history_detail_is_prompt_bound_read_only_and_repeatable(self):
        import python_patch_queue_dispatcher as dispatcher
        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_HISTORY")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = "1"
        seen = {}
        snapshot = {
            "status": "available",
            "runs": [{"run_id": "run-1", "status": "PASS"}, {"run_id": "run-2", "status": "FAIL"}],
            "total": 2, "default_run_id": "run-1", "truncated": False,
        }

        def respond():
            nonlocal command_write
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
            seen["prompt"] = prompt
            for seq, run_id in ((1, "run-1"), (2, "run-2")):
                command = {
                    "protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": seq,
                    "command": "history_detail",
                    "payload": {"prompt_id": prompt["prompt_id"], "run_id": run_id},
                }
                os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))
            os.close(command_write)
            command_write = -1

        worker = threading.Thread(target=respond)
        worker.start()
        try:
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-history-") as td, \
                 mock.patch.object(dispatcher, "protocol_history_view", return_value=snapshot), \
                 mock.patch.object(dispatcher, "protocol_history_report_view", side_effect=lambda _root, run_id: {
                     "status": "available", "run": {"run_id": run_id, "status": "PASS"},
                     "items": [], "total_items": 0, "items_truncated": False, "files": [], "warnings": [],
                 }):
                handled = dispatcher._protocol_history_detail_session(Path(td))
            worker.join(timeout=2)
            self.assertTrue(handled)
            self.assertFalse(worker.is_alive())
            os.close(event_write); event_write = -1
            remaining = os.read(event_read, 1 << 20).decode("utf-8")
        finally:
            if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
            else: os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
            else: os.environ[entry.COMMAND_FD_ENV] = old_command
            if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_HISTORY", None)
            else: os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = old_native
            for fd in (event_read, event_write, command_read, command_write):
                if fd >= 0:
                    try: os.close(fd)
                    except OSError: pass

        self.assertEqual(seen["prompt"]["prompt_kind"], "history_action")
        self.assertEqual(seen["prompt"]["actions"], ["detail", "pin", "unpin", "delete", "export", "cleanup"])
        self.assertTrue(seen["prompt"]["constraints"]["detail_read_only"])
        self.assertEqual(seen["prompt"]["constraints"]["destructive_actions"], ["delete", "cleanup"])
        self.assertIn("pin", seen["prompt"]["runs"][0]["actions"])
        self.assertNotIn("unpin", seen["prompt"]["runs"][0]["actions"])
        events = [json.loads(line) for line in remaining.splitlines() if line.strip()]
        reports = [event for event in events if event.get("type") == "history_report"]
        self.assertEqual([event["run"]["run_id"] for event in reports], ["run-1", "run-2"])
        self.assertTrue(all(event["prompt_id"] == seen["prompt"]["prompt_id"] for event in reports))

    @unittest.skipUnless(os.name == "posix", "native History command channel requires POSIX FDs")
    def test_native_history_rejects_unadvertised_run_id(self):
        import python_patch_queue_dispatcher as dispatcher
        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_HISTORY")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = "1"

        def respond():
            nonlocal command_write
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                prompt = json.loads(stream.readline())
            command = {
                "protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": 1,
                "command": "history_detail",
                "payload": {"prompt_id": prompt["prompt_id"], "run_id": "not-advertised"},
            }
            os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))
            os.close(command_write); command_write = -1

        worker = threading.Thread(target=respond); worker.start()
        try:
            snapshot = {"status": "available", "runs": [{"run_id": "run-1"}], "total": 1, "default_run_id": "run-1", "truncated": False}
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-history-invalid-") as td, \
                 mock.patch.object(dispatcher, "protocol_history_view", return_value=snapshot), \
                 mock.patch.object(dispatcher, "protocol_history_report_view") as report_view:
                handled = dispatcher._protocol_history_detail_session(Path(td))
                report_view.assert_not_called()
            worker.join(timeout=2)
            os.close(event_write); event_write = -1
            remaining = os.read(event_read, 1 << 20).decode("utf-8")
        finally:
            if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
            else: os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
            else: os.environ[entry.COMMAND_FD_ENV] = old_command
            if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_HISTORY", None)
            else: os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = old_native
            for fd in (event_read, event_write, command_read, command_write):
                if fd >= 0:
                    try: os.close(fd)
                    except OSError: pass
        self.assertTrue(handled)
        events = [json.loads(line) for line in remaining.splitlines() if line.strip()]
        self.assertTrue(any(event.get("type") == "error" and event.get("phase") == "history_action" for event in events))
        self.assertFalse(any(event.get("type") == "history_report" for event in events))

    @unittest.skipUnless(os.name == "posix", "native History management requires POSIX FDs")
    def test_native_history_pin_refreshes_snapshot_and_prompt_capabilities(self):
        import python_patch_queue_dispatcher as dispatcher
        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_HISTORY")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = "1"
        state = {"pinned": False}
        seen = {}

        def history_view(_root):
            return {
                "status": "available",
                "runs": [{"run_id": "run-1", "status": "PASS", "pinned": state["pinned"]}],
                "total": 1,
                "default_run_id": "run-1",
                "truncated": False,
            }

        def pin_history(_root, run_id, pin):
            self.assertEqual(run_id, "run-1")
            state["pinned"] = bool(pin)
            return 0

        def respond():
            nonlocal command_write
            with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                first = json.loads(stream.readline())
                seen["first"] = first
                command = {
                    "protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": 1,
                    "command": "history_manage",
                    "payload": {
                        "prompt_id": first["prompt_id"],
                        "management_id": "manage-1",
                        "action": "pin",
                        "run_id": "run-1",
                    },
                }
                os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))
                seen["result"] = json.loads(stream.readline())
                seen["snapshot"] = json.loads(stream.readline())
                seen["second"] = json.loads(stream.readline())
            os.close(command_write)
            command_write = -1

        worker = threading.Thread(target=respond)
        worker.start()
        try:
            with tempfile.TemporaryDirectory(prefix="taskdeck-native-history-pin-") as td, \
                 mock.patch.object(dispatcher, "protocol_history_view", side_effect=history_view), \
                 mock.patch.object(dispatcher, "_pin_history", side_effect=pin_history):
                handled = dispatcher._protocol_history_detail_session(Path(td))
            worker.join(timeout=2)
        finally:
            if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
            else: os.environ[entry.EVENT_FD_ENV] = old_event
            if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
            else: os.environ[entry.COMMAND_FD_ENV] = old_command
            if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_HISTORY", None)
            else: os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = old_native
            for fd in (event_read, event_write, command_read, command_write):
                if fd >= 0:
                    try: os.close(fd)
                    except OSError: pass

        self.assertTrue(handled)
        self.assertFalse(worker.is_alive())
        self.assertIn("pin", seen["first"]["runs"][0]["actions"])
        self.assertNotIn("unpin", seen["first"]["runs"][0]["actions"])
        self.assertEqual(seen["result"]["type"], "history_management_result")
        self.assertEqual(seen["result"]["management_id"], "manage-1")
        self.assertEqual(seen["result"]["action"], "pin")
        self.assertEqual(seen["result"]["status"], "PASS")
        self.assertTrue(seen["result"]["history_changed"])
        self.assertEqual(seen["snapshot"]["type"], "history_snapshot")
        self.assertTrue(seen["snapshot"]["runs"][0]["pinned"])
        self.assertNotEqual(seen["first"]["prompt_id"], seen["second"]["prompt_id"])
        self.assertIn("unpin", seen["second"]["runs"][0]["actions"])
        self.assertNotIn("pin", seen["second"]["runs"][0]["actions"])

    @unittest.skipUnless(os.name == "posix", "native History management requires POSIX FDs")
    def test_native_history_export_returns_verified_project_artifact_without_prompt_refresh(self):
        import python_patch_queue_dispatcher as dispatcher
        event_read, event_write = os.pipe()
        command_read, command_write = os.pipe()
        old_event = os.environ.get(entry.EVENT_FD_ENV)
        old_command = os.environ.get(entry.COMMAND_FD_ENV)
        old_native = os.environ.get("TASKDECK_PATCH_NATIVE_HISTORY")
        os.environ[entry.EVENT_FD_ENV] = str(event_write)
        os.environ[entry.COMMAND_FD_ENV] = str(command_read)
        os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = "1"
        seen = {}
        snapshot = {
            "status": "available",
            "runs": [{"run_id": "run-1", "status": "PASS", "pinned": False}],
            "total": 1, "default_run_id": "run-1", "truncated": False,
        }

        with tempfile.TemporaryDirectory(prefix="taskdeck-native-history-export-") as td:
            root = Path(td)

            def export_history(_root, run_id):
                out = dispatcher._artifact_subdir(root, "exports") / f"PTV_RUN_{dispatcher._safe_slug(run_id,64)}.zip"
                out.write_bytes(b"history-export")
                return 0

            def respond():
                nonlocal command_write
                with os.fdopen(os.dup(event_read), "r", encoding="utf-8") as stream:
                    prompt = json.loads(stream.readline())
                    seen["prompt"] = prompt
                    command = {
                        "protocol": "taskdeck.patch", "version": 1, "type": "command", "seq": 1,
                        "command": "history_manage",
                        "payload": {
                            "prompt_id": prompt["prompt_id"],
                            "management_id": "export-1",
                            "action": "export",
                            "run_id": "run-1",
                        },
                    }
                    os.write(command_write, (json.dumps(command) + "\n").encode("utf-8"))
                    seen["result"] = json.loads(stream.readline())
                os.close(command_write)
                command_write = -1

            worker = threading.Thread(target=respond)
            worker.start()
            try:
                with mock.patch.object(dispatcher, "protocol_history_view", return_value=snapshot), \
                     mock.patch.object(dispatcher, "_export_history", side_effect=export_history):
                    handled = dispatcher._protocol_history_detail_session(root)
                worker.join(timeout=2)
            finally:
                if old_event is None: os.environ.pop(entry.EVENT_FD_ENV, None)
                else: os.environ[entry.EVENT_FD_ENV] = old_event
                if old_command is None: os.environ.pop(entry.COMMAND_FD_ENV, None)
                else: os.environ[entry.COMMAND_FD_ENV] = old_command
                if old_native is None: os.environ.pop("TASKDECK_PATCH_NATIVE_HISTORY", None)
                else: os.environ["TASKDECK_PATCH_NATIVE_HISTORY"] = old_native
                for fd in (event_read, event_write, command_read, command_write):
                    if fd >= 0:
                        try: os.close(fd)
                        except OSError: pass

        self.assertTrue(handled)
        self.assertFalse(worker.is_alive())
        result = seen["result"]
        self.assertEqual(result["type"], "history_management_result")
        self.assertEqual(result["action"], "export")
        self.assertEqual(result["status"], "PASS")
        self.assertFalse(result["history_changed"])
        self.assertEqual(result["artifact"]["label"], "History export")
        self.assertTrue(result["artifact"]["path"].startswith("artifacts/"))
        self.assertTrue(result["artifact"]["path"].endswith(".zip"))

    def test_history_projection_is_python_owned_bounded_and_sanitized(self):
        import python_patch_queue_dispatcher as dispatcher
        from python_patch_protocol import build_history_snapshot, build_history_report

        with tempfile.TemporaryDirectory(prefix="taskdeck-history-view-") as td:
            root = Path(td)
            artifact_dir = root / "artifacts" / "patch_tool" / "runs" / "run-1"
            artifact_dir.mkdir(parents=True)
            summary_path = artifact_dir / "SUMMARY.txt"
            log_path = artifact_dir / "batch.log"
            handoff_path = root / "artifacts" / "ptv_to_ai" / "FAIL_HANDOFF.zip"
            handoff_path.parent.mkdir(parents=True)
            summary_path.write_text("summary", encoding="utf-8")
            log_path.write_text("log", encoding="utf-8")
            handoff_path.write_bytes(b"zip")
            report = {
                "run_id": "run-1",
                "status": "FAIL",
                "started_at": "2026-09-23T10:00:00+00:00",
                "elapsed_seconds": 1.25,
                "selected": ["patch.zip"],
                "failure_policy": "continue_independent",
                "transaction_policy": "patch",
                "batch_summary": summary_path.relative_to(root).as_posix(),
                "batch_log": log_path.relative_to(root).as_posix(),
                "results": [{
                    "name": "patch.zip",
                    "kind": "PATCH",
                    "status": "FAIL",
                    "rc": 2,
                    "elapsed_seconds": 1.1,
                    "fail_handoff": handoff_path.relative_to(root).as_posix(),
                    "patch_result": {
                        "diagnosis": {"kind": "build_failed", "message": "internal raw detail"},
                        "project_delta": {"changed_paths": ["a.py"]},
                    },
                    "source_compare": {"diff_path": "private/raw.diff"},
                }],
                "report_warnings": ["warning\nline"],
                "internal_secret": "must-not-leak",
            }
            fake_path = root / "history.json"
            with mock.patch.object(dispatcher, "_visible_history_entries", return_value=[(fake_path, report)]), \
                 mock.patch.object(dispatcher, "_load_pinned_runs", return_value={"run-1"}), \
                 mock.patch.object(dispatcher, "_find_history_entry", return_value=(fake_path, report)):
                snapshot = build_history_snapshot(str(root))
                detail = build_history_report(str(root), "run-1")

        self.assertEqual(snapshot["status"], "available")
        self.assertEqual(snapshot["total"], 1)
        self.assertEqual(snapshot["default_run_id"], "run-1")
        self.assertTrue(snapshot["runs"][0]["pinned"])
        self.assertEqual(snapshot["runs"][0]["counts"]["FAIL"], 1)
        self.assertEqual(detail["status"], "available")
        self.assertEqual(detail["run"]["run_id"], "run-1")
        self.assertEqual(detail["items"][0]["diagnosis"], "build_failed")
        self.assertEqual(detail["items"][0]["changed_count"], 1)
        self.assertEqual(detail["items"][0]["artifacts"][0]["path"], "artifacts/ptv_to_ai/FAIL_HANDOFF.zip")
        self.assertTrue(detail["items"][0]["artifacts"][0]["upload_required"])
        self.assertEqual({row["path"] for row in detail["files"]}, {
            "artifacts/patch_tool/runs/run-1/SUMMARY.txt",
            "artifacts/patch_tool/runs/run-1/batch.log",
        })
        encoded = json.dumps(detail)
        for forbidden in ("patch_result", "source_compare", "internal_secret", str(root)):
            self.assertNotIn(forbidden, encoded)
        self.assertNotIn("\n", detail["warnings"][0])

    def test_history_projection_does_not_materialize_artifacts_on_empty_project(self):
        from python_patch_protocol import build_history_snapshot

        with tempfile.TemporaryDirectory(prefix="taskdeck-history-empty-") as td:
            root = Path(td)
            snapshot = build_history_snapshot(str(root))
            self.assertEqual(snapshot["status"], "empty")
            self.assertEqual(snapshot["runs"], [])
            self.assertFalse((root / "artifacts").exists())

    def test_history_report_projection_rejects_unknown_run(self):
        from python_patch_protocol import build_history_report

        with tempfile.TemporaryDirectory(prefix="taskdeck-history-missing-") as td:
            detail = build_history_report(td, "../not-a-run")
        self.assertEqual(detail["status"], "not_found")
        self.assertNotIn("items", detail)

    def test_protocol_supervisor_emits_history_snapshot_before_report_run(self):
        with tempfile.TemporaryDirectory() as tmp:
            child = Path(tmp) / "child.py"
            child.write_text("raise SystemExit(0)\n", encoding="utf-8")
            read_fd, write_fd = os.pipe()
            try:
                from python_patch_protocol import EventWriter
                writer = EventWriter(write_fd)
                rc = entry._run_with_protocol(writer, [str(child)], tmp, ["report"])
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
        self.assertEqual([event["type"] for event in events], ["hello", "history_snapshot", "run_started", "run_finished"])
        self.assertEqual(events[1]["status"], "empty")
        self.assertFalse((Path(tmp) / "artifacts").exists())

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


class CollectProgressRelayTests(unittest.TestCase):
    @unittest.skipUnless(os.name == "posix", "FD relay integration is Unix-only")
    def test_nested_collect_supervisor_emits_live_progress_through_entry_relay(self):
        base = Path(__file__).resolve().parent
        entry._prepare_environment(base)
        progress = base / "_patch_lib" / "python_patch_collect_progress_v6_7.py"

        with tempfile.TemporaryDirectory(prefix="taskdeck-collect-progress-") as td:
            root = Path(td)
            collector = root / "collector.py"
            result_zip = root / "artifacts" / "collect-live.zip"
            collector.write_text(
                "import json,time,zipfile\n"
                "from pathlib import Path\n"
                f"out=Path({str(result_zip)!r})\n"
                "print('search candidates', flush=True)\n"
                "time.sleep(0.45)\n"
                "print('compressing ZIP archive', flush=True)\n"
                "time.sleep(0.45)\n"
                "out.parent.mkdir(parents=True, exist_ok=True)\n"
                "with zipfile.ZipFile(out, 'w') as zf:\n"
                "    zf.writestr('COLLECTION_MANIFEST.json', json.dumps({'file_count':0,'files':[]}))\n"
                "print(f'ZIP : {out}', flush=True)\n",
                encoding="utf-8",
            )

            bridge = root / "bridge.py"
            bridge.write_text(
                "import os,sys\n"
                "from pathlib import Path\n"
                "from python_patch_queue_dispatcher import _run_foreground_child\n"
                f"root=Path({str(root)!r})\n"
                f"progress=Path({str(progress)!r})\n"
                f"collector=Path({str(collector)!r})\n"
                "env=dict(os.environ)\n"
                "env['PTV_COLLECT_HEARTBEAT_SECONDS']='0.2'\n"
                "env['TASKDECK_PATCH_PROGRESS_RUN_ID']='run-live'\n"
                "env['TASKDECK_PATCH_PROGRESS_INDEX']='1'\n"
                "env['TASKDECK_PATCH_PROGRESS_TOTAL']='1'\n"
                "env['TASKDECK_PATCH_PROGRESS_ITEM_NAME']='request.zip'\n"
                "env['TASKDECK_PATCH_PROGRESS_ITEM_KIND']='COLLECT'\n"
                "cmd=[sys.executable,str(progress),'--project-root',str(root),'--collector',str(collector),'--','request','request.zip']\n"
                "raise SystemExit(_run_foreground_child(root,cmd,env=env,timeout=None,label='COLLECT'))\n",
                encoding="utf-8",
            )

            read_fd, write_fd = os.pipe()
            try:
                from python_patch_protocol import EventWriter
                writer = EventWriter(write_fd)
                rc = entry._run_with_protocol(writer, [str(bridge)], str(root), ["run"])
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
        progress_events = [event for event in events if event.get("type") == "progress"]
        running = [event for event in progress_events if event.get("status") == "RUNNING"]
        self.assertEqual(rc, 0)
        self.assertGreaterEqual(len(running), 2, progress_events)
        self.assertTrue(any(float(event.get("elapsed_seconds", 0)) > 0 for event in running))
        self.assertTrue(any(event.get("phase") in {"search", "zip"} for event in running), progress_events)
        self.assertTrue(any(event.get("status") == "PASS" for event in progress_events), progress_events)
        self.assertEqual([event["seq"] for event in events], list(range(1, len(events) + 1)))


    @unittest.skipUnless(os.name == "posix", "FD relay integration is Unix-only")
    def test_direct_collect_progress_emits_item_and_artifact_events(self):
        base = Path(__file__).resolve().parent
        entry._prepare_environment(base)
        progress = base / "_patch_lib" / "python_patch_collect_progress_v6_7.py"
        with tempfile.TemporaryDirectory(prefix="taskdeck-direct-collect-") as td:
            root = Path(td)
            collector = root / "collector.py"
            result_zip = root / "artifacts" / "direct-result.zip"
            result_txt = root / "artifacts" / "direct-result.txt"
            collector.write_text(
                "import json,zipfile\n"
                "from pathlib import Path\n"
                f"z=Path({str(result_zip)!r}); t=Path({str(result_txt)!r})\n"
                "z.parent.mkdir(parents=True,exist_ok=True)\n"
                "with zipfile.ZipFile(z,'w') as zf: zf.writestr('COLLECTION_MANIFEST.json',json.dumps({'file_count':0,'files':[]}))\n"
                "t.write_text('result',encoding='utf-8')\n"
                "print(f'ZIP : {z}',flush=True)\n",
                encoding="utf-8",
            )
            read_fd, write_fd = os.pipe()
            old = dict(os.environ)
            try:
                os.environ["TASKDECK_PATCH_EVENT_FD"] = str(write_fd)
                os.environ["TASKDECK_PATCH_DIRECT_COLLECT"] = "1"
                os.environ["TASKDECK_PATCH_PROGRESS_INDEX"] = "1"
                os.environ["TASKDECK_PATCH_PROGRESS_TOTAL"] = "1"
                os.environ["TASKDECK_PATCH_PROGRESS_ITEM_NAME"] = "CODE_COLLECTION_REQUEST_demo.zip"
                os.environ["TASKDECK_PATCH_PROGRESS_ITEM_KIND"] = "COLLECT"
                cp = subprocess.run([
                    sys.executable,str(progress),"--project-root",str(root),"--collector",str(collector),"--","request","dummy.zip"
                ],env=dict(os.environ),stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,timeout=10,pass_fds=(write_fd,))
                os.close(write_fd); write_fd=-1
                chunks=[]
                while True:
                    data=os.read(read_fd,65536)
                    if not data: break
                    chunks.append(data)
            finally:
                os.environ.clear(); os.environ.update(old)
                if write_fd>=0: os.close(write_fd)
                os.close(read_fd)
            events=[json.loads(line) for line in b"".join(chunks).decode("utf-8").splitlines()]
        self.assertEqual(cp.returncode,0,(cp.stdout,cp.stderr))
        types=[event["type"] for event in events]
        self.assertIn("item_started",types)
        self.assertIn("item_finished",types)
        artifacts=[event for event in events if event["type"]=="artifact"]
        self.assertEqual({row["artifact_kind"] for row in artifacts},{"collect_result_zip","collect_result_text"})
        self.assertTrue(all(row["item_name"]=="CODE_COLLECTION_REQUEST_demo.zip" for row in artifacts))


if __name__ == "__main__":
    unittest.main()
