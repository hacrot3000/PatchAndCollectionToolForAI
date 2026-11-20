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

try:
    from python_patch_version import VERSION
except ImportError:
    # Standalone compatibility for historical/minimal COLLECT module sets.
    import json as _ptv_version_json
    from pathlib import Path as _PTVVersionPath
    try:
        VERSION = str(_ptv_version_json.loads((_PTVVersionPath(__file__).resolve().parent / "docs" / "COLLECT_ACTION_SCHEMA.json").read_text(encoding="utf-8")).get("tool_version") or "unknown")
    except Exception:
        VERSION = "unknown"


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


def _bounded_payload_json(payload: dict, limits: dict) -> str:
    """Keep worker IPC bounded while preserving useful PARTIAL evidence.

    Search payload growth can come from the rendered report, match details,
    coverage diagnostics, candidate filenames, skip lists, or combinations of
    them.  Build a compact contract when the raw payload crosses the IPC cap
    instead of trimming only one field and leaving a different large field for
    the parent to reject.
    """
    hard = max(int(limits.get("max_report_bytes", 0) or 0), 1024 * 1024) * 2

    def dump(value: dict) -> str:
        return json.dumps(value, ensure_ascii=False)

    def utf8_prefix(value: object, max_bytes: int) -> str:
        raw = str(value or "").encode("utf-8", errors="replace")
        if len(raw) <= max_bytes:
            return raw.decode("utf-8", errors="replace")
        return raw[:max_bytes].decode("utf-8", errors="ignore")

    def compact_rows(value, *, limit: int):
        if not isinstance(value, list):
            return []
        out=[]
        for row in value[:limit]:
            if isinstance(row, dict):
                clean={}
                for key in ("path", "reason", "name", "kind"):
                    if key in row:
                        clean[key]=utf8_prefix(row.get(key), 512)
                out.append(clean or {"summary": utf8_prefix(row, 512)})
            elif isinstance(row, (list, tuple)):
                out.append([utf8_prefix(x, 512) for x in row[:4]])
            else:
                out.append(utf8_prefix(row, 512))
        return out

    def compact_coverage(value) -> dict:
        if not isinstance(value, dict):
            return {"worker_result_truncated": True}
        out={"worker_result_truncated": True}
        for key in (
            "directories_visited", "files_considered", "limit_reached",
            "time_limit_reached", "skipped_dirs_count", "skipped_files_count",
        ):
            if key in value:
                out[key]=value[key]
        ext=value.get("extension_counts")
        if isinstance(ext, dict):
            out["extension_counts"]={str(k): v for k,v in list(ext.items())[:50]}
        modules=value.get("modules")
        if isinstance(modules, list):
            out["modules"]=[utf8_prefix(x, 512) for x in modules[:80]]
        candidates=value.get("candidate_filenames")
        if isinstance(candidates, list):
            out["candidate_filenames"]=[utf8_prefix(x, 512) for x in candidates[:100]]
            out["candidate_filenames_total_before_worker_cap"]=len(candidates)
        out["errors"]=compact_rows(value.get("errors"), limit=8)
        out["skipped_dirs"]=compact_rows(value.get("skipped_dirs"), limit=40)
        out["skipped_files"]=compact_rows(value.get("skipped_files"), limit=40)
        return out

    raw = dump(payload)
    original_bytes = len(raw.encode("utf-8"))
    if original_bytes <= hard:
        return raw

    details=list(payload.get("match_details") or [])
    cap_reason=(
        f"regex search worker result exceeded safety cap ({original_bytes} bytes); "
        "bounded partial evidence preserved"
    )
    reasons=[]
    for value in list(payload.get("reasons") or []) + [cap_reason]:
        text=utf8_prefix(value, 1200)
        if text and text not in reasons:
            reasons.append(text)
        if len(reasons) >= 32:
            break

    report_budget=min(512 * 1024, max(4096, hard // 4))
    report=utf8_prefix(payload.get("report"), report_budget)
    if report:
        report += "\n\n[PTV: report truncated/bounded to keep regex worker IPC within safety cap]\n"

    out={
        "report": report,
        "incomplete": True,
        "inconsistency": bool(payload.get("inconsistency")),
        "must_find_failed": bool(payload.get("must_find_failed")),
        "matches": payload.get("matches", len(details)),
        "match_details": [],
        "coverage": compact_coverage(payload.get("coverage")),
        "coverage_status": "INCONSISTENT" if payload.get("coverage_status") == "INCONSISTENT" else "PARTIAL",
        "execution_status": "INCONSISTENT" if payload.get("execution_status") == "INCONSISTENT" else "PARTIAL",
        "reasons": reasons,
        "worker_result_truncated": True,
        "worker_result_original_bytes": original_bytes,
        "match_details_total_before_worker_cap": len(details),
    }

    # Preserve the largest safe prefix of detailed matches.  Reserve headroom
    # for the bookkeeping fields added after the search.
    lo=0; hi=min(len(details), 512); best=0
    target=max(0, hard - 4096)
    while lo <= hi:
        mid=(lo + hi) // 2
        out["match_details"]=details[:mid]
        size=len(dump(out).encode("utf-8"))
        if size <= target:
            best=mid; lo=mid + 1
        else:
            hi=mid - 1
    out["match_details"]=details[:best]
    out["match_details_preserved_after_worker_cap"]=best
    raw=dump(out)

    # Emergency minimum is still a useful PARTIAL contract: total match count,
    # reason, and coverage counters survive even if one context row is huge.
    if len(raw.encode("utf-8")) > hard:
        out["match_details"]=[]
        out["match_details_preserved_after_worker_cap"]=0
        out["report"]=utf8_prefix(payload.get("report"), 4096) + "\n[PTV: report heavily truncated by worker safety cap]\n"
        out["coverage"]={"worker_result_truncated": True}
        raw=dump(out)
    if len(raw.encode("utf-8")) > hard:
        raise ValueError(
            f"cannot bound regex worker partial payload below safety cap "
            f"({len(raw.encode('utf-8'))} > {hard})"
        )
    return raw


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
            _atomic_text(result, _bounded_payload_json(payload, data["limits"]))
        payload = _search_action_payload(root, data["action"], data["limits"], deadline=deadline, checkpoint_cb=checkpoint)
        _atomic_text(result, _bounded_payload_json(payload, data["limits"]))
        return 0
    except Exception as exc:
        print(f"{type(exc).__name__}: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
