@echo off
:: ============================================================
:: Attiva proxy sul PC studente + watchdog che lo riattiva ogni 5s
:: e pinga il server docente per segnalare la presenza (colonna WD)
:: Distribuire via Veyon (o equivalente) e lanciare sui PC studenti
::
:: !!! PRIMA DI DISTRIBUIRE: aggiornare IP_PROF con l'IP del PC docente !!!
:: La PORTA deve corrispondere a proxy.port in config.json del docente.
:: ============================================================

set IP_PROF=192.168.6.100
set PORTA=9090

:: Attiva subito (con bypass per il server del prof e localhost)
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f >nul 2>&1
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer /t REG_SZ /d "%IP_PROF%:%PORTA%" /f >nul 2>&1
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyOverride /t REG_SZ /d "%IP_PROF%;localhost;127.0.0.1;<local>" /f >nul 2>&1

:: Crea lo script watchdog nascosto
echo Set ws = CreateObject("WScript.Shell") > "%TEMP%\proxy_watchdog.vbs"
echo Do >> "%TEMP%\proxy_watchdog.vbs"
echo   ws.Run "reg add ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings"" /v ProxyEnable /t REG_DWORD /d 1 /f", 0, True >> "%TEMP%\proxy_watchdog.vbs"
echo   ws.Run "reg add ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings"" /v ProxyServer /t REG_SZ /d ""%IP_PROF%:%PORTA%"" /f", 0, True >> "%TEMP%\proxy_watchdog.vbs"
echo   ws.Run "reg add ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings"" /v ProxyOverride /t REG_SZ /d ""%IP_PROF%;localhost;127.0.0.1;<local>"" /f", 0, True >> "%TEMP%\proxy_watchdog.vbs"
echo   On Error Resume Next >> "%TEMP%\proxy_watchdog.vbs"
echo   Dim http : Set http = CreateObject("MSXML2.ServerXMLHTTP.6.0") >> "%TEMP%\proxy_watchdog.vbs"
echo   http.setProxy 1 >> "%TEMP%\proxy_watchdog.vbs"
echo   http.setTimeouts 3000, 3000, 3000, 3000 >> "%TEMP%\proxy_watchdog.vbs"
echo   http.Open "GET", "http://%IP_PROF%:%PORTA%/_alive", False >> "%TEMP%\proxy_watchdog.vbs"
echo   http.Send >> "%TEMP%\proxy_watchdog.vbs"
echo   Set http = Nothing >> "%TEMP%\proxy_watchdog.vbs"
echo   Err.Clear >> "%TEMP%\proxy_watchdog.vbs"
echo   On Error Goto 0 >> "%TEMP%\proxy_watchdog.vbs"
echo   WScript.Sleep 5000 >> "%TEMP%\proxy_watchdog.vbs"
echo Loop >> "%TEMP%\proxy_watchdog.vbs"

:: Lancia il watchdog nascosto (nessuna finestra visibile)
start "" /b wscript.exe "%TEMP%\proxy_watchdog.vbs"

echo Proxy attivato: %IP_PROF%:%PORTA%

(goto) 2>nul & del "%~f0"
