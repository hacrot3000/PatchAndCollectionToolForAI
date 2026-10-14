# TaskDeck Windows launcher: Docker Desktop + Linux PTY backend.
# PowerShell 5.1+ / PowerShell 7; all project paths passed as argv (never eval).
[CmdletBinding()]
param(
  [ValidateSet('Start','Stop','Status','Open','Logs','Build','Update','Doctor','Credentials')]
  [string]$Action = 'Start',
  [string]$ProjectPath = (Get-Location).Path,
  [ValidateSet('runtime','dev')][string]$Profile = 'runtime',
  [string]$Image = 'taskdeck:local',
  [ValidateRange(1024,65535)][int]$Port = 42882,
  [switch]$Browse,
  [switch]$NoBrowser,
  [switch]$Follow,
  [switch]$PullImage,
  [switch]$RevealPassword
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

function Resolve-ProjectPath([string]$RawPath,[bool]$ChooseFolder) {
  if ($ChooseFolder) {
    if ($env:OS -ne 'Windows_NT') { throw 'Folder picker requires Windows desktop; supply -ProjectPath.' }
    $shell = New-Object -ComObject Shell.Application
    $selected = $shell.BrowseForFolder(0,'Select TaskDeck project folder',0,0)
    if ($null -eq $selected) { throw 'Project selection canceled.' }
    $RawPath = [string]$selected.Self.Path
  }
  $folder = Get-Item -LiteralPath $RawPath -ErrorAction Stop
  if (-not $folder.PSIsContainer) { throw "Not a directory: $RawPath" }
  $full = [IO.Path]::GetFullPath($folder.FullName).TrimEnd('\')
  if ($full -notmatch '^[A-Za-z]:\\') {
    throw 'Docker Desktop bind mounts require a local Windows drive path (example: D:\Projects). For WSL paths, use taskdeck-wsl.ps1.'
  }
  if ($full.Contains(',')) {
    throw 'Folder paths with commas are unsupported by Docker --mount; choose a path without commas.'
  }
  return $full
}
function Get-ProjectHash([string]$FullPath) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try {
    $bytes = [Text.Encoding]::UTF8.GetBytes($FullPath.ToLowerInvariant())
    return [BitConverter]::ToString($sha.ComputeHash($bytes)).Replace('-','').Substring(0,12).ToLowerInvariant()
  } finally { $sha.Dispose() }
}
function Docker-Run([string[]]$Arguments) {
  & docker.exe @Arguments
  if ($LASTEXITCODE -ne 0) { throw "Docker command failed (exit $LASTEXITCODE): docker $($Arguments -join ' ')" }
}
function Docker-Exists([string]$Kind,[string]$Name) {
  try {
    & docker.exe $Kind inspect $Name 1>$null 2>$null
    return ($LASTEXITCODE -eq 0)
  } catch { return $false }
}
function Require-Docker {
  if (-not (Get-Command docker.exe -ErrorAction SilentlyContinue)) {
    throw 'Docker Desktop CLI was not found. Install/start Docker Desktop with WSL2 backend.'
  }
  & docker.exe info --format '{{.ServerVersion}}' 1>$null 2>$null
  if ($LASTEXITCODE -ne 0) { throw 'Docker Desktop engine is not responding. Start Docker Desktop first.' }
}
function Build-Image {
  if (-not (Test-Path -LiteralPath $dockerfile -PathType Leaf)) {
    throw "Dockerfile unavailable at $dockerfile. Supply -PullImage with a published image or run launcher from the TaskDeck repository."
  }
  Docker-Run @('build','--pull','--file',$dockerfile,'--target',$Profile,'--tag',$Image,$repoRoot)
}
function Ensure-Image {
  if (Docker-Exists 'image' $Image) { return }
  if ($PullImage) { Docker-Run @('pull',$Image) }
  else { Build-Image }
}
function Container-Status {
  if (-not (Docker-Exists 'container' $containerName)) { return 'not-created' }
  $status = & docker.exe inspect --format '{{.State.Status}}' $containerName
  if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect Docker container.' }
  return ([string]$status).Trim()
}
function Start-TaskDeck {
  Ensure-Image
  $status = Container-Status
  if ($status -eq 'running') {
    Write-Host "TaskDeck already running: $containerName"
  } elseif ($status -ne 'not-created') {
    Docker-Run @('start',$containerName)
  } else {
    # No shell string concatenation/eval: Docker receives one argv per option.
    $mountProject = "type=bind,source=$project,target=/workspace"
    $mountHome = "type=volume,source=$homeVolume,target=/home/taskdeck"
    $args = @('run','--detach','--init','--name',$containerName,
      '--restart','unless-stopped','--security-opt','no-new-privileges',
      '--publish',"127.0.0.1:$($Port):42882",
      '--mount',$mountProject,'--mount',$mountHome,
      '--env','TASKDECK_INIT_WORKSPACE=1',
      '--env','TASKDECK_AUTO_CONFIG=1',
      $Image)
    Docker-Run $args
  }
  Write-Host "TaskDeck: https://localhost:$Port"
  Write-Host "Project: $project"
  Write-Host "Initial login config: $project\.vscode\vscode_tasks_menu.ini"
  if (-not $NoBrowser) { Start-Process "https://localhost:$Port" }
}

# Both repository checkout (deploy/windows/) and the distributable ZIP
# (scripts and Docker context beside the EXE) must build the same image.
$repoRoot = $null
foreach ($candidate in @($PSScriptRoot, (Join-Path $PSScriptRoot '..\..'))) {
  $absolute = [IO.Path]::GetFullPath($candidate)
  if (Test-Path -LiteralPath (Join-Path $absolute 'Dockerfile.taskdeck') -PathType Leaf) {
    $repoRoot = $absolute
    break
  }
}
if ($null -eq $repoRoot) { $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..')) }
$dockerfile = Join-Path $repoRoot 'Dockerfile.taskdeck'
$project = Resolve-ProjectPath $ProjectPath $Browse.IsPresent
$hash = Get-ProjectHash $project
$containerName = "taskdeck-$hash"
$homeVolume = "taskdeck-home-$hash"
Require-Docker

switch ($Action) {
 'Doctor' {
   Write-Host 'TaskDeck Windows environment'
   Docker-Run @('--version')
   Docker-Run @('info','--format','Server {{.ServerVersion}} · OS {{.OSType}}')
   Write-Host "Project: $project"
   Write-Host "Dockerfile: $dockerfile"
   Write-Host "Image: $Image"
   Write-Host "Container: $containerName ($((Container-Status)))"
 }
 'Build' { Build-Image }
 'Update' {
   if ($PullImage) { Docker-Run @('pull',$Image) } else { Build-Image }
   if (Docker-Exists 'container' $containerName) {
     # Explicit Update recreates only this container. Never removes home volume
     # or project files; a failed build/pull leaves the old container untouched.
     Docker-Run @('rm','--force',$containerName)
   }
   Start-TaskDeck
 }
 'Start' { Start-TaskDeck }
 'Stop' {
   if (Docker-Exists 'container' $containerName) {
     if ((Container-Status) -eq 'running') { Docker-Run @('stop','--time','25',$containerName) }
     Write-Host "TaskDeck stopped; project and $homeVolume preserved."
   } else { Write-Host 'TaskDeck container not created for this project.' }
 }
 'Status' {
   Write-Host "Container: $containerName"
   Write-Host "Status: $(Container-Status)"
   Write-Host "URL: https://localhost:$Port"
   Write-Host "Project: $project"
 }
 'Open' { Start-Process "https://localhost:$Port" }
 'Logs' {
   if (-not (Docker-Exists 'container' $containerName)) { throw 'TaskDeck container not found; start it first.' }
   $args = @('logs','--tail','200')
   if ($Follow) { $args += '--follow' }
   Docker-Run ($args + @($containerName))
 }
 'Credentials' {
   $ini = Join-Path $project '.vscode\vscode_tasks_menu.ini'
   if (-not (Test-Path -LiteralPath $ini -PathType Leaf)) {
     throw "No credentials file found yet: $ini (start TaskDeck first)."
   }
   Write-Host "Private config: $ini"
   if ($RevealPassword) {
     if ((Read-Host 'Type YES to display the admin password in this console') -ne 'YES') {
       throw 'Credential display canceled.'
     }
     $matches = @(Select-String -LiteralPath $ini -Pattern '^password\s*=\s*(.*)$')
     if ($matches.Count -eq 0) { throw 'No password line found in config.' }
     Write-Output $matches[0].Matches[0].Groups[1].Value
   } else { Write-Host 'Add -RevealPassword to display after confirmation; never commit the config.' }
 }
}
