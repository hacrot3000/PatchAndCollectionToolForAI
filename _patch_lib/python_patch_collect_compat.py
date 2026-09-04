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

VERSION = "6.18.0"
REQUEST_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
MAX_REQUEST_JSON_BYTES = 1024 * 1024
REGEX_SEARCH_TIMEOUT_SECONDS = 60.0
SEARCH_BACKEND_TIMEOUT_SECONDS = 60.0
# Search intentionally scans the filesystem, including untracked/gitignored source.
# Only clearly non-source/tool-internal trees are skipped by default; every skip is reported.
SEARCH_DEFAULT_EXCLUDED_DIRS = {".git", "node_modules", ".venv", "venv", "__pycache__"}
IGNORED_DIRS = SEARCH_DEFAULT_EXCLUDED_DIRS | {"build", "dist"}
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
        self.collection_warnings: list[dict[str, object]] = []
        self.collection_status = "PASS"
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

    def mark_incomplete(self, *, action: int, kind: str, reasons: list[str]) -> None:
        self.collection_status = "INCOMPLETE"
        self.collection_warnings.append({"action": action, "type": kind, "reasons": list(reasons)})

    def finish(self) -> Path:
        manifest = {
            "format": "python-patch-tool-code-collection",
            "format_version": 3,
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
            "collection_status": self.collection_status,
            "collection_warnings": self.collection_warnings,
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
        with path.open("rb") as fh:
            sample = fh.read(4096)
    except OSError:
        return False
    return b"\x00" not in sample


def _resolve_search_scope(root: Path, raw: str, *, file_ok: bool = True) -> tuple[str, Path]:
    """Search-only resolver: accepts project-relative or absolute paths contained by root."""
    value = str(raw).strip()
    candidate = Path(value)
    if not candidate.is_absolute():
        rel = _safe_rel_path(value)
        candidate = root if rel.as_posix() == "." else root.joinpath(*rel.parts)
    try:
        st = candidate.lstat()
    except FileNotFoundError as exc:
        raise ValueError(f"missing search path: {value}") from exc
    if stat.S_ISLNK(st.st_mode):
        raise ValueError(f"search root/anchor may not itself be a symlink: {value}")
    if file_ok:
        if not (stat.S_ISDIR(st.st_mode) or stat.S_ISREG(st.st_mode)):
            raise ValueError(f"unsupported search filesystem entry: {value}")
    elif not stat.S_ISDIR(st.st_mode):
        raise ValueError(f"search path must be a directory: {value}")
    resolved = candidate.resolve(strict=True)
    try:
        rel = resolved.relative_to(root.resolve(strict=True)).as_posix()
    except ValueError as exc:
        raise ValueError(f"search path escapes project root: {value}") from exc
    return (rel or "."), candidate


def _search_scope_list(root: Path, action: dict) -> list[tuple[str, Path, str]]:
    scopes: list[tuple[str, Path, str]] = []
    seen: set[str] = set()
    for kind, values in (("anchor", action.get("anchor_paths", [])), ("requested", action.get("paths", ["."]))):
        for raw in values:
            rel, path = _resolve_search_scope(root, raw, file_ok=True)
            key = path.resolve(strict=True).as_posix()
            if key in seen:
                continue
            seen.add(key); scopes.append((rel, path, kind))
    for raw in action.get("expected_files", []):
        rel, path = _resolve_search_scope(root, raw, file_ok=True)
        if not path.is_file():
            raise ValueError(f"expected_files entry must be a regular file: {raw}")
        key = path.resolve(strict=True).as_posix()
        if key not in seen:
            seen.add(key); scopes.insert(0, (rel, path, "expected_file"))
    return scopes


def _search_skip_record(diag: dict, rel: str, reason: str, *, is_dir: bool = True) -> None:
    key = "skipped_dirs" if is_dir else "skipped_files"
    diag[key + "_count"] = int(diag.get(key + "_count", 0)) + 1
    rows = diag.setdefault(key, [])
    if len(rows) < 250:
        rows.append({"path": rel, "reason": reason})


def _search_rel(root: Path, path: Path) -> str:
    try:
        return path.relative_to(root).as_posix()
    except ValueError:
        return str(path)


def _gitignored_relpaths(root: Path, rels: list[str]) -> tuple[set[str], str | None]:
    if not rels:
        return set(), None
    if not (root/'.git').exists():
        return set(), "project root is not a Git working tree; respect_gitignore could not be verified"
    ignored: set[str] = set()
    try:
        for offset in range(0, len(rels), 4096):
            chunk = rels[offset:offset+4096]
            payload = b"".join(x.encode('utf-8', errors='surrogateescape') + b"\0" for x in chunk)
            proc = subprocess.run(
                ["git", "-c", "core.fsmonitor=false", "check-ignore", "-z", "--stdin"],
                cwd=root, input=payload, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                timeout=30,
            )
            if proc.returncode not in {0, 1}:
                detail=proc.stderr.decode('utf-8',errors='replace').strip().replace('\n',' ')[:500]
                return ignored, f"git check-ignore failed rc={proc.returncode}: {detail}"
            for raw in proc.stdout.split(b"\0"):
                if raw:
                    ignored.add(raw.decode('utf-8', errors='surrogateescape'))
    except Exception as exc:
        return ignored, f"gitignore check failed: {type(exc).__name__}: {exc}"
    return ignored, None


def _discover_search_files(root: Path, scopes: list[tuple[str, Path, str]], action: dict, limits: dict, *, walker: str) -> tuple[list[Path], dict]:
    max_files=int(limits.get("max_search_files", 250000))
    follow=bool(action.get("follow_symlinks", False))
    diag={
        "walker": walker, "directories_visited": 0, "files_considered": 0,
        "files_searched": 0, "limit_reached": False, "errors": [],
        "skipped_dirs": [], "skipped_dirs_count": 0, "skipped_files": [], "skipped_files_count": 0,
        "top_dirs": {}, "modules": set(), "extension_counts": Counter(), "candidate_filenames": [],
    }
    files=[]; seen_files=set(); seen_dirs=set()
    marker_names={"pom.xml","build.gradle","build.gradle.kts","settings.gradle","settings.gradle.kts","package.json","pyproject.toml","CMakeLists.txt"}
    query_tokens=[x.lower() for x in re.findall(r'[A-Z]?[a-z]+|[A-Z]+(?![a-z])|\d+',str(action.get("query", ""))) if len(x)>=3]
    query_tokens=[x for x in query_tokens if x not in {"cmd","msg","req"}]

    # Explicit Git-index mode must not depend on a preceding bounded filesystem walk.
    # It is deliberately narrower than the filesystem default and therefore uses
    # git ls-files as its authoritative candidate inventory from the start.
    if action.get("source_scope") == "git_tracked":
        if not (root/'.git').exists():
            diag["errors"].append("source_scope=git_tracked requested but project root is not a Git working tree")
            diag["modules"]=[]; diag["extension_counts"]={}
            return [],diag
        requested=[]
        for rel,path,kind in scopes:
            if kind == "expected_file":
                requested.append(rel)
            elif rel != '.':
                requested.append(rel)
        cmd=["git","-c","core.fsmonitor=false","ls-files","-z"] + (["--",*requested] if requested else [])
        try:
            proc=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=30)
        except Exception as exc:
            diag["errors"].append(f"git ls-files failed: {type(exc).__name__}: {exc}")
            diag["modules"]=[]; diag["extension_counts"]={}
            return [],diag
        if proc.returncode:
            diag["errors"].append(f"git ls-files failed rc={proc.returncode}")
            diag["modules"]=[]; diag["extension_counts"]={}
            return [],diag
        max_files=int(limits.get("max_search_files",250000))
        for raw in proc.stdout.split(b'\0'):
            if not raw: continue
            rel=raw.decode('utf-8',errors='surrogateescape'); path=root/rel
            try:
                st=path.stat(); path.resolve(strict=True).relative_to(root.resolve(strict=True))
            except Exception:
                _search_skip_record(diag,rel,"tracked_path_missing_or_unsafe",is_dir=False); continue
            if not stat.S_ISREG(st.st_mode): continue
            files.append(path); diag["files_considered"] += 1; diag["extension_counts"][path.suffix.lower() or '<no-ext>'] += 1
            parts=Path(rel).parts
            for depth in range(1,min(3,len(parts)-1)+1): diag["modules"].add(Path(*parts[:depth]).as_posix())
            if path.name in marker_names: diag["modules"].add(Path(rel).parent.as_posix() or '.')
            if len(diag["candidate_filenames"])<500 and (not query_tokens or any(t in path.name.lower() for t in query_tokens)):
                diag["candidate_filenames"].append(rel)
            if len(files)>=max_files:
                diag["limit_reached"]=True; break
        diag["modules"]=sorted(diag["modules"])[:500]
        diag["extension_counts"]={k:v for k,v in diag["extension_counts"].most_common()}
        return files,diag

    def add_file(path: Path, scope_rel: str):
        if len(files) >= max_files:
            diag["limit_reached"] = True; return False
        try:
            st=path.stat(follow_symlinks=follow)
        except (OSError, TypeError) as exc:
            _search_skip_record(diag, _search_rel(root,path), f"stat_error:{type(exc).__name__}", is_dir=False); return True
        if not stat.S_ISREG(st.st_mode):
            _search_skip_record(diag, _search_rel(root,path), "not_regular", is_dir=False); return True
        try:
            resolved=path.resolve(strict=True)
            resolved.relative_to(root.resolve(strict=True))
        except Exception:
            _search_skip_record(diag, _search_rel(root,path), "escapes_project_root", is_dir=False); return True
        key=(st.st_dev,st.st_ino) if getattr(st,'st_ino',0) else resolved.as_posix()
        if key in seen_files: return True
        seen_files.add(key); files.append(path); diag["files_considered"] += 1
        rel=_search_rel(root,path); diag["extension_counts"][path.suffix.lower() or '<no-ext>'] += 1
        parts=Path(rel).parts
        if path.name in marker_names:
            diag["modules"].add(Path(rel).parent.as_posix() or ".")
        if len(diag["candidate_filenames"]) < 500 and (not query_tokens or any(t in path.name.lower() for t in query_tokens)):
            diag["candidate_filenames"].append(rel)
        return len(files) < max_files

    def note_dir(path: Path, scope: Path):
        try:
            st=path.stat(follow_symlinks=follow)
            key=(st.st_dev,st.st_ino) if getattr(st,'st_ino',0) else path.resolve(strict=True).as_posix()
        except Exception as exc:
            _search_skip_record(diag,_search_rel(root,path),f"dir_stat_error:{type(exc).__name__}"); return False
        if key in seen_dirs:
            _search_skip_record(diag,_search_rel(root,path),"directory_cycle_or_duplicate"); return False
        seen_dirs.add(key); diag["directories_visited"] += 1
        try:
            local=path.relative_to(scope)
            if len(local.parts) <= 3 and local.as_posix() != '.':
                diag["modules"].add(_search_rel(root,path))
        except ValueError: pass
        return True

    for scope_rel, scope, scope_kind in scopes:
        if diag["limit_reached"]: break
        if scope.is_file():
            add_file(scope, scope_rel); continue
        if walker == "oswalk":
            for current, dirs, names in os.walk(scope, topdown=True, followlinks=follow):
                cur=Path(current)
                if not note_dir(cur, scope):
                    dirs[:] = []; continue
                filtered=[]
                for name in sorted(dirs):
                    p=cur/name; rel=_search_rel(root,p)
                    try: islink=p.is_symlink()
                    except OSError: islink=False
                    if name in SEARCH_DEFAULT_EXCLUDED_DIRS or _is_internal_output_path(root,p):
                        _search_skip_record(diag,rel,"default_exclude" if name in SEARCH_DEFAULT_EXCLUDED_DIRS else "patch_tool_internal"); continue
                    if islink and not follow:
                        _search_skip_record(diag,rel,"symlink_follow_disabled"); continue
                    if islink and follow:
                        try: p.resolve(strict=True).relative_to(root.resolve(strict=True))
                        except Exception:
                            _search_skip_record(diag,rel,"symlink_escapes_project_root"); continue
                    filtered.append(name)
                dirs[:] = filtered
                for name in sorted(names):
                    p=cur/name
                    if p.is_symlink() and not follow:
                        _search_skip_record(diag,_search_rel(root,p),"symlink_follow_disabled",is_dir=False); continue
                    if _is_internal_output_path(root,p):
                        _search_skip_record(diag,_search_rel(root,p),"patch_tool_internal",is_dir=False); continue
                    if not add_file(p,scope_rel): break
                if diag["limit_reached"]: break
        else:
            stack=[scope]
            while stack and not diag["limit_reached"]:
                current=stack.pop()
                if current.is_file(): add_file(current,scope_rel); continue
                if not note_dir(current,scope): continue
                try: entries=sorted(os.scandir(current), key=lambda e:e.name.lower(), reverse=True)
                except OSError as exc:
                    diag["errors"].append(f"{_search_rel(root,current)}: {type(exc).__name__}: {exc}"); continue
                for entry in entries:
                    p=Path(entry.path); rel=_search_rel(root,p)
                    try:
                        islink=entry.is_symlink()
                        if entry.is_dir(follow_symlinks=follow):
                            if entry.name in SEARCH_DEFAULT_EXCLUDED_DIRS or _is_internal_output_path(root,p):
                                _search_skip_record(diag,rel,"default_exclude" if entry.name in SEARCH_DEFAULT_EXCLUDED_DIRS else "patch_tool_internal"); continue
                            if islink and not follow:
                                _search_skip_record(diag,rel,"symlink_follow_disabled"); continue
                            if islink and follow:
                                try: p.resolve(strict=True).relative_to(root.resolve(strict=True))
                                except Exception:
                                    _search_skip_record(diag,rel,"symlink_escapes_project_root"); continue
                            stack.append(p)
                        elif entry.is_file(follow_symlinks=follow):
                            if islink and not follow:
                                _search_skip_record(diag,rel,"symlink_follow_disabled",is_dir=False); continue
                            if _is_internal_output_path(root,p):
                                _search_skip_record(diag,rel,"patch_tool_internal",is_dir=False); continue
                            if not add_file(p,scope_rel): break
                    except OSError as exc:
                        _search_skip_record(diag,rel,f"entry_error:{type(exc).__name__}",is_dir=False)

    if action.get("respect_gitignore", False):
        rels=[_search_rel(root,p) for p in files]
        ignored, err=_gitignored_relpaths(root,rels)
        if err: diag["errors"].append(err)
        if ignored:
            kept=[]
            for p,rel in zip(files,rels):
                if rel in ignored: _search_skip_record(diag,rel,"gitignored",is_dir=False)
                else: kept.append(p)
            files=kept
            diag["files_considered"]=len(files)
    diag["modules"]=sorted(diag["modules"])[:500]
    diag["extension_counts"]={k:v for k,v in diag["extension_counts"].most_common()}
    return files,diag


def _match_files(root: Path, files: list[Path], action: dict, limits: dict) -> dict:
    query=action["query"]; regex_mode=bool(action.get("regex",False)); max_matches=int(action.get("max_matches",500)); context=int(action.get("context_lines",4))
    try: pattern=re.compile(query) if regex_mode else None
    except re.error as exc: raise ValueError(f"invalid search regex: {exc}") from exc
    max_bytes=int(limits.get("max_search_file_bytes",64*1024*1024))
    matches=[]; total=0; searched=0; skipped=[]; ext=Counter(); truncated=False
    for path in files:
        rel=_search_rel(root,path)
        try:
            size=path.stat().st_size
            if size > max_bytes:
                skipped.append((rel,f"oversize>{max_bytes}")); continue
            if not _looks_text(path):
                skipped.append((rel,"binary_or_nontext")); continue
            text=path.read_text(encoding='utf-8',errors='replace')
        except OSError as exc:
            skipped.append((rel,f"read_error:{type(exc).__name__}")); continue
        searched += 1; ext[path.suffix.lower() or '<no-ext>'] += 1
        lines=text.splitlines()
        for idx,line in enumerate(lines):
            hit=bool(pattern.search(line)) if pattern else query in line
            if not hit: continue
            total += 1
            if len(matches) < max_matches:
                start=max(0,idx-context); end=min(len(lines),idx+context+1)
                matches.append({"path":rel,"line":idx+1,"context":[(n+1,lines[n],n==idx) for n in range(start,end)]})
            else:
                truncated=True
    return {"matches":matches,"match_count":total,"truncated":truncated,"files_searched":searched,"content_skips":skipped[:250],"content_skip_count":len(skipped),"searched_extension_counts":dict(ext.most_common())}


def _rg_candidate_files(root: Path, scopes: list[tuple[str,Path,str]], action: dict, limits: dict) -> tuple[list[Path], dict]:
    rg=shutil.which('rg')
    diag={"backend":"rg","available":bool(rg),"error":None,"candidate_files":0,"truncated":False}
    if not rg: return [],diag
    if action.get("source_scope") == "git_tracked":
        diag["error"]="rg primary disabled for source_scope=git_tracked"; return [],diag
    cmd=[rg,'-l','-0','--no-messages','--hidden']
    if not action.get('respect_gitignore',False): cmd.append('--no-ignore')
    if action.get('follow_symlinks',False): cmd.append('--follow')
    for name in sorted(SEARCH_DEFAULT_EXCLUDED_DIRS): cmd += ['-g',f'!**/{name}/**']
    for prefix in IGNORED_REL_PREFIXES: cmd += ['-g',f'!{prefix}**']
    if not action.get('regex',False): cmd.append('--fixed-strings')
    cmd += ['-e',action['query'],'--']
    cmd += [str(p if p.is_absolute() else root/p) for _,p,_ in scopes]
    try:
        proc=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=SEARCH_BACKEND_TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        diag['error']=f"rg timeout after {SEARCH_BACKEND_TIMEOUT_SECONDS:g}s"; return [],diag
    if proc.returncode not in {0,1}:
        detail=proc.stderr.decode('utf-8',errors='replace').strip().replace('\n',' ')[:800]
        diag['error']=f"rg failed rc={proc.returncode}: {detail}"; return [],diag
    max_files=int(limits.get('max_search_files',250000)); out=[]; seen=set()
    for raw in proc.stdout.split(b'\0'):
        if not raw: continue
        p=Path(raw.decode('utf-8',errors='surrogateescape'))
        if not p.is_absolute(): p=root/p
        try: key=p.resolve(strict=True).as_posix(); p.resolve(strict=True).relative_to(root.resolve(strict=True))
        except Exception: continue
        if key in seen: continue
        seen.add(key); out.append(p)
        if len(out)>=max_files:
            diag['truncated']=True; break
    diag['candidate_files']=len(out)
    return out,diag


def _candidate_filename_hits(query: str, filenames: list[str]) -> list[str]:
    tokens=[x.lower() for x in re.findall(r'[A-Z]?[a-z]+|[A-Z]+(?![a-z])|\d+',query) if len(x)>=3]
    tokens=[x for x in tokens if x not in {'cmd','msg','req'}]
    if not tokens:
        tokens=[query.lower()] if len(query)>=3 else []
    hits=[]
    for rel in filenames:
        name=Path(rel).name.lower()
        if any(t in name for t in tokens):
            hits.append(rel)
            if len(hits)>=30: break
    return hits


def _format_search_report(root: Path, action: dict, scopes, primary: dict, fallback: dict|None, coverage: dict, canonical: dict, *, inconsistency: bool, incomplete_reasons: list[str]) -> str:
    query=action['query']; regex_mode=bool(action.get('regex',False)); lines=["# Search results","",f"Query: `{query}`",f"Regex: {regex_mode}",f"Matches: {canonical['match_count']}",""]
    lines += ["=== SEARCH COVERAGE ===","", "Requested:"]
    for rel,path,kind in scopes:
        lines.append(f"  [{kind.upper()}] {rel}")
    lines += ["", "Resolved:"]
    for rel,path,kind in scopes: lines.append(f"  {path.resolve(strict=True)}")
    lines += ["",f"Source scope: {action.get('source_scope','filesystem')}",f"Backend requested: {action.get('backend','auto')}",f"Primary backend: {primary.get('backend','python')}",f"Primary matches: {primary.get('match_count',0)}"]
    if fallback is not None:
        lines.append(f"Fallback backend: {fallback.get('backend','python-oswalk')}")
        lines.append(f"Fallback matches: {fallback.get('match_count',0)}")
    else: lines.append("Fallback backend: disabled")
    lines += [f"Directories visited: {coverage.get('directories_visited',0)}",f"Files considered: {coverage.get('files_considered',0)}",f"Files searched: {canonical.get('files_searched',0)}"]
    ext=canonical.get('searched_extension_counts') or {}
    if ext:
        lines.append("Files scanned by extension:")
        for k,v in list(ext.items())[:30]: lines.append(f"  {k}: {v}")
    if action.get('module_discovery',True):
        modules=coverage.get('modules') or []
        lines += ["", "Candidate modules/directories (depth<=3 or build marker):"]
        if modules:
            for module in modules[:80]:
                count=sum(1 for m in canonical['matches'] if m['path']==module or m['path'].startswith(module.rstrip('/')+'/'))
                lines.append(f"  {module}: {count}")
        else: lines.append("  (none discovered)")
    if action.get('report_skipped_dirs',True):
        lines += ["",f"Skipped directories: {coverage.get('skipped_dirs_count',0)}"]
        for row in coverage.get('skipped_dirs',[])[:120]: lines.append(f"  {row['path']} -> {row['reason']}")
        lines.append(f"Skipped/unreadable files: {coverage.get('skipped_files_count',0) + canonical.get('content_skip_count',0)}")
        for row in coverage.get('skipped_files',[])[:40]: lines.append(f"  {row['path']} -> {row['reason']}")
        for rel,reason in canonical.get('content_skips',[])[:80]: lines.append(f"  {rel} -> {reason}")
    status='INCONSISTENT' if inconsistency else ('PARTIAL' if incomplete_reasons else 'VERIFIED')
    lines += ["",f"Coverage status: {status}"]
    if incomplete_reasons:
        lines.append("Coverage/integrity notes:")
        for reason in incomplete_reasons: lines.append(f"  - {reason}")
    if inconsistency:
        lines += ["", "SEARCH_INCONSISTENCY", f"primary_matches={primary.get('match_count',0)}", f"fallback_matches={fallback.get('match_count',0) if fallback else 'n/a'}"]
    if canonical['match_count']==0 and action.get('diagnose_on_zero',True):
        lines += ["", "=== ZERO MATCH DIAGNOSTIC ===", "", "Requested roots:"]
        for rel,path,kind in scopes: lines.append(f"  {rel} -> {'exists' if path.exists() else 'missing'}")
        lines.append("Candidate filename evidence:")
        hits=_candidate_filename_hits(query,coverage.get('candidate_filenames',[]))
        if hits:
            for hit in hits: lines.append(f"  {hit}")
        else: lines.append("  (no related filenames in scanned coverage)")
        lines += [f"Symlink policy: {'follow safely' if action.get('follow_symlinks',False) else 'do not follow'}",f"Gitignore policy: {'respect' if action.get('respect_gitignore',False) else 'ignore .gitignore; scan filesystem'}",f"Search file limit reached: {bool(coverage.get('limit_reached'))}"]
        if status == 'VERIFIED': lines.append("Zero-result interpretation: VERIFIED absence within the declared searchable filesystem scope.")
        else: lines.append("Zero-result interpretation: UNTRUSTED; zero matches is a search result, not proof of absence.")
    lines += ["", "=== MATCH DETAILS ===", ""]
    for item in canonical['matches']:
        lines.append(f"## {item['path']}:{item['line']}"); lines.append("```text")
        for n,content,is_hit in item['context']:
            lines.append(f"{'>' if is_hit else ' '}{n:6d}: {content}")
        lines += ["```",""]
    if canonical.get('truncated'): lines.append(f"[TRUNCATED at max_matches={action.get('max_matches',500)}]")
    return "\n".join(lines)+"\n"


def _search_action_payload(root: Path, action: dict, limits: dict) -> dict:
    scopes=_search_scope_list(root,action)
    backend=action.get('backend','auto')
    fallback_enabled=bool(action.get('fallback_search',True))
    primary_meta={"backend":"python-stack"}
    primary_cov=None
    if backend in {'auto','rg'}:
        rg_files,rg_diag=_rg_candidate_files(root,scopes,action,limits)
        if rg_diag.get('available') and not rg_diag.get('error'):
            primary=_match_files(root,rg_files,action,limits); primary.update({"backend":"rg","backend_diag":rg_diag})
        elif backend=='rg':
            primary={"matches":[],"match_count":0,"truncated":False,"files_searched":0,"content_skips":[],"content_skip_count":0,"searched_extension_counts":{},"backend":"rg","backend_diag":rg_diag}
            primary_meta['error']=rg_diag.get('error') or 'rg unavailable'
        else:
            pfiles,primary_cov=_discover_search_files(root,scopes,action,limits,walker='stack')
            primary=_match_files(root,pfiles,action,limits); primary['backend']='python-stack'; primary['backend_diag']=rg_diag
    else:
        pfiles,primary_cov=_discover_search_files(root,scopes,action,limits,walker='stack')
        primary=_match_files(root,pfiles,action,limits); primary['backend']='python-stack'
    fallback=None; coverage=primary_cov or {"directories_visited":0,"files_considered":0,"errors":[],"limit_reached":False,"skipped_dirs":[],"skipped_dirs_count":0,"skipped_files":[],"skipped_files_count":0,"modules":[],"extension_counts":{},"candidate_filenames":[]}
    if fallback_enabled:
        ffiles,fcoverage=_discover_search_files(root,scopes,action,limits,walker='oswalk')
        fallback=_match_files(root,ffiles,action,limits); fallback['backend']='python-oswalk'; coverage=fcoverage
    pcount=int(primary.get('match_count',0)); fcount=int(fallback.get('match_count',0)) if fallback else None
    canonical = fallback if fallback is not None and fcount >= pcount else primary
    inconsistency=False
    if fallback is not None:
        # A zero/non-zero disagreement is always dangerous. Exact count mismatch is
        # meaningful only when neither side hit max_matches/report truncation.
        if (pcount==0) != (fcount==0): inconsistency=True
        elif not primary.get('truncated') and not fallback.get('truncated') and pcount != fcount: inconsistency=True
    reasons=[]
    if primary_meta.get('error'): reasons.append(primary_meta['error'])
    if coverage.get('limit_reached'): reasons.append(f"search coverage hit max_search_files={limits.get('max_search_files')}")
    if coverage.get('errors'): reasons.extend(str(x) for x in coverage['errors'][:8])
    if canonical.get('content_skip_count',0): reasons.append(f"{canonical['content_skip_count']} file(s) were not content-scanned (binary/oversize/read error)")
    if not fallback_enabled: reasons.append("fallback_search=false; independent zero verification disabled")
    if inconsistency: reasons.append("primary and fallback backends disagree")
    must_fail=bool(action.get('must_find',False) and canonical.get('match_count',0)==0)
    zero_unverified=bool(canonical.get('match_count',0)==0 and (inconsistency or reasons))
    incomplete=bool(inconsistency or must_fail or zero_unverified)
    if must_fail: reasons.append(f"must_find=true but query produced zero matches")
    report=_format_search_report(root,action,scopes,primary,fallback,coverage,canonical,inconsistency=inconsistency,incomplete_reasons=reasons)
    return {"report":report,"incomplete":incomplete,"inconsistency":inconsistency,"must_find_failed":must_fail,"matches":canonical.get('match_count',0),"coverage_status":"INCONSISTENT" if inconsistency else ('PARTIAL' if reasons else 'VERIFIED'),"reasons":reasons}


def _search_action_direct(root: Path, action: dict, limits: dict) -> str:
    """Compatibility wrapper used by tests and the isolated regex worker."""
    return _search_action_payload(root,action,limits)["report"]


def _search_action(root: Path, action: dict, limits: dict) -> dict:
    """Run regex search out-of-process; literal search runs in-process."""
    if not action.get("regex", False):
        return _search_action_payload(root, action, limits)
    worker = Path(__file__).resolve().parent / "python_patch_collect_regex_worker.py"
    if not worker.is_file(): raise ValueError("regex search worker is missing")
    with tempfile.TemporaryDirectory(prefix="ptv-collect-regex-") as td:
        work=Path(td); request_path=work/"request.json"; result_path=work/"result.json"
        request_path.write_text(json.dumps({"action":action,"limits":limits},ensure_ascii=False),encoding="utf-8")
        env=dict(os.environ); env["PYTHONDONTWRITEBYTECODE"]="1"
        try:
            proc=subprocess.run([sys.executable,str(worker),"--project-root",str(root),"--request",str(request_path),"--result",str(result_path)],cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=REGEX_SEARCH_TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired as exc:
            raise ValueError(f"regex search exceeded hard timeout ({REGEX_SEARCH_TIMEOUT_SECONDS:g}s); narrow paths/query") from exc
        if proc.returncode != 0:
            detail=(proc.stdout or '').strip().replace('\n',' ')[:800]; raise ValueError(f"regex search worker failed rc={proc.returncode}: {detail}")
        if not result_path.is_file() or result_path.is_symlink(): raise ValueError("regex search worker produced no safe result")
        size=result_path.stat().st_size; hard=max(int(limits.get('max_report_bytes',0)),1024*1024)*2
        if size>hard: raise ValueError(f"regex search worker result exceeded safety cap ({size} bytes)")
        data=json.loads(result_path.read_text(encoding='utf-8'))
        if not isinstance(data,dict) or not isinstance(data.get('report'),str): raise ValueError("regex search worker returned invalid payload")
        return data


def _git_action(root: Path, action: dict) -> str:
    if not (root / ".git").exists():
        return "# Git context\n\nNot a Git working tree at the project root.\n"
    commands = {
        "status": ["git", "-c", "core.fsmonitor=false", "status", "--short", "--branch"],
        "log": ["git", "log", "--oneline", "--decorate", "-n", str(action.get("log_entries", 20))],
        "diff_stat": ["git", "-c", "core.fsmonitor=false", "diff", "--stat", "--no-ext-diff", "--no-textconv"],
        "diff": ["git", "-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv", "--unified=3"],
    }
    blocks = ["# Git context", ""]
    for section in action["sections"]:
        cmd = commands[section]
        try:
            proc = subprocess.run(
                cmd, cwd=root, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                text=True, errors="replace", timeout=30,
            )
        except subprocess.TimeoutExpired:
            blocks += [f"## {section}", "```text", "[GIT COMMAND FAILED: timeout after 30s]", "```", ""]
            continue
        text = (proc.stdout or "").rstrip()
        if proc.returncode != 0:
            detail = (proc.stderr or "").strip().replace("\n", " ")[:800]
            text = f"[GIT COMMAND FAILED rc={proc.returncode}]" + (f" {detail}" if detail else "")
        elif proc.stderr:
            warning = proc.stderr.strip().replace("\n", " ")[:800]
            if warning:
                text = (text + "\n" if text else "") + f"[git warning] {warning}"
        blocks += [f"## {section}", "```text", text, "```", ""]
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
                    search_result = _search_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, search_result["report"])
                    if search_result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(search_result.get("reasons") or []))
                elif kind == "git":
                    builder.add_report(index, kind, title, _git_action(root, action))
                else:
                    raise ValueError(f"unsupported action after schema validation: {kind}")
            collection_status = builder.collection_status
            result = builder.finish()
            try:
                archived, lifecycle = _archive_request(root, request_zip, execution_request, request_sha)
            except Exception:
                try: result.unlink()
                except OSError: pass
                raise
            return result, archived, len(request_data["actions"]), lifecycle, collection_status
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
        result, archived, action_count, queue_lifecycle, collection_status = _run_request(root, request_zip)
    except Exception as exc:
        print(f"ERROR: collection failed: {exc}", file=sys.stderr)
        return 2
    if collection_status == "INCOMPLETE":
        print(f"COLLECT: INCOMPLETE | completed {action_count} action(s); inspect SEARCH COVERAGE before inference", flush=True)
    else:
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
    return 3 if collection_status == "INCOMPLETE" else 0


if __name__ == "__main__":
    raise SystemExit(main())
