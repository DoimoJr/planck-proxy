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
go build -o planck.exe -trimpath -ldflags="-s -w" ./cmd/planck
if errorlevel 1 (
    echo Build fallita.
    exit /b 1
)
echo Built planck.exe
