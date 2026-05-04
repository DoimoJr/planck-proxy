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

' --- Step 2: kill watchdog precedenti (proxy + plugin) via PowerShell ---
' WMIC e' deprecato/rimosso in Windows 11 24H2+; usiamo Get-CimInstance
' che e' disponibile ovunque PowerShell e' presente (sempre, su Win 7+).
On Error Resume Next
ws.Run "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -Command """ & _
    "Get-CimInstance Win32_Process -Filter ""Name='wscript.exe' OR Name='powershell.exe'"" | " & _
    "Where-Object { $_.CommandLine -match 'proxy_watchdog.vbs|planck_.*_watchdog.ps1' } | " & _
    "ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }""", 0, True
fso.DeleteFile tmpDir & "\proxy_watchdog.vbs", True
fso.DeleteFile tmpDir & "\planck_usb_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_process_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_network_watchdog.ps1", True
' Cancella il flag stop dell'eventuale sessione precedente, altrimenti
' il watchdog appena lanciato uscirebbe immediatamente.
fso.DeleteFile tmpDir & "\planck_stop.flag", True
On Error Goto 0

' --- Step 3: crea + lancia il watchdog VBS proxy/alive ---
' Il watchdog controlla un flag file (planck_stop.flag) ad ogni iterazione:
' se proxy_off lo crea, il watchdog ESCE invece di riapplicare il proxy —
' kill "gentile" che funziona sempre, indipendente dal kill PowerShell
' che potrebbe fallire su edge case.
watchdogPath = tmpDir & "\proxy_watchdog.vbs"
Dim f
Set f = fso.CreateTextFile(watchdogPath, True)
f.WriteLine "Set ws = CreateObject(""WScript.Shell"")"
f.WriteLine "Set fso = CreateObject(""Scripting.FileSystemObject"")"
f.WriteLine "Dim stopFlag : stopFlag = ws.ExpandEnvironmentStrings(""%TEMP%\planck_stop.flag"")"
f.WriteLine "Do"
f.WriteLine "  If fso.FileExists(stopFlag) Then"
f.WriteLine "    ws.RegWrite ""HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyEnable"", 0, ""REG_DWORD"""
f.WriteLine "    WScript.Quit 0"
f.WriteLine "  End If"
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

' --- Step 4: scarica + lancia plugin watchdog (USB, process, network). ---
DownloadAndRunPS "/api/scripts/watchdog/usb.ps1", "planck_usb_watchdog.ps1"
DownloadAndRunPS "/api/scripts/watchdog/process.ps1", "planck_process_watchdog.ps1"
DownloadAndRunPS "/api/scripts/watchdog/network.ps1", "planck_network_watchdog.ps1"

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
//
// Sequenza (importante l'ordine):
//   1. Crea il flag file `planck_stop.flag`. Il watchdog VBS lo controlla
//      ad ogni iterazione e si auto-termina quando lo trova (kill "gentile",
//      sempre affidabile, non dipende da WMIC ne' privilegi).
//   2. Aspetta 6 secondi (un ciclo intero del watchdog = 5s + margine).
//   3. Disabilita ProxyEnable in HKCU.
//   4. Kill defensivo via PowerShell (`Get-CimInstance` + `Stop-Process`)
//      di eventuali processi residui — sostituisce WMIC che e' deprecato/
//      rimosso in Windows 11 24H2+ e causa il bug "il proxy non si toglie
//      davvero" perche' il watchdog continuava a girare.
//   5. Cleanup temp + self-delete.
const proxyOffTemplate = `' ============================================================
' Planck Proxy v__VERSIONE__ - proxy_off.vbs
' Disattiva il proxy + ferma tutti i watchdog. Invisibile.
' ============================================================

Option Explicit
Dim ws, fso, tmpDir, stopFlag
Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
tmpDir = ws.ExpandEnvironmentStrings("%TEMP%")
stopFlag = tmpDir & "\planck_stop.flag"

' --- Step 1: crea flag stop (kill gentile dei watchdog) ---
' Il watchdog VBS controlla questo file ad ogni loop (~5s) e si auto-
' termina quando lo trova. Funziona sempre, indipendente da WMIC/
' PowerShell e da privilegi.
On Error Resume Next
Dim flagF : Set flagF = fso.CreateTextFile(stopFlag, True)
If Not flagF Is Nothing Then flagF.Close
On Error Goto 0

' --- Step 2: aspetta che il watchdog veda il flag (1 ciclo + margine) ---
WScript.Sleep 6000

' --- Step 3: disabilita proxy in HKCU ---
On Error Resume Next
ws.RegWrite "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings\ProxyEnable", 0, "REG_DWORD"
On Error Goto 0

' --- Step 4: kill defensivo via PowerShell (sostituisce WMIC deprecato) ---
' Targetting per CommandLine: ammazza solo i processi che eseguono
' proxy_watchdog.vbs o planck_*_watchdog.ps1, NON proxy_off.vbs (noi).
On Error Resume Next
ws.Run "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -Command """ & _
    "Get-CimInstance Win32_Process -Filter ""Name='wscript.exe' OR Name='powershell.exe'"" | " & _
    "Where-Object { $_.CommandLine -match 'proxy_watchdog.vbs|planck_.*_watchdog.ps1' } | " & _
    "ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }""", 0, True
On Error Goto 0

' --- Step 5: cleanup temp + flag stop + self-delete ---
On Error Resume Next
fso.DeleteFile tmpDir & "\proxy_watchdog.vbs", True
fso.DeleteFile tmpDir & "\planck_usb_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_process_watchdog.ps1", True
fso.DeleteFile tmpDir & "\planck_network_watchdog.ps1", True
fso.DeleteFile stopFlag, True
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
