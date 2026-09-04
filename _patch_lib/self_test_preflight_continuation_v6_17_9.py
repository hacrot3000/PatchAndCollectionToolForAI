#!/usr/bin/env python3
from __future__ import annotations
import json, os, shutil, subprocess, tempfile, zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
assert (HERE/'VERSION').read_text(encoding='utf-8').strip() == '6.17.9'


def install(root: Path):
    shutil.copytree(HERE.parent, root/'tools')
    (root/'tools'/'run_python_patches.sh').chmod(0o755)
    (root/'patchs').mkdir()
    (root/'.python_patch_tool.json').write_text(json.dumps({
        'automation': {'zero_argument': {'selection':'all','non_interactive_confirmed': True}},
        'batch': {'failure_policy':'continue_independent','transaction_policy':'patch'},
    }), encoding='utf-8')


def make_patch(root: Path, name: str, pid: str, body: str, targets: list[str], *, project_key: str|None=None):
    manifest = {
        'schema_version': 1,
        'patch': {'id': pid},
        'execution': {'timeout_seconds': 30},
        'targets': targets,
    }
    if project_key is not None:
        manifest['project'] = {'key': project_key}
    with zipfile.ZipFile(root/'patchs'/name, 'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json', json.dumps(manifest))
        z.writestr('apply.py', body)


def run(root: Path):
    return subprocess.run(
        [str(root/'tools'/'run_python_patches.sh')], cwd=root, text=True,
        capture_output=True, env={**os.environ, 'PYTHONDONTWRITEBYTECODE':'1'}, timeout=120,
    )

# Regression from a real queue: a later project_identity_unconfigured PATCH must
# not suppress two independent valid PATCHes under continue_independent + patch.
with tempfile.TemporaryDirectory(prefix='ptv6179_preflight_continue_') as td:
    root = Path(td); install(root)
    make_patch(root,'patch_1.zip','p1','from pathlib import Path\nPath("one.txt").write_text("1")\n',['one.txt'])
    make_patch(root,'patch_2.zip','p2','from pathlib import Path\nPath("two.txt").write_text("2")\n',['two.txt'])
    make_patch(root,'patch_3.zip','p3','from pathlib import Path\nPath("three.txt").write_text("3")\n',['three.txt'],project_key='not-configured-here')
    cp = run(root); text = cp.stdout + cp.stderr
    assert cp.returncode == 2, (cp.returncode, text)
    report = json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    rows = [(r.get('name'),r.get('status')) for r in report.get('results',[])]
    assert rows == [('patch_1.zip','PASS'),('patch_2.zip','PASS'),('patch_3.zip','PREFLIGHT_FAIL')], rows
    assert (root/'one.txt').read_text() == '1' and (root/'two.txt').read_text() == '2'
    assert not (root/'three.txt').exists()
    assert report.get('failed_item') == 'patch_3.zip', report
    assert 'BATCH PREFLIGHT: PARTIAL' in text and 'NOT EXECUTED: 2' not in text, text

# A preflight-failed PATCH is still a failed relationship anchor: same-target
# successor is BLOCKED, while an unrelated target continues.
with tempfile.TemporaryDirectory(prefix='ptv6179_preflight_relation_') as td:
    root = Path(td); install(root); (root/'shared.txt').write_text('base')
    make_patch(root,'patch_1.zip','bad','print("no")\n',['shared.txt'],project_key='not-configured-here')
    make_patch(root,'patch_2.zip','related','from pathlib import Path\nPath("related_ran").write_text("bad")\n',['shared.txt'])
    make_patch(root,'patch_3.zip','other','from pathlib import Path\nPath("ok.txt").write_text("ok")\n',['ok.txt'])
    cp = run(root)
    report = json.loads((root/'artifacts/patch_tool/LAST_RUN.json').read_text())
    rows = [(r.get('name'),r.get('status'),(r.get('diagnosis') or {}).get('kind')) for r in report.get('results',[])]
    assert rows == [
        ('patch_1.zip','PREFLIGHT_FAIL','project_identity_unconfigured'),
        ('patch_2.zip','BLOCKED','related_target_failed'),
        ('patch_3.zip','PASS',None),
    ], rows
    assert not (root/'related_ran').exists() and (root/'ok.txt').read_text() == 'ok'

print('PASS: v6.17.9 item-local batch preflight failures continue independent PATCHes and block only related successors')
