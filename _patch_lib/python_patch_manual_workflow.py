#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shlex
import stat
import sys
import tempfile
from datetime import datetime, timezone
from typing import Any, Callable
import zipfile

from python_patch_version import VERSION
_EXIT_RE = re.compile(r"(?m)^\[PTV_MANUAL_EXIT_CODE=(-?\d+)\]\s*$")
_EXIT_TAIL_BYTES = 64 * 1024
_FORBIDDEN_INLINE = {
    "bash": "shell", "sh": "shell", "zsh": "shell", "dash": "shell", "ksh": "shell",
    "python": "python", "python3": "python", "pypy": "python", "pypy3": "python",
    "node": "node",
    "pwsh": "powershell", "powershell": "powershell", "powershell.exe": "powershell", "pwsh.exe": "powershell",
    "cmd": "cmd", "cmd.exe": "cmd",
}


def _basename_lower(value: str) -> str:
    return Path(value.replace("\\", "/")).name.lower()


def _unwrap_manual_argv(argv: list[str]) -> tuple[str, list[str]]:
    """Return the effective executable and its arguments for inline-eval checks.

    Manual commands remain operator-executed and are intentionally not reduced
    to a tiny executable allowlist.  We only unwrap common launchers that can
    otherwise hide an inline shell/evaluator escape from the existing policy.
    """
    exe = _basename_lower(argv[0])
    args = list(argv[1:])
    if exe in {"env", "env.exe"}:
        i = 0
        while i < len(args):
            token = args[i]
            low = token.lower()
            if token == "--":
                i += 1
                break
            if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", token):
                i += 1
                continue
            if token == "-S" or low == "--split-string" or low.startswith("--split-string="):
                # GNU env -S/--split-string reparses one string into argv and
                # can therefore hide bash -c/python -c style evaluators from
                # the structured-argv policy.  Do not attempt to emulate that
                # parser here; reject this wrapper fail-closed.
                return "__ptv_forbidden_inline_wrapper__", []
            if low in {"-i", "--ignore-environment", "-0", "--null"}:
                i += 1
                continue
            if low in {"-u", "--unset", "-c", "--chdir"}:
                i += 2
                continue
            if low.startswith("--unset=") or low.startswith("--chdir="):
                i += 1
                continue
            if token.startswith("-"):
                # Unknown env option: fail closed by treating env itself as the
                # effective program rather than guessing where its command is.
                return exe, args
            break
        if i < len(args):
            exe = _basename_lower(args[i])
            args = args[i + 1 :]
    elif exe in {"busybox", "busybox.exe"} and args:
        applet = _basename_lower(args[0])
        if applet in _FORBIDDEN_INLINE:
            exe, args = applet, args[1:]
    return exe, args


def _has_forbidden_inline_eval(exe: str, args: list[str]) -> bool:
    if exe == "__ptv_forbidden_inline_wrapper__":
        return True
    kind = _FORBIDDEN_INLINE.get(exe)
    if kind is None:
        return False
    for raw in args:
        token = str(raw).strip().lower()
        if kind == "shell":
            # POSIX shells accept short options as clusters: -lc, -xc,
            # -ec, etc.  Any short-option cluster containing c enables an
            # inline command string and must be rejected fail-closed.
            if token.startswith("-") and not token.startswith("--") and "c" in token[1:]:
                return True
        elif kind == "python":
            if token == "-c" or token.startswith("-c"):
                return True
        elif kind == "node":
            if token == "-e" or token.startswith("-e") or token == "--eval" or token.startswith("--eval="):
                return True
        elif kind == "powershell":
            head = token.split("=", 1)[0].split(":", 1)[0]
            if head in {"-c", "/c", "-command", "/command", "-enc", "-encodedcommand"}:
                return True
            if token.startswith("-command:") or token.startswith("-command=") or token.startswith("-encodedcommand:") or token.startswith("-encodedcommand="):
                return True
        elif kind == "cmd":
            if token in {"/c", "/k"} or token.startswith("/c") or token.startswith("/k"):
                return True
    return False


class ManualWorkflowError(ValueError):
    pass


def _safe_rel_dir(value: Any, label: str) -> str:
    if value in (None, "", "."):
        return "."
    if not isinstance(value, str) or "\\" in value:
        raise ManualWorkflowError(f"{label} must be a POSIX project-relative directory")
    rel = PurePosixPath(value.strip())
    if rel.is_absolute() or any(part in {"", ".", ".."} for part in rel.parts) or any(":" in part for part in rel.parts):
        raise ManualWorkflowError(f"unsafe {label}: {value}")
    return rel.as_posix()


def validate_manual_execution(value: Any) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ManualWorkflowError("manual_execution must be an object")
    allowed = {"stop_on_failure", "package_result", "steps"}
    extra = sorted(set(value) - allowed)
    if extra: raise ManualWorkflowError(f"manual_execution contains unsupported field(s): {', '.join(extra)}")
    stop = value.get("stop_on_failure", True)
    package_result = value.get("package_result", True)
    if not isinstance(stop, bool): raise ManualWorkflowError("manual_execution.stop_on_failure must be boolean")
    if not isinstance(package_result, bool): raise ManualWorkflowError("manual_execution.package_result must be boolean")
    raw_steps = value.get("steps")
    if not isinstance(raw_steps, list) or not raw_steps:
        raise ManualWorkflowError("manual_execution.steps must be a non-empty array")
    if len(raw_steps) > 100: raise ManualWorkflowError("manual_execution.steps exceeds maximum 100")
    seen: set[str] = set(); steps: list[dict[str, Any]] = []
    for index, raw in enumerate(raw_steps, 1):
        label=f"manual_execution.steps[{index}]"
        if not isinstance(raw, dict): raise ManualWorkflowError(f"{label} must be an object")
        allowed_step={"id","title","description","cwd","argv","expected_exit_codes"}
        extra=sorted(set(raw)-allowed_step)
        if extra: raise ManualWorkflowError(f"{label} contains unsupported field(s): {', '.join(extra)}")
        step_id=raw.get("id")
        if not isinstance(step_id,str) or not re.fullmatch(r"[A-Za-z0-9_.-]{1,80}",step_id):
            raise ManualWorkflowError(f"{label}.id must match [A-Za-z0-9_.-] and be 1..80 chars")
        if step_id in seen: raise ManualWorkflowError(f"duplicate manual step id: {step_id}")
        seen.add(step_id)
        title=raw.get("title",step_id)
        description=raw.get("description","")
        if not isinstance(title,str) or not title.strip(): raise ManualWorkflowError(f"{label}.title must be a non-empty string")
        if not isinstance(description,str): raise ManualWorkflowError(f"{label}.description must be a string")
        cwd=_safe_rel_dir(raw.get("cwd","."),f"{label}.cwd")
        argv=raw.get("argv")
        if not isinstance(argv,list) or not argv or not all(isinstance(x,str) and x for x in argv):
            raise ManualWorkflowError(f"{label}.argv must be a non-empty string array")
        exe, exe_args = _unwrap_manual_argv(argv)
        if _has_forbidden_inline_eval(exe, exe_args):
            raise ManualWorkflowError(f"{label}.argv inline shell/eval execution is forbidden for {exe}")
        expected=raw.get("expected_exit_codes",[0])
        if not isinstance(expected,list) or not expected or len(expected)>32 or not all(isinstance(x,int) and not isinstance(x,bool) and -255<=x<=255 for x in expected):
            raise ManualWorkflowError(f"{label}.expected_exit_codes must be 1..32 integers from -255 to 255")
        steps.append({"id":step_id,"title":title.strip(),"description":description,"cwd":cwd,"argv":list(argv),"expected_exit_codes":list(dict.fromkeys(expected))})
    return {"stop_on_failure":stop,"package_result":package_result,"steps":steps}


def _unsafe_linkish(path: Path) -> bool:
    try:
        st = path.lstat()
    except OSError:
        return False
    attrs = int(getattr(st, "st_file_attributes", 0) or 0)
    reparse = int(getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0x400))
    return stat.S_ISLNK(st.st_mode) or (os.name == "nt" and bool(attrs & reparse))


def _safe_dir_chain(root: Path, parts: list[str], *, create: bool) -> Path:
    base = root.resolve(strict=True)
    if _unsafe_linkish(root) or not base.is_dir():
        raise ManualWorkflowError("project root must be a real directory")
    current = base
    for part in parts:
        if not part or part in {".", ".."} or "/" in part or "\\" in part:
            raise ManualWorkflowError(f"unsafe manual workflow directory component: {part!r}")
        nxt = current / part
        if nxt.exists() or _unsafe_linkish(nxt):
            if _unsafe_linkish(nxt) or not nxt.is_dir():
                raise ManualWorkflowError(f"unsafe manual workflow directory component: {nxt}")
        elif create:
            try:
                nxt.mkdir(mode=0o700)
            except FileExistsError:
                if _unsafe_linkish(nxt) or not nxt.is_dir():
                    raise ManualWorkflowError(f"unsafe manual workflow directory created concurrently: {nxt}")
        else:
            raise ManualWorkflowError(f"manual step cwd does not exist: {nxt}")
        try:
            resolved = nxt.resolve(strict=True)
            resolved.relative_to(base)
        except (OSError, ValueError) as exc:
            raise ManualWorkflowError(f"manual workflow directory escapes project root: {nxt}") from exc
        if resolved != nxt.absolute():
            # Resolving through any symlink/junction ancestor changes the path.
            raise ManualWorkflowError(f"manual workflow directory has a linked/reparsed ancestor: {nxt}")
        current = nxt
    return current


def resolve_manual_cwd(root: Path, rel: str) -> Path:
    base=root.resolve(strict=True)
    if rel==".":
        if _unsafe_linkish(root) or not base.is_dir(): raise ManualWorkflowError("project root must be a real directory")
        return base
    return _safe_dir_chain(base, list(PurePosixPath(rel).parts), create=False)


def _slug(value: str, limit: int=70) -> str:
    text=re.sub(r"[^A-Za-z0-9._-]+","_",value).strip("._-") or "patch"
    return text[:limit]


def _ps_quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def build_capture_command(cwd: Path, argv: list[str], log_file: Path) -> str:
    if os.name == "nt":
        args=" ".join(_ps_quote(x) for x in argv)
        return (
            f"Set-Location -LiteralPath {_ps_quote(str(cwd))}; "
            f"Remove-Item -LiteralPath {_ps_quote(str(log_file))} -Force -ErrorAction SilentlyContinue; "
            f"& {args} 2>&1 | Tee-Object -FilePath {_ps_quote(str(log_file))}; "
            "$rc=$LASTEXITCODE; Add-Content -LiteralPath " + _ps_quote(str(log_file)) +
            " -Value \"`n[PTV_MANUAL_EXIT_CODE=$rc]\"; exit $rc"
        )
    inner=(
        "set -o pipefail; "
        f"cd -- {shlex.quote(str(cwd))}; "
        f"rm -f -- {shlex.quote(str(log_file))}; "
        f"{shlex.join(argv)} 2>&1 | tee {shlex.quote(str(log_file))}; "
        "rc=${PIPESTATUS[0]}; "
        f"printf '\\n[PTV_MANUAL_EXIT_CODE=%d]\\n' \"$rc\" | tee -a {shlex.quote(str(log_file))}; "
        "exit \"$rc\""
    )
    return "bash -lc " + shlex.quote(inner)


def _instruction_text(index:int,total:int,step:dict[str,Any],cwd:Path,log_file:Path,capture:str)->str:
    return (
        f"MANUAL EXECUTION STEP {index}/{total}\n\n"
        f"Title:\n{step['title']}\n\n"
        + (f"Description:\n{step['description']}\n\n" if step.get('description') else "")
        + f"Working dir:\n{cwd}\n\n"
        f"Command:\n{shlex.join(step['argv'])}\n\n"
        f"Expected exit codes:\n{', '.join(str(x) for x in step['expected_exit_codes'])}\n\n"
        f"Log file:\n{log_file}\n\n"
        "Run this in A NEW TERMINAL (capture command):\n"
        f"{capture}\n\n"
        "Then return to Patch Tool:\n"
        "  Enter = verify log and continue\n"
        "  r     = show these instructions again\n"
        "  m     = use console log copied manually + enter exit code\n"
        "  q     = abort manual workflow\n"
    )


def _open_regular_nofollow(path: Path, flags: int) -> int:
    if _unsafe_linkish(path):
        raise ManualWorkflowError(f"unsafe linked/reparsed manual evidence file: {path}")
    nofollow = int(getattr(os, "O_NOFOLLOW", 0) or 0)
    try:
        fd = os.open(path, flags | nofollow)
    except OSError as exc:
        raise ManualWorkflowError(f"cannot open manual evidence file safely: {path}: {exc}") from exc
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode):
            raise ManualWorkflowError(f"manual evidence path is not a regular file: {path}")
        return fd
    except Exception:
        os.close(fd)
        raise


def _read_tail_bytes(log_file: Path, limit: int = _EXIT_TAIL_BYTES) -> bytes | None:
    if not log_file.exists() or _unsafe_linkish(log_file):
        return None
    try:
        fd = _open_regular_nofollow(log_file, os.O_RDONLY)
    except ManualWorkflowError:
        return None
    try:
        size = os.fstat(fd).st_size
        os.lseek(fd, max(0, size - max(1024, int(limit))), os.SEEK_SET)
        chunks: list[bytes] = []
        remaining = min(size, max(1024, int(limit)))
        while remaining > 0:
            part = os.read(fd, min(65536, remaining))
            if not part:
                break
            chunks.append(part)
            remaining -= len(part)
        return b"".join(chunks)
    except OSError:
        return None
    finally:
        os.close(fd)


def _read_exit_code(log_file:Path)->int|None:
    raw = _read_tail_bytes(log_file)
    if raw is None:
        return None
    text = raw.decode("utf-8", errors="replace")
    matches=_EXIT_RE.findall(text)
    return int(matches[-1]) if matches else None


def _append_manual_exit(log_file:Path,rc:int)->None:
    fd = _open_regular_nofollow(log_file, os.O_WRONLY | os.O_APPEND)
    try:
        os.write(fd, f"\n[PTV_MANUAL_EXIT_CODE={rc}]\n".encode("utf-8"))
        os.fsync(fd)
    finally:
        os.close(fd)


def _evidence_metadata(log_file: Path) -> tuple[int, str]:
    fd = _open_regular_nofollow(log_file, os.O_RDONLY)
    try:
        h = hashlib.sha256()
        total = 0
        while True:
            chunk = os.read(fd, 1024 * 1024)
            if not chunk:
                break
            total += len(chunk)
            h.update(chunk)
        return total, h.hexdigest()
    finally:
        os.close(fd)


def _write_text_new(path: Path, text: str) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    nofollow = int(getattr(os, "O_NOFOLLOW", 0) or 0)
    try:
        fd = os.open(path, flags | nofollow, 0o600)
    except OSError as exc:
        raise ManualWorkflowError(f"cannot create manual workflow file safely: {path}: {exc}") from exc
    try:
        with os.fdopen(fd, "w", encoding="utf-8", errors="strict", newline="") as fh:
            fd = -1
            fh.write(text)
            fh.flush()
            os.fsync(fh.fileno())
    finally:
        if fd >= 0:
            os.close(fd)


def _write_json_atomic(path:Path,data:dict[str,Any])->None:
    path.parent.mkdir(parents=True,exist_ok=True)
    fd,tmp=tempfile.mkstemp(prefix=".ptv-manual-",suffix=".json",dir=path.parent); os.close(fd)
    t=Path(tmp)
    try:
        t.write_text(json.dumps(data,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
        os.replace(t,path)
    finally:
        try:t.unlink()
        except OSError:pass


def _report_md(report:dict[str,Any])->str:
    lines=["# Manual Execution Report","",f"- Tool version: `{VERSION}`",f"- Patch: `{report['patch']}`",f"- Status: **{report['status']}**",f"- Started: `{report['started_at']}`",f"- Finished: `{report.get('finished_at','')}`","","## Steps",""]
    for row in report.get("steps",[]):
        lines += [f"### {row['index']}. {row['title']}","",f"- Status: **{row['status']}**",f"- Exit code: `{row.get('exit_code')}`",f"- Expected: `{row.get('expected_exit_codes')}`",f"- CWD: `{row['cwd']}`",f"- Command: `{shlex.join(row['argv'])}`",f"- Log: `{row['log_file']}`",""]
    return "\n".join(lines)+"\n"


def _package_result(root:Path,work_dir:Path,report:dict[str,Any])->tuple[Path,Path]:
    out=_safe_dir_chain(root,["artifacts","patch_tool","manual"],create=True)
    stamp=datetime.now().strftime("%Y%m%d_%H%M%S_%f")
    final=out/f"MANUAL_EXECUTION_RESULT_{_slug(str(report['patch']))}_{stamp}.zip"
    report_json=json.dumps(report,ensure_ascii=False,indent=2)+"\n"
    report_md=_report_md(report)
    fd,tmp_name=tempfile.mkstemp(prefix=".ptv-manual-result-",suffix=".zip",dir=out)
    temp=Path(tmp_name)
    try:
        with os.fdopen(fd,"w+b") as raw_fh:
            fd=-1
            with zipfile.ZipFile(raw_fh,"w",compression=zipfile.ZIP_DEFLATED,compresslevel=6) as zf:
                zf.writestr("MANUAL_EXECUTION.json",report_json)
                zf.writestr("MANUAL_EXECUTION_REPORT.md",report_md)
                for p in sorted((work_dir/"steps").glob("*")):
                    if _unsafe_linkish(p) or not p.is_file():
                        continue
                    # Read through a no-follow descriptor before putting evidence
                    # into the ZIP so a concurrent symlink swap cannot redirect it.
                    evid_fd=_open_regular_nofollow(p,os.O_RDONLY)
                    try:
                        chunks=[]
                        while True:
                            chunk=os.read(evid_fd,1024*1024)
                            if not chunk: break
                            chunks.append(chunk)
                        zf.writestr(f"steps/{p.name}",b"".join(chunks))
                    finally:
                        os.close(evid_fd)
        os.replace(temp,final)
        with zipfile.ZipFile(final) as zf:
            if zf.testzip() is not None: raise ManualWorkflowError("manual execution result ZIP CRC check failed")
        from python_patch_cleartext_companion import create_zip_cleartext_companion
        txt=create_zip_cleartext_companion(final,artifact_kind="MANUAL EXECUTION RESULT")
        return final,txt
    finally:
        try:
            if 'fd' in locals() and fd >= 0: os.close(fd)
        except OSError: pass
        try:temp.unlink()
        except OSError:pass


def run_manual_workflow(root:Path, manifest:dict[str,Any], patch_name:str, *, input_fn:Callable[[str],str]=input)->dict[str,Any]|None:
    raw=manifest.get("manual_execution")
    if raw is None:return None
    cfg=validate_manual_execution(raw)
    started=datetime.now(timezone.utc).isoformat()
    stamp=datetime.now().strftime("%Y%m%d_%H%M%S_%f")
    work=_safe_dir_chain(root,["artifacts","ptv_manual",f"M_{_slug(patch_name,50)}_{stamp}_{os.getpid()}"],create=True)
    steps_dir=_safe_dir_chain(root,["artifacts","ptv_manual",work.name,"steps"],create=True)
    report:dict[str,Any]={"format":"python-patch-tool-manual-execution","format_version":1,"tool_version":VERSION,"patch":patch_name,"status":"RUNNING","started_at":started,"work_dir":work.relative_to(root).as_posix(),"stop_on_failure":cfg['stop_on_failure'],"package_result":cfg['package_result'],"steps":[]}
    any_fail=False; aborted=False
    try:
        for index,step in enumerate(cfg["steps"],1):
            cwd=resolve_manual_cwd(root,step["cwd"])
            stem=f"{index:03d}_{_slug(step['id'],55)}"
            log_file=steps_dir/f"{stem}_console.log"
            instruction_file=steps_dir/f"{stem}_instruction.txt"
            capture=build_capture_command(cwd,step["argv"],log_file)
            instruction=_instruction_text(index,len(cfg['steps']),step,cwd,log_file,capture)
            _write_text_new(instruction_file,instruction)
            print("\n"+"="*72); print(instruction.rstrip()); print("="*72,flush=True)
            row={"index":index,"id":step['id'],"title":step['title'],"description":step['description'],"cwd":step['cwd'],"argv":step['argv'],"expected_exit_codes":step['expected_exit_codes'],"instruction_file":instruction_file.relative_to(root).as_posix(),"log_file":log_file.relative_to(root).as_posix(),"status":"WAITING","exit_code":None}
            report['steps'].append(row); _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
            while True:
                choice=input_fn("Manual step [Enter=verify, r=repeat, m=manual log, q=abort]: ").strip().lower()
                if choice=="r": print("\n"+instruction.rstrip()+"\n",flush=True); continue
                if choice=="q":
                    row['status']="ABORTED"; aborted=True; any_fail=True; break
                if choice=="m":
                    if not log_file.is_file() or _unsafe_linkish(log_file):
                        print(f"MANUAL FALLBACK: save/copy console output to this file first:\n{log_file}",flush=True); continue
                    raw_rc=input_fn("Exit code of the command: ").strip()
                    try: rc=int(raw_rc)
                    except ValueError:
                        print("Invalid exit code; enter an integer.",flush=True); continue
                    _append_manual_exit(log_file,rc)
                elif choice!="":
                    print("Unknown choice. Use Enter, r, m or q.",flush=True); continue
                rc=_read_exit_code(log_file)
                if rc is None:
                    print(f"Evidence not ready: log missing or [PTV_MANUAL_EXIT_CODE=N] marker not found:\n{log_file}",flush=True); continue
                row['exit_code']=rc
                try:
                    log_size, log_sha = _evidence_metadata(log_file)
                    row['log_size_bytes']=log_size
                    row['log_sha256']=log_sha
                except ManualWorkflowError as exc:
                    print(f"Evidence rejected: {exc}",file=sys.stderr,flush=True)
                    continue
                if rc in step['expected_exit_codes']:
                    row['status']="PASS"; print(f"MANUAL STEP PASS: {step['id']} | exit={rc}",flush=True)
                else:
                    row['status']="FAIL"; any_fail=True; print(f"MANUAL STEP FAIL: {step['id']} | exit={rc} | expected={step['expected_exit_codes']}",file=sys.stderr,flush=True)
                break
            _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
            if aborted or (row['status']=="FAIL" and cfg['stop_on_failure']):break
        report['status']="ABORTED" if aborted else ("FAIL" if any_fail else "PASS")
        report['finished_at']=datetime.now(timezone.utc).isoformat()
        report['rc']=130 if aborted else (1 if any_fail else 0)
        _write_text_new(work/"MANUAL_EXECUTION_REPORT.md",_report_md(report))
        _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
        if cfg['package_result']:
            z,t=_package_result(root,work,report)
            report['result_zip']=z.relative_to(root).as_posix(); report['result_text']=t.relative_to(root).as_posix()
            _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
            print(f"MANUAL EXECUTION RESULT ZIP: {z}",flush=True)
            print(f"MANUAL EXECUTION RESULT TXT: {t}",flush=True)
        return report
    except KeyboardInterrupt:
        for row in reversed(report.get('steps', [])):
            if row.get('status') == 'WAITING':
                row['status'] = 'ABORTED'
                break
        report['status']="ABORTED"; report['rc']=130; report['finished_at']=datetime.now(timezone.utc).isoformat()
        report_md_path=work/"MANUAL_EXECUTION_REPORT.md"
        if not report_md_path.exists():
            _write_text_new(report_md_path,_report_md(report))
        _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
        if cfg['package_result']:
            z,t=_package_result(root,work,report)
            report['result_zip']=z.relative_to(root).as_posix(); report['result_text']=t.relative_to(root).as_posix()
            _write_json_atomic(work/"MANUAL_EXECUTION.json",report)
            print(f"MANUAL EXECUTION RESULT ZIP: {z}",flush=True)
            print(f"MANUAL EXECUTION RESULT TXT: {t}",flush=True)
        raise
