@echo off
:: Build script per Windows. Produce planck.exe nella radice.
go build -o planck.exe -trimpath -ldflags="-s -w" ./cmd/planck
if errorlevel 1 (
    echo Build fallita.
    exit /b 1
)
echo Built planck.exe
