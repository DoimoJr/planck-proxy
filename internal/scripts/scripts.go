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
const proxyOnTemplate = `' ============================================================
' Planck Proxy v__VERSIONE__ - proxy_on.vbs
' PC docente: __IP_DOCENTE__:__PORTA_PROXY__
'
' VBScript invece di .bat per essere TOTALMENTE invisibile sul PC
' studente. Windows apre .vbs con wscript.exe (subsystem GUI =
' nessuna console, nessun flash), .bat con cmd.exe (subsystem
' console = sempre lampeggia).
'
' Logica:
'   1. Setta il proxy in HKCU (registry user, no UAC)
'   2. Killa watchdog precedenti per evitare duplicati su redistribuzione
'   3. Crea+lancia un watchdog VBS che ri-applica il proxy + pinga /_alive
'   4. Scarica + lancia gli script watchdog plugin enabled (USB, process)
'   5. Self-delete del .vbs
' ============================================================

Option Explicit
Dim ipProf, portaProxy, portaWeb, ws, fso, tmpDir, watchdogPath
ipProf = "__IP_DOCENTE__"
portaProxy = "__PORTA_PROXY__"
portaWeb = "__PORTA_WEB__"

Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
tmpDir = ws.ExpandEnvironmentStrings("%TEMP%")

' --- Step 1: set proxy registry ---
ws.RegWrite "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyEnable", 1, "REG_DWORD"
ws.RegWrite "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyServer", ipProf & ":" & portaProxy, "REG_SZ"
ws.RegWrite "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyOverride", ipProf & ";localhost;127.0.0.1;<local>", "REG_SZ"

' --- Step 2: kill watchdog precedenti (proxy + plugin) ---
On Error Resume Next
ws.Run "wmic process where ""commandline like '%%proxy_watchdog.vbs%%'"" delete", 0, True
ws.Run "wmic process where ""commandline like '%%planck_%%_watchdog.ps1%%'"" delete", 0, True
fso.DeleteFile tmpDir & "\proxy_watchdog.vbs", True
fso.DeleteFile tmpDir & "\planck_usb_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_process_watchdog.ps1", True
On Error Goto 0

' --- Step 3: crea + lancia il watchdog VBS proxy/alive ---
watchdogPath = tmpDir & "\proxy_watchdog.vbs"
Dim f
Set f = fso.CreateTextFile(watchdogPath, True)
f.WriteLine "Set ws = CreateObject(""WScript.Shell"")"
f.WriteLine "Do"
f.WriteLine "  ws.RegWrite ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyEnable"", 1, ""REG_DWORD"""
f.WriteLine "  ws.RegWrite ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyServer"", """ & ipProf & ":" & portaProxy & """, ""REG_SZ"""
f.WriteLine "  ws.RegWrite ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyOverride"", """ & ipProf & ";localhost;127.0.0.1;<local>"", ""REG_SZ"""
f.WriteLine "  On Error Resume Next"
f.WriteLine "  Dim h : Set h = CreateObject(""MSXML2.ServerXMLHTTP.6.0"")"
f.WriteLine "  h.setProxy 1"
f.WriteLine "  h.setTimeouts 3000, 3000, 3000, 3000"
f.WriteLine "  h.Open ""GET"", ""http://" & ipProf & ":" & portaProxy & "/_alive"", False"
f.WriteLine "  h.Send"
f.WriteLine "  Set h = Nothing"
f.WriteLine "  Err.Clear"
f.WriteLine "  On Error Goto 0"
f.WriteLine "  WScript.Sleep 5000"
f.WriteLine "Loop"
f.Close

' Lancia hidden (intWindowStyle=0, bWaitOnReturn=False).
ws.Run "wscript.exe """ & watchdogPath & """", 0, False

' --- Step 4: scarica + lancia plugin watchdog (USB, process). ---
DownloadAndRunPS "/api/scripts/watchdog/usb.ps1", "planck_usb_watchdog.ps1"
DownloadAndRunPS "/api/scripts/watchdog/process.ps1", "planck_process_watchdog.ps1"

' --- Step 5: self-delete del .vbs (best-effort) ---
On Error Resume Next
fso.DeleteFile WScript.ScriptFullName, True
On Error Goto 0

WScript.Quit 0

' Helper: scarica uno script .ps1 da Planck (404 -> skip),
' lo salva e lo lancia con powershell hidden.
Sub DownloadAndRunPS(urlPath, localFile)
    Dim psPath, h, stream
    psPath = tmpDir & "\" & localFile
    On Error Resume Next
    Set h = CreateObject("MSXML2.ServerXMLHTTP.6.0")
    h.setTimeouts 3000, 3000, 3000, 3000
    h.Open "GET", "http://" & ipProf & ":" & portaWeb & urlPath, False
    h.Send
    If Err.Number = 0 And h.Status = 200 Then
        Set stream = CreateObject("ADODB.Stream")
        stream.Type = 1 ' adTypeBinary
        stream.Open
        stream.Write h.ResponseBody
        stream.SaveToFile psPath, 2 ' adSaveCreateOverWrite
        stream.Close
        ws.Run "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File """ & psPath & """", 0, False
    End If
    Err.Clear
    On Error Goto 0
End Sub
`

// proxyOffTemplate e' il contenuto di proxy_off.vbs.
// VBScript invece di .bat per essere invisibile sullo studente
// (subsystem GUI). Uccide il watchdog VBS proxy + i watchdog ps1
// plugin, disattiva il proxy in HKCU, fa pulizia del temp, self-delete.
const proxyOffTemplate = `' ============================================================
' Planck Proxy v__VERSIONE__ - proxy_off.vbs
' Disattiva il proxy + ferma tutti i watchdog. Invisibile.
' ============================================================

Option Explicit
Dim ws, fso, tmpDir
Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
tmpDir = ws.ExpandEnvironmentStrings("%TEMP%")

' --- Step 1: kill watchdog proxy (VBS) ---
' WMIC filter su CommandLine: kill solo i wscript che girano
' proxy_watchdog.vbs (NON proxy_off.vbs che siamo noi).
On Error Resume Next
ws.Run "wmic process where ""commandline like '%%proxy_watchdog.vbs%%'"" delete", 0, True
ws.Run "wmic process where ""commandline like '%%planck_%%_watchdog.ps1%%'"" delete", 0, True
On Error Goto 0

' --- Step 2: cleanup temp ---
On Error Resume Next
fso.DeleteFile tmpDir & "\proxy_watchdog.vbs", True
fso.DeleteFile tmpDir & "\planck_usb_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_process_watchdog.ps1", True
On Error Goto 0

' --- Step 3: disabilita proxy ---
ws.RegWrite "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyEnable", 0, "REG_DWORD"

' --- Step 4: self-delete ---
On Error Resume Next
fso.DeleteFile WScript.ScriptFullName, True
On Error Goto 0

WScript.Quit 0
`

// Generate scrive proxy_on.bat e proxy_off.bat in outDir, sostituendo i
// segnaposto con i valori forniti.
//
// `portaWeb` e' la porta del web server di Planck (default 9999),
// usata da proxy_on.bat per scaricare gli script watchdog (Phase 5).
//
// Ritorna i due path assoluti dei file scritti.
func Generate(outDir, versione, ipDocente string, portaProxy, portaWeb int) (onPath, offPath string, err error) {
	if ipDocente == "" {
		return "", "", fmt.Errorf("ipDocente vuoto")
	}
	if portaProxy <= 0 || portaProxy > 65535 {
		return "", "", fmt.Errorf("porta proxy invalida: %d", portaProxy)
	}
	if portaWeb <= 0 || portaWeb > 65535 {
		return "", "", fmt.Errorf("porta web invalida: %d", portaWeb)
	}

	onContent := strings.NewReplacer(
		"__VERSIONE__", versione,
		"__IP_DOCENTE__", ipDocente,
		"__PORTA_PROXY__", strconv.Itoa(portaProxy),
		"__PORTA_WEB__", strconv.Itoa(portaWeb),
	).Replace(proxyOnTemplate)

	offContent := strings.NewReplacer(
		"__VERSIONE__", versione,
	).Replace(proxyOffTemplate)

	onPath = filepath.Join(outDir, "proxy_on.vbs")
	offPath = filepath.Join(outDir, "proxy_off.vbs")

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
// Algoritmo (in ordine, primo che funziona vince):
//
//  1. **UDP dial trick** verso 8.8.8.8: l'OS sceglie l'IP sorgente
//     in base alla tabella di routing — che e' l'interfaccia di
//     default (quella che raggiunge internet). Su PC multi-interface
//     (Wi-Fi + Ethernet + VirtualBox host-only + VPN) prende
//     SEMPRE quella giusta, evitando l'ambiguita' della scansione.
//     Niente pacchetti spediti davvero (UDP e' connectionless).
//  2. Scansione interfacce: preferisce IP in range privati RFC 1918
//     (10/8, 172.16/12, 192.168/16). Fallback se l'host non ha
//     connettivita' internet (lab air-gapped).
//  3. 127.0.0.1: ultima spiaggia, utile solo per smoke test.
//
// Override sempre disponibile via env var PLANCK_LAN_IP.
func LocalLANIP() string {
	// Step 1: UDP dial trick. Best-effort: in caso di errore o IP
	// loopback-ish, cade allo step 2.
	if ip := lanIPViaUDPDial(); ip != "" && ip != "127.0.0.1" {
		return ip
	}

	// Step 2: scansione interfacce.
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

// lanIPViaUDPDial usa il "trick UDP" per ottenere l'IP sorgente che
// l'OS userebbe per uscire verso internet — cioe' l'IP dell'interfaccia
// di default route. Niente pacchetti realmente inviati (UDP e' un
// dial "fittizio", risolve solo la routing table).
//
// Funziona su Wi-Fi, Ethernet, VPN — qualunque setup abbia internet.
// In una rete air-gapped (lab senza internet) il dial fallisce e
// torniamo alla scansione manuale.
func lanIPViaUDPDial() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	ip4 := addr.IP.To4()
	if ip4 == nil {
		return ""
	}
	return ip4.String()
}
