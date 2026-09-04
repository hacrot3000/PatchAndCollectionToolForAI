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
from python_patch_database_select import DatabaseSelectError, execute_database_select

VERSION = "6.20.0"
REQUEST_RE = re.compile(r"^CODE_COLLECTION_REQUEST(?:_[A-Za-z0-9._-]+)?\.json$", re.I)
MAX_REQUEST_JSON_BYTES = 1024 * 1024
REGEX_SEARCH_TIMEOUT_SECONDS = 60.0
SEARCH_BACKEND_TIMEOUT_SECONDS = 60.0
REGEX_SEARCH_SOFT_TIMEOUT_MARGIN_SECONDS = 3.0
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

# v6.19.2 database profiles are operator-local configuration and form a hard
# evidence boundary.  Unlike generic sensitive source (which may be included
# exactly with an explicit warning), these profile files must never be copied
# into COLLECT output or searched for content.
LOCAL_DB_PROFILE_REL_PATHS = {
    "tools/db_profiles.local.json",
    ".python_patch_tool/db_profiles.local.json",
}
DB_PROFILE_ENV = "PTV_DB_PROFILES_FILE"

def _protected_local_profile_rel(root: Path, path: Path | str) -> str | None:
    try:
        root_real = root.resolve(strict=True)
        candidate = Path(path)
        if not candidate.is_absolute():
            candidate = root_real / candidate
        candidate_abs = candidate.resolve(strict=False)
        rel = candidate_abs.relative_to(root_real).as_posix()
    except Exception:
        return None
    if rel in LOCAL_DB_PROFILE_REL_PATHS:
        return rel
    raw = os.environ.get(DB_PROFILE_ENV)
    if raw:
        try:
            override = Path(raw).expanduser()
            if not override.is_absolute():
                override = root_real / override
            if override.resolve(strict=False) == candidate_abs:
                return rel
        except Exception:
            pass
    return None

def _is_protected_local_profile(root: Path, path: Path | str) -> bool:
    return _protected_local_profile_rel(root, path) is not None


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
    if _is_protected_local_profile(root, candidate):
        raise ValueError(f"refusing to collect local database profile: {rel.as_posix()}")
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
                    if _is_internal_output_path(root, entry) or _is_protected_local_profile(root, entry):
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
        from python_patch_ai_sync import decide_sync
        self.ai_sync_decision = decide_sync(
            root,
            ai_context=request_data.get("ai_context"),
            fallback_known_tool_version=None,
            channel="collect",
        )
        self.ai_sync_manifest: dict[str, object] | None = None
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
        if _is_protected_local_profile(self.root, src):
            raise ValueError(f"refusing to collect local database profile: {rel}")
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

    def add_discovered_file(self, rel: str, src: Path, *, source_action: int) -> tuple[bool, str | None]:
        """Best-effort collection for discovery-driven actions.

        Exact ``pack`` remains fail-closed.  Discovery actions instead preserve
        files collected before a quota boundary and mark the result INCOMPLETE.
        Integrity failures while copying still raise.
        """
        if _is_protected_local_profile(self.root, src):
            return False, "local_database_profile_excluded"
        if rel in self._added_files:
            return True, None
        try:
            size=src.stat().st_size
        except OSError as exc:
            return False, f"cannot stat discovered file {rel}: {type(exc).__name__}"
        if size > self.limits["max_file_bytes"]:
            return False, f"discovered file omitted: {rel} exceeds max_file_bytes={self.limits['max_file_bytes']}"
        if self.file_count >= self.limits["max_files"]:
            return False, f"discovered files truncated at max_files={self.limits['max_files']}"
        if self.total_bytes + size > self.limits["max_total_bytes"]:
            return False, f"discovered files truncated at max_total_bytes={self.limits['max_total_bytes']}"
        self.add_exact_file(rel,src,source_action=source_action)
        return True, None

    def add_generated_artifact(self, src: Path, arcname: str, *, source_action: int) -> tuple[bool, str | None]:
        """Add a tool-generated evidence file under an explicit archive path.

        Generated DB/query artifacts are never treated as project source and are
        still bounded by the same global file/byte package quotas.  Hitting a
        package quota is fail-partial for generated evidence: the caller keeps
        earlier chunks and marks the COLLECT result INCOMPLETE.
        """
        if not isinstance(arcname, str) or not arcname or arcname.startswith('/') or '..' in PurePosixPath(arcname).parts:
            raise ValueError(f"unsafe generated artifact archive path: {arcname}")
        try:
            st = src.lstat()
        except OSError as exc:
            raise ValueError(f"generated artifact missing/unreadable: {src} ({type(exc).__name__})") from exc
        attrs = int(getattr(st, 'st_file_attributes', 0) or 0)
        reparse = int(getattr(stat, 'FILE_ATTRIBUTE_REPARSE_POINT', 0x400))
        if stat.S_ISLNK(st.st_mode) or (os.name == 'nt' and attrs & reparse) or not stat.S_ISREG(st.st_mode):
            raise ValueError(f"generated artifact must be a regular non-symlink file: {src}")
        size = st.st_size
        if size > self.limits['max_file_bytes']:
            return False, f"generated artifact omitted: {arcname} exceeds max_file_bytes={self.limits['max_file_bytes']}"
        if self.file_count >= self.limits['max_files']:
            return False, f"generated artifacts truncated at max_files={self.limits['max_files']}"
        if self.total_bytes + size > self.limits['max_total_bytes']:
            return False, f"generated artifacts truncated at max_total_bytes={self.limits['max_total_bytes']}"
        digest = hashlib.sha256()
        copied = 0
        with src.open('rb') as source, self.zf.open(arcname, 'w') as target:
            for chunk in iter(lambda: source.read(1024 * 1024), b''):
                target.write(chunk); digest.update(chunk); copied += len(chunk)
        after = src.stat()
        if (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns):
            raise ValueError(f"generated artifact changed while being packaged: {src}")
        self.file_count += 1
        self.total_bytes += copied
        self.entries.append({
            'path': arcname,
            'archive_path': arcname,
            'size': copied,
            'sha256': digest.hexdigest(),
            'source_action': source_action,
            'generated': True,
        })
        return True, None

    def add_report(self, index: int, kind: str, title: str, text: str) -> None:
        text = _redact_text(text)
        raw = text.encode("utf-8", errors="replace")
        remaining = self.limits["max_report_bytes"] - self.report_bytes
        if remaining <= 0:
            self.mark_incomplete(action=index,kind=kind,reasons=[f"report omitted because max_report_bytes={self.limits['max_report_bytes']} was exhausted"])
            return
        # Any report truncation means evidence was intentionally omitted.  Keep
        # the report/result ZIP, but do not advertise the collection as complete.
        truncated = "[TRUNCATED" in text or "[PARTIAL RESULT" in text
        reasons=[]
        if "[TRUNCATED" in text:
            reasons.append(f"{kind} report/action limit omitted additional evidence")
        if "[PARTIAL RESULT" in text:
            reasons.append(f"{kind} report contains bounded partial evidence")
        if len(raw) > remaining:
            raw = raw[:remaining]
            text = raw.decode("utf-8", errors="ignore") + "\n\n[TRUNCATED BY max_report_bytes]\n"
            raw = text.encode("utf-8")
            truncated = True
            reasons.append(f"report truncated by max_report_bytes={self.limits['max_report_bytes']}")
        arcname = f"reports/{index:03d}_{kind}.md"
        self.zf.writestr(arcname, raw)
        self.report_bytes += len(raw)
        self.reports.append({"action": index, "type": kind, "title": title, "archive_path": arcname, "truncated": truncated})
        if reasons:
            self.mark_incomplete(action=index,kind=kind,reasons=list(dict.fromkeys(reasons)))

    def mark_incomplete(self, *, action: int, kind: str, reasons: list[str]) -> None:
        self.collection_status = "INCOMPLETE"
        self.collection_warnings.append({"action": action, "type": kind, "reasons": list(reasons)})

    def finish(self) -> Path:
        from python_patch_ai_sync import write_sync_bundle_to_zip
        self.ai_sync_manifest = write_sync_bundle_to_zip(self.zf, self.root, self.ai_sync_decision)
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
            "ai_tool_sync": self.ai_sync_manifest,
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
        try:
            from python_patch_cleartext_companion import create_zip_cleartext_companion
            create_zip_cleartext_companion(self.final, artifact_kind="COLLECT RESULT")
        except Exception:
            # The clear-text companion is part of the v6.19.2 COLLECT deliverable.
            # Avoid publishing a ZIP-only success that would violate the output contract.
            try:
                self.final.unlink()
            except OSError:
                pass
            raise
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


def _globstar_compat_patterns(pattern: str) -> list[str]:
    """Return additive fnmatch variants where ``**/`` may match zero directories.

    Python's :mod:`fnmatch` treats ``**`` like two ordinary ``*`` tokens, so
    ``**/*.java`` does not match ``Foo.java``.  Historical COLLECT requests and
    common shell/pathlib glob expectations treat globstar as *zero or more*
    directory levels.  Keep the original pattern (preserving every previous
    match) and add only the zero-directory variants.
    """
    normalized = str(pattern).replace("\\", "/")
    while normalized.startswith("./"):
        normalized = normalized[2:]
    out = [normalized]
    queue = [normalized]
    seen = {normalized}
    while queue:
        current = queue.pop(0)
        start = 0
        while True:
            pos = current.find("**/", start)
            if pos < 0:
                break
            candidate = current[:pos] + current[pos + 3:]
            if candidate not in seen:
                seen.add(candidate)
                out.append(candidate)
                queue.append(candidate)
            start = pos + 1
    return out


def _matches_discovery_pattern(*, name: str, local_rel: str, project_rel: str, pattern: str) -> bool:
    """Match a discovery glob without narrowing historical behavior.

    A path-bearing ``find`` pattern is naturally relative to each requested
    scope.  Older self-contained code checked only basename and the full
    project-relative path, causing false zero results for requests such as
    ``paths=["projects/.../jdqs_server"]`` plus
    ``patterns=["src/main/java/.../*.java"]``.

    Test all three historical/useful views (basename, scope-relative path,
    project-relative path) and globstar zero-directory variants.
    """
    candidates = []
    for value in (str(name), str(local_rel), str(project_rel)):
        value = value.replace("\\", "/")
        if value not in candidates:
            candidates.append(value)
    for variant in _globstar_compat_patterns(pattern):
        if any(fnmatch.fnmatchcase(candidate, variant) for candidate in candidates):
            return True
    return False


def _find_action_result(root: Path, action: dict, limits: dict) -> dict:
    """Coverage-aware filename/path discovery used by ``find``.

    Discovery traversal uses ``max_search_files`` rather than the collection
    quota ``max_files``.  The latter still bounds how many files a collect=true
    action may package, but it must not silently truncate the tree being
    searched for filenames.
    """
    matches: list[tuple[str, Path]] = []
    requested_max = int(action.get("max_results", 1000))
    max_results = min(requested_max, int(limits["max_files"])) if action.get("collect") else requested_max
    max_scan_files = int(limits.get("max_search_files", 250000))
    seen: set[str] = set()
    scanned = 0
    limit_reached = False
    resolved_scopes: list[str] = []

    for raw_scope in action.get("paths", ["."]):
        scope_rel, scope = _resolve_scope(root, raw_scope, file_ok=True)
        resolved_scopes.append(scope_rel)
        remaining = max_scan_files - scanned
        if remaining <= 0:
            limit_reached = True
            break
        # Ask for one sentinel entry beyond the remaining budget so an exact
        # boundary is not incorrectly reported as complete coverage.
        for path in _iter_files(root, scope, max_files=remaining + 1):
            scanned += 1
            if scanned > max_scan_files:
                limit_reached = True
                break
            rel = _project_rel(root, path)
            if rel in seen:
                continue
            try:
                local_rel = path.relative_to(scope).as_posix() if scope.is_dir() else path.name
            except ValueError:
                local_rel = path.name
            if any(
                _matches_discovery_pattern(
                    name=path.name, local_rel=local_rel, project_rel=rel, pattern=pat
                )
                for pat in action["patterns"]
            ):
                seen.add(rel)
                matches.append((rel, path))
                if len(matches) >= max_results:
                    break
        if limit_reached or len(matches) >= max_results:
            break

    coverage = "PARTIAL" if limit_reached else "VERIFIED"
    lines = [
        "# Find results",
        "",
        f"Patterns: `{action['patterns']}`",
        f"Matches: {len(matches)}",
        "",
        "=== FIND COVERAGE ===",
        "",
        f"Requested scopes: {len(action.get('paths', ['.']))}",
        f"Resolved scopes: {', '.join(resolved_scopes) if resolved_scopes else '(none)'}",
        f"Files considered: {min(scanned, max_scan_files)}",
        f"Discovery budget: max_search_files={max_scan_files}",
        "Pattern semantics: basename + scope-relative path + project-relative path; **/ may match zero or more directories.",
        f"Coverage status: {coverage}",
        "",
    ]
    lines += [f"- `{rel}`" for rel, _ in matches]
    if len(matches) >= max_results:
        lines.append(f"[TRUNCATED at max_results={max_results}]")
    if limit_reached:
        lines.append(f"[INCOMPLETE: filename discovery hit max_search_files={max_scan_files}]")
    return {
        "report": "\n".join(lines) + "\n",
        "matches": matches,
        "incomplete": limit_reached,
        "reasons": [f"find coverage hit max_search_files={max_scan_files}"] if limit_reached else [],
        "files_considered": min(scanned, max_scan_files),
        "coverage_status": coverage,
    }


def _find_action(root: Path, action: dict, limits: dict) -> tuple[str, list[tuple[str, Path]]]:
    """Backward-compatible two-value wrapper used by older internal tests/API."""
    result = _find_action_result(root, action, limits)
    return result["report"], result["matches"]


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


def _discover_search_files(root: Path, scopes: list[tuple[str, Path, str]], action: dict, limits: dict, *, walker: str, deadline: float | None = None) -> tuple[list[Path], dict]:
    max_files=int(limits.get("max_search_files", 250000))
    follow=bool(action.get("follow_symlinks", False))
    diag={
        "walker": walker, "directories_visited": 0, "files_considered": 0,
        "files_searched": 0, "limit_reached": False, "time_limit_reached": False, "errors": [],
        "skipped_dirs": [], "skipped_dirs_count": 0, "skipped_files": [], "skipped_files_count": 0,
        "top_dirs": {}, "modules": set(), "extension_counts": Counter(), "candidate_filenames": [],
    }
    files=[]; seen_files=set(); seen_dirs=set()

    def out_of_time() -> bool:
        if deadline is not None and time.monotonic() >= deadline:
            diag["time_limit_reached"] = True
            return True
        return False

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
            if out_of_time(): break
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
        if out_of_time():
            return False
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
        if diag["limit_reached"] or out_of_time(): break
        if scope.is_file():
            add_file(scope, scope_rel); continue
        if walker == "oswalk":
            for current, dirs, names in os.walk(scope, topdown=True, followlinks=follow):
                if out_of_time(): break
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
                    if out_of_time(): break
                    p=cur/name
                    if p.is_symlink() and not follow:
                        _search_skip_record(diag,_search_rel(root,p),"symlink_follow_disabled",is_dir=False); continue
                    if _is_internal_output_path(root,p):
                        _search_skip_record(diag,_search_rel(root,p),"patch_tool_internal",is_dir=False); continue
                    if _is_protected_local_profile(root,p):
                        _search_skip_record(diag,_search_rel(root,p),"local_database_profile",is_dir=False); continue
                    if not add_file(p,scope_rel): break
                if diag["limit_reached"]: break
        else:
            stack=[scope]
            while stack and not diag["limit_reached"] and not out_of_time():
                current=stack.pop()
                if current.is_file(): add_file(current,scope_rel); continue
                if not note_dir(current,scope): continue
                try: entries=sorted(os.scandir(current), key=lambda e:e.name.lower(), reverse=True)
                except OSError as exc:
                    diag["errors"].append(f"{_search_rel(root,current)}: {type(exc).__name__}: {exc}"); continue
                for entry in entries:
                    if out_of_time(): break
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
                            if _is_protected_local_profile(root,p):
                                _search_skip_record(diag,rel,"local_database_profile",is_dir=False); continue
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


def _match_files(root: Path, files: list[Path], action: dict, limits: dict, *, deadline: float | None = None) -> dict:
    query=action["query"]; regex_mode=bool(action.get("regex",False)); max_matches=int(action.get("max_matches",500)); context=int(action.get("context_lines",4))
    try: pattern=re.compile(query) if regex_mode else None
    except re.error as exc: raise ValueError(f"invalid search regex: {exc}") from exc
    max_bytes=int(limits.get("max_search_file_bytes",64*1024*1024))
    matches=[]; total=0; searched=0; skipped=[]; ext=Counter(); truncated=False; skip_reasons=Counter()
    time_limit_reached=False; processed_files=0; stop=False
    for path in files:
        if deadline is not None and time.monotonic() >= deadline:
            time_limit_reached=True; break
        rel=_search_rel(root,path)
        try:
            size=path.stat().st_size
            if size > max_bytes:
                why=f"oversize>{max_bytes}"; skipped.append((rel,why)); skip_reasons[why] += 1; processed_files += 1; continue
            if not _looks_text(path):
                why="binary_or_nontext"; skipped.append((rel,why)); skip_reasons[why] += 1; processed_files += 1; continue
            text=path.read_text(encoding='utf-8',errors='replace')
        except OSError as exc:
            why=f"read_error:{type(exc).__name__}"; skipped.append((rel,why)); skip_reasons[why] += 1; processed_files += 1; continue
        searched += 1; processed_files += 1; ext[path.suffix.lower() or '<no-ext>'] += 1
        lines=text.splitlines()
        for idx,line in enumerate(lines):
            if deadline is not None and time.monotonic() >= deadline:
                time_limit_reached=True; stop=True; break
            hit=bool(pattern.search(line)) if pattern else query in line
            if not hit: continue
            total += 1
            if len(matches) < max_matches:
                start=max(0,idx-context); end=min(len(lines),idx+context+1)
                matches.append({"path":rel,"line":idx+1,"context":[(n+1,lines[n],n==idx) for n in range(start,end)]})
            else:
                truncated=True
        if stop: break
    return {
        "matches":matches,
        "match_count":total,
        "truncated":truncated,
        "files_searched":searched,
        "content_skips":skipped[:250],
        "content_skip_count":len(skipped),
        "content_skip_reason_counts":dict(skip_reasons),
        "searched_extension_counts":dict(ext.most_common()),
        "time_limit_reached":time_limit_reached,
        "files_input":len(files),
        "files_processed":processed_files,
        "files_remaining":max(0,len(files)-processed_files),
    }

def _rg_candidate_files(root: Path, scopes: list[tuple[str,Path,str]], action: dict, limits: dict, *, deadline: float | None = None) -> tuple[list[Path], dict]:
    rg=shutil.which('rg')
    diag={"backend":"rg","available":bool(rg),"error":None,"candidate_files":0,"truncated":False,"time_limit_reached":False}
    if not rg: return [],diag
    if action.get("source_scope") == "git_tracked":
        diag["error"]="rg primary disabled for source_scope=git_tracked"; return [],diag
    cmd=[rg,'-l','-0','--no-messages','--hidden']
    if not action.get('respect_gitignore',False): cmd.append('--no-ignore')
    if action.get('follow_symlinks',False): cmd.append('--follow')
    for name in sorted(SEARCH_DEFAULT_EXCLUDED_DIRS): cmd += ['-g',f'!**/{name}/**']
    for prefix in IGNORED_REL_PREFIXES: cmd += ['-g',f'!{prefix}**']
    for rel in sorted(LOCAL_DB_PROFILE_REL_PATHS): cmd += ['-g',f'!{rel}']
    raw_profile = os.environ.get(DB_PROFILE_ENV)
    if raw_profile:
        try:
            override = Path(raw_profile).expanduser()
            if not override.is_absolute(): override = root / override
            rel_override = override.resolve(strict=False).relative_to(root.resolve(strict=True)).as_posix()
            cmd += ['-g',f'!{rel_override}']
        except Exception:
            pass
    if not action.get('regex',False): cmd.append('--fixed-strings')
    cmd += ['-e',action['query'],'--']
    cmd += [str(p if p.is_absolute() else root/p) for _,p,_ in scopes]
    timeout=SEARCH_BACKEND_TIMEOUT_SECONDS
    if deadline is not None:
        remaining=deadline-time.monotonic()
        if remaining <= 0:
            diag['error']='search soft time budget exhausted before rg'; diag['time_limit_reached']=True; return [],diag
        timeout=min(timeout,max(0.05,remaining))
    try:
        proc=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=timeout)
    except subprocess.TimeoutExpired:
        diag['error']=f"rg timeout after {timeout:g}s"; diag['time_limit_reached']=True; return [],diag
    if proc.returncode not in {0,1}:
        detail=proc.stderr.decode('utf-8',errors='replace').strip().replace('\n',' ')[:800]
        diag['error']=f"rg failed rc={proc.returncode}: {detail}"; return [],diag
    max_files=int(limits.get('max_search_files',250000)); out=[]; seen=set()
    for raw in proc.stdout.split(b'\0'):
        if deadline is not None and time.monotonic() >= deadline:
            diag['time_limit_reached']=True; break
        if not raw: continue
        p=Path(raw.decode('utf-8',errors='surrogateescape'))
        if not p.is_absolute(): p=root/p
        try: key=p.resolve(strict=True).as_posix(); p.resolve(strict=True).relative_to(root.resolve(strict=True))
        except Exception: continue
        if _is_protected_local_profile(root,p): continue
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


def _empty_search_coverage() -> dict:
    return {
        "directories_visited":0,"files_considered":0,"errors":[],"limit_reached":False,
        "time_limit_reached":False,"skipped_dirs":[],"skipped_dirs_count":0,
        "skipped_files":[],"skipped_files_count":0,"modules":[],"extension_counts":{},
        "candidate_filenames":[],
    }


def _format_search_report(
    root: Path,
    action: dict,
    scopes,
    primary: dict,
    fallback: dict|None,
    coverage: dict,
    canonical: dict,
    *,
    inconsistency: bool,
    incomplete_reasons: list[str],
    fallback_note: str | None = None,
) -> str:
    query=action['query']; regex_mode=bool(action.get('regex',False))
    execution_status='INCONSISTENT' if inconsistency else ('PARTIAL' if incomplete_reasons else 'COMPLETED')
    lines=[
        "# Search results","",f"Query: `{query}`",f"Regex: {regex_mode}",
        f"Search execution status: {execution_status}",f"Matches: {canonical.get('match_count',0)}","",
    ]
    lines += ["=== SEARCH COVERAGE ===","", "Requested:"]
    for rel,path,kind in scopes:
        lines.append(f"  [{kind.upper()}] {rel}")
    lines += ["", "Resolved:"]
    for rel,path,kind in scopes: lines.append(f"  {path.resolve(strict=True)}")
    lines += [
        "",f"Source scope: {action.get('source_scope','filesystem')}",
        f"Backend requested: {action.get('backend','auto')}",
        f"Primary backend: {primary.get('backend','python')}",
        f"Primary matches: {primary.get('match_count',0)}",
    ]
    if fallback is not None:
        lines.append(f"Fallback backend: {fallback.get('backend','python-oswalk')}")
        lines.append(f"Fallback matches: {fallback.get('match_count',0)}")
    elif fallback_note:
        lines.append(f"Fallback backend: {fallback_note}")
    else:
        lines.append("Fallback backend: disabled")
    lines += [
        f"Directories visited: {coverage.get('directories_visited',0)}",
        f"Files considered: {coverage.get('files_considered',0)}",
        f"Files searched: {canonical.get('files_searched',0)}",
    ]
    if canonical.get('time_limit_reached'):
        lines.append(f"Files remaining unsearched (estimated): {canonical.get('files_remaining',0)}")
    ext=canonical.get('searched_extension_counts') or {}
    if ext:
        lines.append("Files scanned by extension:")
        for k,v in list(ext.items())[:30]: lines.append(f"  {k}: {v}")
    if action.get('module_discovery',True):
        modules=coverage.get('modules') or []
        lines += ["", "Candidate modules/directories (depth<=3 or build marker):"]
        if modules:
            for module in modules[:80]:
                count=sum(1 for m in canonical.get('matches',[]) if m['path']==module or m['path'].startswith(module.rstrip('/')+'/'))
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
    if canonical.get('truncated'):
        omitted=max(0,int(canonical.get('match_count',0))-len(canonical.get('matches',[])))
        lines += ["",f"[PARTIAL RESULT: max_matches={action.get('max_matches',500)} reached; at least {omitted} additional match detail(s) omitted]"]
    if canonical.get('match_count',0)==0 and action.get('diagnose_on_zero',True):
        lines += ["", "=== ZERO MATCH DIAGNOSTIC ===", "", "Requested roots:"]
        for rel,path,kind in scopes: lines.append(f"  {rel} -> {'exists' if path.exists() else 'missing'}")
        lines.append("Candidate filename evidence:")
        hits=_candidate_filename_hits(query,coverage.get('candidate_filenames',[]))
        if hits:
            for hit in hits: lines.append(f"  {hit}")
        else: lines.append("  (no related filenames in scanned coverage)")
        lines += [
            f"Symlink policy: {'follow safely' if action.get('follow_symlinks',False) else 'do not follow'}",
            f"Gitignore policy: {'respect' if action.get('respect_gitignore',False) else 'ignore .gitignore; scan filesystem'}",
            f"Search file limit reached: {bool(coverage.get('limit_reached'))}",
            f"Search time limit reached: {bool(coverage.get('time_limit_reached') or canonical.get('time_limit_reached'))}",
        ]
        if status == 'VERIFIED': lines.append("Zero-result interpretation: VERIFIED absence within the declared searchable filesystem scope.")
        else: lines.append("Zero-result interpretation: UNTRUSTED; zero matches is a search result, not proof of absence.")
    lines += ["", "=== MATCH DETAILS ===", ""]
    for item in canonical.get('matches',[]):
        lines.append(f"## {item['path']}:{item['line']}"); lines.append("```text")
        for n,content,is_hit in item['context']:
            lines.append(f"{'>' if is_hit else ' '}{n:6d}: {content}")
        lines += ["```",""]
    return "\n".join(lines)+"\n"


def _search_result_payload(
    root: Path,
    action: dict,
    scopes,
    primary: dict,
    fallback: dict|None,
    coverage: dict,
    *,
    primary_error: str | None = None,
    fallback_enabled: bool = True,
    fallback_note: str | None = None,
    extra_reasons: list[str] | None = None,
    force_incomplete: bool = False,
) -> dict:
    pcount=int(primary.get('match_count',0)); fcount=int(fallback.get('match_count',0)) if fallback else None
    canonical = fallback if fallback is not None and fcount >= pcount else primary
    inconsistency=False
    if fallback is not None:
        if (pcount==0) != (fcount==0): inconsistency=True
        elif not primary.get('truncated') and not fallback.get('truncated') and not primary.get('time_limit_reached') and not fallback.get('time_limit_reached') and pcount != fcount:
            inconsistency=True
    reasons=list(extra_reasons or [])
    if primary_error: reasons.append(primary_error)
    if coverage.get('limit_reached'): reasons.append(f"search coverage hit max_search_files={action.get('_max_search_files_for_report','configured limit')}")
    if coverage.get('time_limit_reached'): reasons.append("search coverage inventory stopped at the action soft timeout")
    if coverage.get('errors'): reasons.extend(str(x) for x in coverage['errors'][:8])
    pdiag=primary.get('backend_diag') or {}
    if pdiag.get('truncated'): reasons.append("primary backend candidate list hit max_search_files; additional matching files may exist")
    if pdiag.get('time_limit_reached'): reasons.append("primary backend stopped at the action soft timeout")
    if canonical.get('time_limit_reached'):
        reasons.append(f"content scan stopped at the action soft timeout with about {canonical.get('files_remaining',0)} candidate file(s) remaining")
    reason_counts=canonical.get('content_skip_reason_counts') or {}
    significant_skip_count=sum(int(v) for k,v in reason_counts.items() if k != 'binary_or_nontext')
    if significant_skip_count:
        reasons.append(f"{significant_skip_count} searchable file(s) were not content-scanned due oversize/read errors")
    if canonical.get('truncated'):
        reasons.append(f"match details exceeded max_matches={action.get('max_matches',500)}; partial match evidence preserved")
    if not fallback_enabled and canonical.get('match_count',0)==0:
        reasons.append("fallback_search=false; independent zero verification disabled")
    if inconsistency: reasons.append("primary and fallback backends disagree")
    must_fail=bool(action.get('must_find',False) and canonical.get('match_count',0)==0)
    if must_fail: reasons.append("must_find=true but query produced zero matches")
    reasons=list(dict.fromkeys(str(x) for x in reasons if str(x)))
    incomplete=bool(force_incomplete or inconsistency or must_fail or reasons)
    report=_format_search_report(
        root,action,scopes,primary,fallback,coverage,canonical,
        inconsistency=inconsistency,incomplete_reasons=reasons,fallback_note=fallback_note,
    )
    return {
        "report":report,
        "incomplete":incomplete,
        "inconsistency":inconsistency,
        "must_find_failed":must_fail,
        "matches":canonical.get('match_count',0),
        "match_details":canonical.get('matches',[]),
        "coverage":coverage,
        "coverage_status":"INCONSISTENT" if inconsistency else ('PARTIAL' if incomplete else 'VERIFIED'),
        "execution_status":"INCONSISTENT" if inconsistency else ('PARTIAL' if incomplete else 'COMPLETED'),
        "reasons":reasons,
    }


def _search_action_payload(root: Path, action: dict, limits: dict, *, deadline: float | None = None, checkpoint_cb=None) -> dict:
    # Internal report-only value; it is never read from the request schema.
    action=dict(action)
    action['_max_search_files_for_report']=int(limits.get('max_search_files',250000))
    scopes=_search_scope_list(root,action)
    backend=action.get('backend','auto')
    fallback_enabled=bool(action.get('fallback_search',True))
    verify_nonzero=bool(action.get('verify_nonzero_with_fallback',False))
    primary_error=None; primary_cov=None

    if backend in {'auto','rg'}:
        rg_files,rg_diag=_rg_candidate_files(root,scopes,action,limits,deadline=deadline)
        if rg_diag.get('available') and not rg_diag.get('error'):
            primary=_match_files(root,rg_files,action,limits,deadline=deadline); primary.update({"backend":"rg","backend_diag":rg_diag})
        elif backend=='rg':
            primary={"matches":[],"match_count":0,"truncated":False,"files_searched":0,"content_skips":[],"content_skip_count":0,"searched_extension_counts":{},"time_limit_reached":bool(rg_diag.get('time_limit_reached')),"files_input":0,"files_processed":0,"files_remaining":0,"backend":"rg","backend_diag":rg_diag}
            primary_error=rg_diag.get('error') or 'rg unavailable'
        else:
            pfiles,primary_cov=_discover_search_files(root,scopes,action,limits,walker='stack',deadline=deadline)
            primary=_match_files(root,pfiles,action,limits,deadline=deadline); primary['backend']='python-stack'; primary['backend_diag']=rg_diag
    else:
        pfiles,primary_cov=_discover_search_files(root,scopes,action,limits,walker='stack',deadline=deadline)
        primary=_match_files(root,pfiles,action,limits,deadline=deadline); primary['backend']='python-stack'

    initial_coverage=primary_cov or _empty_search_coverage()
    if checkpoint_cb is not None:
        checkpoint_cb(_search_result_payload(
            root,action,scopes,primary,None,initial_coverage,
            primary_error=primary_error,fallback_enabled=fallback_enabled,
            fallback_note='pending; primary checkpoint preserved',
            extra_reasons=['search action checkpoint saved before coverage/fallback verification completed'],
            force_incomplete=True,
        ))

    primary_timed_out=bool(primary.get('time_limit_reached') or initial_coverage.get('time_limit_reached') or (primary.get('backend_diag') or {}).get('time_limit_reached'))
    if primary_timed_out:
        return _search_result_payload(
            root,action,scopes,primary,None,initial_coverage,
            primary_error=primary_error,fallback_enabled=fallback_enabled,
            fallback_note='not run; action soft timeout reached during primary phase',
            extra_reasons=['regex search reached its soft timeout; partial primary evidence preserved'],
            force_incomplete=True,
        )

    pcount=int(primary.get('match_count',0))
    need_fallback=bool(fallback_enabled and (pcount==0 or primary_error or verify_nonzero))
    fallback=None; coverage=initial_coverage
    fallback_note=None

    # Positive evidence does not need a second full content scan.  Keep the old
    # consistency behavior available via verify_nonzero_with_fallback=true, but
    # default fallback verification is reserved for zero/error where false-zero
    # conclusions are dangerous.  We still inventory the filesystem for coverage.
    if not need_fallback:
        if primary_cov is None:
            _coverage_files,coverage=_discover_search_files(root,scopes,action,limits,walker='oswalk',deadline=deadline)
        fallback_note='not run (positive primary result; zero/error verification policy)'
        if not fallback_enabled:
            fallback_note='disabled by request'
    else:
        ffiles,coverage=_discover_search_files(root,scopes,action,limits,walker='oswalk',deadline=deadline)
        if checkpoint_cb is not None:
            checkpoint_cb(_search_result_payload(
                root,action,scopes,primary,None,coverage,
                primary_error=primary_error,fallback_enabled=fallback_enabled,
                fallback_note='pending; fallback content verification not complete',
                extra_reasons=['search action checkpoint saved before fallback content verification completed'],
                force_incomplete=True,
            ))
        fallback=_match_files(root,ffiles,action,limits,deadline=deadline); fallback['backend']='python-oswalk'

    extra=[]
    if coverage.get('time_limit_reached') or (fallback and fallback.get('time_limit_reached')):
        extra.append('regex search reached its soft timeout; partial results found before timeout were preserved')
    return _search_result_payload(
        root,action,scopes,primary,fallback,coverage,
        primary_error=primary_error,fallback_enabled=fallback_enabled,
        fallback_note=fallback_note,extra_reasons=extra,
        force_incomplete=bool(extra),
    )


def _search_action_direct(root: Path, action: dict, limits: dict) -> str:
    """Compatibility wrapper used by tests and the isolated regex worker."""
    return _search_action_payload(root,action,limits)["report"]


def _mark_timeout_partial_payload(data: dict, reason: str) -> dict:
    out=dict(data)
    reasons=list(out.get('reasons') or [])
    if reason not in reasons: reasons.append(reason)
    out['reasons']=reasons; out['incomplete']=True
    if out.get('coverage_status') != 'INCONSISTENT': out['coverage_status']='PARTIAL'
    if out.get('execution_status') != 'INCONSISTENT': out['execution_status']='PARTIAL'
    report=str(out.get('report') or '')
    report=report.replace('Search execution status: COMPLETED','Search execution status: PARTIAL',1)
    report=report.replace('Coverage status: VERIFIED','Coverage status: PARTIAL',1)
    report += "\n=== REGEX TIMEOUT RECOVERY ===\n\n" + reason + "\nPartial evidence above is preserved. Search coverage is incomplete and must not be interpreted as proof of absence.\n"
    out['report']=report
    return out


def _generic_timeout_partial_payload(root: Path, action: dict, limits: dict, reason: str) -> dict:
    action=dict(action); action['_max_search_files_for_report']=int(limits.get('max_search_files',250000))
    scopes=_search_scope_list(root,action)
    empty={"matches":[],"match_count":0,"truncated":False,"files_searched":0,"content_skips":[],"content_skip_count":0,"searched_extension_counts":{},"time_limit_reached":True,"files_input":0,"files_processed":0,"files_remaining":0,"backend":"regex-worker"}
    return _search_result_payload(
        root,action,scopes,empty,None,_empty_search_coverage(),
        fallback_enabled=bool(action.get('fallback_search',True)),
        fallback_note='worker timed out before a safe later checkpoint was published',
        extra_reasons=[reason,'no later safe checkpoint was available; zero matches is untrusted'],
        force_incomplete=True,
    )


def _search_action(root: Path, action: dict, limits: dict) -> dict:
    """Run regex search out-of-process; literal search runs in-process.

    Regex timeout is fail-partial rather than fail-destructive: the worker keeps
    a checkpoint after its primary phase.  If the hard watchdog fires, the
    newest safe checkpoint is returned as COLLECT INCOMPLETE so already found
    evidence is not discarded.
    """
    if not action.get("regex", False):
        return _search_action_payload(root, action, limits)
    worker = Path(__file__).resolve().parent / "python_patch_collect_regex_worker.py"
    if not worker.is_file(): raise ValueError("regex search worker is missing")
    with tempfile.TemporaryDirectory(prefix="ptv-collect-regex-") as td:
        work=Path(td); request_path=work/"request.json"; result_path=work/"result.json"
        hard_timeout=float(REGEX_SEARCH_TIMEOUT_SECONDS)
        margin=min(REGEX_SEARCH_SOFT_TIMEOUT_MARGIN_SECONDS,max(0.02,hard_timeout*0.15))
        soft_timeout=max(0.01,hard_timeout-margin)
        request_path.write_text(json.dumps({"action":action,"limits":limits,"soft_timeout_seconds":soft_timeout},ensure_ascii=False),encoding="utf-8")
        env=dict(os.environ); env["PYTHONDONTWRITEBYTECODE"]="1"
        try:
            proc=subprocess.run([sys.executable,str(worker),"--project-root",str(root),"--request",str(request_path),"--result",str(result_path)],cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=hard_timeout)
        except subprocess.TimeoutExpired:
            reason=f"regex search exceeded hard timeout ({hard_timeout:g}s); latest partial checkpoint preserved"
            if result_path.is_file() and not result_path.is_symlink():
                try:
                    size=result_path.stat().st_size; hard=max(int(limits.get('max_report_bytes',0)),1024*1024)*2
                    if size<=hard:
                        data=json.loads(result_path.read_text(encoding='utf-8'))
                        if isinstance(data,dict) and isinstance(data.get('report'),str):
                            return _mark_timeout_partial_payload(data,reason)
                except Exception:
                    pass
            return _generic_timeout_partial_payload(root,action,limits,reason)
        if proc.returncode != 0:
            detail=(proc.stdout or '').strip().replace('\n',' ')[:800]; raise ValueError(f"regex search worker failed rc={proc.returncode}: {detail}")
        if not result_path.is_file() or result_path.is_symlink(): raise ValueError("regex search worker produced no safe result")
        size=result_path.stat().st_size; hard=max(int(limits.get('max_report_bytes',0)),1024*1024)*2
        if size>hard: raise ValueError(f"regex search worker result exceeded safety cap ({size} bytes)")
        data=json.loads(result_path.read_text(encoding='utf-8'))
        if not isinstance(data,dict) or not isinstance(data.get('report'),str): raise ValueError("regex search worker returned invalid payload")
        return data


# ---------------------------------------------------------------------------
# Historical COLLECT action compatibility (v6.18.3)
# ---------------------------------------------------------------------------


def _compat_search_spec(action: dict, *, query: str | None = None, paths: list[str] | None = None, max_matches: int | None = None) -> dict:
    """Map historical search-like actions onto the coverage-aware v6.18 engine."""
    source_scope = action.get("source_scope", "filesystem")
    spec = {
        "type": "search",
        "query": str(query if query is not None else action.get("query", "")),
        "regex": bool(action.get("regex", False)),
        "paths": list(paths if paths is not None else action.get("paths", ["."])),
        "context_lines": int(action.get("context_lines", 4)),
        "max_matches": int(max_matches if max_matches is not None else action.get("max_matches", 500)),
        "backend": action.get("backend", "auto"),
        "source_scope": source_scope,
        "filesystem": source_scope == "filesystem",
        "respect_gitignore": bool(action.get("respect_gitignore", False)),
        "follow_symlinks": bool(action.get("follow_symlinks", False)),
        "must_find": bool(action.get("must_find", False)),
        "diagnose_on_zero": bool(action.get("diagnose_on_zero", True)),
        "fallback_search": bool(action.get("fallback_search", True)),
        "verify_nonzero_with_fallback": bool(action.get("verify_nonzero_with_fallback", False)),
        "report_coverage": bool(action.get("report_coverage", True)),
        "report_skipped_dirs": bool(action.get("report_skipped_dirs", True)),
        "module_discovery": bool(action.get("module_discovery", True)),
        "anchor_paths": list(action.get("anchor_paths", [])),
        "expected_files": list(action.get("expected_files", [])),
    }
    return spec


def _ls_action(root: Path, action: dict) -> str:
    rel, scope = _resolve_scope(root, action.get("path", "."), file_ok=False)
    max_entries = int(action.get("max_entries", 1000))
    lines = ["# Directory listing", "", f"Path: `{rel}`", f"Max entries: {max_entries}", ""]
    rows: list[tuple[str, str, int | None]] = []
    try:
        entries = sorted(scope.iterdir(), key=lambda p: (p.name.lower(), p.name))
    except OSError as exc:
        return "\n".join(lines + [f"[READ ERROR: {type(exc).__name__}: {exc}]", ""]) + "\n"
    for entry in entries[:max_entries]:
        try:
            st = entry.lstat()
            if stat.S_ISLNK(st.st_mode):
                kind = "symlink (not followed)"
                size = None
            elif stat.S_ISDIR(st.st_mode):
                kind = "dir"
                size = None
            elif stat.S_ISREG(st.st_mode):
                kind = "file"
                size = st.st_size
            else:
                kind = "other"
                size = None
        except OSError:
            kind = "unreadable"
            size = None
        rows.append((entry.name, kind, size))
    for name, kind, size in rows:
        suffix = "" if size is None else f"  {size} bytes"
        lines.append(f"- [{kind}] `{name}`{suffix}")
    if len(entries) > max_entries:
        lines += ["", f"[TRUNCATED at max_entries={max_entries}; total immediate entries={len(entries)}]"]
    return "\n".join(lines) + "\n"


def _tree_action(root: Path, action: dict) -> str:
    rel, scope = _resolve_scope(root, action.get("path", "."), file_ok=False)
    max_depth = int(action.get("max_depth", 4))
    max_entries = int(action.get("max_entries", 5000))
    lines = ["# Directory tree", "", f"Path: `{rel}`", f"Max depth: {max_depth}", f"Max entries: {max_entries}", "", f"{rel}/"]
    emitted = 0
    skipped = 0
    stack: list[tuple[Path, int, str]] = [(scope, 0, "")]
    while stack and emitted < max_entries:
        current, depth, prefix = stack.pop()
        if depth >= max_depth:
            continue
        try:
            entries = sorted(current.iterdir(), key=lambda p: (p.name.lower(), p.name), reverse=True)
        except OSError:
            skipped += 1
            continue
        pending: list[tuple[Path, int, str]] = []
        for entry in reversed(entries):
            if emitted >= max_entries:
                break
            try:
                st = entry.lstat()
            except OSError:
                skipped += 1
                continue
            marker = ""
            is_dir = stat.S_ISDIR(st.st_mode)
            if stat.S_ISLNK(st.st_mode):
                marker = " -> [symlink not followed]"
                is_dir = False
            elif is_dir and (entry.name in IGNORED_DIRS or _is_internal_output_path(root, entry)):
                marker = " [default-excluded]"
            lines.append(f"{prefix}├── {entry.name}{'/' if is_dir else ''}{marker}")
            emitted += 1
            if is_dir and not marker and depth + 1 < max_depth:
                pending.append((entry, depth + 1, prefix + "│   "))
        # Stack is LIFO: reverse so lexical order remains stable.
        stack.extend(reversed(pending))
    if stack:
        lines += ["", f"[TRUNCATED at max_entries={max_entries}]"]
    if skipped:
        lines += ["", f"Unreadable entries/directories skipped: {skipped}"]
    return "\n".join(lines) + "\n"


def _read_text_file_bounded(root: Path, raw: str, limits: dict) -> tuple[str, Path, str]:
    rel, src = _resolve_exact_file(root, raw)
    size = src.stat().st_size
    cap = int(limits.get("max_search_file_bytes", 64 * 1024 * 1024))
    if size > cap:
        raise ValueError(f"text reader source exceeds max_search_file_bytes: {rel} ({size}>{cap})")
    raw_bytes = src.read_bytes()
    if b"\x00" in raw_bytes[:65536]:
        raise ValueError(f"text reader refuses binary/NUL-containing file: {rel}")
    return rel, src, raw_bytes.decode("utf-8", errors="replace")


def _line_reader_action(root: Path, action: dict, limits: dict) -> str:
    rel, _src, text = _read_text_file_bounded(root, action["path"], limits)
    rows = text.splitlines()
    kind = action["type"]
    total = len(rows)
    if kind == "head":
        start, end = 1, min(total, int(action.get("lines", 100)))
    elif kind == "tail":
        count = int(action.get("lines", 100))
        start, end = max(1, total - count + 1), total
    else:
        start = int(action.get("start_line", 1))
        end = int(action.get("end_line", total if total else 1))
        start = min(max(1, start), max(1, total))
        end = min(max(start, end), total) if total else 0
    lines = [f"# {kind} reader", "", f"Path: `{rel}`", f"Total lines: {total}", f"Selected: {start}-{end}", "", "```text"]
    if total and end >= start:
        width = len(str(end))
        for number in range(start, end + 1):
            lines.append(f"{number:>{width}}: {rows[number - 1]}")
    lines += ["```", ""]
    return "\n".join(lines) + "\n"


def _brace_end(text: str, brace_pos: int) -> int | None:
    depth = 0
    i = brace_pos
    in_string: str | None = None
    escaped = False
    line_comment = False
    block_comment = False
    while i < len(text):
        ch = text[i]
        nxt = text[i + 1] if i + 1 < len(text) else ""
        if line_comment:
            if ch == "\n":
                line_comment = False
            i += 1
            continue
        if block_comment:
            if ch == "*" and nxt == "/":
                block_comment = False
                i += 2
            else:
                i += 1
            continue
        if in_string is not None:
            if escaped:
                escaped = False
            elif ch == "\\":
                escaped = True
            elif ch == in_string:
                in_string = None
            i += 1
            continue
        if ch in {"'", '"', '`'}:
            in_string = ch
            i += 1
            continue
        if ch == "/" and nxt == "/":
            line_comment = True
            i += 2
            continue
        if ch == "/" and nxt == "*":
            block_comment = True
            i += 2
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return i + 1
        i += 1
    return None


def _line_start_offsets(text: str) -> list[int]:
    offsets = [0]
    for match in re.finditer("\n", text):
        offsets.append(match.end())
    return offsets


def _offset_to_line(offsets: list[int], pos: int) -> int:
    import bisect
    return bisect.bisect_right(offsets, pos)


def _extract_symbol_blocks(text: str, symbol: str, *, context_lines: int, max_blocks: int) -> list[dict]:
    offsets = _line_start_offsets(text)
    lines = text.splitlines()
    occurrences = [m.start() for m in re.finditer(re.escape(symbol), text)]
    blocks: list[dict] = []
    seen: set[tuple[int, int]] = set()
    for pos in occurrences:
        if len(blocks) >= max_blocks:
            break
        line_no = _offset_to_line(offsets, pos)
        line_idx = max(0, line_no - 1)
        # Prefer declaration-like occurrences (symbol followed by '(' or class-ish token).
        tail = text[pos:pos + len(symbol) + 32]
        declaration_hint = bool(re.match(re.escape(symbol) + r"\s*\(", tail))
        start_line = line_idx
        if declaration_hint:
            # Preserve annotations/modifiers/signature continuations directly above declaration.
            for prev in range(line_idx - 1, max(-1, line_idx - 10), -1):
                stripped = lines[prev].strip()
                if not stripped:
                    break
                if stripped.endswith(";") or stripped.endswith("}"):
                    break
                start_line = prev
        search_from = offsets[line_idx]
        brace_pos = text.find("{", search_from, min(len(text), search_from + 4096))
        if declaration_hint and brace_pos >= 0:
            end_pos = _brace_end(text, brace_pos)
        else:
            end_pos = None
        if end_pos is not None:
            start_pos = offsets[start_line]
            end_line = _offset_to_line(offsets, end_pos)
            end_slice = offsets[end_line] if end_line < len(offsets) else len(text)
        else:
            start_line = max(0, line_idx - context_lines)
            end_line_idx = min(len(lines), line_idx + context_lines + 1)
            start_pos = offsets[start_line] if offsets else 0
            end_slice = offsets[end_line_idx] if end_line_idx < len(offsets) else len(text)
            end_line = end_line_idx
        key = (start_pos, end_slice)
        if key in seen:
            continue
        seen.add(key)
        blocks.append({
            "line": line_no,
            "start_line": start_line + 1,
            "end_line": max(start_line + 1, end_line),
            "declaration_hint": declaration_hint,
            "text": text[start_pos:end_slice].rstrip(),
        })
    return blocks


def _symbol_action(root: Path, action: dict, limits: dict) -> str:
    rel, _src, text = _read_text_file_bounded(root, action["path"], limits)
    symbol = action["symbol"]
    blocks = _extract_symbol_blocks(
        text,
        symbol,
        context_lines=int(action.get("context_lines", 8)),
        max_blocks=int(action.get("max_blocks", 20)),
    )
    lines = ["# Symbol extraction", "", f"Path: `{rel}`", f"Symbol: `{symbol}`", f"Blocks: {len(blocks)}", ""]
    for idx, block in enumerate(blocks, 1):
        lines += [
            f"## Block {idx} lines {block['start_line']}-{block['end_line']}",
            f"Declaration-like occurrence: {block['declaration_hint']}",
            "```text",
            block["text"],
            "```",
            "",
        ]
    if not blocks:
        lines += ["NO SYMBOL OCCURRENCES IN REQUESTED FILE", ""]
    return "\n".join(lines) + "\n"


_DEP_PATTERNS = [
    ("c_include", re.compile(r'^\s*#\s*include\s*[<\"]([^>\"]+)[>\"]')),
    ("java_import", re.compile(r'^\s*import\s+(?:static\s+)?([A-Za-z0-9_.*]+)\s*;?')),
    ("python_from", re.compile(r'^\s*from\s+([A-Za-z0-9_\.]+)\s+import\s+')),
    ("python_import", re.compile(r'^\s*import\s+([A-Za-z0-9_\.]+)')),
    ("js_from", re.compile(r'\bfrom\s+[\"\']([^\"\']+)[\"\']')),
    ("js_require", re.compile(r'\brequire\s*\(\s*[\"\']([^\"\']+)[\"\']\s*\)')),
    ("rust_use", re.compile(r'^\s*use\s+([^;]+);')),
    ("go_import", re.compile(r'^\s*[\"`]([^\"`]+)[\"`]\s*$')),
]


def _dependency_rows_for_file(root: Path, path: Path, limits: dict) -> list[dict]:
    rel = _project_rel(root, path)
    try:
        if path.stat().st_size > int(limits.get("max_search_file_bytes", 64 * 1024 * 1024)):
            return []
        if not _looks_text(path):
            return []
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return []
    rows: list[dict] = []
    for line_no, line in enumerate(text.splitlines(), 1):
        for kind, pattern in _DEP_PATTERNS:
            match = pattern.search(line)
            if match:
                rows.append({"path": rel, "line": line_no, "kind": kind, "target": match.group(1).strip(), "source": line.strip()})
                break
    return rows


def _dependency_action(root: Path, action: dict, limits: dict) -> tuple[str, list[dict]]:
    max_results = int(action.get("max_results", action.get("max_dependency_files", 2000)))
    paths = action.get("paths", [action.get("path", ".")])
    rows: list[dict] = []
    seen_files: set[str] = set()
    for raw in paths:
        _rel, scope = _resolve_scope(root, raw, file_ok=True)
        candidates = [scope] if scope.is_file() else _iter_files(root, scope, max_files=limits["max_search_files"])
        for src in candidates:
            rel = _project_rel(root, src)
            if rel in seen_files:
                continue
            seen_files.add(rel)
            for row in _dependency_rows_for_file(root, src, limits):
                rows.append(row)
                if len(rows) >= max_results:
                    break
            if len(rows) >= max_results:
                break
        if len(rows) >= max_results:
            break
    lines = ["# Dependency inventory", "", f"Requested paths: {', '.join(f'`{p}`' for p in paths)}", f"Dependency records: {len(rows)}", f"Files inspected: {len(seen_files)}", "Semantics: bounded source-level include/import/use/require discovery; no arbitrary command execution.", ""]
    for row in rows:
        lines.append(f"- `{row['path']}:{row['line']}` [{row['kind']}] `{row['target']}`")
    if len(rows) >= max_results:
        lines += ["", f"[TRUNCATED at max_results={max_results}]"]
    return "\n".join(lines) + "\n", rows


def _directory_action(root: Path, action: dict, limits: dict) -> tuple[str, list[tuple[str, Path]]]:
    rel, scope = _resolve_scope(root, action["path"], file_ok=False)
    includes = action.get("include", ["*"])
    excludes = action.get("exclude", [])
    max_results = int(action.get("max_results", 5000))
    matches: list[tuple[str, Path]] = []
    for src in _iter_files(root, scope, max_files=limits["max_search_files"]):
        file_rel = _project_rel(root, src)
        local = src.relative_to(scope).as_posix()
        include = any(
            _matches_discovery_pattern(name=src.name, local_rel=local, project_rel=file_rel, pattern=pat)
            for pat in includes
        )
        exclude = any(
            _matches_discovery_pattern(name=src.name, local_rel=local, project_rel=file_rel, pattern=pat)
            for pat in excludes
        )
        if include and not exclude:
            # Historical directory collection was discovery-driven, so unlike an
            # explicit exact-file pack it must not automatically ingest obvious
            # credential/private-key files.
            if _sensitive_file_reasons(file_rel, src):
                continue
            matches.append((file_rel, src))
            if len(matches) >= max_results:
                break
    lines = [
        "# Directory collection", "", f"Path: `{rel}`", f"Include: {includes}",
        f"Exclude: {excludes}", f"Matches: {len(matches)}",
        "Pattern semantics: scope-relative/project-relative glob; **/ may match zero or more directories.", ""
    ]
    lines.extend(f"- `{item_rel}`" for item_rel, _ in matches)
    if len(matches) >= max_results:
        lines += ["", f"[TRUNCATED at max_results={max_results}]"]
    return "\n".join(lines) + "\n", matches


def _call_tokens(block_text: str, own_symbol: str, max_callees: int) -> list[str]:
    keywords = {
        "if", "for", "while", "switch", "return", "sizeof", "catch", "new", "delete",
        "typeof", "require", "assert", "print", "printf", "log", "lambda",
    }
    tokens: list[str] = []
    for match in re.finditer(r"\b([A-Za-z_~][A-Za-z0-9_:~<>.]*)\s*\(", block_text):
        token = match.group(1)
        base = token.split("::")[-1].split(".")[-1]
        if token == own_symbol or base == own_symbol or base.lower() in keywords:
            continue
        if token not in tokens:
            tokens.append(token)
            if len(tokens) >= max_callees:
                break
    return tokens


def _candidate_symbol_blocks(root: Path, details: list[dict], symbol: str, limits: dict, *, context_lines: int, max_files: int = 40) -> list[dict]:
    out: list[dict] = []
    seen: set[str] = set()
    for hit in details:
        rel = str(hit.get("path", ""))
        if not rel or rel in seen:
            continue
        seen.add(rel)
        if len(seen) > max_files:
            break
        try:
            _resolved_rel, _src, text = _read_text_file_bounded(root, rel, limits)
        except Exception:
            continue
        blocks = _extract_symbol_blocks(text, symbol, context_lines=context_lines, max_blocks=4)
        for block in blocks:
            out.append({"path": rel, **block})
    return out


def _symbol_graph_action(root: Path, action: dict, limits: dict) -> dict:
    paths = list(action.get("paths", ["."]))
    symbols = list(action.get("symbols", [action.get("symbol")]))
    symbols = [s for s in symbols if s]
    context_lines = int(action.get("context_lines", 8))
    max_occurrences = int(action.get("max_occurrences", action.get("max_callers", 1000)))
    max_callers = int(action.get("max_callers", 300))
    max_callees = int(action.get("max_callees", 100))
    include_refs = bool(action.get("include_references", True))
    include_callers = bool(action.get("include_callers", True))
    include_callees = bool(action.get("include_callees", True))
    include_deps = bool(action.get("include_dependencies", True))
    incomplete_reasons: list[str] = []
    lines = ["# Symbol graph compatibility report", "", f"Paths: {paths}", f"Symbols: {symbols}", "Semantics: filesystem-verified references plus bounded source heuristics for declaration blocks/callees/dependencies.", ""]
    for symbol in symbols:
        search_spec = _compat_search_spec(
            action,
            query=symbol,
            paths=paths,
            max_matches=max_occurrences,
        )
        search_spec["regex"] = False
        search_spec["must_find"] = False
        payload = _search_action(root, search_spec, limits)
        if payload.get("incomplete"):
            incomplete_reasons.extend(f"{symbol}: {x}" for x in payload.get("reasons", []))
        details = list(payload.get("match_details", []))[:max_occurrences]
        blocks = _candidate_symbol_blocks(root, details, symbol, limits, context_lines=context_lines)
        lines += [
            f"## Symbol `{symbol}`",
            f"Coverage status: {payload.get('coverage_status')}",
            f"Occurrences: {payload.get('matches', 0)}",
            f"Candidate declaration/context blocks: {len(blocks)}",
            "",
        ]
        if include_refs or include_callers:
            lines.append("### References/callers")
            for hit in details[:max_callers]:
                lines.append(f"- `{hit.get('path')}:{hit.get('line')}`")
                for n, content, is_hit in hit.get("context", []):
                    if is_hit:
                        lines.append(f"  `{n}: {content.strip()[:600]}`")
            if not details:
                lines.append("- (none)")
            lines.append("")
        if blocks:
            lines.append("### Candidate symbol bodies")
            for block in blocks[:20]:
                lines += [
                    f"#### `{block['path']}:{block['start_line']}-{block['end_line']}`",
                    "```text",
                    block["text"],
                    "```",
                    "",
                ]
        if include_callees:
            callees: list[str] = []
            for block in blocks:
                for token in _call_tokens(block["text"], symbol, max_callees):
                    if token not in callees:
                        callees.append(token)
                        if len(callees) >= max_callees:
                            break
                if len(callees) >= max_callees:
                    break
            lines += ["### Heuristic callees"]
            lines.extend(f"- `{token}`" for token in callees)
            if not callees:
                lines.append("- (none identified)")
            lines.append("")
        if include_deps:
            candidate_paths = sorted({block["path"] for block in blocks})[: int(action.get("max_dependency_files", 400))]
            dep_rows: list[dict] = []
            for rel in candidate_paths:
                try:
                    _r, src = _resolve_exact_file(root, rel)
                except Exception:
                    continue
                dep_rows.extend(_dependency_rows_for_file(root, src, limits))
            lines += ["### Dependencies of candidate definition files"]
            for row in dep_rows[: int(action.get("max_dependency_files", 400))]:
                lines.append(f"- `{row['path']}:{row['line']}` [{row['kind']}] `{row['target']}`")
            if not dep_rows:
                lines.append("- (none identified)")
            lines.append("")
    reasons = list(dict.fromkeys(incomplete_reasons))
    return {"report": "\n".join(lines) + "\n", "incomplete": bool(reasons), "reasons": reasons}


def _research_action(root: Path, action: dict, limits: dict) -> dict:
    overview_action = {"path": action.get("path", action.get("paths", ["."])[0]), "tree_depth": int(action.get("tree_depth", 2))}
    overview = _overview(root, overview_action, limits)
    search_spec = _compat_search_spec(action)
    search = _search_action(root, search_spec, limits)
    report = "# Research bundle\n\n" + overview + "\n---\n\n" + search["report"]
    return {"report": report, "incomplete": bool(search.get("incomplete")), "reasons": list(search.get("reasons", []))}


def _decompile_action(root: Path, action: dict, limits: dict) -> str:
    from python_patch_decompile_compat import extract_decompile_report
    source_raw = action.get("source") or action.get("path")
    rel, src = _resolve_exact_file(root, source_raw)
    report = extract_decompile_report(
        src,
        action,
        max_file_bytes=int(limits.get("max_decompile_file_bytes", 512 * 1024 * 1024)),
    )
    return report.replace(f"Source: `{src.name}`", f"Source: `{rel}`", 1)


def _git_action(root: Path, action: dict) -> str:
    from python_patch_git_safe import run_git_operations
    return run_git_operations(root, action)


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
                if kind in {"pack", "zip"}:
                    lines = ["# Pack" if kind == "pack" else "# Historical ZIP/pack compatibility", ""]
                    for raw in action["paths"]:
                        rel, src = _resolve_exact_file(root, raw)
                        builder.add_exact_file(rel, src, source_action=index)
                        lines.append(f"- `{rel}`")
                    builder.add_report(index, kind, title, "\n".join(lines) + "\n")
                elif kind == "overview":
                    builder.add_report(index, kind, title, _overview(root, action, request_data["limits"]))
                elif kind == "ls":
                    builder.add_report(index, kind, title, _ls_action(root, action))
                elif kind == "tree":
                    builder.add_report(index, kind, title, _tree_action(root, action))
                elif kind == "find":
                    find_result = _find_action_result(root, action, request_data["limits"])
                    matches = find_result["matches"]
                    builder.add_report(index, kind, title, find_result["report"])
                    if find_result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(find_result.get("reasons") or []))
                    if action.get("collect"):
                        for rel, src in matches:
                            added,reason=builder.add_discovered_file(rel,src,source_action=index)
                            if not added:
                                builder.mark_incomplete(action=index,kind=kind,reasons=[reason or "discovered file omitted by collection quota"])
                                if reason and ("max_files=" in reason or "max_total_bytes=" in reason):
                                    break
                elif kind in {"search", "search_files", "content"}:
                    search_action = action if kind == "search" else _compat_search_spec(action)
                    search_result = _search_action(root, search_action, request_data["limits"])
                    builder.add_report(index, kind, title, search_result["report"])
                    if search_result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(search_result.get("reasons") or []))
                elif kind == "research":
                    result = _research_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, result["report"])
                    if result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(result.get("reasons") or []))
                elif kind in {"file", "range", "head", "tail"}:
                    builder.add_report(index, kind, title, _line_reader_action(root, action, request_data["limits"]))
                elif kind == "symbol":
                    builder.add_report(index, kind, title, _symbol_action(root, action, request_data["limits"]))
                elif kind == "references":
                    search_action = _compat_search_spec(
                        action,
                        query=action["symbol"],
                        paths=action.get("paths", ["."]),
                        max_matches=int(action.get("max_matches", 500)),
                    )
                    search_action["regex"] = False
                    result = _search_action(root, search_action, request_data["limits"])
                    builder.add_report(index, kind, title, result["report"])
                    if result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(result.get("reasons") or []))
                elif kind == "callgraph":
                    graph_action = {
                        **action,
                        "symbols": [action["symbol"]],
                        "include_references": True,
                        "include_callers": True,
                        "include_callees": True,
                        "include_dependencies": False,
                    }
                    result = _symbol_graph_action(root, graph_action, request_data["limits"])
                    builder.add_report(index, kind, title, result["report"])
                    if result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(result.get("reasons") or []))
                elif kind == "dependencies":
                    report, _rows = _dependency_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, report)
                elif kind == "directory":
                    report, matches = _directory_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, report)
                    for rel, src in matches:
                        added,reason=builder.add_discovered_file(rel,src,source_action=index)
                        if not added:
                            builder.mark_incomplete(action=index,kind=kind,reasons=[reason or "discovered file omitted by collection quota"])
                            if reason and ("max_files=" in reason or "max_total_bytes=" in reason):
                                break
                elif kind == "symbol_graph":
                    result = _symbol_graph_action(root, action, request_data["limits"])
                    builder.add_report(index, kind, title, result["report"])
                    if result.get("incomplete"):
                        builder.mark_incomplete(action=index, kind=kind, reasons=list(result.get("reasons") or []))
                elif kind in {"decompile", "ida", "ghidra"}:
                    builder.add_report(index, kind, title, _decompile_action(root, action, request_data["limits"]))
                elif kind == "database_select":
                    with tempfile.TemporaryDirectory(prefix=f"ptv-db-select-{index:03d}-") as dbtmp:
                        try:
                            db_result = execute_database_select(root, action, request_data["limits"], Path(dbtmp))
                        except DatabaseSelectError as exc:
                            raise ValueError(f"database_select failed: {exc}") from exc
                        builder.add_report(index, kind, title, db_result["report"])
                        artifact_root = Path(db_result["artifact_root"])
                        quota_reasons: list[str] = []
                        for artifact in sorted(db_result["artifacts"], key=lambda p: p.relative_to(artifact_root).as_posix()):
                            rel_art = artifact.relative_to(artifact_root).as_posix()
                            arcname = f"database_queries/{index:03d}_{_safe_id(action.get('id') or action.get('title') or 'select')}/{rel_art}"
                            added, reason = builder.add_generated_artifact(artifact, arcname, source_action=index)
                            if not added:
                                quota_reasons.append(reason or "database_select generated artifact omitted by package quota")
                                if reason and ("max_files=" in reason or "max_total_bytes=" in reason):
                                    break
                        if db_result.get("incomplete"):
                            builder.mark_incomplete(action=index, kind=kind, reasons=list(db_result.get("reasons") or ["database_select returned bounded partial evidence"]))
                        if quota_reasons:
                            builder.mark_incomplete(action=index, kind=kind, reasons=quota_reasons)
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
                try: result.with_suffix(".txt").unlink()
                except OSError: pass
                raise
            # Delivery is acknowledged only after ZIP + TXT + request archive
            # all survived publication. This prevents one-shot sync state from
            # consuming the only update when a later lifecycle step fails.
            try:
                from python_patch_ai_sync import mark_sync_delivered
                companion = result.with_suffix(".txt")
                if companion.is_file() and not companion.is_symlink():
                    mark_sync_delivered(
                        root,
                        builder.ai_sync_decision,
                        artifact=result.relative_to(root).as_posix(),
                    )
            except Exception:
                pass
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
