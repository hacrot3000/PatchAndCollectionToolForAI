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
