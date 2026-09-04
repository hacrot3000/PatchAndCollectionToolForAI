#!/usr/bin/env python3
from __future__ import annotations

import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
from typing import Any

VERSION = "6.20.0"

ALLOWED_OPERATIONS = frozenset({
    "status", "current_branch", "branches", "log", "show",
    "diff_worktree", "diff_staged", "diff_refs", "diff_ref_worktree", "switch",
})
MUTATING_OPERATIONS = frozenset({
    "add", "commit", "merge", "rebase", "reset", "push", "pull", "cherry-pick", "checkout",
})
FORBIDDEN_ESCAPE_FIELDS = frozenset({"argv", "command", "raw_git", "args", "shell"})

class GitSafeError(ValueError):
    pass


def _safe_rel_dir(value: Any, label: str) -> str:
    if value in (None, "", "."):
        return "."
    if not isinstance(value, str) or "\\" in value:
        raise GitSafeError(f"{label} must be a POSIX project-relative directory")
    rel = PurePosixPath(value.strip())
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts) or any(":" in part for part in rel.parts):
        raise GitSafeError(f"unsafe {label}: {value}")
    return rel.as_posix()


def _safe_paths(value: Any, label: str) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise GitSafeError(f"{label} must be an array of project-relative paths")
    out: list[str] = []
    for i, raw in enumerate(value, 1):
        if not isinstance(raw, str) or not raw.strip() or "\\" in raw:
            raise GitSafeError(f"{label}[{i}] must be a non-empty POSIX relative path")
        rel = PurePosixPath(raw.strip())
        if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts) or any(":" in part for part in rel.parts):
            raise GitSafeError(f"unsafe {label}[{i}]: {raw}")
        if rel.as_posix() not in out:
            out.append(rel.as_posix())
    return out


def _safe_ref(value: Any, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise GitSafeError(f"{label} must be a non-empty Git ref")
    ref = value.strip()
    if ref.startswith("-") or "\x00" in ref or any(ch.isspace() for ch in ref):
        raise GitSafeError(f"unsafe {label}: {value!r}")
    if len(ref) > 512 or re.search(r"[~^:?*\\\[]", ref):
        # Revisions with ^/~ and ref globs make option/ref parsing less obvious.
        # Exact commit ids, tags, branch names and HEAD are sufficient here.
        raise GitSafeError(f"{label} must be an exact branch/tag/commit ref without revision operators")
    return ref


def _safe_branch(value: Any, label: str) -> str:
    branch = _safe_ref(value, label)
    if branch in {"HEAD", "@", "-"} or branch.startswith("refs/"):
        raise GitSafeError(f"{label} must name an existing local branch")
    return branch


def validate_git_action(action: dict[str, Any], *, schema_git_sections: list[str] | None = None) -> dict[str, Any]:
    if not isinstance(action, dict):
        raise GitSafeError("git action must be an object")
    allowed_action_fields = {"id", "title", "type", "repo", "operations", "sections", "log_entries"}
    extra = sorted(set(action) - allowed_action_fields)
    if extra:
        raise GitSafeError(f"git action contains unsupported field(s): {', '.join(extra)}")
    norm = dict(action)
    norm["repo"] = _safe_rel_dir(norm.get("repo", "."), "git.repo")

    operations = norm.get("operations")
    # Historical v6.19.x fixed-section COLLECT requests remain accepted and are
    # normalized into the strict v6.20.0 operation model.
    if operations is None and norm.get("sections") is not None:
        sections = norm.get("sections")
        allowed_sections = set(schema_git_sections or ["status", "log", "diff_stat", "diff"])
        if not isinstance(sections, list) or not sections:
            raise GitSafeError("git.sections must be a non-empty array")
        max_entries = norm.get("log_entries", 20)
        if not isinstance(max_entries, int) or isinstance(max_entries, bool) or not 1 <= max_entries <= 200:
            raise GitSafeError("git.log_entries must be an integer from 1 to 200")
        converted: list[dict[str, Any]] = []
        for i, section in enumerate(sections, 1):
            if not isinstance(section, str) or section not in allowed_sections:
                raise GitSafeError(f"git.sections[{i}] unsupported; allowed={','.join(sorted(allowed_sections))}")
            if section == "status": converted.append({"op": "status"})
            elif section == "log": converted.append({"op": "log", "ref": "HEAD", "max_entries": max_entries})
            elif section == "diff_stat": converted.append({"op": "diff_worktree", "stat_only": True})
            elif section == "diff": converted.append({"op": "diff_worktree"})
        operations = converted
    if not isinstance(operations, list) or not operations:
        raise GitSafeError("git.operations must be a non-empty array")

    normalized: list[dict[str, Any]] = []
    for index, raw in enumerate(operations, 1):
        label = f"git.operations[{index}]"
        if not isinstance(raw, dict):
            raise GitSafeError(f"{label} must be an object")
        escape = sorted(set(raw) & FORBIDDEN_ESCAPE_FIELDS)
        if escape:
            raise GitSafeError(f"{label} raw Git escape field(s) forbidden: {', '.join(escape)}")
        op = raw.get("op")
        if not isinstance(op, str) or not op.strip():
            raise GitSafeError(f"{label}.op must be a non-empty string")
        op = op.strip().lower()
        if op in MUTATING_OPERATIONS:
            raise GitSafeError(f"{label}.op={op!r} is forbidden; Git mutation is not supported")
        if op not in ALLOWED_OPERATIONS:
            raise GitSafeError(f"{label}.op unsupported; allowed={','.join(sorted(ALLOWED_OPERATIONS))}")
        allowed_by_op = {
            "status": {"op"},
            "current_branch": {"op"},
            "branches": {"op"},
            "log": {"op", "ref", "max_entries", "paths"},
            "show": {"op", "ref", "paths"},
            "diff_worktree": {"op", "paths", "stat_only"},
            "diff_staged": {"op", "paths", "stat_only"},
            "diff_refs": {"op", "from", "to", "paths", "stat_only"},
            "diff_ref_worktree": {"op", "ref", "paths", "stat_only"},
            "switch": {"op", "branch"},
        }[op]
        unknown = sorted(set(raw) - allowed_by_op)
        if unknown:
            raise GitSafeError(f"{label} contains unsupported field(s) for {op}: {', '.join(unknown)}")
        item: dict[str, Any] = {"op": op}
        if "paths" in allowed_by_op:
            item["paths"] = _safe_paths(raw.get("paths"), f"{label}.paths")
        if "stat_only" in allowed_by_op:
            stat = raw.get("stat_only", False)
            if not isinstance(stat, bool): raise GitSafeError(f"{label}.stat_only must be boolean")
            item["stat_only"] = stat
        if op == "log":
            item["ref"] = _safe_ref(raw.get("ref", "HEAD"), f"{label}.ref")
            n = raw.get("max_entries", 20)
            if not isinstance(n, int) or isinstance(n, bool) or not 1 <= n <= 500:
                raise GitSafeError(f"{label}.max_entries must be an integer from 1 to 500")
            item["max_entries"] = n
        elif op == "show": item["ref"] = _safe_ref(raw.get("ref"), f"{label}.ref")
        elif op == "diff_refs":
            item["from"] = _safe_ref(raw.get("from"), f"{label}.from")
            item["to"] = _safe_ref(raw.get("to"), f"{label}.to")
        elif op == "diff_ref_worktree": item["ref"] = _safe_ref(raw.get("ref"), f"{label}.ref")
        elif op == "switch": item["branch"] = _safe_branch(raw.get("branch"), f"{label}.branch")
        normalized.append(item)
    norm["operations"] = normalized
    norm.pop("sections", None)
    norm.pop("log_entries", None)
    return norm


def _linkish(path: Path) -> bool:
    try:
        st = path.lstat()
    except OSError:
        return False
    attrs = int(getattr(st, "st_file_attributes", 0) or 0)
    reparse = int(getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
    return stat.S_ISLNK(st.st_mode) or (os.name == "nt" and bool(attrs & reparse))


def _repo_path(root: Path, rel: str) -> Path:
    original_root = root
    root = root.resolve(strict=True)
    if _linkish(original_root) or not root.is_dir():
        raise GitSafeError("project root must be a real directory")
    if rel == ".":
        return root
    current = root
    for part in PurePosixPath(rel).parts:
        current = current / part
        if _linkish(current) or not current.is_dir():
            raise GitSafeError(f"git.repo contains a missing/linked/non-directory component: {rel}")
        resolved = current.resolve(strict=True)
        try: resolved.relative_to(root)
        except ValueError as exc: raise GitSafeError(f"git.repo escapes project root: {rel}") from exc
        if resolved != current.absolute():
            raise GitSafeError(f"git.repo contains a linked/reparsed ancestor: {rel}")
    return current


def _env() -> dict[str, str]:
    env = dict(os.environ)
    env.update({"GIT_PAGER": "cat", "PAGER": "cat", "GIT_OPTIONAL_LOCKS": "0"})
    return env


def _run(repo: Path, args: list[str], *, timeout: int = 30) -> subprocess.CompletedProcess[str]:
    cmd = ["git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", *args]
    try:
        cp = subprocess.run(cmd, cwd=repo, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, errors="replace", timeout=timeout, env=_env())
    except subprocess.TimeoutExpired as exc:
        raise GitSafeError(f"Git operation timed out after {timeout}s") from exc
    except OSError as exc:
        raise GitSafeError(f"Git executable unavailable: {type(exc).__name__}: {exc}") from exc
    return cp


def _ensure_repo(repo: Path) -> None:
    cp = _run(repo, ["rev-parse", "--show-toplevel"])
    if cp.returncode != 0:
        raise GitSafeError("git.repo is not a Git working tree")
    try: top = Path(cp.stdout.strip()).resolve(strict=True)
    except Exception as exc: raise GitSafeError("Git returned an invalid worktree root") from exc
    if top != repo:
        raise GitSafeError(f"git.repo must point to the repository root, not a parent/subdirectory: {repo}")


def _verify_ref(repo: Path, ref: str) -> None:
    cp = _run(repo, ["rev-parse", "--verify", "--quiet", "--end-of-options", f"{ref}^{{commit}}"])
    if cp.returncode != 0:
        raise GitSafeError(f"Git ref not found or not a commit: {ref}")


def _fmt_result(op: str, cp: subprocess.CompletedProcess[str]) -> str:
    text = (cp.stdout or "").rstrip()
    if cp.returncode != 0:
        detail = (cp.stderr or "").strip().replace("\n", " ")[:1200]
        return f"[GIT OP FAILED rc={cp.returncode}]" + (f" {detail}" if detail else "")
    warning = (cp.stderr or "").strip().replace("\n", " ")[:1200]
    if warning:
        text = (text + "\n" if text else "") + f"[git warning] {warning}"
    return text


def _diff_args(*, cached: bool = False, stat_only: bool = False) -> list[str]:
    args = ["diff", "--no-ext-diff", "--no-textconv"]
    if cached: args.append("--cached")
    if stat_only: args.append("--stat")
    else: args.append("--unified=3")
    return args


def execute_git_operation(repo: Path, op: dict[str, Any]) -> tuple[str, str]:
    kind = str(op["op"])
    paths = list(op.get("paths") or [])
    path_tail = ["--", *paths] if paths else []
    if kind == "status":
        cp = _run(repo, ["status", "--short", "--branch", "--untracked-files=all"])
    elif kind == "current_branch":
        cp = _run(repo, ["branch", "--show-current"])
    elif kind == "branches":
        cp = _run(repo, ["for-each-ref", "--format=%(refname:short)%09%(objectname:short)%09%(HEAD)", "refs/heads/"])
    elif kind == "log":
        _verify_ref(repo, op["ref"])
        cp = _run(repo, ["log", "--no-show-signature", "--decorate", "--date=iso-strict", f"-n{op['max_entries']}",
                         "--pretty=format:%h%x09%ad%x09%d%x09%s%x09[%an]", op["ref"], *path_tail], timeout=60)
    elif kind == "show":
        _verify_ref(repo, op["ref"])
        cp = _run(repo, ["show", "--no-show-signature", "--no-ext-diff", "--no-textconv", "--decorate", "--stat", "--patch", op["ref"], *path_tail], timeout=60)
    elif kind == "diff_worktree":
        cp = _run(repo, [*_diff_args(stat_only=bool(op.get("stat_only"))), *path_tail], timeout=60)
    elif kind == "diff_staged":
        cp = _run(repo, [*_diff_args(cached=True, stat_only=bool(op.get("stat_only"))), *path_tail], timeout=60)
    elif kind == "diff_refs":
        _verify_ref(repo, op["from"]); _verify_ref(repo, op["to"])
        cp = _run(repo, [*_diff_args(stat_only=bool(op.get("stat_only"))), op["from"], op["to"], *path_tail], timeout=60)
    elif kind == "diff_ref_worktree":
        _verify_ref(repo, op["ref"])
        cp = _run(repo, [*_diff_args(stat_only=bool(op.get("stat_only"))), op["ref"], *path_tail], timeout=60)
    elif kind == "switch":
        branch = op["branch"]
        exists = _run(repo, ["show-ref", "--verify", "--quiet", f"refs/heads/{branch}"])
        if exists.returncode != 0:
            raise GitSafeError(f"switch only allows an existing local branch: {branch}")
        dirty = _run(repo, ["status", "--porcelain=v1", "--untracked-files=all"])
        if dirty.returncode != 0:
            raise GitSafeError("cannot verify worktree cleanliness before switch")
        if dirty.stdout:
            raise GitSafeError("switch refused: worktree/index/untracked state is not clean")
        cp = _run(repo, ["switch", "--no-guess", branch], timeout=60)
        if cp.returncode == 0:
            verify = _run(repo, ["branch", "--show-current"])
            if verify.returncode != 0 or verify.stdout.strip() != branch:
                raise GitSafeError("switch verification failed; current branch did not match requested local branch")
    else:
        raise GitSafeError(f"unsupported Git operation: {kind}")
    return kind, _fmt_result(kind, cp)


def run_git_operations(project_root: Path, action: dict[str, Any]) -> str:
    norm = validate_git_action(action)
    repo = _repo_path(project_root, norm["repo"])
    _ensure_repo(repo)
    blocks = ["# Git safe operations", "", f"Repository: `{norm['repo']}`", ""]
    for index, op in enumerate(norm["operations"], 1):
        kind = op["op"]
        blocks += [f"## {index}. {kind}", "```text"]
        try:
            _, text = execute_git_operation(repo, op)
        except GitSafeError as exc:
            text = f"[GIT OP REJECTED] {exc}"
        blocks += [text, "```", ""]
    return "\n".join(blocks) + "\n"
