# Python Patch Tool v6.17.13 public Windows launcher.
# PowerShell 5.1+ compatible. SANDBOX/worktree transaction mode is permanently disabled.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$ToolArgs = @($args)
$ToolsDir = Split-Path -LiteralPath $MyInvocation.MyCommand.Path -Parent
$ProjectRoot = Split-Path -LiteralPath $ToolsDir -Parent
$LibDir = Join-Path $ToolsDir '_patch_lib'
$Runner = Join-Path $LibDir 'python_patch_runner.py'
$Collector = Join-Path $LibDir 'python_patch_readonly_collector.py'
$CollectCompat = Join-Path $LibDir 'python_patch_collect_compat.py'
$Dispatcher = Join-Path $LibDir 'python_patch_queue_dispatcher.py'
$CollectProgress = Join-Path $LibDir 'python_patch_collect_progress_v6_7.py'
$CollectRegexWorker = Join-Path $LibDir 'python_patch_collect_regex_worker.py'

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

# Keep the installed tool tree immutable during normal execution so Tool Health
# does not warn about bytecode caches created by the tool itself.
$env:PYTHONDONTWRITEBYTECODE = '1'
$pathSep = [IO.Path]::PathSeparator
if ([string]::IsNullOrEmpty($env:PYTHONPATH)) {
    $env:PYTHONPATH = $LibDir
} else {
    $env:PYTHONPATH = $LibDir + $pathSep + $env:PYTHONPATH
}

function Invoke-PatchPython([string[]]$Arguments) {
    $invokeArgs = @()
    if ($Python.Prefix) { $invokeArgs += $Python.Prefix }
    $invokeArgs += $Arguments
    & $Python.Exe @invokeArgs
    if ($null -eq $LASTEXITCODE) { return 1 }
    return [int]$LASTEXITCODE
}

if ($ToolArgs.Count -eq 0) {
    if (-not (Test-Path -LiteralPath $Dispatcher -PathType Leaf)) {
        Write-ToolError "ERROR: Missing queue dispatcher: $Dispatcher"
        exit 2
    }
    exit (Invoke-PatchPython @($Dispatcher, '--project-root', $ProjectRoot))
}

if ([string]$ToolArgs[0] -in @('report', 'run', 'resume', 'plan')) {
    $command = [string]$ToolArgs[0]
    $rest = @()
    if ($ToolArgs.Count -gt 1) { $rest = @($ToolArgs[1..($ToolArgs.Count - 1)]) }
    exit (Invoke-PatchPython (@($Dispatcher, '--project-root', $ProjectRoot, $command) + $rest))
}

if ([string]::Equals([string]$ToolArgs[0], 'collect', [StringComparison]::OrdinalIgnoreCase)) {
    if (-not (Test-Path -LiteralPath $CollectCompat -PathType Leaf)) {
        Write-ToolError "ERROR: Missing COLLECT compatibility layer: $CollectCompat"
        exit 2
    }
    if (-not (Test-Path -LiteralPath $CollectProgress -PathType Leaf)) {
        Write-ToolError "ERROR: Missing collect progress supervisor: $CollectProgress"
        exit 2
    }
    if (-not (Test-Path -LiteralPath $CollectRegexWorker -PathType Leaf)) {
        Write-ToolError "ERROR: Missing COLLECT regex worker: $CollectRegexWorker"
        exit 2
    }
    $rest = @()
    if ($ToolArgs.Count -gt 1) { $rest = @($ToolArgs[1..($ToolArgs.Count - 1)]) }
    $collectArgs = @($CollectProgress, '--project-root', $ProjectRoot, '--collector', $CollectCompat, '--') + $rest
    exit (Invoke-PatchPython $collectArgs)
}

if (-not (Test-Path -LiteralPath $Runner -PathType Leaf)) {
    Write-ToolError "ERROR: Missing Patch Tool core: $Runner"
    exit 2
}

# Match run_python_patches.sh: strip obsolete transaction/SANDBOX switches and
# force every PATCH-capable route to the in-place execution contract.
$filtered = New-Object System.Collections.Generic.List[string]
$forceInplace = $false
$skipTransactionValue = $false
$strippedLegacyTransaction = $false

foreach ($rawArg in $ToolArgs) {
    $arg = [string]$rawArg
    $lower = $arg.ToLowerInvariant()
    if ($skipTransactionValue) {
        $skipTransactionValue = $false
        if ($lower -in @('off', 'auto', 'required')) { continue }
    }
    if ($lower -eq '--transaction') {
        $strippedLegacyTransaction = $true
        $skipTransactionValue = $true
        continue
    }
    if ($lower.StartsWith('--transaction=')) {
        $strippedLegacyTransaction = $true
        continue
    }
    if ($lower -eq '--keep-failed-sandbox' -or $lower.StartsWith('--keep-failed-sandbox=')) {
        $strippedLegacyTransaction = $true
        continue
    }

    [void]$filtered.Add($arg)
    if ($lower -in @('--patch', '--all', '--select') -or
        $lower.EndsWith('.zip') -or $lower.EndsWith('.py') -or
        $lower.EndsWith('.tar.gz') -or $lower.EndsWith('.tgz')) {
        $forceInplace = $true
    }
}

if (-not $forceInplace -and $filtered.Count -gt 0) {
    $first = $filtered[0].ToLowerInvariant()
    if ($first -notin @('paths', 'help', '--help', '-h', 'version', '--version')) {
        $forceInplace = $true
    }
}

if ($forceInplace) {
    $runnerArgs = @($Runner) + @($filtered) + @('--transaction', 'off')
    exit (Invoke-PatchPython $runnerArgs)
}

if ($strippedLegacyTransaction -and $filtered.Count -eq 0) {
    Write-ToolError 'ERROR: obsolete transaction/SANDBOX flags cannot be used as a standalone command.'
    Write-ToolError 'Use tools\run_python_patches.bat (or .ps1) with no arguments for the normal queue.'
    exit 2
}

exit (Invoke-PatchPython (@($Runner) + @($filtered)))
