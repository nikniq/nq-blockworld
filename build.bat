@echo off
rem build.bat - runs build.ps1 even where PowerShell scripts are blocked by policy.
rem Options pass straight through, for example: build.bat -Version 1.2 -Run
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
exit /b %ERRORLEVEL%
