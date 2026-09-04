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
        [sys.executable, str(MOD), '--project-root', str(root)],
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
        [sys.executable, str(MOD), '--project-root', str(root)],
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

print('PASS: v6.8.0 local-only SHA-256 duplicate PATCH skip contract')
