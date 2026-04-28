// Package scripts genera in automatico i file proxy_on.bat e proxy_off.bat
// per i PC studenti, con l'IP del PC docente e la porta del proxy
// pre-compilati. Cosi' il prof non deve piu' modificare i .bat a mano
// prima di distribuirli via Veyon.
//
// I file vengono rigenerati ad ogni boot del binario (sovrascrivendo
// eventuali versioni precedenti) per riflettere sempre l'IP corrente.
package scripts

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// proxyOnTemplate e' il contenuto di proxy_on.bat. I segnaposto
// `__IP_DOCENTE__`, `__PORTA_PROXY__`, `__VERSIONE__` vengono sostituiti
// in Generate.
const proxyOnTemplate = `@echo off
:: ============================================================
:: Generato automaticamente da Planck Proxy v__VERSIONE__
:: PC docente: __IP_DOCENTE__:__PORTA_PROXY__
:: Distribuire via Veyon ai PC studenti.
:: ============================================================

set IP_PROF=__IP_DOCENTE__
set PORTA=__PORTA_PROXY__

:: Attiva subito (con bypass per il server del prof e localhost)
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 1 /f >nul 2>&1
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer /t REG_SZ /d "%IP_PROF%:%PORTA%" /f >nul 2>&1
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyOverride /t REG_SZ /d "%IP_PROF%;localhost;127.0.0.1;<local>" /f >nul 2>&1

:: Crea lo script watchdog nascosto che riapplica il proxy + ping di presenza
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

start "" /b wscript.exe "%TEMP%\proxy_watchdog.vbs"

echo Proxy attivato: %IP_PROF%:%PORTA%

(goto) 2>nul & del "%~f0"
`

// proxyOffTemplate e' il contenuto di proxy_off.bat (il server non e'
// referenziato — disattiva semplicemente il proxy locale di Windows).
const proxyOffTemplate = `@echo off
:: ============================================================
:: Generato automaticamente da Planck Proxy v__VERSIONE__
:: Disattiva il proxy e ferma il watchdog di presenza.
:: ============================================================

:: Ferma il watchdog
taskkill /f /im wscript.exe >nul 2>&1
del "%TEMP%\proxy_watchdog.vbs" >nul 2>&1

:: Disattiva proxy
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f >nul 2>&1

echo Proxy disattivato.

(goto) 2>nul & del "%~f0"
`

// Generate scrive proxy_on.bat e proxy_off.bat in outDir, sostituendo i
// segnaposto con i valori forniti.
//
// Ritorna i due path assoluti dei file scritti.
func Generate(outDir, versione, ipDocente string, portaProxy int) (onPath, offPath string, err error) {
	if ipDocente == "" {
		return "", "", fmt.Errorf("ipDocente vuoto")
	}
	if portaProxy <= 0 || portaProxy > 65535 {
		return "", "", fmt.Errorf("porta proxy invalida: %d", portaProxy)
	}

	onContent := strings.NewReplacer(
		"__VERSIONE__", versione,
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_PROXY__", strconv.Itoa(portaProxy),
	).Replace(proxyOnTemplate)

	offContent := strings.NewReplacer(
		"__VERSIONE__", versione,
	).Replace(proxyOffTemplate)

	onPath = filepath.Join(outDir, "proxy_on.bat")
	offPath = filepath.Join(outDir, "proxy_off.bat")

	if err := os.WriteFile(onPath, []byte(onContent), 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", onPath, err)
	}
	if err := os.WriteFile(offPath, []byte(offContent), 0o644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", offPath, err)
	}
	return onPath, offPath, nil
}

// LocalLANIP individua l'IPv4 della LAN locale del PC docente.
//
// Algoritmo:
//  1. Scansiona tutte le interfacce di rete attive (non-loopback, up).
//  2. Preferisce IP in range privati (RFC 1918: 10/8, 172.16/12, 192.168/16).
//  3. Fallback al primo IPv4 non-loopback trovato.
//  4. Ultima spiaggia: 127.0.0.1 (utile solo per smoke test).
//
// Per scuole con piu' interfacce (es. PC con Wi-Fi + Ethernet + VPN),
// l'override e' possibile via env var PLANCK_LAN_IP gestito dal main.
func LocalLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if ip4.IsPrivate() {
				return ip4.String()
			}
			if fallback == "" {
				fallback = ip4.String()
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}
