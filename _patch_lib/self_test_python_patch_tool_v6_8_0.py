#!/usr/bin/env python3
from pathlib import Path
import os, signal, subprocess, sys

root=Path(__file__).resolve().parent
names=[
    'self_test_version_consistency_v6_8_0.py',
    'self_test_inplace_routing_v6_8_0.py',
    'self_test_queue_discovery_v6_8_0.py',
    'self_test_local_duplicate_v6_8_0.py',
    'self_test_selection_contract_v6_8_0.py',
    'self_test_fail_fast_v6_8_0.py',
    'self_test_collect_progress_v6_8_0.py',
    'self_test_ai_collect_contract_v6_8_0.py',
    'self_test_package_checksums_v6_8_0.py',
]
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
for name in names:
    print(f'RUNNING: {name}',flush=True)
    kwargs = dict(cwd=root, env=env)
    if os.name == 'posix':
        kwargs['start_new_session'] = True
    proc=subprocess.Popen([sys.executable,str(root/name)], **kwargs)
    try:
        rc=proc.wait(timeout=60)
    except subprocess.TimeoutExpired:
        print(f'FAIL: self-test timeout: {name}', file=sys.stderr, flush=True)
        try:
            if os.name == 'posix': os.killpg(proc.pid, signal.SIGKILL)
            else: proc.kill()
        except (ProcessLookupError, PermissionError, OSError):
            pass
        proc.wait(timeout=5)
        raise SystemExit(124)
    if rc:
        raise SystemExit(rc)
print('PASS: Python Patch Tool v6.8.0 regression suite')
