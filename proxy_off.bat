@echo off
:: ============================================================
:: Disattiva proxy e ferma il watchdog
:: ============================================================

:: Ferma il watchdog
taskkill /f /im wscript.exe >nul 2>&1
del "%TEMP%\proxy_watchdog.vbs" >nul 2>&1

:: Disattiva proxy
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1

echo Proxy disattivato.

(goto) 2>nul & del "%~f0"
