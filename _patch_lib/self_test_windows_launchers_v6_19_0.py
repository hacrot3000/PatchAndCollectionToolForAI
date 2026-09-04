#!/usr/bin/env python3
from __future__ import annotations
import os, shutil, subprocess, sys
from pathlib import Path

HERE=Path(__file__).resolve().parent
TOOLS=HERE.parent
ps=TOOLS/'run_python_patches.ps1'
bat=TOOLS/'run_python_patches.bat'
sh=TOOLS/'run_python_patches.sh'
for p in (ps,bat,sh):
    assert p.is_file(),p

pst=ps.read_text(encoding='utf-8')
bt=bat.read_text(encoding='utf-8')
assert 'v6.19.0' in pst and 'v6.19.0' in bt
for phrase in [
    "Join-Path $LibDir 'python_patch_queue_dispatcher.py'",
    "Join-Path $LibDir 'python_patch_runner.py'",
    "Join-Path $LibDir 'python_patch_collect_compat.py'",
    "Join-Path $LibDir 'python_patch_collect_progress_v6_7.py'",
    "Join-Path $LibDir 'python_patch_collect_regex_worker.py'",
    "sys.version_info >= (3, 10)",
    "$env:PYTHONDONTWRITEBYTECODE = '1'",
    "'--transaction', 'off'",
    "'--project-root', $ProjectRoot",
]:
    assert phrase in pst,phrase
for candidate in ['py.exe','python.exe','python3.exe']:
    assert candidate in pst,candidate
assert 'powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File' in bt
assert 'run_python_patches.ps1' in bt
assert '%*' in bt

# The Windows launcher must preserve the public zero-argument contract rather
# than inventing a separate queue or project-root convention.
assert "if ($ToolArgs.Count -eq 0)" in pst
assert "if ([string]::Equals([string]$ToolArgs[0], 'collect'" in pst
assert "if ([string]$ToolArgs[0] -in @('report', 'run', 'resume', 'plan'))" in pst
assert "Use tools\\run_python_patches.bat (or .ps1) with no arguments" in pst

# When PowerShell is available in CI, parse the script with PowerShell's own
# parser and execute a non-mutating --version smoke test. Linux release hosts
# without PowerShell still get the static contract checks above.
pwsh=shutil.which('pwsh') or shutil.which('powershell') or shutil.which('powershell.exe')
if pwsh:
    escaped=str(ps).replace("'","''")
    parser=(
        "$e=$null;$t=$null;"
        f"[System.Management.Automation.Language.Parser]::ParseFile('{escaped}',[ref]$t,[ref]$e)|Out-Null;"
        "if($e.Count -ne 0){$e|ForEach-Object{Write-Error $_};exit 9}"
    )
    cp=subprocess.run([pwsh,'-NoLogo','-NoProfile','-Command',parser],text=True,capture_output=True,timeout=30)
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    cp=subprocess.run([pwsh,'-NoLogo','-NoProfile','-ExecutionPolicy','Bypass','-File',str(ps),'--version'],cwd=TOOLS.parent,text=True,capture_output=True,timeout=30)
    assert cp.returncode==0,(cp.stdout,cp.stderr)
    assert '6.19.0' in cp.stdout+cp.stderr,(cp.stdout,cp.stderr)

print('PASS: v6.19.0 Windows BAT/PowerShell launcher contract and optional native PowerShell smoke test')
