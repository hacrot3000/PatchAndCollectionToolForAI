#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import time
from typing import Any

PROTOCOL_NAME = "taskdeck.patch"
PROTOCOL_VERSION = 1
MAX_COMMAND_BYTES = 1 << 20


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

    def emit(self, event_type: str, **payload: Any) -> bool:
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

    def close(self) -> None:
        try:
            self._stream.close()
        except OSError:
            pass


def build_queue_snapshot(project_root: str) -> dict[str, Any]:
    """Return the stable protocol view of the authoritative Python queue.

    Queue discovery remains owned by python_patch_queue_dispatcher. This bridge
    intentionally exposes only protocol fields and does not leak internal report
    or history schemas to TaskDeck.
    """
    from pathlib import Path
    from python_patch_queue_dispatcher import discover_queue

    root = Path(project_root).expanduser().resolve()
    items, warnings = discover_queue(root)
    rows = [
        {
            "name": str(item.name),
            "kind": str(item.kind),
            "detail": str(item.detail or ""),
        }
        for item in items
    ]
    counts: dict[str, int] = {}
    for row in rows:
        kind = row["kind"]
        counts[kind] = counts.get(kind, 0) + 1
    return {
        "status": "runnable" if rows else "empty",
        "items": rows,
        "warnings": [str(value) for value in warnings],
        "counts": counts,
        "total": len(rows),
    }


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
