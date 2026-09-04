#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import io
import os
import shutil
import subprocess
import stat
import sys
import tempfile
import zipfile
from contextlib import redirect_stdout
from pathlib import Path

HERE = Path(__file__).resolve().parent
MOD = HERE / 'python_patch_queue_dispatcher.py'
spec = importlib.util.spec_from_file_location('ptv_queue_v680_duplicate', MOD)
m = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = m
assert spec.loader is not None
spec.loader.exec_module(m)




def install_runner_shim(root: Path):
    lib=root/'tools'/'_patch_lib'; lib.mkdir(parents=True,exist_ok=True)
    shim=lib/'python_patch_runner.py'
    shim.write_text(
        '#!/usr/bin/env python3\nimport subprocess,sys\nfrom pathlib import Path\nroot=Path(__file__).resolve().parents[2]\nlauncher=root/"tools"/"run_python_patches.sh"\nraise SystemExit(subprocess.run([str(launcher),*sys.argv[1:]],cwd=root).returncode)\n',
        encoding='utf-8')

def make_patch(path: Path, marker: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(path, 'w') as zf:
        zf.writestr('PATCH_TOOL_MANIFEST.json', '{"format":5}')
        zf.writestr('payload.txt', marker)


# Exact bytes under patchs/patched are the only authoritative history source.
# Renaming does not bypass the fingerprint; same name with different bytes does.
with tempfile.TemporaryDirectory(prefix='ptv680dup_split_') as td:
    root = Path(td)
    queued = root / 'patchs'
    history = queued / 'patched'
    history.mkdir(parents=True)

    duplicate = queued / 'renamed_duplicate.zip'
    make_patch(duplicate, 'same payload')
    shutil.copy2(duplicate, history / 'historical_name.zip')

    same_name_new = queued / 'same_name.zip'
    make_patch(same_name_new, 'new payload')
    make_patch(history / 'same_name.zip', 'old payload')

    items, warnings = m.discover_queue(root)
    assert not [w for w in warnings if 'renamed_duplicate.zip' in w], warnings
    runnable, duplicates, duplicate_warnings = m._split_local_duplicate_patches(root, items)
    assert duplicate_warnings == [], duplicate_warnings
    assert [d.item.name for d in duplicates] == ['renamed_duplicate.zip'], duplicates
    assert duplicates[0].history_name == 'historical_name.zip', duplicates[0]
    assert len(duplicates[0].sha256) == 64
    runnable_names = {x.name for x in runnable}
    assert 'same_name.zip' in runnable_names, runnable_names
    assert 'renamed_duplicate.zip' not in runnable_names, runnable_names
    # Skip-only means the user's duplicate queue file is not moved/deleted.
    assert duplicate.is_file(), duplicate

    buf = io.StringIO()
    with redirect_stdout(buf):
        m._print_local_duplicate_skips(duplicates)
    out = buf.getvalue()
    assert out.count('renamed_duplicate.zip') == 1, out
    assert '[SKIPPED:DUPLICATE_LOCAL]' in out, out
    assert 'patchs/patched/historical_name.zip' in out, out
    assert 'SHA-256:' not in out, out


# End-to-end zero-argument dispatcher: a duplicate-only queue does not invoke
# the PATCH launcher and exits successfully with a local duplicate skip record.
with tempfile.TemporaryDirectory(prefix='ptv680dup_e2e_only_') as td:
    root = Path(td)
    tools = root / 'tools'
    queue = root / 'patchs'
    history = queue / 'patched'
    tools.mkdir(parents=True)
    history.mkdir(parents=True)
    calls = root / 'calls.txt'

    duplicate = queue / 'patch_again.zip'
    make_patch(duplicate, 'already passed here')
    shutil.copy2(duplicate, history / 'patch_previous_name.zip')

    launcher = tools / 'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        f"echo called >> {str(calls)!r}\n"
        'exit 99\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    install_runner_shim(root)

    cp = subprocess.run(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        text=True,
        capture_output=True,
        timeout=15,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    assert not calls.exists(), calls.read_text() if calls.exists() else ''
    assert 'no new runnable package; local duplicate PATCHes were skipped' in cp.stdout, cp.stdout
    assert '[SKIPPED:DUPLICATE_LOCAL] patch_again.zip' in cp.stdout, cp.stdout
    assert cp.stdout.count('patch_again.zip') == 1, cp.stdout
    assert duplicate.is_file(), duplicate


# Mixed queue: the duplicate is excluded before selection; the sole new PATCH
# is selected by default and is the only child invocation.
with tempfile.TemporaryDirectory(prefix='ptv680dup_e2e_mixed_') as td:
    root = Path(td)
    tools = root / 'tools'
    queue = root / 'patchs'
    history = queue / 'patched'
    tools.mkdir(parents=True)
    history.mkdir(parents=True)
    calls = root / 'calls.txt'

    duplicate = queue / 'duplicate.zip'
    make_patch(duplicate, 'same')
    shutil.copy2(duplicate, history / 'older.zip')
    make_patch(queue / 'new_patch.zip', 'new')

    launcher = tools / 'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        f"printf '%s\\n' \"$*\" >> {str(calls)!r}\n"
        'exit 0\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    install_runner_shim(root)

    cp = subprocess.run(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        input='\n',
        text=True,
        capture_output=True,
        timeout=15,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    invoked = calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked) == 1, invoked
    assert 'new_patch.zip' in invoked[0], invoked
    assert 'duplicate.zip' not in invoked[0], invoked
    assert '[SKIPPED:DUPLICATE_LOCAL] duplicate.zip' in cp.stdout, cp.stdout
    assert duplicate.is_file(), duplicate


# Local-only invariant: the same bytes in another project/machine-like root are
# runnable when that root has no matching patchs/patched history.
with tempfile.TemporaryDirectory(prefix='ptv680dup_localonly_a_') as a, tempfile.TemporaryDirectory(prefix='ptv680dup_localonly_b_') as b:
    root_a = Path(a)
    root_b = Path(b)
    (root_a / 'patchs' / 'patched').mkdir(parents=True)
    (root_b / 'patchs').mkdir(parents=True)
    historical = root_a / 'patchs' / 'patched' / 'shared_patch.zip'
    make_patch(historical, 'portable patch')
    queued_b = root_b / 'patchs' / 'shared_patch.zip'
    shutil.copy2(historical, queued_b)
    items_b, _ = m.discover_queue(root_b)
    runnable_b, duplicates_b, warnings_b = m._split_local_duplicate_patches(root_b, items_b)
    assert warnings_b == [], warnings_b
    assert duplicates_b == [], duplicates_b
    assert [x.name for x in runnable_b] == ['shared_patch.zip'], runnable_b


# A symlinked local-history directory must never suppress a patch.  This keeps
# the duplicate decision physically project-local even when users have shared
# folders or copied legacy symlink layouts.
with tempfile.TemporaryDirectory(prefix='ptv681dup_symlink_project_') as project_td, tempfile.TemporaryDirectory(prefix='ptv681dup_symlink_shared_') as shared_td:
    root = Path(project_td)
    shared = Path(shared_td)
    queue = root / 'patchs'
    queue.mkdir(parents=True)
    shared.mkdir(parents=True, exist_ok=True)
    historical = shared / 'old.zip'
    make_patch(historical, 'shared-history')
    queued = queue / 'same.zip'
    shutil.copy2(historical, queued)
    (queue / 'patched').symlink_to(shared, target_is_directory=True)
    items, _ = m.discover_queue(root)
    runnable, duplicates, warnings = m._split_local_duplicate_patches(root, items)
    assert [x.name for x in runnable] == ['same.zip'], runnable
    assert duplicates == [], duplicates
    assert any('patchs/patched/ is a symlink' in x for x in warnings), warnings

# v6.15.0 hardens the whole queue boundary: patchs/ itself must never be a
# symlink, because otherwise PATCH/COLLECT execution and archive lifecycle can
# cross into another project/shared directory. Discovery fails closed.
with tempfile.TemporaryDirectory(prefix='ptv681dup_queue_project_') as project_td, tempfile.TemporaryDirectory(prefix='ptv681dup_queue_shared_') as shared_td:
    root = Path(project_td)
    shared_queue = Path(shared_td)
    (shared_queue / 'patched').mkdir(parents=True)
    historical = shared_queue / 'patched' / 'old.zip'
    make_patch(historical, 'shared-root')
    queued = shared_queue / 'same.zip'
    shutil.copy2(historical, queued)
    (root / 'patchs').symlink_to(shared_queue, target_is_directory=True)
    try:
        m.discover_queue(root)
    except m.QueueSafetyError as exc:
        assert 'patchs/' in str(exc) and 'symlink' in str(exc), exc
    else:
        raise AssertionError('symlinked patchs/ must fail closed')

# Late duplicate recheck: if two byte-identical patches were selected before
# history existed, the first successful PATCH archives itself and the second is
# skipped without a second launcher execution.
with tempfile.TemporaryDirectory(prefix='ptv681dup_late_') as td:
    root = Path(td)
    tools = root / 'tools'
    queue = root / 'patchs'
    history = queue / 'patched'
    tools.mkdir(parents=True)
    history.mkdir(parents=True)
    first = queue / 'first.zip'
    second = queue / 'second.zip'
    make_patch(first, 'same-late-payload')
    shutil.copy2(first, second)
    calls = root / 'calls.txt'
    launcher = tools / 'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        'set -e\n'
        f'echo "$*" >> {str(calls)!r}\n'
        'src="${2#patchs/}"\n'
        'mv "patchs/$src" "patchs/patched/$src"\n'
        'exit 0\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    install_runner_shim(root)
    chosen = [m.QueueItem(first.name, 'PATCH'), m.QueueItem(second.name, 'PATCH')]
    rc, executed, remaining, late_duplicates, warnings = m.execute_items(root, chosen)
    assert rc == 0, rc
    assert [x[0] for x in executed] == ['first.zip'], executed
    assert remaining == [], remaining
    assert [x.item.name for x in late_duplicates] == ['second.zip'], late_duplicates
    assert warnings == [], warnings
    invoked = calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked) == 1 and 'first.zip' in invoked[0], invoked
    assert second.is_file(), second


# End-to-end current-session duplicate collapse: byte-identical queue files
# are collapsed before the selector. The natural-order first copy is canonical
# and the redundant file is removed from patchs/ immediately.
with tempfile.TemporaryDirectory(prefix='ptv681dup_late_main_') as td:
    root = Path(td)
    tools = root / 'tools'
    queue = root / 'patchs'
    history = queue / 'patched'
    tools.mkdir(parents=True)
    history.mkdir(parents=True)
    first = queue / 'copy_1.zip'
    second = queue / 'copy_2.zip'
    make_patch(first, 'late-main')
    shutil.copy2(first, second)
    calls = root / 'calls.txt'
    launcher = tools / 'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        'set -e\n'
        f'echo "$*" >> {str(calls)!r}\n'
        'src="${2#patchs/}"\n'
        'mv "patchs/$src" "patchs/patched/$src"\n'
        'exit 0\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    install_runner_shim(root)
    cp = subprocess.run(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        input='a\n', text=True, capture_output=True, timeout=15,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    invoked = calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked) == 1 and 'copy_1.zip' in invoked[0], invoked
    assert '[REMOVED:DUPLICATE_SESSION] copy_2.zip' in cp.stdout, cp.stdout
    assert 'Same content as: patchs/copy_1.zip' in cp.stdout, cp.stdout
    assert '1 duplicate file(s) collapsed in-session' in cp.stdout, cp.stdout
    assert not second.exists(), second



# Explicit 3-file acceptance from the user requirement: 3 queued PATCHes, two
# are byte-identical under different names. Exactly one duplicate is removed
# before selection while the unique PATCH remains runnable.
with tempfile.TemporaryDirectory(prefix='ptv612_session_three_') as td:
    root=Path(td); q=root/'patchs'; (q/'patched').mkdir(parents=True)
    a=q/'patch_1.zip'; b=q/'renamed_copy.zip'; c=q/'patch_3.zip'
    make_patch(a,'same-session-bytes'); shutil.copy2(a,b); make_patch(c,'unique-session-bytes')
    items=[m.QueueItem(a.name,'PATCH'),m.QueueItem(b.name,'PATCH'),m.QueueItem(c.name,'PATCH')]
    runnable,dups,warns=m._split_session_duplicate_patches(root,items)
    assert [x.name for x in runnable]==['patch_1.zip','patch_3.zip'],runnable
    assert len(dups)==1 and dups[0].item.name=='renamed_copy.zip' and dups[0].canonical_name=='patch_1.zip',dups
    assert dups[0].removed and not b.exists(),b
    assert a.is_file() and c.is_file() and not warns,warns


# v6.15.0 deliberately has no project/process queue lock. Independent terminal
# windows are operator-controlled and must not be rejected as BUSY. A stale
# .ptv_queue.lock from an older release is ignored and is never created by this
# dispatcher.
assert not hasattr(m, '_acquire_project_queue_lock')
assert not hasattr(m, '_release_project_queue_lock')
with tempfile.TemporaryDirectory(prefix='ptv610_no_process_lock_') as td:
    root = Path(td)
    tools = root/'tools'; queue=root/'patchs'; history=queue/'patched'
    tools.mkdir(parents=True); history.mkdir(parents=True)
    patch=queue/'concurrent.zip'; make_patch(patch,'concurrent-payload')
    calls=root/'calls.txt'; launcher=tools/'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        f'printf "%s\\n" "$*" >> {str(calls)!r}\n'
        'sleep 0.35\n'
        'exit 0\n', encoding='utf-8')
    launcher.chmod(0o755)
    install_runner_shim(root)
    p1=subprocess.Popen([sys.executable,'-S',str(MOD),'--project-root',str(root)],
                        stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    p2=subprocess.Popen([sys.executable,'-S',str(MOD),'--project-root',str(root)],
                        stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    out1,err1=p1.communicate(input='\n',timeout=5)
    out2,err2=p2.communicate(input='\n',timeout=5)
    assert p1.returncode==0,(p1.returncode,out1,err1)
    assert p2.returncode==0,(p2.returncode,out2,err2)
    assert 'BUSY:' not in err1+err2,(err1,err2)
    assert not (queue/'.ptv_queue.lock').exists()
    invoked=calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked)==2,invoked

print('PASS: v6.15.0 current-session duplicate collapse plus local-history duplicate contract')
