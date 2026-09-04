#!/usr/bin/env python3
from pathlib import Path
import runpy,sys
root=Path(__file__).resolve().parent
for name in [
    'self_test_version_consistency_v6_7_9.py',
    'self_test_inplace_routing_v6_7_9.py',
    'self_test_queue_discovery_v6_7_9.py',
    'self_test_selection_contract_v6_7_9.py',
    'self_test_fail_fast_v6_7_9.py',
    'self_test_collect_progress_v6_7_9.py',
    'self_test_ai_collect_contract_v6_7_9.py',
    'self_test_package_checksums_v6_7_9.py',
]:
    print(f'RUNNING: {name}',flush=True)
    runpy.run_path(str(root/name),run_name=f'__ptv_test_{name}__')
print('PASS: Python Patch Tool v6.7.9 regression suite')
