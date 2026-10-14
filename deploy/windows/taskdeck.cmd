@echo off
setlocal
REM TaskDeck Windows Docker Launcher: forwards quoted arguments to PowerShell.
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0taskdeck.ps1" %*
exit /b %ERRORLEVEL%
