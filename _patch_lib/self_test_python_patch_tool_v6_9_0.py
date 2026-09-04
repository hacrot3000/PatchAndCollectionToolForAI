#!/usr/bin/env python3
from pathlib import Path
import os, shutil, subprocess, sys

root=Path(__file__).resolve().parent
names=[
    'self_test_version_consistency_v6_9_0.py',
    'self_test_inplace_routing_v6_9_0.py',
    'self_test_queue_discovery_v6_9_0.py',
    'self_test_local_duplicate_v6_9_0.py',
    'self_test_selection_contract_v6_9_0.py',
    'self_test_fail_fast_v6_9_0.py',
    'self_test_collect_progress_v6_9_0.py',
    'self_test_ai_collect_contract_v6_9_0.py',
    'self_test_package_checksums_v6_9_0.py',
]
env=dict(os.environ); env['PYTHONDONTWRITEBYTECODE']='1'
timeout_bin=shutil.which('timeout')
for name in names:
    print(f'RUNNING: {name}',flush=True)
    test=[sys.executable,str(root/name)]
    # The release is Bash/Linux-oriented. When GNU/coreutils timeout is present,
    # put it between this master process and each nested regression test. Some
    # tests intentionally exercise signals and subprocess trees; the wrapper
    # makes their lifecycle independent instead of letting a stale descendant
    # keep the Python master waiting after the test logic is complete.
    if timeout_bin:
        cmd=[timeout_bin,'--kill-after=5s','60s',*test]
        rc=subprocess.run(cmd,cwd=root,env=env).returncode
        if rc==124:
            print(f'FAIL: self-test timeout: {name}',file=sys.stderr,flush=True)
            raise SystemExit(124)
    else:
        try:
            rc=subprocess.run(test,cwd=root,env=env,timeout=60).returncode
        except subprocess.TimeoutExpired:
            print(f'FAIL: self-test timeout: {name}',file=sys.stderr,flush=True)
            raise SystemExit(124)
    if rc:
        raise SystemExit(rc)
print('PASS: Python Patch Tool v6.9.0 regression suite')
