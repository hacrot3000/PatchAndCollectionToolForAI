# Python Patch Tool v6.17.1 native Windows runtime lane.
# Run from Windows PowerShell 5.1+ or PowerShell 7+. This script creates only a temporary test project.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
    [Console]::Error.WriteLine('SKIP: native Windows runtime lane requires Windows.')
    exit 3
}

$ToolsDir = Split-Path -LiteralPath $MyInvocation.MyCommand.Path -Parent
$SourceRoot = Split-Path -LiteralPath $ToolsDir -Parent
$Stamp = [Guid]::NewGuid().ToString('N').Substring(0, 8)
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("PTV Windows Unicode Ω $Stamp")

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw "ASSERT FAILED: $Message" }
}

try {
    New-Item -ItemType Directory -Path $TestRoot -Force | Out-Null
    Copy-Item -LiteralPath $ToolsDir -Destination (Join-Path $TestRoot 'tools') -Recurse -Force
    New-Item -ItemType Directory -Path (Join-Path $TestRoot 'patchs') -Force | Out-Null

    $Bat = Join-Path $TestRoot 'tools\run_python_patches.bat'
    $Ps1 = Join-Path $TestRoot 'tools\run_python_patches.ps1'
    Assert-True (Test-Path -LiteralPath $Bat -PathType Leaf) 'BAT launcher missing'
    Assert-True (Test-Path -LiteralPath $Ps1 -PathType Leaf) 'PowerShell launcher missing'

    # Native idle smoke through both public Windows entry points in a path with spaces + Unicode.
    & $Ps1 *> (Join-Path $TestRoot 'ps_idle.log')
    Assert-True ($LASTEXITCODE -eq 0) 'PowerShell zero-argument idle smoke failed'
    & cmd.exe /d /c ('"' + $Bat + '"') *> (Join-Path $TestRoot 'bat_idle.log')
    Assert-True ($LASTEXITCODE -eq 0) 'BAT zero-argument idle smoke failed'

    # Build a two-PATCH batch. First fails without touching source; second must run under controlled continue mode.
    $Python = Get-Command py.exe -ErrorAction SilentlyContinue | Select-Object -First 1
    $PythonExe = if ($Python) { $Python.Source } else { (Get-Command python.exe -ErrorAction Stop).Source }
    $Prefix = if ($Python) { @('-3') } else { @() }
    $Builder = @'
import json, pathlib, zipfile
root=pathlib.Path(r"__ROOT__")
def make(name, body):
    manifest={"schema_version":1,"patch":{"id":name[:-4]},"execution":{"timeout_seconds":30},"targets":["sentinel.txt"]}
    with zipfile.ZipFile(root/'patchs'/name,'w') as z:
        z.writestr('PATCH_TOOL_MANIFEST.json',json.dumps(manifest))
        z.writestr('apply.py',body)
make('patch_1_fail.zip','print("WIN-NATIVE-FAIL")\nraise SystemExit(7)\n')
make('patch_2_pass.zip','print("WIN-NATIVE-PASS")\n')
(root/'sentinel.txt').write_text('stable\n',encoding='utf-8')
(root/'.python_patch_tool.json').write_text(json.dumps({"automation":{"zero_argument":{"selection":"all","non_interactive_confirmed":True}},"batch":{"failure_policy":"continue_independent","transaction_policy":"patch"}}),encoding='utf-8')
'@.Replace('__ROOT__', $TestRoot.Replace('\','\\'))
    $BuildFile = Join-Path $TestRoot 'build_native_batch.py'
    Set-Content -LiteralPath $BuildFile -Value $Builder -Encoding UTF8
    & $PythonExe @Prefix $BuildFile
    Assert-True ($LASTEXITCODE -eq 0) 'cannot build native test PATCHes'

    & $Ps1 run --failure-policy continue_independent *> (Join-Path $TestRoot 'continue.log')
    Assert-True ($LASTEXITCODE -eq 7) 'continue-on-failure batch should return first failure rc=7'
    $Last = Get-Content -LiteralPath (Join-Path $TestRoot 'artifacts\patch_tool\LAST_RUN.json') -Raw | ConvertFrom-Json
    $Statuses = @($Last.results | ForEach-Object { $_.status })
    Assert-True ($Statuses -contains 'FAIL') 'native batch did not record FAIL'
    Assert-True ($Statuses -contains 'PASS') 'native continue policy did not run independent PATCH after failure'

    & $Ps1 report --list *> (Join-Path $TestRoot 'history.log')
    Assert-True ($LASTEXITCODE -eq 0) 'native report history command failed'

    # Run the installed Windows-specific Python contract suite on the real Windows interpreter.
    & $PythonExe @Prefix (Join-Path $TestRoot 'tools\_patch_lib\self_test_windows_runtime_v6_17_1.py')
    Assert-True ($LASTEXITCODE -eq 0) 'Windows runtime contract self-test failed'

    Write-Host "PASS: Python Patch Tool v6.17.1 native Windows lane (BAT + PowerShell + Unicode/space path + continue batch + report)"
    exit 0
}
finally {
    if (Test-Path -LiteralPath $TestRoot) {
        Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
