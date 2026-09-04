#!/usr/bin/env python3
from __future__ import annotations
import hashlib, json, os, stat, subprocess, tempfile
from pathlib import Path

from python_patch_release_metadata import managed_relpaths

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
    "tools/_patch_lib/python_patch_git_safe.py",
    "tools/_patch_lib/python_patch_manual_workflow.py",
    "tools/_patch_lib/python_patch_decompile_compat.py",
    "tools/_patch_lib/install_python_patch_tool_v6.py",
    "tools/_patch_lib/install_python_patch_tool_v5.py",
    "tools/_patch_lib/docs/NO_SILENT_REMOVAL_POLICY.md",
    "tools/_patch_lib/docs/CAPABILITY_LEDGER.md",
    "tools/_patch_lib/docs/CURRENT_CAPABILITY_DISPOSITION.json",
    "tools/_patch_lib/docs/HISTORICAL_FEATURE_BASELINE_V5_15.md",
    "tools/_patch_lib/docs/HISTORICAL_FEATURE_STATUS_V5_15.json",
    "tools/_patch_lib/docs/LAYOUT_AND_MIGRATION.md",
    "tools/_patch_lib/docs/OUTPUT_FILES_GUIDE.md",
    "tools/_patch_lib/python_patch_package_schema.py",
    "tools/_patch_lib/python_patch_project_state.py",
    "tools/_patch_lib/python_patch_diagnostics_compat.py",
    "tools/_patch_lib/python_patch_database_select.py",
    "tools/_patch_lib/python_patch_cleartext_companion.py",
    "tools/_patch_lib/python_patch_ai_sync.py",
    "tools/_patch_lib/python_patch_upload_alias.py",
    "tools/_patch_lib/docs/DATABASE_SELECT_ACTIVE_BUILDER.md",
    "tools/db_profiles.example.json",
    "tools/_patch_lib/python_patch_health.py",
    "tools/_patch_lib/python_patch_version.py",
    "tools/_patch_lib/python_patch_release_metadata.py",
    "tools/_patch_lib/PACKAGE_CONTENTS.txt",
    "tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json",
    "tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json",
    "tools/_patch_lib/docs/PATCH_PACKAGE_CHECKLIST.json",
    "tools/_patch_lib/docs/AI_USAGE_CONTRACT.md",
    "tools/_patch_lib/docs/GIT_SAFE_OPERATIONS.md",
    "tools/_patch_lib/docs/MANUAL_EXECUTION_WORKFLOW.md",
    "tools/_patch_lib/docs/AI_TOOL_SYNC_CONTRACT.md",
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
    # Reuse the exact same managed-file definition as release metadata so
    # Tool Health cannot drift from packaging semantics. This intentionally
    # excludes repository metadata (.git), repo-only helpers/README, cache
    # artifacts and SHA256SUMS itself.
    try:
        actual_managed=set(managed_relpaths(root/'tools'))
    except OSError as exc:
        actual_managed=set()
        errors.append(f"managed tools tree unavailable: {type(exc).__name__}: {exc}")
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

    missing_coverage=sorted(actual_managed-seen)
    stale_coverage=sorted(seen-actual_managed)
    if missing_coverage:
        for rel in missing_coverage:
            errors.append(f"SHA256SUMS missing managed path: {rel}")
        checksum_failures+=len(missing_coverage)
    if stale_coverage:
        for rel in stale_coverage:
            errors.append(f"SHA256SUMS stale/unmanaged path: {rel}")
        checksum_failures+=len(stale_coverage)

    # PACKAGE_CONTENTS has a generated exact index, independently checked against
    # both the installed tree and SHA256SUMS so release metadata cannot silently drift.
    package_contents=lib/'PACKAGE_CONTENTS.txt'
    managed_begin='--- BEGIN GENERATED MANAGED FILE INDEX ---'
    managed_end='--- END GENERATED MANAGED FILE INDEX ---'
    package_index:set[str]=set()
    package_index_ok=True
    try:
        package_text=package_contents.read_text(encoding='utf-8')
        if managed_begin not in package_text or managed_end not in package_text:
            raise ValueError('generated managed-file index markers missing')
        block=package_text.split(managed_begin,1)[1].split(managed_end,1)[0]
        for raw in block.splitlines():
            line=raw.strip()
            if not line or line.startswith('#'):
                continue
            if line in package_index:
                errors.append(f"PACKAGE_CONTENTS duplicate managed path: {line}")
                package_index_ok=False
            package_index.add(line)
        missing_pc=sorted(actual_managed-package_index)
        stale_pc=sorted(package_index-actual_managed)
        if missing_pc or stale_pc:
            package_index_ok=False
            if missing_pc:
                errors.append(f"PACKAGE_CONTENTS missing managed paths: {missing_pc[:10]}" + (f" (+{len(missing_pc)-10} more)" if len(missing_pc)>10 else ''))
            if stale_pc:
                errors.append(f"PACKAGE_CONTENTS stale/unmanaged paths: {stale_pc[:10]}" + (f" (+{len(stale_pc)-10} more)" if len(stale_pc)>10 else ''))
        if package_index != seen:
            package_index_ok=False
            errors.append('PACKAGE_CONTENTS managed index differs from SHA256SUMS coverage')
    except Exception as exc:
        package_index_ok=False
        errors.append(f"PACKAGE_CONTENTS managed index unreadable/invalid: {type(exc).__name__}: {exc}")

    missing_required=sorted(set(REQUIRED_RUNTIME)-actual_managed)
    if missing_required:
        for rel in missing_required:
            errors.append(f"required runtime path is outside/missing from managed tree: {rel}")
        checksum_failures+=len(missing_required)

    checks.append({
        'name':'sha256sums','status':'PASS' if checksum_failures==0 else 'FAIL',
        'entries':manifest_entries,'failures':checksum_failures,
        'missing_managed':len(missing_coverage),'stale_managed':len(stale_coverage),
    })
    checks.append({
        'name':'package_contents_exact_coverage',
        'status':'PASS' if package_index_ok else 'FAIL',
        'entries':len(package_index),'actual':len(actual_managed),
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


def run_search_health(root: Path, *, compact: bool = False) -> int:
    """Exercise discovery/search against a disposable fixture; never touches project source."""
    from python_patch_collect_compat import _directory_action, _find_action, _search_action_payload
    from python_patch_collect_schema import validate_request_data
    checks=[]
    def record(name: str, ok: bool, detail: str=''):
        checks.append((name,ok,detail))
    with tempfile.TemporaryDirectory(prefix='ptv-search-health-') as td:
        fixture=Path(td).resolve()
        (fixture/'module_a').mkdir(); (fixture/'module_b'/'nested').mkdir(parents=True)
        (fixture/'module_a'/'A.java').write_text('class A { String literal = "HEALTH_LITERAL"; }\n',encoding='utf-8')
        (fixture/'module_b'/'nested'/'B.java').write_text('class B { int value123 = 7; }\n',encoding='utf-8')
        (fixture/'module_b'/'nested'/'TênUnicode.java').write_text('String unicode = "TÌM_KIẾM_ĐƯỢC";\n',encoding='utf-8')
        (fixture/'scoped'/'src'/'main'/'java'/'pkg').mkdir(parents=True)
        (fixture/'scoped'/'src'/'main'/'java'/'pkg'/'Scoped.java').write_text('class Scoped {}\n',encoding='utf-8')
        (fixture/'.gitignore').write_text('ignored_module/\n',encoding='utf-8')
        (fixture/'ignored_module').mkdir(); (fixture/'ignored_module'/'Ignored.java').write_text('String x="IGNORED_BUT_PRESENT";\n',encoding='utf-8')
        subprocess.run(['git','init','-q'],cwd=fixture,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        subprocess.run(['git','add','.gitignore','module_a/A.java','module_b/nested/B.java'],cwd=fixture,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            (fixture/'link_outside').symlink_to(Path(td).parent, target_is_directory=True)
            symlink_created=True
        except OSError:
            symlink_created=False
        # Reproduce the v6.17.14 false-zero class: target is beyond 5,000 ordinary files.
        large=fixture/'large_tree'; large.mkdir()
        for i in range(5050): (large/f'n{i:05}.txt').touch()
        (large/'zz_target.java').write_text('CmdMineInfoCSReqMsg health;\n',encoding='utf-8')

        def search(query: str, **overrides):
            action={'type':'search','query':query,'paths':['.'],'context_lines':0,'max_matches':50}
            action.update(overrides)
            req=validate_request_data({'actions':[action]})
            return _search_action_payload(fixture,req['actions'][0],req['limits'])

        out=search('HEALTH_LITERAL'); record('literal search',out['matches']==1 and out['coverage_status']=='VERIFIED',out['coverage_status'])
        out=search(r'value\d+',regex=True); record('regex search',out['matches']==1,out['coverage_status'])
        req=validate_request_data({'actions':[{'type':'find','paths':['module_b'],'patterns':['*.java'],'max_results':20}]})
        report,matches=_find_action(fixture,req['actions'][0],req['limits']); record('filename find',len(matches)>=2,str(len(matches)))
        req=validate_request_data({'actions':[{'type':'find','paths':['scoped'],'patterns':['src/main/java/pkg/*.java'],'max_results':20}]})
        report,matches=_find_action(fixture,req['actions'][0],req['limits']); record('scope-relative find glob',any(rel.endswith('/Scoped.java') for rel,_ in matches) and 'Coverage status: VERIFIED' in report,str(len(matches)))
        req=validate_request_data({'actions':[{'type':'directory','path':'module_a','include':['**/*.java'],'exclude':[],'max_results':20}]})
        d_report,d_matches=_directory_action(fixture,req['actions'][0],req['limits']); record('globstar direct-child directory',any(rel.endswith('/A.java') for rel,_ in d_matches),str(len(d_matches)))
        out=search('IGNORED_BUT_PRESENT'); record('filesystem sees gitignored/untracked',out['matches']==1,out['coverage_status'])
        out=search('IGNORED_BUT_PRESENT',source_scope='git_tracked'); record('git_tracked remains explicit/narrow',out['matches']==0,str(out['matches']))
        out=search('TÌM_KIẾM_ĐƯỢC'); record('Unicode filename/content',out['matches']==1,out['coverage_status'])
        out=search('CmdMineInfoCSReqMsg',must_find=True); record('large tree beyond old 5000 limit',out['matches']==1 and not out['incomplete'],out['coverage_status'])
        out=search('HEALTH_LITERAL',paths=[str(fixture/'module_a')]); record('absolute in-project search path',out['matches']==1,out['coverage_status'])
        out=search('HEALTH_LITERAL',paths=['module_a']); record('relative search path',out['matches']==1,out['coverage_status'])
        if symlink_created:
            out=search('NO_SUCH_HEALTH_TOKEN')
            record('symlink safety/reporting', 'symlink_follow_disabled' in out['report'] or 'symlink_escapes_project_root' in out['report'], out['coverage_status'])
        else:
            record('symlink safety/reporting',True,'fixture symlink unavailable on host; skipped')
        out=search('NO_SUCH_HEALTH_TOKEN',must_find=True,diagnose_on_zero=True)
        record('must_find + zero diagnostic',out['incomplete'] and 'ZERO MATCH DIAGNOSTIC' in out['report'] and 'must_find=true' in out['report'],out['coverage_status'])
        out=search('HEALTH_LITERAL',anchor_paths=['module_a'],expected_files=['module_a/A.java'])
        record('anchor_paths + expected_files',out['matches']>=1 and '[ANCHOR]' in out['report'] and '[EXPECTED_FILE]' in out['report'],out['coverage_status'])
    failed=[x for x in checks if not x[1]]
    if compact:
        print(f"SEARCH HEALTH: {'PASS' if not failed else 'FAIL'} | version={VERSION} | pass={len(checks)-len(failed)} fail={len(failed)}")
    else:
        print('=== SEARCH HEALTH / DISCOVERY SELF-TEST ===')
        print(f"Version     : {VERSION}")
        for name,ok,detail in checks:
            suffix=f" | {detail}" if detail else ''
            print(f"  [{'PASS' if ok else 'FAIL'}] {name}{suffix}")
        print('Principle   : Zero matches is a search result, not proof of absence.')
        print('=== END SEARCH HEALTH ===')
    return 0 if not failed else 2
