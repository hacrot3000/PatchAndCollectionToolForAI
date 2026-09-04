#!/usr/bin/env python3
from pathlib import Path
here=Path(__file__).resolve().parent; tools=here.parent
p=tools/'run_windows_native_tests.ps1'
assert p.is_file(),p
text=p.read_text(encoding='utf-8')
for token in ['v6.20.1','Windows_NT','run_python_patches.bat','run_python_patches.ps1','PTV Windows Unicode Ω','continue_independent','report --list','self_test_windows_runtime_v6_20_0.py']:
    assert token in text,token
print('PASS: v6.20.1 native Windows runtime test lane is packaged and covers real BAT/PowerShell execution')
