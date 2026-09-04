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
import stat
import subprocess
import sys
import tempfile
import zipfile

from python_patch_collect_schema import CollectSchemaError, validate_request_data

VERSION = "6.12.0"
REQUEST_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
MAX_REQUEST_JSON_BYTES = 1024 * 1024
IGNORED_DIRS = {".git", "node_modules", ".venv", "venv", "__pycache__", "build", "dist"}
TEXT_SUFFIXES = {
    ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".inc", ".py", ".js", ".ts", ".tsx",
    ".java", ".kt", ".kts", ".go", ".rs", ".cs", ".php", ".rb", ".sh", ".bash", ".zsh",
    ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".properties", ".xml", ".html",
    ".css", ".scss", ".md", ".txt", ".gradle", ".cmake", ".mk", ".proto", ".sql", ".log",
}
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
            raw = json.loads(zf.read(request_members[0]).decode("utf-8"))
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


def _iter_files(root: Path, scope: Path, *, max_files: int):
    count = 0
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
                    if entry.name in IGNORED_DIRS:
                        continue
                    stack.append(entry)
                elif entry.is_file():
                    yield entry
                    count += 1
                    if count >= max_files:
                        return
            except OSError:
                continue


def _project_rel(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


class ResultBuilder:
    def __init__(self, root: Path, request_data: dict, request_member: str):
        self.root = root
        self.request_data = request_data
        self.request_member = request_member
        self.limits = request_data["limits"]
        self.entries: list[dict[str, object]] = []
        self.reports: list[dict[str, object]] = []
        self._added_files: set[str] = set()
        self.total_bytes = 0
        self.report_bytes = 0
        self.file_count = 0
        self.out_dir = root / "artifacts" / "patch_tool_code_collections"
        self.out_dir.mkdir(parents=True, exist_ok=True)
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
        before = src.stat()
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
                target.write(chunk)
                digest.update(chunk)
                copied += len(chunk)
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
        truncated = False
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
        }
        self.zf.writestr("COLLECTION_MANIFEST.json", json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        self.zf.close()
        with zipfile.ZipFile(self.temp) as zf:
            bad = zf.testzip()
            if bad is not None:
                raise ValueError(f"generated result ZIP failed CRC at {bad}")
        if self.final.exists() or self.final.is_symlink():
            raise ValueError(f"result ZIP name collision: {self.final.name}")
        os.link(self.temp, self.final)
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
    return "\n".join(lines) + "\n", matches


def _looks_text(path: Path) -> bool:
    if path.suffix.lower() in TEXT_SUFFIXES or path.name in {"Makefile", "Dockerfile", "CMakeLists.txt"}:
        return True
    try:
        sample = path.read_bytes()[:4096]
    except OSError:
        return False
    return b"\x00" not in sample


def _search_action(root: Path, action: dict, limits: dict) -> str:
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


def _archive_request(root: Path, request_zip: Path) -> Path:
    queued = request_zip.resolve(strict=True)
    patchs = (root / "patchs").resolve(strict=False)
    try:
        queued.relative_to(patchs)
    except ValueError as exc:
        raise ValueError("request ZIP must be located under project patchs/") from exc
    if queued.parent != patchs:
        raise ValueError("request ZIP must be a direct file under project patchs/")
    archived_dir = root / "patchs" / "patched"
    archived_dir.mkdir(parents=True, exist_ok=True)
    destination = archived_dir / request_zip.name
    if destination.exists() or destination.is_symlink():
        if destination.is_symlink() or not destination.is_file():
            raise ValueError(f"archive destination is unsafe: patchs/patched/{request_zip.name}")
        def sha(path: Path):
            h=hashlib.sha256();
            with path.open('rb') as fh:
                for chunk in iter(lambda: fh.read(1024*1024), b''): h.update(chunk)
            return h.hexdigest()
        if sha(destination) == sha(request_zip):
            request_zip.unlink()
            return destination
        raise ValueError(f"archive destination already exists with different content: patchs/patched/{request_zip.name}")
    os.replace(request_zip, destination)
    return destination


def _run_request(root: Path, request_zip: Path) -> tuple[Path, Path, int]:
    request_data, request_member = _load_request(request_zip)
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
            archived = _archive_request(root, request_zip)
        except Exception:
            try:
                result.unlink()
            except OSError:
                pass
            raise
        return result, archived, len(request_data["actions"])
    except Exception:
        builder.abort()
        raise


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
        result, archived, action_count = _run_request(root, request_zip)
    except Exception as exc:
        print(f"ERROR: collection failed: {exc}", file=sys.stderr)
        return 2
    print(f"COLLECT: completed {action_count} action(s)", flush=True)
    print(f"ZIP : {result}", flush=True)
    try:
        rel_archived = archived.relative_to(root).as_posix()
    except ValueError:
        rel_archived = str(archived)
    print(f"REQUEST : {rel_archived}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
