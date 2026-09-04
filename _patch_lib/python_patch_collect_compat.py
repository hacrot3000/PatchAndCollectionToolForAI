#!/usr/bin/env python3
from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime
import fnmatch
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import time
import zipfile

from python_patch_collect_schema import CollectSchemaError, validate_request_data

VERSION = "6.17.1"
REQUEST_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
MAX_REQUEST_JSON_BYTES = 1024 * 1024
REGEX_SEARCH_TIMEOUT_SECONDS = 60.0
IGNORED_DIRS = {".git", "node_modules", ".venv", "venv", "__pycache__", "build", "dist"}
IGNORED_REL_PREFIXES = (
    "artifacts/patch_tool_code_collections/",
    "artifacts/patch_tool/",
    "patchs/patched/",
)
TEXT_SUFFIXES = {
    ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".inc", ".py", ".js", ".ts", ".tsx",
    ".java", ".kt", ".kts", ".go", ".rs", ".cs", ".php", ".rb", ".sh", ".bash", ".zsh",
    ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".properties", ".xml", ".html",
    ".css", ".scss", ".md", ".txt", ".gradle", ".cmake", ".mk", ".proto", ".sql", ".log",
}
SENSITIVE_NAME_SUFFIXES = {".pem", ".key", ".p12", ".pfx", ".jks", ".keystore"}
SENSITIVE_BASENAMES = {".env", ".npmrc", ".pypirc", "id_rsa", "id_ed25519", "credentials", "credentials.json", "secrets.json"}
SECRET_PATTERNS = [
    re.compile(r"(?i)(authorization\s*:\s*bearer\s+)[^\s'\"]+"),
    re.compile(r"(?i)((?:password|passwd|pwd|token|secret|api[_-]?key)\s*[=:]\s*)[^\s,;]+"),
]


def _safe_id(value: object) -> str:
    text = str(value or "collect").strip()
    text = re.sub(r"[^A-Za-z0-9._-]+", "_", text).strip("._-")
    return text[:96] or "collect"


def _redact_text(text: str) -> str:
    out = text
    for pattern in SECRET_PATTERNS:
        out = pattern.sub(lambda m: m.group(1) + "<REDACTED>", out)
    out = re.sub(
        r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH )?PRIVATE KEY-----",
        "<REDACTED_PRIVATE_KEY>",
        out,
        flags=re.S,
    )
    return out


def _sensitive_file_reasons(rel: str, src: Path) -> list[str]:
    reasons: list[str] = []
    name = src.name.lower()
    if name in SENSITIVE_BASENAMES or src.suffix.lower() in SENSITIVE_NAME_SUFFIXES:
        reasons.append("sensitive filename/type")
    try:
        if src.stat().st_size <= 4 * 1024 * 1024:
            raw = src.read_bytes()[:1024 * 1024]
            if b"-----BEGIN " in raw and b"PRIVATE KEY-----" in raw:
                reasons.append("private key marker")
            elif b"\x00" not in raw:
                text = raw.decode("utf-8", errors="ignore")
                if any(p.search(text) for p in SECRET_PATTERNS):
                    reasons.append("credential/token-like text")
    except OSError:
        pass
    return sorted(set(reasons))


def _snapshot_request_input(request_zip: Path) -> tuple[tempfile.TemporaryDirectory[str], Path, str]:
    temp_dir = tempfile.TemporaryDirectory(prefix="ptv-collect-input-")
    snapshot = Path(temp_dir.name) / request_zip.name
    flags = os.O_RDONLY | getattr(os, "O_BINARY", 0) | getattr(os, "O_NOFOLLOW", 0)
    fd = None
    try:
        fd = os.open(request_zip, flags)
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode):
            raise ValueError("request ZIP must be a regular non-symlink file")
        digest = hashlib.sha256(); copied = 0
        with os.fdopen(os.dup(fd), "rb") as src, snapshot.open("wb") as dst:
            for chunk in iter(lambda: src.read(1024 * 1024), b""):
                dst.write(chunk); digest.update(chunk); copied += len(chunk)
            dst.flush()
            try: os.fsync(dst.fileno())
            except OSError: pass
        after = os.fstat(fd)
        if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns) or copied != before.st_size:
            raise ValueError("request ZIP changed while creating execution snapshot")
        return temp_dir, snapshot, digest.hexdigest()
    except Exception:
        temp_dir.cleanup(); raise
    finally:
        if fd is not None:
            try: os.close(fd)
            except OSError: pass


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def _reject_duplicate_json_pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError(f"duplicate JSON key: {key}")
        out[key] = value
    return out


def _load_request(request_zip: Path) -> tuple[dict, str]:
    if request_zip.is_symlink() or not request_zip.is_file():
        raise ValueError("request ZIP must be a regular non-symlink file")
    if request_zip.suffix.lower() != ".zip":
        raise ValueError("request must be a .zip package")
    with zipfile.ZipFile(request_zip) as zf:
        members = [n for n in zf.namelist() if not n.endswith("/")]
        request_members = [n for n in members if REQUEST_RE.match(Path(n).name)]
        if len(request_members) != 1:
            raise ValueError(
                f"request ZIP must contain exactly one CODE_COLLECTION_REQUEST*.json (found {len(request_members)})"
            )
        info = zf.getinfo(request_members[0])
        if info.file_size > MAX_REQUEST_JSON_BYTES:
            raise ValueError(f"request JSON is too large ({info.file_size} bytes)")
        try:
            raw = json.loads(zf.read(request_members[0]).decode("utf-8"), object_pairs_hook=_reject_duplicate_json_pairs)
        except Exception as exc:
            raise ValueError(f"invalid request JSON ({type(exc).__name__})") from exc
    try:
        data = validate_request_data(raw)
    except CollectSchemaError as exc:
        raise ValueError(str(exc)) from exc
    return data, request_members[0]


def _safe_rel_path(raw: str, *, allow_dot: bool = True) -> PurePosixPath:
    text = str(raw).strip()
    if "\\" in text:
        raise ValueError(f"use POSIX '/' separators for project-relative path: {raw}")
    if text == "." and allow_dot:
        return PurePosixPath(".")
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts):
        raise ValueError(f"unsafe/non-relative path: {raw}")
    return rel


def _resolve_scope(root: Path, raw: str, *, file_ok: bool = False) -> tuple[str, Path]:
    rel = _safe_rel_path(raw)
    if rel.as_posix() == ".":
        return ".", root
    candidate = root.joinpath(*rel.parts)
    try:
        st = candidate.lstat()
    except FileNotFoundError as exc:
        raise ValueError(f"missing path: {rel.as_posix()}") from exc
    if stat.S_ISLNK(st.st_mode):
        raise ValueError(f"symlink path is not allowed: {rel.as_posix()}")
    if file_ok:
        if not (stat.S_ISDIR(st.st_mode) or stat.S_ISREG(st.st_mode)):
            raise ValueError(f"unsupported filesystem entry: {rel.as_posix()}")
    elif not stat.S_ISDIR(st.st_mode):
        raise ValueError(f"path must be a directory: {rel.as_posix()}")
    resolved = candidate.resolve(strict=True)
    try:
        resolved.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"path escapes project root: {rel.as_posix()}") from exc
    return rel.as_posix(), candidate


def _resolve_exact_file(root: Path, raw: str) -> tuple[str, Path]:
    rel = _safe_rel_path(raw, allow_dot=False)
    candidate = root.joinpath(*rel.parts)
    try:
        st = candidate.lstat()
    except FileNotFoundError as exc:
        raise ValueError(f"missing source: {rel.as_posix()}") from exc
    if stat.S_ISLNK(st.st_mode) or not stat.S_ISREG(st.st_mode):
        raise ValueError(f"source must be a regular non-symlink file: {rel.as_posix()}")
    resolved = candidate.resolve(strict=True)
    try:
        resolved.relative_to(root)
    except ValueError as exc:
        raise ValueError(f"source escapes project root: {rel.as_posix()}") from exc
    return rel.as_posix(), candidate


def _is_internal_output_path(root: Path, path: Path) -> bool:
    try:
        rel = path.relative_to(root).as_posix()
    except ValueError:
        return True
    rel_slash = rel.rstrip("/") + "/"
    return any(rel == prefix.rstrip("/") or rel_slash.startswith(prefix) for prefix in IGNORED_REL_PREFIXES)


def _iter_files(root: Path, scope: Path, *, max_files: int):
    count = 0
    if _is_internal_output_path(root, scope):
        return
    stack = [scope]
    while stack:
        current = stack.pop()
        if current.is_file():
            yield current
            count += 1
            if count >= max_files:
                return
            continue
        try:
            entries = sorted(current.iterdir(), key=lambda p: p.name.lower(), reverse=True)
        except OSError:
            continue
        for entry in entries:
            try:
                if entry.is_symlink():
                    continue
                if entry.is_dir():
                    if entry.name in IGNORED_DIRS or _is_internal_output_path(root, entry):
                        continue
                    stack.append(entry)
                elif entry.is_file():
                    if _is_internal_output_path(root, entry):
                        continue
                    yield entry
                    count += 1
                    if count >= max_files:
                        return
            except OSError:
                continue


def _project_rel(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


def _ensure_real_dir_chain(root: Path, parts: tuple[str, ...]) -> Path:
    cur = root.resolve(strict=True)
    for part in parts:
        nxt = cur / part
        if nxt.exists() or nxt.is_symlink():
            st = nxt.lstat()
            attrs = getattr(st, "st_file_attributes", 0)
            reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
            if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                raise ValueError(f"unsafe Patch Tool output directory component: {nxt}")
        else:
            try:
                nxt.mkdir(exist_ok=False)
            except FileExistsError:
                st = nxt.lstat()
                attrs = getattr(st, "st_file_attributes", 0)
                reparse = bool(attrs & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
                if stat.S_ISLNK(st.st_mode) or reparse or not stat.S_ISDIR(st.st_mode):
                    raise ValueError(f"unsafe directory component created concurrently: {nxt}")
        cur = nxt
    return cur


class ResultBuilder:
    def __init__(self, root: Path, request_data: dict, request_member: str):
        self.root = root
        self.request_data = request_data
        self.request_member = request_member
        self.limits = request_data["limits"]
        self.entries: list[dict[str, object]] = []
        self.reports: list[dict[str, object]] = []
        self._added_files: set[str] = set()
        self.sensitive_warnings: list[dict[str, object]] = []
        self.total_bytes = 0
        self.report_bytes = 0
        self.file_count = 0
        self.out_dir = _ensure_real_dir_chain(root, ("artifacts", "patch_tool_code_collections"))
        stamp = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
        self.final = self.out_dir / (
            f"CODE_COLLECTION_RESULT_{_safe_id(request_data.get('id'))}_{stamp}_{os.getpid()}.zip"
        )
        fd, name = tempfile.mkstemp(prefix=".ptv-collect-", suffix=".zip", dir=self.out_dir)
        os.close(fd)
        self.temp = Path(name)
        self.zf = zipfile.ZipFile(self.temp, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6)

    def add_exact_file(self, rel: str, src: Path, *, source_action: int) -> None:
        if rel in self._added_files:
            return
        try:
            if src.samefile(self.temp) or (self.final.exists() and src.samefile(self.final)):
                raise ValueError(f"refusing to collect collector output: {rel}")
        except FileNotFoundError:
            pass
        if _is_internal_output_path(self.root, src):
            raise ValueError(f"refusing to collect Patch Tool internal artifact: {rel}")
        before = src.stat()
        reasons = _sensitive_file_reasons(rel, src)
        if reasons:
            warning = {"path": rel, "reasons": reasons}
            if warning not in self.sensitive_warnings:
                self.sensitive_warnings.append(warning)
                print(f"[PTV v{VERSION} WARNING] sensitive exact file included in COLLECT: {rel} ({', '.join(reasons)})", file=sys.stderr, flush=True)
        if before.st_size > self.limits["max_file_bytes"]:
            raise ValueError(f"file exceeds max_file_bytes: {rel} ({before.st_size})")
        if self.file_count + 1 > self.limits["max_files"]:
            raise ValueError("collection exceeds max_files")
        if self.total_bytes + before.st_size > self.limits["max_total_bytes"]:
            raise ValueError("collection exceeds max_total_bytes")
        digest = hashlib.sha256()
        copied = 0
        arcname = f"files/{rel}"
        with src.open("rb") as source, self.zf.open(arcname, "w") as target:
            for chunk in iter(lambda: source.read(1024 * 1024), b""):
                next_copied = copied + len(chunk)
                if next_copied > self.limits["max_file_bytes"]:
                    raise ValueError(f"file exceeded max_file_bytes while being collected: {rel}")
                if self.total_bytes + next_copied > self.limits["max_total_bytes"]:
                    raise ValueError("collection exceeded max_total_bytes while copying")
                target.write(chunk)
                digest.update(chunk)
                copied = next_copied
        after = src.stat()
        identity_before = (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns)
        identity_after = (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns)
        if identity_before != identity_after:
            raise ValueError(f"source changed while being collected; retry: {rel}")
        self._added_files.add(rel)
        self.file_count += 1
        self.total_bytes += copied
        self.entries.append({
            "path": rel,
            "archive_path": arcname,
            "size": copied,
            "sha256": digest.hexdigest(),
            "source_action": source_action,
        })

    def add_report(self, index: int, kind: str, title: str, text: str) -> None:
        text = _redact_text(text)
        raw = text.encode("utf-8", errors="replace")
        remaining = self.limits["max_report_bytes"] - self.report_bytes
        if remaining <= 0:
            return
        # Reports can be semantically bounded by action limits even when the
        # byte cap is not reached. Preserve that quality signal in the manifest.
        truncated = "[TRUNCATED" in text
        if len(raw) > remaining:
            raw = raw[:remaining]
            text = raw.decode("utf-8", errors="ignore") + "\n\n[TRUNCATED BY max_report_bytes]\n"
            raw = text.encode("utf-8")
            truncated = True
        arcname = f"reports/{index:03d}_{kind}.md"
        self.zf.writestr(arcname, raw)
        self.report_bytes += len(raw)
        self.reports.append({"action": index, "type": kind, "title": title, "archive_path": arcname, "truncated": truncated})

    def finish(self) -> Path:
        manifest = {
            "format": "python-patch-tool-code-collection",
            "format_version": 2,
            "tool_version": VERSION,
            "request_id": self.request_data.get("id"),
            "title": self.request_data.get("title"),
            "request_member": self.request_member,
            "action_count": len(self.request_data["actions"]),
            "file_count": self.file_count,
            "total_file_bytes": self.total_bytes,
            "report_bytes": self.report_bytes,
            "files": self.entries,
            "reports": self.reports,
            "sensitive_warnings": self.sensitive_warnings,
        }
        self.zf.writestr("COLLECTION_MANIFEST.json", json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        self.zf.close()
        with zipfile.ZipFile(self.temp) as zf:
            bad = zf.testzip()
            if bad is not None:
                raise ValueError(f"generated result ZIP failed CRC at {bad}")
        if self.final.exists() or self.final.is_symlink():
            raise ValueError(f"result ZIP name collision: {self.final.name}")
        try:
            os.link(self.temp, self.final, follow_symlinks=False)
        except OSError:
            flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_BINARY", 0)
            fd = os.open(self.final, flags, 0o600)
            try:
                with os.fdopen(fd, "wb") as dst, self.temp.open("rb") as src:
                    shutil.copyfileobj(src, dst, length=1024 * 1024)
                    dst.flush()
                    try: os.fsync(dst.fileno())
                    except OSError: pass
                fd = -1
                if _sha256_file(self.final) != _sha256_file(self.temp):
                    raise ValueError("result ZIP fallback copy hash mismatch")
            except Exception:
                if fd >= 0:
                    try: os.close(fd)
                    except OSError: pass
                try: self.final.unlink()
                except OSError: pass
                raise
        self.temp.unlink()
        return self.final

    def abort(self) -> None:
        try:
            self.zf.close()
        except Exception:
            pass
        for p in (self.temp, self.final):
            try:
                p.unlink()
            except FileNotFoundError:
                pass
            except OSError:
                pass


def _overview(root: Path, action: dict, limits: dict) -> str:
    rel, scope = _resolve_scope(root, action.get("path", "."), file_ok=True)
    depth = action.get("tree_depth", 2)
    lines = [f"# Project overview", "", f"Scope: `{rel}`", f"Tree depth: {depth}", ""]
    ext_counter: Counter[str] = Counter()
    total_files = 0
    total_bytes = 0
    tree_lines: list[str] = []
    for path in _iter_files(root, scope, max_files=limits["max_files"]):
        try:
            st = path.stat()
        except OSError:
            continue
        total_files += 1
        total_bytes += st.st_size
        suffix = path.suffix.lower() or "<no-ext>"
        ext_counter[suffix] += 1
        relpath = _project_rel(root, path)
        scope_parts = Path(relpath).parts
        base_parts = () if rel == "." else Path(rel).parts
        local_depth = max(0, len(scope_parts) - len(base_parts) - 1)
        if local_depth <= depth:
            tree_lines.append(relpath)
    lines += [f"Files scanned: {total_files}", f"Bytes scanned: {total_bytes}", "", "## File types"]
    for ext, count in ext_counter.most_common(40):
        lines.append(f"- `{ext}`: {count}")
    lines += ["", "## Bounded tree"]
    lines += [f"- `{x}`" for x in tree_lines[:2000]]
    if len(tree_lines) > 2000:
        lines.append(f"- ... {len(tree_lines)-2000} more paths omitted")
        lines.append("[TRUNCATED tree at 2000 entries]")
    notable = [
        "CMakeLists.txt", "Makefile", "package.json", "pyproject.toml", "requirements.txt", "Cargo.toml",
        "go.mod", "build.gradle", "settings.gradle", "pom.xml", ".gitmodules", "README.md",
    ]
    found = [name for name in notable if (root / name).is_file()]
    lines += ["", "## Notable project files"]
    lines += [f"- `{x}`" for x in found] or ["- none at project root"]
    return "\n".join(lines) + "\n"


def _find_action(root: Path, action: dict, limits: dict) -> tuple[str, list[tuple[str, Path]]]:
    matches: list[tuple[str, Path]] = []
    max_results = min(action.get("max_results", 1000), limits["max_files"])
    seen: set[str] = set()
    for raw_scope in action.get("paths", ["."]):
        _, scope = _resolve_scope(root, raw_scope, file_ok=True)
        for path in _iter_files(root, scope, max_files=limits["max_files"]):
            rel = _project_rel(root, path)
            if rel in seen:
                continue
            if any(fnmatch.fnmatch(path.name, pat) or fnmatch.fnmatch(rel, pat) for pat in action["patterns"]):
                seen.add(rel)
                matches.append((rel, path))
                if len(matches) >= max_results:
                    break
        if len(matches) >= max_results:
            break
    lines = ["# Find results", "", f"Patterns: `{action['patterns']}`", f"Matches: {len(matches)}", ""]
    lines += [f"- `{rel}`" for rel, _ in matches]
    if len(matches) >= max_results:
        lines.append(f"[TRUNCATED at max_results={max_results}]")
    return "\n".join(lines) + "\n", matches


def _looks_text(path: Path) -> bool:
    if path.suffix.lower() in TEXT_SUFFIXES or path.name in {"Makefile", "Dockerfile", "CMakeLists.txt"}:
        return True
    try:
        sample = path.read_bytes()[:4096]
    except OSError:
        return False
    return b"\x00" not in sample


def _search_action_direct(root: Path, action: dict, limits: dict) -> str:
    query = action["query"]
    regex_mode = action.get("regex", False)
    try:
        pattern = re.compile(query) if regex_mode else None
    except re.error as exc:
        raise ValueError(f"invalid search regex: {exc}") from exc
    context = action.get("context_lines", 4)
    max_matches = action.get("max_matches", 500)
    found = 0
    blocks: list[str] = ["# Search results", "", f"Query: `{query}`", f"Regex: {regex_mode}", ""]
    seen_files: set[str] = set()
    for raw_scope in action.get("paths", ["."]):
        _, scope = _resolve_scope(root, raw_scope, file_ok=True)
        for path in _iter_files(root, scope, max_files=limits["max_files"]):
            rel = _project_rel(root, path)
            if rel in seen_files:
                continue
            seen_files.add(rel)
            try:
                if path.stat().st_size > limits["max_file_bytes"] or not _looks_text(path):
                    continue
                text = path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            lines = text.splitlines()
            match_lines: list[int] = []
            for idx, line in enumerate(lines):
                hit = bool(pattern.search(line)) if pattern else query in line
                if hit:
                    match_lines.append(idx)
                    found += 1
                    if found >= max_matches:
                        break
            for idx in match_lines:
                start = max(0, idx - context)
                end = min(len(lines), idx + context + 1)
                blocks.append(f"## {rel}:{idx+1}")
                blocks.append("```text")
                for n in range(start, end):
                    marker = ">" if n == idx else " "
                    blocks.append(f"{marker}{n+1:6d}: {lines[n]}")
                blocks.append("```")
                blocks.append("")
            if found >= max_matches:
                break
        if found >= max_matches:
            break
    blocks.insert(4, f"Matches: {found}")
    if found >= max_matches:
        blocks.append(f"[TRUNCATED at max_matches={max_matches}]")
    return "\n".join(blocks) + "\n"



def _search_action(root: Path, action: dict, limits: dict) -> str:
    """Run regex search out-of-process so pathological `re` cannot hang COLLECT."""
    if not action.get("regex", False):
        return _search_action_direct(root, action, limits)
    worker = Path(__file__).resolve().parent / "python_patch_collect_regex_worker.py"
    if not worker.is_file():
        raise ValueError("regex search worker is missing")
    with tempfile.TemporaryDirectory(prefix="ptv-collect-regex-") as td:
        work = Path(td)
        request_path = work / "request.json"
        result_path = work / "result.txt"
        request_path.write_text(
            json.dumps({"action": action, "limits": limits}, ensure_ascii=False),
            encoding="utf-8",
        )
        env = dict(os.environ)
        env["PYTHONDONTWRITEBYTECODE"] = "1"
        try:
            proc = subprocess.run(
                [sys.executable, str(worker), "--project-root", str(root), "--request", str(request_path), "--result", str(result_path)],
                cwd=root,
                env=env,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=REGEX_SEARCH_TIMEOUT_SECONDS,
            )
        except subprocess.TimeoutExpired as exc:
            raise ValueError(
                f"regex search exceeded hard timeout ({REGEX_SEARCH_TIMEOUT_SECONDS:g}s); narrow paths/query"
            ) from exc
        if proc.returncode != 0:
            detail = (proc.stdout or "").strip().replace("\n", " ")[:800]
            raise ValueError(f"regex search worker failed rc={proc.returncode}: {detail}")
        if not result_path.is_file() or result_path.is_symlink():
            raise ValueError("regex search worker produced no safe result")
        size = result_path.stat().st_size
        hard_result_cap = max(int(limits.get("max_report_bytes", 0)), 1024 * 1024) * 2
        if size > hard_result_cap:
            raise ValueError(f"regex search worker result exceeded safety cap ({size} bytes)")
        return result_path.read_text(encoding="utf-8")


def _git_action(root: Path, action: dict) -> str:
    if not (root / ".git").exists():
        return "# Git context\n\nNot a Git working tree at the project root.\n"
    commands = {
        "status": ["git", "status", "--short", "--branch"],
        "log": ["git", "log", "--oneline", "--decorate", "-n", str(action.get("log_entries", 20))],
        "diff_stat": ["git", "diff", "--stat", "--no-ext-diff"],
        "diff": ["git", "diff", "--no-ext-diff", "--unified=3"],
    }
    blocks = ["# Git context", ""]
    for section in action["sections"]:
        cmd = commands[section]
        proc = subprocess.run(cmd, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, errors="replace", timeout=30)
        blocks += [f"## {section}", "```text", proc.stdout.rstrip(), "```", ""]
    return "\n".join(blocks) + "\n"


def _collect_queue_dirs(root: Path) -> tuple[Path, Path]:
    queue = root / "patchs"
    if queue.is_symlink() or not queue.is_dir():
        raise ValueError("project patchs/ must be a real directory")
    if queue.resolve(strict=True).parent != root.resolve(strict=True):
        raise ValueError("project patchs/ escapes project root")
    history = queue / "patched"
    if history.exists() or history.is_symlink():
        if history.is_symlink() or not history.is_dir():
            raise ValueError("patchs/patched/ must be a real directory")
    else:
        history.mkdir(parents=False, exist_ok=False)
    if history.resolve(strict=True).parent != queue.resolve(strict=True):
        raise ValueError("patchs/patched/ escapes project queue")
    return queue, history


def _publish_request_snapshot(snapshot: Path, dst: Path, expected_sha: str) -> None:
    if dst.exists() or dst.is_symlink():
        if dst.is_symlink() or not dst.is_file() or _sha256_file(dst) != expected_sha:
            raise ValueError(f"archive destination already exists with different/unsafe content: patchs/patched/{dst.name}")
        return
    fd, tmp_name = tempfile.mkstemp(prefix=".ptv-collect-archive-", suffix=".tmp", dir=dst.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "wb") as out, snapshot.open("rb") as src:
            shutil.copyfileobj(src, out, length=1024 * 1024)
            out.flush()
            try: os.fsync(out.fileno())
            except OSError: pass
        if _sha256_file(tmp) != expected_sha:
            raise ValueError("executed COLLECT request snapshot failed archive hash verification")
        try:
            os.link(tmp, dst, follow_symlinks=False)
        except FileExistsError:
            if dst.is_symlink() or not dst.is_file() or _sha256_file(dst) != expected_sha:
                raise ValueError(f"archive destination raced with different content: {dst.name}")
        except OSError:
            flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_BINARY", 0)
            try:
                copy_fd = os.open(dst, flags, 0o600)
            except FileExistsError:
                if dst.is_symlink() or not dst.is_file() or _sha256_file(dst) != expected_sha:
                    raise ValueError(f"archive destination raced with different content: {dst.name}")
            else:
                try:
                    with os.fdopen(copy_fd, "wb") as out, tmp.open("rb") as src:
                        copy_fd = -1
                        shutil.copyfileobj(src, out, length=1024 * 1024)
                        out.flush()
                        try: os.fsync(out.fileno())
                        except OSError: pass
                    if _sha256_file(dst) != expected_sha:
                        raise ValueError("archive fallback copy hash verification failed")
                except Exception:
                    if copy_fd >= 0:
                        try: os.close(copy_fd)
                        except OSError: pass
                    try: dst.unlink()
                    except OSError: pass
                    raise
    finally:
        try: tmp.unlink()
        except FileNotFoundError: pass


def _remove_request_if_executed(request_zip: Path, expected_sha: str) -> str:
    if not request_zip.exists() and not request_zip.is_symlink():
        return "already_absent"
    guard = request_zip.parent / f".ptv-collect-guard-{os.getpid()}-{time.time_ns()}-{request_zip.name}"
    try:
        os.replace(request_zip, guard)
    except FileNotFoundError:
        return "already_absent"
    try:
        current_sha = None if guard.is_symlink() or not guard.is_file() else _sha256_file(guard)
        if current_sha == expected_sha:
            guard.unlink()
            return "removed_executed_input"
        if not request_zip.exists() and not request_zip.is_symlink():
            os.replace(guard, request_zip)
            return "replacement_restored"
        preserved = request_zip.parent / f"PTV_UNEXPECTED_COLLECT_REPLACEMENT_{time.time_ns()}_{request_zip.name}"
        os.replace(guard, preserved)
        return f"replacement_preserved:{preserved.name}"
    finally:
        if guard.exists() or guard.is_symlink():
            try:
                if not request_zip.exists() and not request_zip.is_symlink():
                    os.replace(guard, request_zip)
            except OSError:
                pass


def _archive_request(root: Path, request_zip: Path, executed_snapshot: Path, expected_sha: str) -> tuple[Path, str]:
    queue, history = _collect_queue_dirs(root)
    if request_zip.parent.resolve(strict=True) != queue.resolve(strict=True):
        raise ValueError("request ZIP must be a direct file under project patchs/")
    destination = history / request_zip.name
    _publish_request_snapshot(executed_snapshot, destination, expected_sha)
    lifecycle = _remove_request_if_executed(request_zip, expected_sha)
    return destination, lifecycle


def _run_request(root: Path, request_zip: Path) -> tuple[Path, Path, int, str]:
    input_temp, execution_request, request_sha = _snapshot_request_input(request_zip)
    try:
        request_data, request_member = _load_request(execution_request)
        builder = ResultBuilder(root, request_data, request_member)
        try:
            for index, action in enumerate(request_data["actions"], 1):
                kind = action["type"]
                title = action.get("title") or action.get("id") or kind
                if kind == "pack":
                    lines = ["# Pack", ""]
                    for raw in action["paths"]:
                        rel, src = _resolve_exact_file(root, raw)
                        builder.add_exact_file(rel, src, source_action=index)
                        lines.append(f"- `{rel}`")
                    builder.add_report(index, kind, title, "\n".join(lines) + "\n")
                elif kind == "overview":
                    builder.add_report(index, kind, title, _overview(root, action, request_data["limits"]))
                elif kind == "find":
                    report, matches = _find_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, report)
                    if action.get("collect"):
                        for rel, src in matches:
                            builder.add_exact_file(rel, src, source_action=index)
                elif kind == "search":
                    builder.add_report(index, kind, title, _search_action(root, action, request_data["limits"]))
                elif kind == "git":
                    builder.add_report(index, kind, title, _git_action(root, action))
                else:
                    raise ValueError(f"unsupported action after schema validation: {kind}")
            result = builder.finish()
            try:
                archived, lifecycle = _archive_request(root, request_zip, execution_request, request_sha)
            except Exception:
                try: result.unlink()
                except OSError: pass
                raise
            return result, archived, len(request_data["actions"]), lifecycle
        except Exception:
            builder.abort()
            raise
    finally:
        input_temp.cleanup()


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=f"Python Patch Tool v{VERSION} self-contained readonly collector")
    ap.add_argument("--project-root", required=True)
    ap.add_argument("rest", nargs=argparse.REMAINDER)
    ns = ap.parse_args(argv)
    root = Path(ns.project_root).resolve()
    rest = list(ns.rest)
    if len(rest) != 2 or rest[0] != "request":
        print("ERROR: self-contained COLLECT accepts only: request <request.zip>", file=sys.stderr)
        return 2
    request_arg = Path(rest[1])
    request_zip = request_arg if request_arg.is_absolute() else root / request_arg
    try:
        result, archived, action_count, queue_lifecycle = _run_request(root, request_zip)
    except Exception as exc:
        print(f"ERROR: collection failed: {exc}", file=sys.stderr)
        return 2
    print(f"COLLECT: completed {action_count} action(s)", flush=True)
    if queue_lifecycle == "replacement_restored" or str(queue_lifecycle).startswith("replacement_preserved:"):
        print(
            f"[PTV v{VERSION} WARNING] request ZIP changed while COLLECT was running; "
            "the exact executed request was archived and the replacement remains queued.",
            file=sys.stderr, flush=True,
        )
    print(f"ZIP : {result}", flush=True)
    try:
        rel_archived = archived.relative_to(root).as_posix()
    except ValueError:
        rel_archived = str(archived)
    print(f"REQUEST : {rel_archived}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
