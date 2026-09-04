#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, stat
from pathlib import Path

VERSION = "6.17.4"
REQUIRED_RUNTIME = [
    "tools/run_python_patches.sh",
    "tools/run_python_patches.ps1",
    "tools/run_python_patches.bat",
    "tools/run_windows_native_tests.ps1",
    "tools/_patch_lib/VERSION",
    "tools/_patch_lib/python_patch_queue_dispatcher.py",
    "tools/_patch_lib/python_patch_batch.py",
    "tools/_patch_lib/python_patch_runner.py",
    "tools/_patch_lib/python_patch_ops_worker.py",
    "tools/_patch_lib/python_patch_utils.py",
    "tools/_patch_lib/python_patch_readonly_collector.py",
    "tools/_patch_lib/python_patch_collect_compat.py",
    "tools/_patch_lib/python_patch_collect_regex_worker.py",
    "tools/_patch_lib/python_patch_collect_schema.py",
    "tools/_patch_lib/python_patch_package_schema.py",
    "tools/_patch_lib/python_patch_health.py",
    "tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json",
    "tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json",
    "tools/_patch_lib/docs/PATCH_PACKAGE_CHECKLIST.json",
    "tools/_patch_lib/docs/AI_USAGE_CONTRACT.md",
    "tools/implementing.md",
    "tools/PYTHON_PATCH_TOOL_FEATURES_VI.md",
]


def _sha256(path: Path) -> str:
    h=hashlib.sha256()
    with path.open('rb') as fh:
        for chunk in iter(lambda: fh.read(1024*1024), b''):
            h.update(chunk)
    return h.hexdigest()


def _safe_regular(root: Path, rel: str) -> bool:
    """Require every installed path component to be real, not a symlink."""
    current=root.resolve(strict=True)
    parts=Path(rel).parts
    for i,part in enumerate(parts):
        current=current/part
        try:
            st=current.lstat()
        except OSError:
            return False
        attrs=int(getattr(st,'st_file_attributes',0) or 0)
        reparse=int(getattr(stat,'FILE_ATTRIBUTE_REPARSE_POINT',0x400))
        if stat.S_ISLNK(st.st_mode) or (os.name=='nt' and attrs & reparse):
            return False
        if i < len(parts)-1 and not stat.S_ISDIR(st.st_mode):
            return False
        if i == len(parts)-1 and not stat.S_ISREG(st.st_mode):
            return False
    return True


def audit_tool(root: Path) -> dict[str, object]:
    root=root.resolve()
    lib=root/'tools'/'_patch_lib'
    errors: list[str]=[]
    warnings: list[str]=[]
    checks: list[dict[str, object]]=[]

    version_path=lib/'VERSION'
    try:
        installed=version_path.read_text(encoding='utf-8').strip()
    except Exception as exc:
        installed=''
        errors.append(f"VERSION unreadable: {type(exc).__name__}")
    if installed != VERSION:
        errors.append(f"VERSION mismatch: file={installed or '<missing>'} runtime={VERSION}")
    checks.append({'name':'version','status':'PASS' if installed==VERSION else 'FAIL','detail':installed})

    for rel in REQUIRED_RUNTIME:
        path=root/rel
        ok=_safe_regular(root,rel)
        checks.append({'name':f'file:{rel}','status':'PASS' if ok else 'FAIL'})
        if not ok:
            errors.append(f"missing/unsafe required file or symlinked ancestor: {rel}")

    sh_launcher=root/'tools'/'run_python_patches.sh'
    ps_launcher=root/'tools'/'run_python_patches.ps1'
    bat_launcher=root/'tools'/'run_python_patches.bat'
    if os.name=='nt':
        executable=ps_launcher.is_file() and bat_launcher.is_file()
        launcher_detail='windows:.bat+.ps1'
    else:
        executable=sh_launcher.is_file() and os.access(sh_launcher,os.X_OK)
        launcher_detail='posix:.sh+0755'
    checks.append({'name':'launcher_executable','status':'PASS' if executable else 'FAIL','detail':launcher_detail})
    if not executable:
        errors.append('public launcher is missing or not executable for this platform')

    manifest=lib/'SHA256SUMS'
    manifest_entries=0
    checksum_failures=0
    seen:set[str]=set()
    try:
        lines=manifest.read_text(encoding='utf-8').splitlines()
        for lineno,line in enumerate(lines,1):
            if not line.strip():
                continue
            if '  ' not in line:
                errors.append(f"SHA256SUMS malformed line {lineno}")
                checksum_failures+=1
                continue
            digest,rel=line.split('  ',1)
            rel=rel.strip()
            if rel in seen:
                errors.append(f"SHA256SUMS duplicate path: {rel}")
                checksum_failures+=1
                continue
            seen.add(rel); manifest_entries+=1
            path=root/rel
            if not _safe_regular(root,rel):
                errors.append(f"checksum target missing/unsafe or symlinked ancestor: {rel}")
                checksum_failures+=1
                continue
            actual=_sha256(path)
            if actual.lower()!=digest.lower():
                errors.append(f"checksum mismatch: {rel}")
                checksum_failures+=1
    except Exception as exc:
        errors.append(f"SHA256SUMS unreadable: {type(exc).__name__}: {exc}")
        checksum_failures+=1
    missing_coverage=sorted(set(REQUIRED_RUNTIME)-seen)
    if missing_coverage:
        for rel in missing_coverage:
            errors.append(f"SHA256SUMS missing required managed path: {rel}")
        checksum_failures+=len(missing_coverage)
    checks.append({
        'name':'sha256sums','status':'PASS' if checksum_failures==0 else 'FAIL',
        'entries':manifest_entries,'failures':checksum_failures,'missing_required':len(missing_coverage),
    })

    for name in ('COLLECT_ACTION_SCHEMA.json','PATCH_PACKAGE_SCHEMA.json'):
        path=lib/'docs'/name
        try:
            data=json.loads(path.read_text(encoding='utf-8'))
            ok=isinstance(data,dict) and data.get('tool_version')==VERSION
            if not ok:
                errors.append(f"{name} tool_version/schema mismatch")
        except Exception as exc:
            ok=False; errors.append(f"{name} invalid JSON: {type(exc).__name__}: {exc}")
        checks.append({'name':f'schema:{name}','status':'PASS' if ok else 'FAIL'})

    # Extra cache files do not break runtime, but release/install hygiene should surface them.
    caches=[p.relative_to(root).as_posix() for p in (root/'tools').rglob('*') if p.is_file() and p.suffix=='.pyc']
    pycache_dirs=[p.relative_to(root).as_posix() for p in (root/'tools').rglob('__pycache__') if p.is_dir()]
    if caches or pycache_dirs:
        warnings.append(f"Python cache artifacts present: files={len(caches)} dirs={len(pycache_dirs)}")
    checks.append({'name':'python_cache_hygiene','status':'WARN' if caches or pycache_dirs else 'PASS','files':len(caches),'dirs':len(pycache_dirs)})

    status='FAIL' if errors else ('WARN' if warnings else 'PASS')
    return {'format':'python-patch-tool-health','format_version':1,'tool_version':VERSION,'status':status,'checks':checks,'errors':errors,'warnings':warnings}


def print_health(root: Path, *, compact: bool=False) -> int:
    report=audit_tool(root)
    status=report['status']
    checks=report['checks']
    passed=sum(1 for c in checks if c.get('status')=='PASS')
    failed=sum(1 for c in checks if c.get('status')=='FAIL')
    warned=sum(1 for c in checks if c.get('status')=='WARN')
    if compact:
        print(f"TOOL HEALTH: {status} | version={VERSION} | pass={passed} warn={warned} fail={failed}")
    else:
        print("=== TOOL HEALTH / SELF-AUDIT ===")
        print(f"Status      : {status}")
        print(f"Version     : {VERSION}")
        print(f"Checks      : PASS={passed} WARN={warned} FAIL={failed}")
        for c in checks:
            name=str(c.get('name')); st=str(c.get('status'))
            extra=''
            if name=='sha256sums': extra=f" entries={c.get('entries',0)} failures={c.get('failures',0)}"
            print(f"  [{st}] {name}{extra}")
        for msg in report.get('warnings') or []:
            print(f"  WARNING: {msg}")
        for msg in report.get('errors') or []:
            print(f"  ERROR: {msg}")
        print("=== END TOOL HEALTH ===")
    return 0 if status in {'PASS','WARN'} else 2
