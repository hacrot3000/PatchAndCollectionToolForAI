#!/usr/bin/env python3
from pathlib import Path
HERE=Path(__file__).resolve().parent
D=(HERE/'python_patch_queue_dispatcher.py').read_text(encoding='utf-8')
R=(HERE/'python_patch_runner.py').read_text(encoding='utf-8')
S=(HERE/'python_patch_package_schema.py').read_text(encoding='utf-8')
PS=(HERE.parent/'run_python_patches.ps1').read_text(encoding='utf-8')
BAT=(HERE.parent/'run_python_patches.bat').read_text(encoding='utf-8')
assert 'def _runner_command' in D and 'sys.executable' in D
assert '_runner_command(root, "execute", item)' in D
assert 'python_patch_collect_progress_v6_7.py' in D and '[sys.executable, str(progress)' in D
assert 'def _read_key_windows' in D and 'msvcrt.getwch()' in D
assert 'def _enable_windows_vt' in D and 'ENABLE_VIRTUAL_TERMINAL_PROCESSING' in D
assert 'use_windows_tty' in D and 'v: validate' in D
assert 'def _batch_report_menu' in D and 'def _report_command' in D
assert 'CREATE_NEW_PROCESS_GROUP' in R
assert 'taskkill' in R and '"/T"' in R and '"/F"' in R
assert 'path_is_link_or_reparse' in S and 'FILE_ATTRIBUTE_REPARSE_POINT' in S
assert 'Python 3.10+' in PS and 'ExecutionPolicy Bypass' in BAT
assert "'report'" in PS
print('PASS: v6.17.11 Windows native internal routing, fullscreen input, process-tree containment and reparse safety contracts')
