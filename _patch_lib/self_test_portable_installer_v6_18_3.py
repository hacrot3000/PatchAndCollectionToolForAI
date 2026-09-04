#!/usr/bin/env python3
from __future__ import annotations
import importlib.util, json, subprocess, sys, tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
MOD = HERE / 'install_python_patch_tool_v6.py'
spec = importlib.util.spec_from_file_location('ptv_installer_v6183', MOD)
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
assert m.VERSION == '6.18.3'


def make_root(base: Path) -> Path:
    root = base / 'project'; lib = root / 'tools' / '_patch_lib'
    lib.mkdir(parents=True)
    launcher = root / 'tools' / 'run_python_patches.sh'
    launcher.write_text('#!/bin/sh\nexit 0\n', encoding='utf-8')
    return root

with tempfile.TemporaryDirectory(prefix='ptv6183_installer_') as td:
    root = make_root(Path(td))
    stale = root / 'tools' / 'python_patch_runner.py'
    stale.write_text('legacy-runner', encoding='utf-8')
    custom = root / 'tools' / 'custom_project_tool.sh'
    custom.write_text('do-not-delete', encoding='utf-8')
    cfg = root / '.python_patch_tool.json'
    original_cfg = {'project': {'key': 'keep-me'}, 'custom': {'x': 1}}
    cfg.write_text(json.dumps(original_cfg), encoding='utf-8')

    preview = m.run(m._safe_root(str(root)), dry_run=True, create_config=True)
    assert preview['legacy_files'] == ['tools/python_patch_runner.py'], preview
    assert stale.is_file() and custom.is_file()
    assert json.loads(cfg.read_text()) == original_cfg

    result = m.run(m._safe_root(str(root)), dry_run=False, create_config=True)
    assert not stale.exists(), result
    assert custom.read_text(encoding='utf-8') == 'do-not-delete'
    assert json.loads(cfg.read_text()) == original_cfg
    backup = root / str(result['backup_dir']) / 'tools' / 'python_patch_runner.py'
    assert backup.read_text(encoding='utf-8') == 'legacy-runner', (result, backup)
    assert result['config_created'] is False and result['config_preserved'] is True

with tempfile.TemporaryDirectory(prefix='ptv6183_installer_cfg_') as td:
    root = make_root(Path(td))
    result = m.run(m._safe_root(str(root)), dry_run=False, create_config=True)
    cfg = json.loads((root/'.python_patch_tool.json').read_text(encoding='utf-8'))
    assert result['config_created'] is True
    assert cfg['automation']['zero_argument']['selection'] == 'prompt'
    assert cfg['automation']['zero_argument']['non_interactive_confirmed'] is False
    assert cfg['batch']['failure_policy'] == 'continue_independent'

# Historical filename wrapper must remain executable and delegate to current helper.
with tempfile.TemporaryDirectory(prefix='ptv6183_installer_wrapper_') as td:
    root = make_root(Path(td))
    # Wrapper imports its sibling, so execute from the real package path while
    # targeting the temporary project.
    proc = subprocess.run(
        [sys.executable, str(HERE/'install_python_patch_tool_v5.py'), '--project-root', str(root), '--dry-run'],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=10,
    )
    assert proc.returncode == 0, (proc.returncode, proc.stdout, proc.stderr)
    assert 'RESULT       : PASS' in proc.stdout, proc.stdout

# Symlink safety: a historical managed path must never be followed/deleted.
with tempfile.TemporaryDirectory(prefix='ptv6183_installer_symlink_') as td:
    root = make_root(Path(td)); external = Path(td)/'external.txt'; external.write_text('keep')
    stale = root/'tools'/'python_patch_runner.py'; stale.symlink_to(external)
    try:
        m.run(m._safe_root(str(root)), dry_run=False, create_config=False)
    except m.InstallerError:
        pass
    else:
        raise AssertionError('installer must reject symlinked managed file')
    assert external.read_text() == 'keep'

print('PASS: v6.18.3 optional controlled installer preserves config/unrelated tools and safely migrates fixed legacy files')
