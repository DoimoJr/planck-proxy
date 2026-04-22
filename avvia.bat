@echo off
:: ============================================================
:: Avvia il server unificato (proxy + monitor web)
:: Eseguire sul PC del docente
:: ============================================================

set NODE_PATH=

:: Node.js nella cartella corrente (portable)
if exist "%~dp0node.exe" set NODE_PATH=%~dp0node.exe

:: Posizioni comuni
if "%NODE_PATH%"=="" if exist "C:\Program Files\nodejs\node.exe" set NODE_PATH=C:\Program Files\nodejs\node.exe

:: Fallback PATH
if "%NODE_PATH%"=="" (
    where node >nul 2>&1
    if %errorlevel%==0 (
        set NODE_PATH=node
    ) else (
        echo ERRORE: Node.js non trovato!
        echo.
        echo Opzioni:
        echo   1. Scarica Node.js portable da https://nodejs.org/en/download
        echo      e metti node.exe nella stessa cartella di questo file
        echo   2. Installa Node.js normalmente
        echo.
        pause
        exit /b 1
    )
)

echo ===========================================
echo   Avvio Monitor Traffico
echo ===========================================
echo.

"%NODE_PATH%" "%~dp0server.js"

pause
