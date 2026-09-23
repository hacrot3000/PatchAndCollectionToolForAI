#!/usr/bin/env python3
from __future__ import annotations

import json
import os
import time
from typing import Any

PROTOCOL_NAME = "taskdeck.patch"
PROTOCOL_VERSION = 1


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
