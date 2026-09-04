#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, subprocess, sys, tempfile, zipfile
from pathlib import Path
HERE=Path(__file__).resolve().parent; MOD=HERE/'python_patch_queue_dispatcher.py'
spec=importlib.util.spec_from_file_location('ptv_queue_v677_failfast',MOD)
m=importlib.util.module_from_spec(spec); sys.modules[spec.name]=m; spec.loader.exec_module(m)

with tempfile.TemporaryDirectory(prefix='ptv678ff_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); log=root/'calls.txt'
    launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        + "printf '%s\\n' \"$*\" >> " + repr(str(log)) + '\n'
        + 'case "$*" in *patch_1.zip*) exit 9;; *) exit 0;; esac\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    chosen=[m.QueueItem('patch_1.zip','PATCH'),m.QueueItem('patch_2.zip','PATCH'),m.QueueItem('patch_3.zip','PATCH')]
    rc,executed,remaining,late_duplicates,duplicate_warnings=m.execute_items(root,chosen)
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
            z.writestr('identity.txt', name)
    cp=subprocess.run(
        [sys.executable,'-S',str(MOD),'--project-root',str(root)],
        input='a\n', text=True, capture_output=True,
    )
    assert cp.returncode==9,(cp.returncode,cp.stdout,cp.stderr)
    assert len(log.read_text(encoding='utf-8').splitlines())==1
    assert 'SKIPPED / NOT EXECUTED: 2 selected item(s)' in cp.stderr,cp.stderr
    assert 'patch_2.zip' in cp.stderr and 'patch_3.zip' in cp.stderr,cp.stderr


# Signal return codes from a child PATCH must use shell convention (128+signal),
# never leak negative subprocess return codes that the shell turns into 24x.
with tempfile.TemporaryDirectory(prefix='ptv6711sig_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir()
    launcher=tools/'run_python_patches.sh'
    launcher.write_text('#!/usr/bin/env bash\nkill -TERM $$\n', encoding='utf-8')
    launcher.chmod(0o755)
    rc,executed,remaining,late_duplicates,duplicate_warnings=m.execute_items(root,[m.QueueItem('patch_signal.zip','PATCH')])
    assert rc==143,(rc,executed,remaining)
    assert executed==[('patch_signal.zip',143)],executed
    assert remaining==[],remaining

# COLLECT rc=0 is not a complete PASS unless the established queue lifecycle
# moved the request ZIP from patchs/ to patchs/patched/.
with tempfile.TemporaryDirectory(prefix='ptv6711collectarchive_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); q=root/'patchs'; q.mkdir()
    request=q/'collect_request.zip'; request.write_bytes(b'request')
    launcher=tools/'run_python_patches.sh'
    launcher.write_text('#!/usr/bin/env bash\nexit 0\n', encoding='utf-8'); launcher.chmod(0o755)
    rc,executed,remaining,late_duplicates,duplicate_warnings=m.execute_items(root,[m.QueueItem(request.name,'COLLECT')])
    assert rc==3,(rc,executed,remaining)
    assert request.exists(),request

with tempfile.TemporaryDirectory(prefix='ptv6711collectarchiveok_') as td:
    root=Path(td); tools=root/'tools'; tools.mkdir(); q=root/'patchs'; q.mkdir(); (q/'patched').mkdir()
    request=q/'collect_request.zip'; request.write_bytes(b'request')
    launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        'mkdir -p patchs/patched\n'
        'mv patchs/collect_request.zip patchs/patched/collect_request.zip\n'
        'exit 0\n', encoding='utf-8')
    launcher.chmod(0o755)
    rc,executed,remaining,late_duplicates,duplicate_warnings=m.execute_items(root,[m.QueueItem(request.name,'COLLECT')])
    assert rc==0,(rc,executed,remaining)
    assert (q/'patched'/request.name).is_file()
    assert not request.exists()


# PASS summary must distinguish completed work from local duplicate skips.
# A late duplicate was selected by the operator but not executed; it must not
# be counted as a completed selected item.
with tempfile.TemporaryDirectory(prefix="ptv691_summary_dup_") as td:
    root = Path(td)
    (root / "tools").mkdir()
    (root / "patchs" / "patched").mkdir(parents=True)
    first = root / "patchs" / "first.zip"
    second = root / "patchs" / "second.zip"
    import zipfile, shutil
    with zipfile.ZipFile(first, "w") as zf:
        zf.writestr("PATCH_TOOL_MANIFEST.json", "{}")
        zf.writestr("payload.txt", "same")
    shutil.copy2(first, second)
    calls = root / "calls.txt"
    launcher = root / "tools" / "run_python_patches.sh"
    launcher.write_text(
        "#!/usr/bin/env bash\n"
        f"echo \"$*\" >> {str(calls)!r}\n"
        "src=\"${2#patchs/}\"\n"
        "mv \"patchs/$src\" \"patchs/patched/$src\"\n",
        encoding="utf-8",
    )
    launcher.chmod(0o755)
    chosen=[m.QueueItem("first.zip","PATCH"),m.QueueItem("second.zip","PATCH")]
    rc,executed,remaining,late_dups,warns=m.execute_items(root,chosen)
    assert rc==0 and len(executed)==1 and len(late_dups)==1 and not remaining and not warns

print('PASS: v6.14.1 fail-fast, signal status and COLLECT archive lifecycle')
