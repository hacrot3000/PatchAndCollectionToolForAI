from pathlib import Path

p = Path('_patch_lib/python_patch_collect_compat.py')
text = p.read_text(encoding='utf-8')

old_sig = 'def _match_files(root: Path, files: list[Path], action: dict, limits: dict, *, deadline: float | None = None) -> dict:'
new_sig = 'def _match_files(root: Path, files: list[Path], action: dict, limits: dict, *, deadline: float | None = None, progress_cb=None) -> dict:'
if old_sig not in text:
    raise SystemExit('match_files signature marker missing')
text = text.replace(old_sig, new_sig, 1)

old_init = "\n".join([
    '    max_bytes=int(limits.get("max_search_file_bytes",64*1024*1024))',
    '    matches=[]; total=0; searched=0; skipped=[]; ext=Counter(); truncated=False; skip_reasons=Counter()',
    '    time_limit_reached=False; processed_files=0; stop=False',
    '    for path in files:',
    '',
])
new_init = "\n".join([
    '    max_bytes=int(limits.get("max_search_file_bytes",64*1024*1024))',
    '    # Regex work is intentionally ordered by bounded input size first. This',
    '    # maximizes safe evidence before a later pathological expression/file can',
    '    # consume the worker hard-timeout, without weakening the timeout itself.',
    '    if regex_mode and len(files) > 1:',
    '        def _regex_priority(path: Path):',
    '            try: size=path.stat().st_size',
    '            except OSError: size=max_bytes + 1',
    '            return (size, _search_rel(root,path).lower())',
    '        files=sorted(files,key=_regex_priority)',
    '    matches=[]; total=0; searched=0; skipped=[]; ext=Counter(); truncated=False; skip_reasons=Counter()',
    '    time_limit_reached=False; processed_files=0; stop=False',
    '    def snapshot():',
    '        return {',
    '            "matches":list(matches),',
    '            "match_count":total,',
    '            "truncated":truncated,',
    '            "files_searched":searched,',
    '            "content_skips":list(skipped[:250]),',
    '            "content_skip_count":len(skipped),',
    '            "content_skip_reason_counts":dict(skip_reasons),',
    '            "searched_extension_counts":dict(ext.most_common()),',
    '            "time_limit_reached":time_limit_reached,',
    '            "files_input":len(files),',
    '            "files_processed":processed_files,',
    '            "files_remaining":max(0,len(files)-processed_files),',
    '        }',
    '    for path in files:',
    '',
])
if old_init not in text:
    raise SystemExit('match_files init marker missing')
text = text.replace(old_init, new_init, 1)

old_tail = "\n".join([
    '        if stop: break',
    '    return {',
    '        "matches":matches,',
    '        "match_count":total,',
    '        "truncated":truncated,',
    '        "files_searched":searched,',
    '        "content_skips":skipped[:250],',
    '        "content_skip_count":len(skipped),',
    '        "content_skip_reason_counts":dict(skip_reasons),',
    '        "searched_extension_counts":dict(ext.most_common()),',
    '        "time_limit_reached":time_limit_reached,',
    '        "files_input":len(files),',
    '        "files_processed":processed_files,',
    '        "files_remaining":max(0,len(files)-processed_files),',
    '    }',
    '',
])
new_tail = "\n".join([
    '        # Publish only after a file has completed safely. If the next file',
    '        # hangs in regex evaluation, the parent can recover this atomic snapshot.',
    '        if progress_cb is not None:',
    '            progress_cb(snapshot())',
    '        if stop: break',
    '    return snapshot()',
    '',
])
if old_tail not in text:
    raise SystemExit('match_files return marker missing')
text = text.replace(old_tail, new_tail, 1)

marker = "    primary_error=None; primary_cov=None\n\n    if backend in {'auto','rg'}:\n"
helper = "\n".join([
    '    primary_error=None; primary_cov=None',
    '',
    '    def publish_primary_progress(snapshot: dict, backend_name: str, coverage: dict, backend_diag=None) -> None:',
    '        if checkpoint_cb is None:',
    '            return',
    '        snap=dict(snapshot); snap["backend"]=backend_name',
    '        if backend_diag is not None: snap["backend_diag"]=backend_diag',
    '        checkpoint_cb(_search_result_payload(',
    '            root,action,scopes,snap,None,coverage,',
    '            primary_error=primary_error,fallback_enabled=fallback_enabled,',
    "            fallback_note='pending; primary file checkpoint preserved',",
    "            extra_reasons=['search action checkpoint saved after a safely completed primary file'],",
    '            force_incomplete=True,',
    '        ))',
    '',
    "    if backend in {'auto','rg'}:",
    '',
])
if marker not in text:
    raise SystemExit('search payload helper marker missing')
text = text.replace(marker, helper, 1)

repls = [
    (
        'primary=_match_files(root,rg_files,action,limits,deadline=deadline); primary.update({"backend":"rg","backend_diag":rg_diag})',
        'primary=_match_files(root,rg_files,action,limits,deadline=deadline,progress_cb=lambda snap: publish_primary_progress(snap,"rg",_empty_search_coverage(),rg_diag)); primary.update({"backend":"rg","backend_diag":rg_diag})',
    ),
    (
        "primary=_match_files(root,pfiles,action,limits,deadline=deadline); primary['backend']='python-stack'; primary['backend_diag']=rg_diag",
        "primary=_match_files(root,pfiles,action,limits,deadline=deadline,progress_cb=lambda snap: publish_primary_progress(snap,'python-stack',primary_cov,rg_diag)); primary['backend']='python-stack'; primary['backend_diag']=rg_diag",
    ),
    (
        "primary=_match_files(root,pfiles,action,limits,deadline=deadline); primary['backend']='python-stack'",
        "primary=_match_files(root,pfiles,action,limits,deadline=deadline,progress_cb=lambda snap: publish_primary_progress(snap,'python-stack',primary_cov)); primary['backend']='python-stack'",
    ),
]
for old, new in repls:
    if old not in text:
        raise SystemExit(f'match call marker missing: {old}')
    text = text.replace(old, new, 1)

p.write_text(text, encoding='utf-8')
