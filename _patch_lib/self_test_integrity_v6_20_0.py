#!/usr/bin/env python3
from __future__ import annotations

import hashlib
import tempfile
from pathlib import Path

import python_patch_batch as batch
import python_patch_queue_dispatcher as queue
import python_patch_runner as runner

assert queue.VERSION == '6.20.1'
assert batch.VERSION == '6.20.1'
assert runner.VERSION == '6.20.1'

# Selected batch packages are mandatory transaction inputs: disappearance is
# never silently omitted from the replay snapshot.
with tempfile.TemporaryDirectory(prefix='ptv6174_missing_pkg_') as td:
    root = Path(td); (root / 'patchs').mkdir()
    try:
        batch.snapshot_package_bytes(root, ['missing.zip'], root / 'snap')
        raise AssertionError('missing selected package was silently omitted')
    except batch.BatchPlanError as exc:
        assert exc.kind == 'batch_transaction_snapshot_race', exc.kind

# Package metadata is bound to planned bytes. A replacement before snapshot or
# before child spawn must fail before payload execution.
with tempfile.TemporaryDirectory(prefix='ptv6174_identity_') as td:
    root = Path(td); (root / 'patchs').mkdir()
    package = root / 'patchs' / 'patch_x.zip'; package.write_bytes(b'NEW-PACKAGE')
    old_sha = hashlib.sha256(b'OLD-PACKAGE').hexdigest()
    try:
        batch.snapshot_package_bytes(
            root, ['patch_x.zip'], root / 'snap', expected_sha256={'patch_x.zip': old_sha}
        )
        raise AssertionError('batch snapshot accepted bytes different from planned SHA')
    except batch.BatchPlanError as exc:
        assert exc.kind == 'package_input_changed', exc.kind
    rc, text, result = queue._run_patch_child(
        root, ['this-command-must-never-spawn'], queue.QueueItem('patch_x.zip', 'PATCH'),
        expected_patch_sha256=old_sha, expected_targets=['src/a.c'],
    )
    assert rc == 2 and result and result['diagnosis']['kind'] == 'package_input_changed', (rc, text, result)
    assert result['partial_modification']['detected'] is False
    assert result['preflight']['target_paths'] == ['src/a.c']

# Replay snapshots carry immutable SHA/size metadata; corruption after the
# transaction snapshot is a REQUEUE failure, never requeued as trusted bytes.
with tempfile.TemporaryDirectory(prefix='ptv6174_replay_corrupt_') as td:
    root = Path(td); (root / 'patchs').mkdir(); snap = root / 'snap'; snap.mkdir()
    stored = snap / '0000_patch_x.zip'; stored.write_bytes(b'ORIGINAL')
    meta = {
        'patch_x.zip': {
            'stored': stored.name,
            'sha256': hashlib.sha256(b'ORIGINAL').hexdigest(),
            'size': len(b'ORIGINAL'),
        }
    }
    stored.write_bytes(b'CORRUPTED')
    try:
        batch.requeue_packages(root, snap, meta)
        raise AssertionError('corrupted replay snapshot was accepted')
    except batch.BatchPlanError as exc:
        assert exc.kind == 'batch_requeue_failed', exc.kind
    assert not (root / 'patchs' / 'patch_x.zip').exists()

# Mutation lock leaf symlinks/reparse paths must fail closed and must never
# truncate/write through to the linked target.
for module, key_fn, acquire_fn in (
    (queue, queue._project_mutation_lock_key, queue._acquire_batch_mutation_lock),
    (runner, runner._mutation_lock_key, runner._acquire_project_mutation_lock),
):
    with tempfile.TemporaryDirectory(prefix='ptv6174_lock_') as td:
        fake = Path(td); project = fake / 'project'; project.mkdir()
        victim = fake / 'victim.txt'; victim.write_text('DO-NOT-TOUCH', encoding='utf-8')
        old_gettempdir = module.tempfile.gettempdir
        module.tempfile.gettempdir = lambda: str(fake / 'tmp')
        try:
            lock_dir = Path(module.tempfile.gettempdir()) / 'python_patch_tool_locks' / key_fn(project)
            lock_dir.mkdir(parents=True)
            (lock_dir / 'mutation.lock').symlink_to(victim)
            try:
                acquire_fn(project)
                raise AssertionError('mutation lock accepted symlink')
            except Exception:
                pass
            assert victim.read_text(encoding='utf-8') == 'DO-NOT-TOUCH'
        finally:
            module.tempfile.gettempdir = old_gettempdir

# A broken/symlinked fail_handoffs subdirectory cannot redirect output and does
# not crash failure reporting: bundle falls back to the hardened artifact root.
with tempfile.TemporaryDirectory(prefix='ptv6174_handoff_dir_') as td:
    root = Path(td); (root / 'patchs').mkdir(); (root / 'tools' / '_patch_lib').mkdir(parents=True)
    artifact_root = queue._artifact_run_root(root)
    outside = root.parent / f'{root.name}_outside'; outside.mkdir(exist_ok=True)
    (artifact_root / 'fail_handoffs').symlink_to(outside, target_is_directory=True)
    handoff = queue._create_fail_handoff(
        root, queue.QueueItem('missing.zip', 'PATCH'), 2, 'ERR\n',
        {'status': 'FAIL', 'patch_sha256': None}, None,
    )
    assert handoff is not None and handoff.is_file(), handoff
    assert handoff.parent == artifact_root, handoff
    assert not list(outside.glob('FAIL_HANDOFF_*.zip'))
    (artifact_root / 'fail_handoffs').unlink()
    outside.rmdir()

# Per-source snapshot failures are isolated. Failure on item B must not unlink
# the already frozen snapshot for item A.
with tempfile.TemporaryDirectory(prefix='ptv6174_handoff_snapshot_') as td:
    root = Path(td); (root / 'a.c').write_text('A', encoding='utf-8'); (root / 'b.c').write_text('B', encoding='utf-8')
    snap = root / 'snap'; snap.mkdir()
    original_safe = queue._safe_handoff_source
    def hooked(project: Path, rel: str):
        out = original_safe(project, rel)
        if rel == 'b.c' and out is not None:
            out.unlink()
        return out
    queue._safe_handoff_source = hooked
    try:
        frozen, skipped = queue._snapshot_handoff_sources(
            root, [('a.c', root / 'a.c'), ('b.c', root / 'b.c')], snap
        )
    finally:
        queue._safe_handoff_source = original_safe
    assert [(rel, path.read_text(encoding='utf-8')) for rel, path in frozen] == [('a.c', 'A')], frozen
    assert any(row.get('path') == 'b.c' for row in skipped), skipped

print('PASS: v6.20.1 package identity, replay snapshot, mutation lock and FAIL_HANDOFF artifact integrity')
