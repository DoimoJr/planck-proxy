@echo off
:: Build script per Windows. Produce planck.exe nella radice.
::
:: L'icona del binario e' embeddata via cmd/planck/rsrc_windows_amd64.syso
:: che e' committato in repo. Per rigenerarla (es. dopo aver cambiato
:: assets/planck.ico):
::
::   go run ./tools/genicon
::   go install github.com/akavel/rsrc@latest
::   rsrc -ico assets/planck.ico -o cmd/planck/rsrc_windows_amd64.syso -arch amd64
::
:: Il "go build" sotto include automaticamente il .syso (nome con
:: suffix _windows_amd64 = arch-specific, linkato solo per quel target).
:: -H=windowsgui: subsystem GUI invece di console (no cmd flash all'avvio,
:: icona dell'.exe nella taskbar, log redirected su planck.log).
:: La versione viene iniettata dal tag git, non tenuta a mano nel sorgente:
:: era gia' andata storta una volta (il binario si dichiarava 2.9.25 fino
:: alla 2.9.28) e l'auto-update confronta versioni. Senza git resta il
:: fallback definito in main.go.
for /f "delims=" %%v in ('git describe --tags --abbrev^=0 2^>nul') do set TAG=%%v
set VERSIONE=%TAG:v=%

if defined VERSIONE (
    go build -o planck.exe -trimpath -ldflags="-s -w -H=windowsgui -X main.Versione=%VERSIONE%" ./cmd/planck
) else (
    go build -o planck.exe -trimpath -ldflags="-s -w -H=windowsgui" ./cmd/planck
)
if errorlevel 1 (
    echo Build fallita.
    exit /b 1
)
echo Built planck.exe
