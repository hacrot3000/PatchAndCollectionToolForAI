#!/usr/bin/env python3
from __future__ import annotations

import importlib.util
import io
import os
import shutil
import subprocess
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

# Same invariant when patchs/ itself is a symlink: discovery may remain
# compatible with an old layout, but duplicate history must not cross roots.
with tempfile.TemporaryDirectory(prefix='ptv681dup_queue_project_') as project_td, tempfile.TemporaryDirectory(prefix='ptv681dup_queue_shared_') as shared_td:
    root = Path(project_td)
    shared_queue = Path(shared_td)
    (shared_queue / 'patched').mkdir(parents=True)
    historical = shared_queue / 'patched' / 'old.zip'
    make_patch(historical, 'shared-root')
    queued = shared_queue / 'same.zip'
    shutil.copy2(historical, queued)
    (root / 'patchs').symlink_to(shared_queue, target_is_directory=True)
    items, _ = m.discover_queue(root)
    runnable, duplicates, warnings = m._split_local_duplicate_patches(root, items)
    assert [x.name for x in runnable] == ['same.zip'], runnable
    assert duplicates == [], duplicates
    assert any('patchs/ is a symlink' in x for x in warnings), warnings

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


# End-to-end late duplicate reporting: both copies are initially selectable,
# but after the first archives itself the second is reported as a local skip.
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
    cp = subprocess.run(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        input='a\n', text=True, capture_output=True, timeout=15,
    )
    assert cp.returncode == 0, (cp.returncode, cp.stdout, cp.stderr)
    invoked = calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked) == 1 and 'copy_1.zip' in invoked[0], invoked
    assert '[SKIPPED:DUPLICATE_LOCAL] copy_2.zip' in cp.stdout, cp.stdout
    assert 'SUMMARY: PASS | 1 item(s) completed | 1 item(s) skipped as local duplicate' in cp.stdout, cp.stdout
    assert second.is_file(), second



# Concurrent zero-argument sessions for the same project must never execute the
# same PATCH twice. The project-local queue lock is acquired before discovery,
# so a second session fails temporarily while the first one owns the queue.
with tempfile.TemporaryDirectory(prefix='ptv692dup_concurrent_') as td:
    root = Path(td)
    tools = root / 'tools'
    queue = root / 'patchs'
    history = queue / 'patched'
    tools.mkdir(parents=True)
    history.mkdir(parents=True)
    patch = queue / 'concurrent.zip'
    make_patch(patch, 'concurrent-payload')
    calls = root / 'calls.txt'
    started = root / 'started.txt'
    launcher = tools / 'run_python_patches.sh'
    launcher.write_text(
        '#!/usr/bin/env bash\n'
        f'echo "$*" >> {str(calls)!r}\n'
        f'echo started > {str(started)!r}\n'
        'sleep 1\n'
        'src="${2#patchs/}"\n'
        'if [ -f "patchs/$src" ]; then mv "patchs/$src" "patchs/patched/$src"; fi\n'
        'exit 0\n',
        encoding='utf-8',
    )
    launcher.chmod(0o755)
    p1 = subprocess.Popen(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    assert p1.stdin is not None
    p1.stdin.write('\n'); p1.stdin.flush()
    import time
    deadline=time.monotonic()+3
    while not started.exists() and time.monotonic() < deadline:
        time.sleep(0.03)
    assert started.exists(), 'first queue session never launched patch'
    p2 = subprocess.run(
        [sys.executable, '-S', str(MOD), '--project-root', str(root)],
        input='\n', text=True, capture_output=True, timeout=5,
    )
    out1, err1 = p1.communicate(timeout=5)
    assert p1.returncode == 0, (p1.returncode, out1, err1)
    assert p2.returncode == getattr(os, 'EX_TEMPFAIL', 75), (p2.returncode,p2.stdout,p2.stderr)
    assert 'BUSY:' in p2.stderr and 'nothing executed' in p2.stderr, p2.stderr
    invoked = calls.read_text(encoding='utf-8').splitlines()
    assert len(invoked) == 1 and 'concurrent.zip' in invoked[0], invoked

print('PASS: v6.9.2 local-only SHA-256 duplicate PATCH skip contract')
