#!/usr/bin/env python3
from pathlib import Path
import os, subprocess, sys
root=Path(__file__).resolve().parent
tests=[
    'self_test_version_consistency_v6_7_13.py',
    'self_test_inplace_routing_v6_7_13.py',
    'self_test_queue_discovery_v6_7_13.py',
    'self_test_selection_contract_v6_7_13.py',
    'self_test_fail_fast_v6_7_13.py',
    'self_test_collect_progress_v6_7_13.py',
    'self_test_ai_collect_contract_v6_7_13.py',
    'self_test_package_checksums_v6_7_13.py',
]
env=dict(os.environ)
env['PYTHONDONTWRITEBYTECODE']='1'
for name in tests:
    print(f'RUNNING: {name}',flush=True)
    try:
        cp=subprocess.run([sys.executable,str(root/name)],env=env,timeout=45)
    except subprocess.TimeoutExpired:
        print(f'FAIL: {name} timed out',file=sys.stderr)
        raise SystemExit(124)
    if cp.returncode:
        print(f'FAIL: {name} rc={cp.returncode}',file=sys.stderr)
        raise SystemExit(cp.returncode)
print('PASS: Python Patch Tool v6.7.13 regression suite')
