#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sys
import tempfile
import time

from python_patch_collect_compat import _search_action_payload

VERSION = "6.19.3"


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _atomic_text(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix=".ptv-regex-result-", suffix=".tmp", dir=path.parent)
    temp = Path(name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as out:
            out.write(text)
            out.flush()
            try:
                os.fsync(out.fileno())
            except OSError:
                pass
        os.replace(temp, path)
    finally:
        try:
            temp.unlink()
        except FileNotFoundError:
            pass


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--project-root", required=True)
    ap.add_argument("--request", required=True)
    ap.add_argument("--result", required=True)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve(strict=True)
    req = Path(ns.request)
    result = Path(ns.result)
    try:
        data = json.loads(req.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        if not isinstance(data, dict) or not isinstance(data.get("action"), dict) or not isinstance(data.get("limits"), dict):
            raise ValueError("invalid regex worker request")
        soft_timeout = float(data.get("soft_timeout_seconds", 0) or 0)
        deadline = time.monotonic() + soft_timeout if soft_timeout > 0 else None
        def checkpoint(payload):
            _atomic_text(result, json.dumps(payload, ensure_ascii=False))
        payload = _search_action_payload(root, data["action"], data["limits"], deadline=deadline, checkpoint_cb=checkpoint)
        _atomic_text(result, json.dumps(payload, ensure_ascii=False))
        return 0
    except Exception as exc:
        print(f"{type(exc).__name__}: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
