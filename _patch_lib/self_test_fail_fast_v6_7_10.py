#!/usr/bin/env python3
from __future__ import annotations
import contextlib, importlib.util, io, subprocess, sys, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent; MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v677_failfast',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)

with tempfile.TemporaryDirectory(prefix='ptv678ff_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); (root/'patchs').mkdir(); log=root/'calls.txt'
    launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        + "printf '%s\\n' \"$*\" >> " + repr(str(log)) + '\n'
        + 'case "$*" in *patch_1.zip*) exit 9;; *) exit 0;; esac\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    for name in ['patch_1.zip','patch_2.zip','patch_3.zip']:
        with zipfile.ZipFile(root/'patchs'/name,'w') as z:
            z.writestr('PATCH_TOOL_MANIFEST.json','{}')
    chosen=[m.QueueItem('patch_1.zip','PATCH'),m.QueueItem('patch_2.zip','PATCH'),m.QueueItem('patch_3.zip','PATCH')]
    rc,executed,remaining=m.execute_items(root,chosen)
    assert rc==9,rc
    assert executed==[('patch_1.zip',9)],executed
    assert [x.name for x in remaining]==['patch_2.zip','patch_3.zip'],remaining
    assert len(log.read_text(encoding='utf-8').splitlines())==1

# End-to-end dispatcher regression: line `a` selects all and fail-fast leaves
# later selected packages untouched/unexecuted.
with tempfile.TemporaryDirectory(prefix='ptv678ffmain_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); (root/'patchs').mkdir(); log=root/'calls.txt'
    launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        + "printf '%s\\n' \"$*\" >> " + repr(str(log)) + '\n'
        + 'case "$*" in *patch_1.zip*) exit 9;; *) exit 0;; esac\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    for name in ['patch_1.zip','patch_2.zip','patch_3.zip']:
        with zipfile.ZipFile(root/'patchs'/name,'w') as z:
            z.writestr('PATCH_TOOL_MANIFEST.json','{}')
    cp=subprocess.run(
        [sys.executable,str(MOD),'--project-root',str(root)],
        input='a\n', text=True, capture_output=True,
    )
    assert cp.returncode==9,(cp.returncode,cp.stdout,cp.stderr)
    assert len(log.read_text(encoding='utf-8').splitlines())==1
    assert 'SKIPPED / NOT EXECUTED: 2 selected item(s)' in cp.stderr,cp.stderr
    assert 'patch_2.zip' in cp.stderr and 'patch_3.zip' in cp.stderr,cp.stderr



# A queue entry selected as PATCH must be revalidated immediately before
# execution. If another process replaces it with a symlink/non-patch artifact,
# fail closed without invoking the launcher.
with tempfile.TemporaryDirectory(prefix='ptv6710_revalidate_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); patchs=root/'patchs'; patchs.mkdir(); log=root/'calls.txt'
    launcher=tools/'run_python_patches.sh'
    launcher.write_text('#!/usr/bin/env bash\necho called >> '+repr(str(log))+'\nexit 0\n', encoding='utf-8')
    launcher.chmod(0o755)
    target=patchs/'patch_race.zip'
    with zipfile.ZipFile(target,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json','{}')
    selected=m.QueueItem(target.name,'PATCH')
    target.unlink()
    outside=root/'outside.zip'
    with zipfile.ZipFile(outside,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json','{}')
    target.symlink_to(outside)
    errbuf=io.StringIO()
    with contextlib.redirect_stderr(errbuf):
        rc,executed,remaining=m.execute_items(root,[selected])
    assert rc==2,(rc,executed,remaining)
    assert 'queue item changed after selection' in errbuf.getvalue(),errbuf.getvalue()
    assert executed==[(selected.name,2)],executed
    assert not log.exists(), 'launcher must not run a replaced/symlinked queue entry'

print('PASS: v6.7.10 selected queue stops on first failure')
