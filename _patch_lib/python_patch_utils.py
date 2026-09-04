#!/usr/bin/env python3
from __future__ import annotations

from dataclasses import dataclass, field
from difflib import SequenceMatcher
from pathlib import Path, PurePosixPath
import os
import re
import shutil
import tempfile
import contextlib
import io
import sys
import zipfile
import datetime as _dt
from typing import Any, Callable, Iterable, Optional

VERSION = "6.20.0"


class PatchFailure(RuntimeError):
    def __init__(self, rel_path: str, message: str, *, expected: str | None = None, anchor: str | None = None,
                 context: str | None = None, op_id: str | None = None, strategy: str | None = None):
        super().__init__(message)
        self.rel_path = rel_path
        self.message = message
        self.expected = expected
        self.anchor = anchor
        self.context = context
        self.op_id = op_id
        self.strategy = strategy


@dataclass
class PatchStats:
    patched: int = 0
    created: int = 0
    unchanged: int = 0
    failed: int = 0
    backups: int = 0
    skipped: int = 0
    ignored: int = 0


@dataclass
class PatchRunState:
    project_root: Path
    patch_name: str = "patch"
    stats: PatchStats = field(default_factory=PatchStats)
    failures: list[PatchFailure] = field(default_factory=list)
    failed_files: set[str] = field(default_factory=set)
    changed_files: set[str] = field(default_factory=set)
    backup_generation: str = field(default_factory=lambda: f"{os.getpid()}_{__import__('time').time_ns()}")
    backups: dict[str, Path] = field(default_factory=dict)

    @property
    def backed_up_files(self) -> dict[str, Path]:
        return self.backups

    def record_failure(self, exc: PatchFailure) -> None:
        self.failures.append(exc)
        self.failed_files.add(exc.rel_path)
        self.stats.failed += 1


def _safe_rel(rel_path: str) -> PurePosixPath:
    if not isinstance(rel_path, str) or not rel_path.strip():
        raise PatchFailure(str(rel_path), "file path must be a non-empty project-relative string")
    text = rel_path.strip()
    if "\\" in text:
        raise PatchFailure(text, "use POSIX '/' separators in patch file paths")
    rel = PurePosixPath(text)
    if rel.is_absolute() or any(p in {"", ".", ".."} for p in rel.parts):
        raise PatchFailure(text, "unsafe/non-relative file path")
    return rel


def resolve_project_path(project_root: Path, rel_path: str, *, may_create: bool = False) -> Path:
    root = project_root.resolve()
    rel = _safe_rel(rel_path)
    path = root.joinpath(*rel.parts)
    parent = path.parent.resolve(strict=False)
    try:
        parent.relative_to(root)
    except ValueError as exc:
        raise PatchFailure(rel_path, "file path escapes project root") from exc
    if path.exists() or path.is_symlink():
        if path.is_symlink():
            raise PatchFailure(rel_path, "refusing to patch a symlink")
        try:
            path.resolve(strict=True).relative_to(root)
        except ValueError as exc:
            raise PatchFailure(rel_path, "file path resolves outside project root") from exc
    elif not may_create:
        raise PatchFailure(rel_path, "target file does not exist")
    return path


def read_text(project_root: Path, rel_path: str) -> str:
    path = resolve_project_path(project_root, rel_path)
    if not path.is_file():
        raise PatchFailure(rel_path, "target is not a regular file")
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError as exc:
        raise PatchFailure(rel_path, "target is not UTF-8 text") from exc


def _backup_file(state: PatchRunState, rel_path: str) -> Path | None:
    if rel_path in state.backups:
        return state.backups[rel_path]
    src = resolve_project_path(state.project_root, rel_path)
    if not src.exists():
        return None
    safe_name = re.sub(r"[^A-Za-z0-9._-]+", "_", state.patch_name)[:96] or "patch"
    backup = state.project_root / "patchs" / "backup" / safe_name / state.backup_generation / rel_path
    backup.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, backup)
    state.backups[rel_path] = backup
    state.stats.backups += 1
    return backup


def _write_text(state: PatchRunState, rel_path: str, content: str, *, create: bool = True) -> bool:
    path = resolve_project_path(state.project_root, rel_path, may_create=create)
    existed = path.exists()
    old_mode: int | None = None
    if existed:
        old = path.read_text(encoding="utf-8")
        if old == content:
            state.stats.unchanged += 1
            print(f"unchanged/check: {rel_path}")
            return False
        old_mode = path.stat().st_mode & 0o777
        _backup_file(state, rel_path)
    else:
        if not create:
            raise PatchFailure(rel_path, "target file does not exist and create=false")
        path.parent.mkdir(parents=True, exist_ok=True)
    data = content.encode("utf-8")
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.ptv-write-", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "wb") as out:
            out.write(data)
            out.flush()
            try:
                os.fsync(out.fileno())
            except OSError:
                pass
        if old_mode is not None and os.name != "nt":
            os.chmod(tmp, old_mode)
        os.replace(tmp, path)
        if os.name != "nt":
            try:
                dfd = os.open(path.parent, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
                try: os.fsync(dfd)
                finally: os.close(dfd)
            except OSError:
                pass
    finally:
        try: tmp.unlink()
        except FileNotFoundError: pass
    if existed:
        state.stats.patched += 1
        print(f"patched: {rel_path}")
    else:
        state.stats.created += 1
        print(f"created: {rel_path}")
    state.changed_files.add(rel_path)
    return True


def _normalize_ws(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


def _unique_exact(text: str, needle: str, rel_path: str) -> tuple[int, int]:
    if not needle:
        raise PatchFailure(rel_path, "empty match block is not allowed")
    count = text.count(needle)
    if count == 0:
        raise PatchFailure(rel_path, "expected block not found", expected=needle)
    if count > 1:
        raise PatchFailure(rel_path, f"expected block found {count} times; patch is ambiguous", expected=needle)
    start = text.index(needle)
    return start, start + len(needle)


def _unique_ws(text: str, needle: str, rel_path: str) -> tuple[int, int]:
    # Conservative line-window whitespace normalization.  It never returns a
    # half-expression character window.
    target = _normalize_ws(needle)
    if not target:
        raise PatchFailure(rel_path, "empty normalized match block")
    nlines = max(1, len(needle.splitlines()))
    lines = text.splitlines(keepends=True)
    candidates: list[tuple[int, int]] = []
    offsets = [0]
    for line in lines:
        offsets.append(offsets[-1] + len(line))
    for width in range(max(1, nlines - 2), nlines + 3):
        for i in range(0, max(0, len(lines) - width + 1)):
            block = "".join(lines[i:i+width])
            if _normalize_ws(block) == target:
                candidates.append((offsets[i], offsets[i+width]))
    candidates = sorted(set(candidates))
    if len(candidates) != 1:
        if not candidates:
            raise PatchFailure(rel_path, "normalized whitespace block not found", expected=needle, strategy="normalized_ws")
        raise PatchFailure(rel_path, f"normalized whitespace block found {len(candidates)} times; patch is ambiguous", expected=needle, strategy="normalized_ws")
    return candidates[0]


def _unique_fuzzy(text: str, needle: str, rel_path: str, minimum: float = 0.88, gap: float = 0.04) -> tuple[int, int]:
    target_lines = needle.splitlines()
    if not target_lines:
        raise PatchFailure(rel_path, "empty fuzzy match block")
    lines = text.splitlines(keepends=True)
    offsets = [0]
    for line in lines:
        offsets.append(offsets[-1] + len(line))
    n = len(target_lines)
    ranked: list[tuple[float, int, int]] = []
    for width in range(max(1, n - 2), n + 3):
        for i in range(0, max(0, len(lines) - width + 1)):
            block = "".join(lines[i:i+width])
            score = SequenceMatcher(None, _normalize_ws(needle), _normalize_ws(block)).ratio()
            if score >= minimum:
                ranked.append((score, offsets[i], offsets[i+width]))
    ranked.sort(reverse=True)
    if not ranked:
        raise PatchFailure(rel_path, "fuzzy block not found", expected=needle, strategy="fuzzy")
    if len(ranked) > 1 and ranked[0][0] - ranked[1][0] < gap:
        raise PatchFailure(rel_path, "fuzzy match is ambiguous", expected=needle, strategy="fuzzy")
    return ranked[0][1], ranked[0][2]


def _find_span(text: str, needle: str, rel_path: str, mode: str, op: dict[str, Any]) -> tuple[int, int]:
    mode = str(mode or "auto").lower()
    variants = [needle] + [str(v) for v in (op.get("variants") or op.get("old_variants") or [])]
    errors: list[PatchFailure] = []
    methods = ["exact", "normalized_ws", "fuzzy"] if mode == "auto" else [mode]
    for method in methods:
        for variant in variants:
            try:
                if method in {"exact", "replace_exact"}:
                    return _unique_exact(text, variant, rel_path)
                if method in {"normalized_ws", "ws", "replace_ws"}:
                    return _unique_ws(text, variant, rel_path)
                if method in {"fuzzy", "replace_fuzzy"}:
                    return _unique_fuzzy(text, variant, rel_path, float(op.get("fuzzy_min", 0.88)), float(op.get("fuzzy_unique_gap", 0.04)))
                raise PatchFailure(rel_path, f"unsupported match mode: {method}")
            except PatchFailure as exc:
                errors.append(exc)
    raise errors[-1] if errors else PatchFailure(rel_path, "match failed")


def _already(text: str, new: str | None, already: Any) -> bool:
    # Idempotency must be explicit. Merely finding `new` somewhere else in the
    # file is not proof that the intended anchor/range was already patched.
    checks: list[str] = []
    if isinstance(already, str):
        checks = [already]
    elif isinstance(already, list):
        checks = [str(x) for x in already]
    return any(x and x in text for x in checks)


def op_replace(state: PatchRunState, op: dict[str, Any]) -> bool:
    rel = op["file"]
    old = str(op.get("old", ""))
    new = str(op.get("new", ""))
    text = read_text(state.project_root, rel)
    if _already(text, new, op.get("already")):
        state.stats.unchanged += 1
        print(f"already patched/check: {rel}")
        return False
    start, end = _find_span(text, old, rel, op.get("mode", "auto"), op)
    return _write_text(state, rel, text[:start] + new + text[end:])


def op_replace_any(state: PatchRunState, op: dict[str, Any]) -> bool:
    alternatives = op.get("alternatives") or op.get("old")
    if not isinstance(alternatives, list) or not alternatives:
        raise PatchFailure(op.get("file", "<operation>"), "replace_any requires alternatives[]")
    last: PatchFailure | None = None
    for alt in alternatives:
        candidate = dict(op)
        if isinstance(alt, dict):
            candidate.update(alt)
        else:
            candidate["old"] = str(alt)
        candidate["kind"] = "replace"
        try:
            return op_replace(state, candidate)
        except PatchFailure as exc:
            last = exc
    raise last or PatchFailure(op.get("file", "<operation>"), "replace_any failed")


def op_regex_replace(state: PatchRunState, op: dict[str, Any]) -> bool:
    rel = op["file"]
    text = read_text(state.project_root, rel)
    new = str(op.get("new", op.get("replacement", "")))
    if _already(text, new, op.get("already")):
        state.stats.unchanged += 1
        print(f"already patched/check: {rel}")
        return False
    pattern = op.get("pattern") or op.get("old")
    if not isinstance(pattern, str) or not pattern:
        raise PatchFailure(rel, "regex_replace requires pattern/old")
    flags = int(op.get("regex_flags", 0))
    matches = list(re.finditer(pattern, text, flags))
    if len(matches) != 1:
        raise PatchFailure(rel, f"regex expected exactly one match, found {len(matches)}", expected=pattern)
    updated = re.sub(pattern, new, text, count=1, flags=flags)
    return _write_text(state, rel, updated)


def op_insert(state: PatchRunState, op: dict[str, Any], *, before: bool) -> bool:
    rel = op["file"]
    anchor = str(op.get("anchor", ""))
    insertion = str(op.get("insert", op.get("insertion", "")))
    text = read_text(state.project_root, rel)
    if _already(text, insertion, op.get("already")):
        state.stats.unchanged += 1
        print(f"already patched/check: {rel}")
        return False
    start, end = _find_span(text, anchor, rel, op.get("mode", "auto"), {**op, "old": anchor})
    at = start if before else end
    return _write_text(state, rel, text[:at] + insertion + text[at:])


def op_append(state: PatchRunState, op: dict[str, Any]) -> bool:
    rel = op["file"]
    content = str(op.get("content", op.get("insert", "")))
    text = read_text(state.project_root, rel)
    if _already(text, content, op.get("already")):
        state.stats.unchanged += 1
        print(f"already patched/check: {rel}")
        return False
    sep = "" if not text or text.endswith("\n") else "\n"
    return _write_text(state, rel, text + sep + content)


def op_prepend(state: PatchRunState, op: dict[str, Any]) -> bool:
    rel = op["file"]
    content = str(op.get("content", op.get("insert", "")))
    text = read_text(state.project_root, rel)
    if _already(text, content, op.get("already")):
        state.stats.unchanged += 1
        print(f"already patched/check: {rel}")
        return False
    return _write_text(state, rel, content + text)


def op_write(state: PatchRunState, op: dict[str, Any]) -> bool:
    return _write_text(state, op["file"], str(op.get("content", "")), create=bool(op.get("create", True)))


def _condition(state: PatchRunState, cond: dict[str, Any]) -> bool:
    if "path_exists" in cond:
        try:
            path = resolve_project_path(state.project_root, str(cond["path_exists"]), may_create=True)
        except PatchFailure:
            return False
        return path.exists()
    rel = cond.get("file")
    if not rel:
        raise PatchFailure("<condition>", "condition requires file or path_exists")
    try:
        text = read_text(state.project_root, str(rel))
    except PatchFailure:
        if cond.get("exists") is False:
            return True
        raise
    if "contains" in cond:
        return str(cond["contains"]) in text
    if "not_contains" in cond:
        return str(cond["not_contains"]) not in text
    if "regex" in cond:
        return re.search(str(cond["regex"]), text) is not None
    if "exists" in cond:
        return bool(cond["exists"])
    raise PatchFailure(str(rel), "unsupported if condition")


def op_if(state: PatchRunState, op: dict[str, Any]) -> bool:
    cond = op.get("condition")
    if not isinstance(cond, dict):
        raise PatchFailure(op.get("file", "<condition>"), "if requires condition object")
    branch = op.get("then", []) if _condition(state, cond) else op.get("else", [])
    if not isinstance(branch, list):
        raise PatchFailure("<condition>", "if then/else must be arrays")
    before = (state.stats.patched, state.stats.created)
    _apply_ops_state(state, branch, inherited_on_error=op.get("on_error", "stop"))
    return before != (state.stats.patched, state.stats.created)


def op_first_success(state: PatchRunState, op: dict[str, Any]) -> bool:
    alternatives = op.get("alternatives")
    if not isinstance(alternatives, list) or not alternatives:
        raise PatchFailure(op.get("file", "<operation>"), "first_success requires alternatives[]")
    errors: list[PatchFailure] = []
    for i, alt in enumerate(alternatives, 1):
        checkpoint = (state.stats.patched, state.stats.created, state.stats.unchanged)
        try:
            ops = alt if isinstance(alt, list) else [alt]
            _apply_ops_state(state, ops, inherited_on_error="stop")
            print(f"first_success: selected alternative #{i}")
            return True
        except PatchFailure as exc:
            if (state.stats.patched, state.stats.created) != checkpoint[:2]:
                raise PatchFailure(exc.rel_path, f"alternative #{i} failed after writing; refusing next alternative") from exc
            state.stats.unchanged = checkpoint[2]
            errors.append(exc)
    raise errors[0] if errors else PatchFailure("<operation>", "all first_success alternatives failed")


OPS: dict[str, Callable[[PatchRunState, dict[str, Any]], bool]] = {
    "replace": op_replace,
    "replace_exact": lambda s, o: op_replace(s, {**o, "mode": "exact"}),
    "replace_ws": lambda s, o: op_replace(s, {**o, "mode": "normalized_ws"}),
    "replace_fuzzy": lambda s, o: op_replace(s, {**o, "mode": "fuzzy"}),
    "replace_any": op_replace_any,
    "regex_replace": op_regex_replace,
    "insert_after": lambda s, o: op_insert(s, o, before=False),
    "insert_before": lambda s, o: op_insert(s, o, before=True),
    "append": op_append,
    "prepend": op_prepend,
    "write": op_write,
    "if": op_if,
    "first_success": op_first_success,
}


def _apply_ops_state(state: PatchRunState, ops: list[dict[str, Any]], *, inherited_on_error: str = "stop") -> None:
    if not isinstance(ops, list):
        raise PatchFailure("<ops>", "ops must be an array")
    for index, op in enumerate(ops, 1):
        if not isinstance(op, dict):
            raise PatchFailure("<ops>", f"operation #{index} must be an object")
        kind = str(op.get("kind", "replace"))
        handler = OPS.get(kind)
        if handler is None:
            raise PatchFailure(op.get("file", "<operation>"), f"unknown operation kind: {kind}")
        on_error = str(op.get("on_error", inherited_on_error or "stop"))
        try:
            handler(state, op)
        except PatchFailure as exc:
            if not exc.op_id:
                exc.op_id = str(op.get("id") or op.get("op_id") or f"#{index}:{kind}")
            state.record_failure(exc)
            if on_error == "ignore":
                state.stats.ignored += 1
                print(f"WARNING ignored operation failure: {exc.rel_path}: {exc.message}")
                continue
            if on_error == "skip":
                state.stats.skipped += 1
                print(f"WARNING skipped operation: {exc.rel_path}: {exc.message}")
                continue
            raise


def apply_ops(
    project_root_or_state: Path | PatchRunState,
    patch_name_or_ops: str | list[dict[str, Any]],
    ops: Iterable[dict[str, Any]] | None = None,
    *,
    default_on_error: str = "stop",
    inherited_on_error: str | None = None,
) -> PatchRunState | None:
    """Compatibility-preserving public API.

    Historical v4/v5 form: apply_ops(project_root, patch_name, ops,
    default_on_error=...).  Current internal/state form remains accepted so an
    older import cannot force a rewrite of newer patches.
    """
    if isinstance(project_root_or_state, PatchRunState):
        state = project_root_or_state
        if not isinstance(patch_name_or_ops, list):
            raise PatchFailure("<ops>", "state-form apply_ops requires ops array")
        _apply_ops_state(state, patch_name_or_ops, inherited_on_error=inherited_on_error or default_on_error)
        return None
    root = Path(project_root_or_state).resolve()
    patch_name = str(patch_name_or_ops)
    op_list = list(ops or [])
    state = PatchRunState(project_root=root, patch_name=patch_name)
    try:
        _apply_ops_state(state, op_list, inherited_on_error=default_on_error)
    except PatchFailure:
        pass
    return state


def run_ops(project_root: Path, payload: dict[str, Any], *, patch_name: str | None = None) -> PatchRunState:
    if not isinstance(payload, dict):
        raise PatchFailure("<ops>", "PATCH_TOOL_OPS.json root must be an object")
    ops = payload.get("ops")
    if not isinstance(ops, list):
        raise PatchFailure("<ops>", "PATCH_TOOL_OPS.json requires ops[]")
    state = PatchRunState(project_root=project_root.resolve(), patch_name=patch_name or str(payload.get("patch_name") or "patch"))
    _apply_ops_state(state, ops, inherited_on_error=str(payload.get("default_on_error", "stop")))
    return state


def _diagnostic_reference_paths(ops: Any) -> set[str]:
    paths: set[str] = set()
    if not isinstance(ops, list):
        return paths
    for op in ops:
        if not isinstance(op, dict):
            continue
        for raw in (op.get("file"),):
            if isinstance(raw, str) and raw.strip():
                try: paths.add(_safe_rel(raw).as_posix())
                except PatchFailure: pass
        cond = op.get("condition")
        if isinstance(cond, dict):
            for raw in (cond.get("file"), cond.get("path_exists")):
                if isinstance(raw, str) and raw.strip():
                    try: paths.add(_safe_rel(raw).as_posix())
                    except PatchFailure: pass
        for key in ("then", "else"):
            paths.update(_diagnostic_reference_paths(op.get(key)))
        alts = op.get("alternatives")
        if isinstance(alts, list):
            for alt in alts:
                if isinstance(alt, list): paths.update(_diagnostic_reference_paths(alt))
                elif isinstance(alt, dict): paths.update(_diagnostic_reference_paths([alt]))
    return paths


def diagnose_ops(project_root: Path, payload: dict[str, Any]) -> dict[str, Any]:
    """Simulate a data-only OPS patch against a private temporary mirror.

    This is intentionally read-only with respect to the real project while still
    preserving sequential operation semantics, so an operation that depends on
    an earlier operation is evaluated after that virtual change.
    """
    ops = payload.get("ops") if isinstance(payload, dict) else None
    if not isinstance(ops, list):
        return {"status": "FAIL", "kind": "ops_invalid", "message": "PATCH_TOOL_OPS.json requires ops[]"}
    root = project_root.resolve()
    refs = _diagnostic_reference_paths(ops)
    with tempfile.TemporaryDirectory(prefix="ptv-ops-dryrun-") as name:
        mirror = Path(name)
        (mirror / "patchs").mkdir(parents=True, exist_ok=True)
        for rel in sorted(refs):
            pure = _safe_rel(rel)
            src = root.joinpath(*pure.parts)
            dst = mirror.joinpath(*pure.parts)
            try:
                if src.is_symlink():
                    return {"status": "FAIL", "kind": "source_drift", "path": rel, "message": "OPS diagnostic refuses symlink source"}
                if src.is_file():
                    dst.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(src, dst)
                elif src.is_dir():
                    dst.mkdir(parents=True, exist_ok=True)
                else:
                    # Preserve existing parent-directory topology for create/write ops.
                    parent = src.parent
                    chain: list[Path] = []
                    while parent != root and parent != parent.parent:
                        chain.append(parent)
                        parent = parent.parent
                    for original in reversed(chain):
                        if original.is_dir() and not original.is_symlink():
                            rel_parent = original.relative_to(root)
                            mirror.joinpath(*rel_parent.parts).mkdir(parents=True, exist_ok=True)
            except (OSError, ValueError) as exc:
                return {"status": "FAIL", "kind": "source_drift", "path": rel, "message": f"OPS diagnostic could not mirror source: {type(exc).__name__}: {exc}"}
        capture = io.StringIO()
        try:
            with contextlib.redirect_stdout(capture):
                state = run_ops(mirror, payload, patch_name="inspect-dry-run")
            return {
                "status": "PASS", "kind": "ops_ready", "operations": len(ops),
                "patched": state.stats.patched, "created": state.stats.created,
                "unchanged": state.stats.unchanged,
            }
        except PatchFailure as exc:
            return {
                "status": "FAIL", "kind": (
                    "anchor_mismatch" if exc.expected or "not found" in exc.message.lower() or "ambiguous" in exc.message.lower()
                    else "source_drift" if "does not exist" in exc.message.lower() or "target" in exc.message.lower()
                    else "patch_operation_failed"
                ),
                "path": exc.rel_path, "operation": exc.op_id, "message": exc.message,
                "expected": exc.expected, "anchor": exc.anchor, "strategy": exc.strategy,
            }


def find_project_root(start: Optional[Path] = None) -> Path:
    cwd = (start or Path.cwd()).resolve()
    if (cwd / "patchs").is_dir():
        return cwd
    for parent in [cwd, *cwd.parents]:
        if parent.name == "patchs":
            return parent.parent
        if (parent / "patchs").is_dir():
            return parent
    raise RuntimeError("Cannot determine project root. Run patch from project root or <project>/patchs.")


def print_summary(state: PatchRunState) -> None:
    st = state.stats
    print("Patch summary:")
    print(f"  patched : {st.patched}")
    print(f"  created : {st.created}")
    print(f"  unchanged/check: {st.unchanged}")
    print(f"  backups : {st.backups}")
    print(f"  failed  : {st.failed}")
    if st.skipped: print(f"  skipped : {st.skipped}")
    if st.ignored: print(f"  ignored : {st.ignored}")
    if state.failed_files:
        print("Failed files:")
        for rel in sorted(state.failed_files): print(f"  - {rel}")


def zip_failed_files(state: PatchRunState) -> Optional[Path]:
    if not state.failed_files:
        return None
    ts = _dt.datetime.now().strftime("%Y%m%d_%H%M%S")
    out_dir = state.project_root / "patchs" / "failed_patch_files"
    out_dir.mkdir(parents=True, exist_ok=True)
    safe = re.sub(r"[^A-Za-z0-9._-]+", "_", state.patch_name)[:96] or "patch"
    zip_path = out_dir / f"{safe}_failed_{ts}.zip"
    failure_map = {exc.rel_path: exc for exc in state.failures}
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        if sys.argv and sys.argv[0]:
            argv0 = Path(sys.argv[0])
            if argv0.is_file() and argv0.suffix.lower() == ".py":
                zf.write(argv0, arcname=f"patchs/{argv0.name}")
        for rel in sorted(state.failed_files):
            path = state.project_root / rel
            if path.is_file() and not path.is_symlink():
                zf.write(path, arcname=rel)
            else:
                exc = failure_map.get(rel)
                text = exc.expected if exc and exc.expected else f"// ERROR: File {rel} was not found during patch execution.\n"
                zf.writestr(rel, text)
    print(f"failed-files zip: {zip_path.resolve()}")
    return zip_path


def maybe_prompt_zip_failed_files(state: PatchRunState, *, force: Optional[bool] = None) -> Optional[Path]:
    if not state.failed_files:
        return None
    if force is None and os.environ.get("PTV_LEGACY_ZIP_FAILED") == "1":
        force = True
    if force is False:
        return None
    if force is None:
        if not sys.stdin.isatty():
            return None
        answer = input("Có zip toàn bộ file patch lỗi để gửi lên ChatGPT không? [Y/n]: ").strip().lower()
        force = answer in {"", "y", "yes", "c", "co", "có"}
    if not force:
        return None
    zip_path = zip_failed_files(state)
    # --keep-failed-zip explicitly suppresses the historical post-create delete prompt.
    if zip_path and os.environ.get("PTV_LEGACY_KEEP_FAILED_ZIP") != "1" and sys.stdin.isatty():
        answer = input(f"Delete this generated zip file: {zip_path.name}? [Y/n]: ").strip().lower()
        if answer in {"", "y", "yes", "c", "co", "có"}:
            zip_path.unlink(missing_ok=True)
    return zip_path


def run_patch(
    patch_name: str,
    ops: Iterable[dict[str, Any]],
    *,
    default_on_error: str = "stop",
    prompt_zip_on_error: Optional[bool] = None,
) -> int:
    root = find_project_root()
    state = apply_ops(root, patch_name, list(ops), default_on_error=default_on_error)
    assert isinstance(state, PatchRunState)
    print_summary(state)
    maybe_prompt_zip_failed_files(state, force=prompt_zip_on_error)
    if state.failures:
        print("Patch completed with errors.")
        return 1
    print("Patch completed successfully.")
    return 0


# Backward-compatible helpers used by existing Python patches.
def replace_exact_once(project_root: Path, rel_path: str, old: str, new: str, patch_name: str, *, anchor: Optional[str] = None, context_lines: int = 6) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_replace(state, {"file": rel_path, "old": old, "new": new, "mode": "exact", "anchor": anchor, "context_lines": context_lines})


def replace_ws_once(project_root: Path, rel_path: str, old: str, new: str, patch_name: str, *, anchor: Optional[str] = None, context_lines: int = 6) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_replace(state, {"file": rel_path, "old": old, "new": new, "mode": "normalized_ws", "anchor": anchor, "context_lines": context_lines})


def replace_fuzzy_once(project_root: Path, rel_path: str, old: str, new: str, patch_name: str, *, anchor: Optional[str] = None, fuzzy_min: float = 0.88, context_lines: int = 6) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_replace(state, {"file": rel_path, "old": old, "new": new, "mode": "fuzzy", "anchor": anchor, "fuzzy_min": fuzzy_min, "context_lines": context_lines})


def replace_once(project_root: Path, rel_path: str, old: str, new: str, patch_name: str, **kwargs) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_replace(state, {"file": rel_path, "old": old, "new": new, "mode": kwargs.get("mode", "auto"), **kwargs})


def insert_after_once(project_root: Path, rel_path: str, anchor: str, insertion: str, patch_name: str, *, context_lines: int = 6) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_insert(state, {"file": rel_path, "anchor": anchor, "insert": insertion, "context_lines": context_lines}, before=False)


def insert_before_once(project_root: Path, rel_path: str, anchor: str, insertion: str, patch_name: str, *, context_lines: int = 6) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_insert(state, {"file": rel_path, "anchor": anchor, "insert": insertion, "context_lines": context_lines}, before=True)


def write_file_if_changed(project_root: Path, rel_path: str, content: str, patch_name: str) -> bool:
    state = PatchRunState(project_root.resolve(), patch_name)
    return op_write(state, {"file": rel_path, "content": content, "create": True})


def finish_success() -> None:
    print("Patch completed successfully.")


def print_patch_error(exc: PatchFailure) -> None:
    print(f"ERROR: {exc.rel_path}: {exc.message}")
    if exc.op_id:
        print(f"Operation: {exc.op_id}")
    if exc.strategy:
        print(f"Strategy: {exc.strategy}")
    if exc.anchor:
        print(f"Anchor: {exc.anchor}")
    if exc.context:
        print(exc.context)


def finish_failure(exc: Exception) -> int:
    if isinstance(exc, PatchFailure):
        print_patch_error(exc)
    else:
        print(f"ERROR: unexpected patch failure: {exc}")
    return 1
