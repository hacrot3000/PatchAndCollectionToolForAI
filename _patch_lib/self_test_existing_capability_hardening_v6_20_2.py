#!/usr/bin/env python3
from __future__ import annotations

import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import python_patch_git_safe as gs
import python_patch_manual_workflow as mw

assert gs.VERSION == '6.20.2', gs.VERSION
assert mw.VERSION == '6.20.2', mw.VERSION


def git(repo: Path, *args: str) -> str:
    cp = subprocess.run(['git', *args], cwd=repo, text=True, capture_output=True, check=True)
    return cp.stdout.strip()


# Git switch must remain local/clean AND fail closed before Git can execute a
# configured clean/smudge/process filter for paths affected by checkout.
with tempfile.TemporaryDirectory(prefix='ptv6202_git_filter_') as td:
    repo = Path(td)
    git(repo, 'init', '-q')
    git(repo, 'config', 'user.name', 'PTV Test')
    git(repo, 'config', 'user.email', 'ptv@example.invalid')
    (repo/'tracked.txt').write_text('main\n', encoding='utf-8')
    git(repo, 'add', 'tracked.txt'); git(repo, 'commit', '-qm', 'main')
    main_branch = git(repo, 'branch', '--show-current')
    git(repo, 'switch', '-qc', 'feature')
    (repo/'.gitattributes').write_text('tracked.txt filter=ptv_external\n', encoding='utf-8')
    (repo/'tracked.txt').write_text('feature\n', encoding='utf-8')
    git(repo, 'add', '.gitattributes', 'tracked.txt'); git(repo, 'commit', '-qm', 'feature')
    git(repo, 'switch', '-q', main_branch)
    marker = repo/'FILTER_SHOULD_NOT_RUN'
    # This command is harmless evidence: an unsafe switch implementation would
    # create the marker when Git invokes the smudge filter.
    git(repo, 'config', 'filter.ptv_external.smudge', f"sh -c 'touch {marker}; cat'")
    git(repo, 'config', 'filter.ptv_external.clean', 'cat')
    before = git(repo, 'branch', '--show-current')
    try:
        gs.execute_git_operation(repo, {'op':'switch', 'branch':'feature'})
        raise AssertionError('switch with external filter unexpectedly allowed')
    except gs.GitSafeError as exc:
        assert 'filter' in str(exc).lower(), exc
    assert not marker.exists(), marker
    assert git(repo, 'branch', '--show-current') == before

    # Git failures preserve earlier evidence but make the COLLECT action incomplete.
    result = gs.run_git_operations_result(repo, {
        'type':'git', 'operations':[{'op':'status'}, {'op':'show', 'ref':'BAD_REF_DOES_NOT_EXIST'}]
    })
    assert result['incomplete'] is True, result
    assert '## 1. status' in result['report'], result['report']
    assert '[GIT OP REJECTED]' in result['report'], result['report']

    # Capture is bounded at subprocess time instead of after a huge PIPE is in RAM.
    (repo/'big.txt').write_text('X' * 20000, encoding='utf-8')
    git(repo, 'add', 'big.txt'); git(repo, 'commit', '-qm', 'big')
    cp = gs._run(repo, ['show', 'HEAD:big.txt'], capture_limit=64)
    assert cp.stdout_truncated and len(cp.stdout.encode('utf-8')) <= 64, (len(cp.stdout), cp)

# Report output is terminal/Markdown safe even when repository content is hostile.
sanitized = gs._sanitize_report_text('\x1b[31mRED\x1b[0m\x00OK')
assert '\x1b' not in sanitized and '\x00' not in sanitized and 'RED' in sanitized, repr(sanitized)
assert len(gs._markdown_fence('payload ``` inside')) >= 4

# Existing manual workflow remains general structured argv, while command-text
# evaluator escape forms are rejected even through common wrappers/attached args.
blocked_argv = [
    ['python3', '-cprint(123)'],
    ['node', '-econsole.log(1)'],
    ['bash', '-lc', 'echo x'],
    ['env', 'bash', '-c', 'echo x'],
    ['/usr/bin/env', 'python3', '-c', 'print(1)'],
    ['powershell.exe', '-Command:Get-ChildItem'],
    ['cmd.exe', '/c', 'echo x'],
    ['busybox', 'sh', '-c', 'echo x'],
    ['env', '-S', 'bash -c echo x'],
]
for argv in blocked_argv:
    cfg = {'steps':[{'id':'s','argv':argv}]}
    try:
        mw.validate_manual_execution(cfg)
        raise AssertionError(f'inline evaluator bypass accepted: {argv!r}')
    except mw.ManualWorkflowError:
        pass
normal = mw.validate_manual_execution({'steps':[{'id':'build','argv':['mvn','-DskipTests','package']}]})
assert normal['steps'][0]['argv'] == ['mvn','-DskipTests','package']

with tempfile.TemporaryDirectory(prefix='ptv6202_manual_log_') as td:
    root = Path(td)
    log = root/'large.log'
    log.write_bytes(b'A' * (2 * 1024 * 1024) + b'\n[PTV_MANUAL_EXIT_CODE=7]\n')
    assert mw._read_exit_code(log) == 7
    size, digest = mw._evidence_metadata(log) or (0, '')
    assert size == log.stat().st_size and len(digest) == 64
    link = root/'link.log'
    try:
        link.symlink_to(log)
    except OSError:
        pass
    else:
        try:
            mw._append_manual_exit(link, 0)
            raise AssertionError('symlink manual log unexpectedly accepted')
        except mw.ManualWorkflowError:
            pass

# Ctrl+C is equivalent to q for finalization: status is ABORTED and available
# instructions/evidence are packaged, while no manual argv is executed.
with tempfile.TemporaryDirectory(prefix='ptv6202_manual_interrupt_') as td:
    root = Path(td)
    manifest = {
        'manual_execution': {
            'package_result': True,
            'steps': [{'id':'never-run','argv':['touch','SHOULD_NOT_BE_CREATED']}],
        }
    }
    def interrupt(_prompt: str) -> str:
        raise KeyboardInterrupt
    out = io.StringIO()
    try:
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
            mw.run_manual_workflow(root, manifest, 'interrupt-test.zip', input_fn=interrupt)
        raise AssertionError('KeyboardInterrupt unexpectedly swallowed')
    except KeyboardInterrupt:
        pass
    assert not (root/'SHOULD_NOT_BE_CREATED').exists()
    reports = list((root/'artifacts'/'ptv_manual').glob('M_*/MANUAL_EXECUTION.json'))
    assert len(reports) == 1, reports
    report = json.loads(reports[0].read_text(encoding='utf-8'))
    assert report['status'] == 'ABORTED' and report['rc'] == 130, report
    assert report['steps'][0]['status'] == 'ABORTED', report
    assert report.get('result_zip') and (root/report['result_zip']).is_file(), report
    assert report.get('result_text') and (root/report['result_text']).is_file(), report

print('PASS: v6.20.2 existing Git/manual/COLLECT hardening remains fail-closed, bounded and human-only')
