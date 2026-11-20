# TaskDeck Linux binary through WSL2; no Docker required.
[CmdletBinding()]
param(
 [ValidateSet('Start','Stop','Status','Update','Doctor')][string]$Action='Start',
 [string]$ProjectPath=(Get-Location).Path,
 [string]$Distribution='Ubuntu',
 [switch]$NoBrowser
)
Set-StrictMode -Version 2.0
$ErrorActionPreference='Stop'
if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) {
 throw 'WSL2 is not installed. Install with: wsl --install -d Ubuntu'
}
$dir=Get-Item -LiteralPath $ProjectPath -ErrorAction Stop
if (-not $dir.PSIsContainer) {throw "Not a folder: $ProjectPath"}
$project=[IO.Path]::GetFullPath($dir.FullName)
if ($project -notmatch '^[A-Za-z]:\\') {
 throw 'Use an absolute local drive folder such as D:\Projects; WSL-native paths can be launched from inside WSL directly.'
}
$linuxPath=& wsl.exe --distribution $Distribution -- wslpath -u $project
if ($LASTEXITCODE -ne 0 -or -not $linuxPath) {
 throw 'WSL path translation failed. Check distribution installation and drive sharing.'
}
$linuxPath=([string]$linuxPath).Trim()
Write-Host "TaskDeck WSL2 workspace: $linuxPath"
# Positional argument $1 preserves spaces/unicode and avoids Bash command injection.
switch($Action) {
 'Doctor' {
  & wsl.exe --distribution $Distribution -- bash -lc 'printf "Tools: "; for c in bash git ssh python3 taskdeck; do command -v "$c" || true; done; printf "Workspace: %s\n" "$1"; test -d "$1"' taskdeck-wsl $linuxPath
 }
 'Status' {
  & wsl.exe --distribution $Distribution -- bash -lc 'taskdeck --workspace "$1" --status' taskdeck-wsl $linuxPath
 }
 'Stop' {
  & wsl.exe --distribution $Distribution -- bash -lc 'taskdeck --workspace "$1" --stop-daemon' taskdeck-wsl $linuxPath
 }
 'Update' {
  & wsl.exe --distribution $Distribution -- bash -lc 'taskdeck --workspace "$1" --self-update' taskdeck-wsl $linuxPath
 }
 'Start' {
  & wsl.exe --distribution $Distribution -- bash -lc 'taskdeck --workspace "$1" --no-browser' taskdeck-wsl $linuxPath
  if ($LASTEXITCODE -eq 0 -and -not $NoBrowser) {
   Write-Host 'Use the HTTPS URL printed above in Windows Firefox/Edge (WSL localhost forwarding).'
  }
 }
}
if ($LASTEXITCODE -ne 0) { throw "TaskDeck WSL command failed (exit $LASTEXITCODE)." }
