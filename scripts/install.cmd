@echo off
REM Reames Agent Installer for Windows CMD users.
REM
REM Usage:
REM   curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.cmd -o install.cmd && install.cmd && del install.cmd

echo.
echo  Reames Agent Installer
echo  Launching PowerShell installer...
echo.

if exist "%~dp0install.ps1" (
    powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0install.ps1" %*
) else (
    powershell -ExecutionPolicy Bypass -NoProfile -Command "iex (irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.ps1)"
)

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo  Installation failed. Try running PowerShell directly:
    echo    powershell -ExecutionPolicy Bypass -File scripts\install.ps1
    echo.
    pause
    exit /b 1
)

echo.
echo  Installation completed.
