# Python Patch Tool compatibility launcher.
# PowerShell 5.1+ compatible. Canonical routing lives in python_patch_entry.py.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$ToolArgs = @($args)
$ToolsDir = [System.IO.Path]::GetDirectoryName($MyInvocation.MyCommand.Path)
$ProjectRoot = [System.IO.Path]::GetDirectoryName($ToolsDir)
$Entry = Join-Path $ToolsDir 'python_patch_entry.py'

function Write-ToolError([string]$Message) {
    [Console]::Error.WriteLine($Message)
}

function Test-PythonCandidate([string]$Exe, [string[]]$Prefix) {
    try {
        $probe = @()
        if ($Prefix) { $probe += $Prefix }
        $probe += @('-c', 'import sys; raise SystemExit(0 if sys.version_info >= (3, 10) else 3)')
        & $Exe @probe 1>$null 2>$null
        return ($LASTEXITCODE -eq 0)
    }
    catch {
        return $false
    }
}

function Resolve-Python3 {
    $candidates = @(
        @{ Name = 'py.exe'; Prefix = @('-3') },
        @{ Name = 'python.exe'; Prefix = @() },
        @{ Name = 'python3.exe'; Prefix = @() },
        @{ Name = 'py'; Prefix = @('-3') },
        @{ Name = 'python'; Prefix = @() },
        @{ Name = 'python3'; Prefix = @() }
    )
    foreach ($candidate in $candidates) {
        $cmd = Get-Command $candidate.Name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -eq $cmd) { continue }
        $exe = $cmd.Source
        if ([string]::IsNullOrWhiteSpace($exe)) { $exe = $cmd.Path }
        if ([string]::IsNullOrWhiteSpace($exe)) { continue }
        if (Test-PythonCandidate $exe $candidate.Prefix) {
            return @{ Exe = $exe; Prefix = [string[]]$candidate.Prefix }
        }
    }
    return $null
}

$Python = Resolve-Python3
if ($null -eq $Python) {
    Write-ToolError 'ERROR: Python 3.10+ was not found. Install Python for Windows, enable the Python Launcher (py.exe) or add python.exe to PATH, then retry.'
    exit 2
}
if (-not (Test-Path -LiteralPath $Entry -PathType Leaf)) {
    Write-ToolError "ERROR: Missing Patch Tool entrypoint: $Entry"
    exit 2
}

$invokeArgs = @()
if ($Python.Prefix) { $invokeArgs += $Python.Prefix }
$invokeArgs += @($Entry, '--project-root', $ProjectRoot, '--')
$invokeArgs += $ToolArgs
& $Python.Exe @invokeArgs
if ($null -eq $LASTEXITCODE) { exit 1 }
exit [int]$LASTEXITCODE
