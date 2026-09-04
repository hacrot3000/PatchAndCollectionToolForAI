@echo off
setlocal
rem Python Patch Tool v6.17.9 Windows launcher wrapper.
set "PTV_TOOLS_DIR=%~dp0"

where powershell.exe >nul 2>nul
if not errorlevel 1 (
  powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%PTV_TOOLS_DIR%run_python_patches.ps1" %*
  exit /b %ERRORLEVEL%
)

where pwsh.exe >nul 2>nul
if not errorlevel 1 (
  pwsh.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%PTV_TOOLS_DIR%run_python_patches.ps1" %*
  exit /b %ERRORLEVEL%
)

echo ERROR: PowerShell was not found. Windows PowerShell 5.1 or PowerShell 7+ is required. 1>&2
exit /b 2
