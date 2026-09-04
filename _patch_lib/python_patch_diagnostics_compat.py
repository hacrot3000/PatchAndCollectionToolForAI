#!/usr/bin/env python3
"""Additive compatibility diagnostics for historical v5 COMPLETE capabilities.

This module never replaces the current exact FAIL_HANDOFF evidence.  It creates
safe/redacted derivative evidence so old diagnostics/redaction capabilities can
coexist with the v6 exact-evidence contract.
"""
from __future__ import annotations

import hashlib
import json
import platform
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

VERSION = "6.18.4"

_PRIVATE_KEY = re.compile(
    r"-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z0-9 ]+ )?PRIVATE KEY-----",
    re.I | re.S,
)
_AUTH = re.compile(r"(?im)^(\s*authorization\s*:\s*)(?:bearer|basic)\s+\S+\s*$")
_COOKIE = re.compile(r"(?im)^(\s*(?:set-)?cookie\s*:\s*).+$")
_ASSIGN = re.compile(
    r"(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|secret|password|passwd|credential)\b(\s*[:=]\s*)([^\s,;]+)"
)
_JWT = re.compile(r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b")
_GITHUB = re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b")
_URL_CREDS = re.compile(r"(?i)\b(https?://)([^\s/@:]+):([^\s/@]+)@")

_DIAG_PATTERNS = [
    ("compiler", re.compile(r"^(?P<file>[^:\n]+\.(?:c|cc|cpp|cxx|h|hpp|m|mm|rs|go|ts|js|java|kt|py)):(?P<line>\d+)(?::(?P<col>\d+))?:\s*(?P<level>fatal error|error|warning|note):\s*(?P<msg>.+)$", re.I)),
    ("maven", re.compile(r"^\[ERROR\]\s+(?P<file>[^:\[]+):\[(?P<line>\d+),(?P<col>\d+)\]\s*(?P<msg>.+)$", re.I)),
    ("msvc", re.compile(r"^(?P<file>.+?)\((?P<line>\d+)(?:,(?P<col>\d+))?\):\s*(?P<level>fatal error|error|warning)\s+[A-Z]+\d+:\s*(?P<msg>.+)$", re.I)),
]
_PY_FILE = re.compile(r'^\s*File "(?P<file>[^"]+)", line (?P<line>\d+)(?:, in (?P<where>.*))?$')
_ERRORISH = re.compile(r"(?i)\b(error|failed|failure|fatal|exception|traceback|assertionerror|syntaxerror|typeerror|valueerror|segmentation fault|undefined reference)\b")
_WARNINGISH = re.compile(r"(?i)\bwarning\b")
_NOISE = re.compile(r"(?i)^(?:\s*(?:download(?:ing)?|progress|building|compiling|linking)\b|\s*\[\d+%\]|\s*\d+%\s*)")


def redact_text(text: str) -> tuple[str, dict[str, int]]:
    counts = {"private_key": 0, "authorization": 0, "cookie": 0, "credential_assignment": 0, "token": 0, "url_credentials": 0}

    def sub_count(pattern: re.Pattern[str], repl, key: str, value: str) -> str:
        def inner(m: re.Match[str]) -> str:
            counts[key] += 1
            return repl(m) if callable(repl) else repl
        return pattern.sub(inner, value)

    out = sub_count(_PRIVATE_KEY, "[REDACTED_PRIVATE_KEY]", "private_key", text)
    out = sub_count(_AUTH, lambda m: m.group(1) + "[REDACTED_AUTHORIZATION]", "authorization", out)
    out = sub_count(_COOKIE, lambda m: m.group(1) + "[REDACTED_COOKIE]", "cookie", out)
    out = sub_count(_ASSIGN, lambda m: m.group(1) + m.group(2) + "[REDACTED_CREDENTIAL]", "credential_assignment", out)
    out = sub_count(_JWT, "[REDACTED_TOKEN]", "token", out)
    out = sub_count(_GITHUB, "[REDACTED_TOKEN]", "token", out)
    out = sub_count(_URL_CREDS, lambda m: m.group(1) + "[REDACTED]@", "url_credentials", out)
    return out, counts


def _suggest(kind: str, level: str, msg: str) -> str | None:
    low = msg.lower()
    if "syntax" in low or "expected" in low or "unexpected" in low:
        return "Inspect the reported line/column and the immediately preceding delimiter/statement."
    if "undefined reference" in low or "cannot find symbol" in low:
        return "Check the first missing symbol and its declaration/link dependency before downstream errors."
    if "no such file" in low or "cannot find" in low:
        return "Verify the reported path/module exists in the effective build/search scope."
    if kind == "python" or "traceback" in low:
        return "Read the final exception first, then inspect the nearest project frame above it."
    if level == "error":
        return "Treat this as a primary candidate; later errors may be cascades."
    return None


def normalize_diagnostics(text: str, *, max_items: int = 300) -> list[dict[str, Any]]:
    lines = text.splitlines()
    out: list[dict[str, Any]] = []
    last_py: tuple[str, int] | None = None
    for idx, raw in enumerate(lines):
        line = raw.strip("\r")
        py = _PY_FILE.match(line)
        if py:
            last_py = (py.group("file"), int(py.group("line")))
            continue
        matched = False
        for kind, pat in _DIAG_PATTERNS:
            m = pat.match(line)
            if not m:
                continue
            gd = m.groupdict()
            level = str(gd.get("level") or "error").lower()
            msg = str(gd.get("msg") or line).strip()
            row: dict[str, Any] = {
                "kind": kind,
                "level": level,
                "message": msg,
                "file": gd.get("file"),
                "line": int(gd["line"]) if gd.get("line") else None,
                "column": int(gd["col"]) if gd.get("col") else None,
                "log_line": idx + 1,
            }
            hint = _suggest(kind, level, msg)
            if hint:
                row["hint"] = hint
            out.append(row)
            matched = True
            break
        if matched:
            if len(out) >= max_items: break
            continue
        if _ERRORISH.search(line):
            kind = "python" if last_py and ("Error" in line or "Exception" in line) else "runtime"
            row = {
                "kind": kind,
                "level": "error",
                "message": line.strip()[:2000],
                "file": last_py[0] if kind == "python" and last_py else None,
                "line": last_py[1] if kind == "python" and last_py else None,
                "column": None,
                "log_line": idx + 1,
            }
            hint = _suggest(kind, "error", line)
            if hint: row["hint"] = hint
            out.append(row)
        elif _WARNINGISH.search(line) and len(out) < max_items:
            out.append({"kind": "runtime", "level": "warning", "message": line.strip()[:2000], "file": None, "line": None, "column": None, "log_line": idx + 1})
        if len(out) >= max_items:
            break
    return out


def _fingerprint_message(message: str) -> str:
    value = message.lower()
    value = re.sub(r"[A-Za-z]:[/\\][^\s:]+|/(?:[^\s/:]+/)+[^\s:]+", "<path>", value)
    value = re.sub(r"0x[0-9a-f]+|\b\d+\b", "<n>", value)
    value = re.sub(r"\s+", " ", value).strip()
    return hashlib.sha256(value.encode("utf-8", errors="replace")).hexdigest()[:16]


def cluster_diagnostics(items: list[dict[str, Any]], *, max_primary: int = 40) -> dict[str, Any]:
    groups: dict[str, dict[str, Any]] = {}
    for item in items:
        fp = _fingerprint_message(str(item.get("message") or ""))
        group = groups.setdefault(fp, {"fingerprint": fp, "count": 0, "primary": item, "examples": []})
        group["count"] += 1
        if len(group["examples"]) < 3:
            group["examples"].append(item)
    ordered = sorted(groups.values(), key=lambda g: (0 if str(g["primary"].get("level")) == "error" else 1, int(g["primary"].get("log_line") or 10**9)))
    return {"primary": ordered[:max_primary], "suppressed_cascade_count": max(0, len(items) - len(ordered)), "unique_clusters": len(ordered)}


def smart_filter(text: str, *, context: int = 2, max_lines: int = 500) -> str:
    lines = text.splitlines()
    keep: set[int] = set()
    for i, line in enumerate(lines):
        if _ERRORISH.search(line) or _WARNINGISH.search(line):
            for j in range(max(0, i-context), min(len(lines), i+context+1)):
                keep.add(j)
    if not keep:
        for i, line in enumerate(lines[-120:]):
            if not _NOISE.match(line):
                keep.add(max(0, len(lines)-120) + i)
    selected = sorted(keep)
    truncated = len(selected) > max_lines
    if truncated:
        head = max_lines // 2
        selected = selected[:head] + selected[-(max_lines-head):]
    rendered: list[str] = []
    previous: int | None = None
    for i in selected:
        if previous is not None and i > previous + 1:
            rendered.append("... [non-diagnostic lines omitted] ...")
        rendered.append(f"{i+1:06d}: {lines[i]}")
        previous = i
    if truncated:
        rendered.append("... [smart-filter output truncated] ...")
    return "\n".join(rendered) + ("\n" if rendered else "")


def environment_fingerprint() -> dict[str, Any]:
    git_version = None
    try:
        cp = subprocess.run(["git", "--version"], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=3)
        if cp.returncode == 0:
            git_version = cp.stdout.strip()[:200]
    except Exception:
        pass
    return {
        "tool_version": VERSION,
        "python": platform.python_version(),
        "implementation": platform.python_implementation(),
        "os": platform.system(),
        "os_release": platform.release(),
        "machine": platform.machine(),
        "git": git_version,
    }


def _previous_failure_signature(root: Path) -> dict[str, Any] | None:
    history = root / "artifacts" / "patch_tool" / "history"
    if not history.is_dir():
        return None
    for path in sorted(history.glob("*.json"), key=lambda p: p.stat().st_mtime_ns if p.exists() else 0, reverse=True):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        status = str(data.get("status") or "").upper()
        if status not in {"FAIL", "PREFLIGHT_FAIL", "INCOMPLETE"}:
            continue
        failed = data.get("failed_item") or data.get("failed_patch")
        details = data.get("execution_details") or data.get("executions") or []
        diagnosis_kind = None
        if isinstance(details, list):
            for row in details:
                if isinstance(row, dict) and str(row.get("status") or "").upper() in {"FAIL", "PREFLIGHT_FAIL"}:
                    diag = row.get("diagnosis")
                    if isinstance(diag, dict): diagnosis_kind = diag.get("kind")
                    failed = failed or row.get("name")
                    break
        return {"status": status, "failed_item": failed, "diagnosis_kind": diagnosis_kind, "history_file": path.name}
    return None


def failure_delta(root: Path, patch_name: str, diagnosis: dict[str, Any] | None, clusters: dict[str, Any]) -> dict[str, Any]:
    current = {
        "failed_item": patch_name,
        "diagnosis_kind": (diagnosis or {}).get("kind"),
        "cluster_fingerprints": [str(x.get("fingerprint")) for x in clusters.get("primary", []) if isinstance(x, dict)][:40],
    }
    previous = _previous_failure_signature(root)
    if previous is None:
        return {"status": "NO_BASELINE", "current": current, "previous": None}
    same_kind = previous.get("diagnosis_kind") == current.get("diagnosis_kind") and current.get("diagnosis_kind") is not None
    same_item = previous.get("failed_item") == current.get("failed_item") and current.get("failed_item") is not None
    return {"status": "SAME_FAILURE_CLASS" if same_kind and same_item else "CHANGED", "current": current, "previous": previous}


def build_compat_evidence(
    root: Path, *, patch_name: str, exact_log: str, diagnosis: dict[str, Any] | None,
    detail_truncated: bool, source_count: int,
) -> dict[str, Any]:
    redacted, redaction_counts = redact_text(exact_log)
    diagnostics = normalize_diagnostics(redacted)
    clusters = cluster_diagnostics(diagnostics)
    filtered = smart_filter(redacted)
    delta = failure_delta(root, patch_name, diagnosis, clusters)
    quality = {
        "exact_log_bytes": len(exact_log.encode("utf-8", errors="replace")),
        "redacted_log_bytes": len(redacted.encode("utf-8", errors="replace")),
        "redactions": redaction_counts,
        "redaction_total": sum(redaction_counts.values()),
        "detail_log_truncated": bool(detail_truncated),
        "context_completeness": "BOUNDED_DETAIL" if detail_truncated else "FULL_CAPTURED_DETAIL",
        "normalized_diagnostics": len(diagnostics),
        "unique_root_cause_clusters": int(clusters.get("unique_clusters") or 0),
        "suppressed_cascade_count": int(clusters.get("suppressed_cascade_count") or 0),
        "source_attachments": int(source_count),
        "smart_log_lines": len(filtered.splitlines()),
    }
    summary_lines = [
        "# AI diagnostic summary",
        "",
        f"- Patch: `{patch_name}`",
        f"- Diagnosis kind: `{(diagnosis or {}).get('kind') or 'unknown'}`",
        f"- Normalized diagnostics: **{len(diagnostics)}**",
        f"- Root-cause clusters: **{quality['unique_root_cause_clusters']}**",
        f"- Suppressed cascade diagnostics: **{quality['suppressed_cascade_count']}**",
        f"- Redactions in compatibility evidence: **{quality['redaction_total']}**",
        f"- Failure delta: **{delta.get('status')}**",
        "",
        "## Primary candidates",
    ]
    for group in clusters.get("primary", [])[:12]:
        if not isinstance(group, dict): continue
        primary = group.get("primary") if isinstance(group.get("primary"), dict) else {}
        loc = ""
        if primary.get("file"):
            loc = f" ({primary.get('file')}:{primary.get('line') or '?'})"
        summary_lines.append(f"- {primary.get('level','error')}: {primary.get('message','')}{loc}")
    if len(summary_lines) == 10:
        summary_lines.append("- No normalized error line was detected; inspect SMART_LOG.txt and REDACTED_DETAIL.log.")
    summary = "\n".join(summary_lines) + "\n"
    return {
        "redacted_detail": redacted,
        "smart_log": filtered,
        "diagnostics": diagnostics,
        "clusters": clusters,
        "environment": environment_fingerprint(),
        "quality": quality,
        "failure_delta": delta,
        "summary": summary,
    }


def write_zip_evidence(zf, evidence: dict[str, Any]) -> None:
    zf.writestr("compat_diagnostics/AI_SUMMARY.md", evidence["summary"])
    zf.writestr("compat_diagnostics/REDACTED_DETAIL.log", evidence["redacted_detail"])
    zf.writestr("compat_diagnostics/SMART_LOG.txt", evidence["smart_log"])
    for name, key in [
        ("DIAGNOSTICS.json", "diagnostics"),
        ("ROOT_CAUSE_CLUSTERS.json", "clusters"),
        ("ENVIRONMENT_FINGERPRINT.json", "environment"),
        ("DIAGNOSTIC_QUALITY.json", "quality"),
        ("FAILURE_DELTA.json", "failure_delta"),
    ]:
        zf.writestr(f"compat_diagnostics/{name}", json.dumps(evidence[key], ensure_ascii=False, indent=2) + "\n")
