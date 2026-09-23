#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import threading
import time
from typing import Any

PROTOCOL_NAME = "taskdeck.patch"
PROTOCOL_VERSION = 1
MAX_COMMAND_BYTES = 1 << 20
MAX_EVENT_BYTES = 1 << 20
EVENT_FD_ENV = "TASKDECK_PATCH_EVENT_FD"
COMMAND_FD_ENV = "TASKDECK_PATCH_COMMAND_FD"


class ProtocolCommandError(RuntimeError):
    pass


class CommandReader:
    """Bounded JSONL command reader for the future native TaskDeck frontend.

    This class is intentionally unused by the current dispatcher. Defining the
    transport separately from stdin preserves the PTY interaction contract until
    prompt events and command semantics are explicitly introduced.
    """

    def __init__(self, fd: int):
        if not isinstance(fd, int) or fd < 3:
            raise ValueError("command fd must be an integer >= 3")
        self.fd = fd
        self._stream = os.fdopen(os.dup(fd), "r", encoding="utf-8", buffering=1)

    def read(self) -> dict[str, Any] | None:
        raw = self._stream.readline(MAX_COMMAND_BYTES + 1)
        if raw == "":
            return None
        if len(raw.encode("utf-8")) > MAX_COMMAND_BYTES:
            raise ProtocolCommandError("Patch protocol command exceeds size limit")
        if not raw.endswith("\n"):
            raise ProtocolCommandError("Patch protocol command must be one complete JSONL record")
        try:
            value = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise ProtocolCommandError(f"invalid Patch protocol command JSON: {exc}") from exc
        if not isinstance(value, dict):
            raise ProtocolCommandError("Patch protocol command must be a JSON object")
        if value.get("protocol") != PROTOCOL_NAME or value.get("version") != PROTOCOL_VERSION:
            raise ProtocolCommandError("unsupported Patch protocol command envelope")
        if value.get("type") != "command":
            raise ProtocolCommandError("Patch protocol command type must be 'command'")
        if not isinstance(value.get("seq"), int) or int(value["seq"]) < 1:
            raise ProtocolCommandError("Patch protocol command seq must be a positive integer")
        command = value.get("command")
        if not isinstance(command, str) or not command.strip():
            raise ProtocolCommandError("Patch protocol command name is required")
        return value

    def close(self) -> None:
        try:
            self._stream.close()
        except OSError:
            pass


class EventWriter:
    """Best-effort JSONL writer for the TaskDeck/Patch Tool machine channel.

    The writer duplicates the supplied FD so the original descriptor can be
    inherited by the Patch Tool child. Machine-channel failure must never turn
    a valid Patch Tool run into a failed run.
    """

    def __init__(self, fd: int):
        if not isinstance(fd, int) or fd < 3:
            raise ValueError("event fd must be an integer >= 3")
        self.fd = fd
        self._stream = os.fdopen(os.dup(fd), "w", encoding="utf-8", buffering=1)
        self._seq = 0
        self._disabled = False
        self._lock = threading.Lock()

    def emit(self, event_type: str, **payload: Any) -> bool:
        with self._lock:
            if self._disabled:
                return False
            self._seq += 1
            event = {
                "protocol": PROTOCOL_NAME,
                "version": PROTOCOL_VERSION,
                "type": str(event_type),
                "seq": self._seq,
                "time_ms": int(time.time() * 1000),
                **payload,
            }
            try:
                self._stream.write(json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n")
                self._stream.flush()
                return True
            except (BrokenPipeError, OSError, ValueError):
                self._disabled = True
                return False

    def forward(self, event: dict[str, Any]) -> bool:
        if not isinstance(event, dict):
            return False
        event_type = event.get("type")
        if not isinstance(event_type, str) or not event_type:
            return False
        payload = {
            key: value
            for key, value in event.items()
            if key not in {"protocol", "version", "type", "seq", "time_ms"}
        }
        return self.emit(event_type, **payload)

    def close(self) -> None:
        try:
            self._stream.close()
        except OSError:
            pass


def event_writer_from_env() -> EventWriter | None:
    """Open the child event channel when available, otherwise fail open.

    Runtime events are observational only. A missing/malformed machine channel
    must never change PATCH/COLLECT execution semantics or terminal behavior.
    """
    raw = os.environ.get(EVENT_FD_ENV, "").strip()
    if not raw:
        return None
    try:
        fd = int(raw, 10)
        if fd < 3:
            return None
        os.fstat(fd)
        return EventWriter(fd)
    except (OSError, ValueError):
        return None


def emit_runtime_event(event_type: str, **payload: Any) -> bool:
    writer = event_writer_from_env()
    if writer is None:
        return False
    try:
        return writer.emit(event_type, **payload)
    except Exception:
        return False
    finally:
        writer.close()


def prompt_channels_from_env() -> tuple[EventWriter | None, CommandReader | None]:
    """Open child prompt channels only when both machine FDs are present.

    Event-only sessions are deliberately ignored here so today's PTY selector
    remains authoritative until TaskDeck explicitly enables protocol commands.
    """
    event_raw = os.environ.get(EVENT_FD_ENV, "").strip()
    command_raw = os.environ.get(COMMAND_FD_ENV, "").strip()
    if not event_raw or not command_raw:
        return None, None
    try:
        event_fd = int(event_raw, 10)
        command_fd = int(command_raw, 10)
    except ValueError as exc:
        raise ProtocolCommandError("Patch prompt channels require integer file descriptors") from exc
    if event_fd < 3 or command_fd < 3 or event_fd == command_fd:
        raise ProtocolCommandError("Patch prompt channels require distinct file descriptors >= 3")
    try:
        os.fstat(event_fd)
        os.fstat(command_fd)
    except OSError as exc:
        raise ProtocolCommandError(f"Patch prompt channel is unavailable: {exc}") from exc
    return EventWriter(event_fd), CommandReader(command_fd)


def request_prompt(
    writer: EventWriter,
    reader: CommandReader,
    prompt_kind: str,
    **payload: Any,
) -> dict[str, Any]:
    prompt_id = os.urandom(12).hex()
    writer.emit(
        "prompt",
        prompt_id=prompt_id,
        prompt_kind=str(prompt_kind),
        **payload,
    )
    command = reader.read()
    if command is None:
        raise ProtocolCommandError("Patch protocol command channel closed while waiting for prompt response")
    if command.get("command") != "prompt_response":
        raise ProtocolCommandError("Patch prompt requires a prompt_response command")
    response = command.get("payload")
    if not isinstance(response, dict):
        raise ProtocolCommandError("Patch prompt_response payload must be an object")
    if response.get("prompt_id") != prompt_id:
        raise ProtocolCommandError("Patch prompt_response prompt_id does not match the active prompt")
    return response


def relay_event_fd(fd: int, writer: EventWriter) -> None:
    """Forward child JSONL events through the entrypoint-owned sequence."""
    try:
        with os.fdopen(fd, "r", encoding="utf-8", buffering=1) as stream:
            while True:
                raw = stream.readline(MAX_EVENT_BYTES + 1)
                if raw == "":
                    return
                if len(raw.encode("utf-8")) > MAX_EVENT_BYTES or not raw.endswith("\n"):
                    writer.emit("error", phase="child_event_relay", message="child event exceeds JSONL size/record boundary")
                    return
                try:
                    event = json.loads(raw)
                except json.JSONDecodeError as exc:
                    writer.emit("error", phase="child_event_relay", message=f"invalid child event JSON: {exc}")
                    continue
                if not isinstance(event, dict):
                    writer.emit("error", phase="child_event_relay", message="child event must be a JSON object")
                    continue
                if event.get("protocol") != PROTOCOL_NAME or event.get("version") != PROTOCOL_VERSION:
                    writer.emit("error", phase="child_event_relay", message="unsupported child event envelope")
                    continue
                writer.forward(event)
    except OSError as exc:
        writer.emit("error", phase="child_event_relay", message=f"child event pipe failed: {exc}")


def build_queue_snapshot(project_root: str) -> dict[str, Any]:
    """Return the stable protocol view of the authoritative Python queue.

    Queue discovery remains owned by python_patch_queue_dispatcher. This bridge
    intentionally exposes only protocol fields and does not leak internal report
    or history schemas to TaskDeck.
    """
    from pathlib import Path
    from python_patch_queue_dispatcher import protocol_queue_view

    root = Path(project_root).expanduser().resolve()
    return protocol_queue_view(root)


def emit_queue_snapshot(writer: EventWriter, project_root: str) -> bool:
    try:
        snapshot = build_queue_snapshot(project_root)
    except Exception as exc:
        writer.emit(
            "error",
            phase="queue_snapshot",
            message=f"{type(exc).__name__}: {exc}",
        )
        return False
    writer.emit("queue_snapshot", **snapshot)
    return True
