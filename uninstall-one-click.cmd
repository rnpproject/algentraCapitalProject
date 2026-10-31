@echo off
setlocal
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0infra\windows\launch-uninstall-one-click.ps1"
if errorlevel 1 (
  echo Uninstaller gagal. Catat pesan error pada jendela PowerShell.
  pause
)
