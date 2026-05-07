package scripts

import "strings"

// FirefoxPoliciesJSON e' il contenuto del file `policies.json` da
// installare in `C:\Program Files\Mozilla Firefox\distribution\` (o
// `Program Files (x86)\` per Firefox 32-bit). Forza Firefox a usare
// le impostazioni proxy di sistema (HKCU registry, dove il
// proxy_watchdog.vbs di Planck scrive ogni 5s) e blocca la
// modifica della voce "Impostazioni di rete" nelle Preferenze
// Firefox (`Locked: true`). Senza policies.json, Firefox espone la
// sua propria configurazione proxy che non eredita Windows e che
// lo studente puo' impostare a "Nessun proxy" bypassando Planck.
//
// Risorse Mozilla:
//
//	https://mozilla.github.io/policy-templates/#proxy
const FirefoxPoliciesJSON = `{
  "policies": {
    "Proxy": {
      "Mode": "system",
      "Locked": true
    }
  }
}
`

// firefoxPoliciesDeployTemplate e' un VBS che, eseguito con privilegi
// admin (UAC prompt), scrive `policies.json` nelle dir distribution di
// tutte le installazioni Firefox trovate. Il file e' embedded nello
// script come literal VBScript (escape `"` -> `""`, newline ->
// `" & vbCrLf & "`). Distribuibile via Veyon FileTransfer +
// OpenFileInApplication=true: lo studente (o l'admin di laboratorio)
// vede un prompt UAC e accetta. Una sola volta per PC, al setup.
const firefoxPoliciesDeployTemplate = `' ============================================================
' Planck Proxy - firefox_lockdown.vbs
' Scrive policies.json nelle distribution dir di Firefox per
' forzare il proxy di sistema (Mode: system, Locked: true) e
' impedire allo studente di disattivarlo dalle Preferenze
' Firefox. Richiede privilegi admin (UAC prompt).
' ============================================================

Option Explicit
Dim ws, fso, args, i

Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")

' Detect privilegi: tenta di scrivere un file probe in Program Files.
' Se ci riesce siamo gia' admin/SYSTEM (caso Veyon Service che gira
' come SYSTEM su Windows → niente UAC, scrive direttamente). Se fail
' E non e' un re-launch (/elevated), self-elevate via UAC.
Dim isAdmin : isAdmin = False
On Error Resume Next
Dim probePath : probePath = "C:\Program Files\.planck_admin_probe"
Dim probeF : Set probeF = fso.CreateTextFile(probePath, True)
If Err.Number = 0 And Not probeF Is Nothing Then
    probeF.Close
    fso.DeleteFile probePath, True
    isAdmin = True
End If
Err.Clear
On Error Goto 0

Set args = WScript.Arguments
Dim alreadyElevated : alreadyElevated = False
For i = 0 To args.Count - 1
    If LCase(args(i)) = "/elevated" Then alreadyElevated = True
Next

If Not isAdmin And Not alreadyElevated Then
    Dim shellApp : Set shellApp = CreateObject("Shell.Application")
    shellApp.ShellExecute "wscript.exe", _
        Chr(34) & WScript.ScriptFullName & Chr(34) & " /elevated", _
        "", "runas", 1
    WScript.Quit 0
End If

' --- da qui in poi gira come admin (o SYSTEM via Veyon Service) ---

Dim policiesContent
policiesContent = "__POLICIES_JSON__"

Dim candidates(1)
candidates(0) = "C:\Program Files\Mozilla Firefox\distribution"
candidates(1) = "C:\Program Files (x86)\Mozilla Firefox\distribution"

Dim writtenCount : writtenCount = 0
For i = 0 To UBound(candidates)
    Dim parentDir : parentDir = Replace(candidates(i), "\distribution", "")
    If fso.FolderExists(parentDir) Then
        On Error Resume Next
        If Not fso.FolderExists(candidates(i)) Then fso.CreateFolder candidates(i)
        Dim f : Set f = fso.CreateTextFile(candidates(i) & "\policies.json", True)
        If Err.Number = 0 And Not f Is Nothing Then
            f.Write policiesContent
            f.Close
            writtenCount = writtenCount + 1
        End If
        Err.Clear
        On Error Goto 0
    End If
Next

__SILENT_HOOK__

WScript.Quit 0
`

// firefoxPoliciesMsgboxBlock e' lo snippet con MsgBox di feedback.
// Inserito solo nella variant non-silent (download manuale).
const firefoxPoliciesMsgboxBlock = `If writtenCount > 0 Then
    MsgBox "Planck: lockdown Firefox completato (" & writtenCount & " installazione/i).", _
        64, "Planck Proxy"
Else
    MsgBox "Planck: nessuna installazione Firefox trovata in Program Files.", _
        48, "Planck Proxy"
End If`

// FirefoxLockdownVBS ritorna il VBS pronto per la distribuzione, con il
// policies.json incorporato come literal VBScript (escape `"` -> `""`,
// newline -> `" & vbCrLf & "`).
//
// Se silent==true, niente MsgBox di feedback finale: utile quando
// distribuito via Veyon FileTransfer su molti PC contemporaneamente
// (un MsgBox per PC sarebbe rumoroso, e in sessione SYSTEM bloccherebbe
// lo script invisibilmente attendendo un click che non arrivera').
func FirefoxLockdownVBS(silent bool) string {
	encoded := vbsEscapeLiteral(FirefoxPoliciesJSON)
	hook := firefoxPoliciesMsgboxBlock
	if silent {
		hook = "' silent: niente MsgBox di feedback (distribuzione Veyon)"
	}
	out := strings.ReplaceAll(firefoxPoliciesDeployTemplate, "__POLICIES_JSON__", encoded)
	out = strings.ReplaceAll(out, "__SILENT_HOOK__", hook)
	return out
}

// vbsEscapeLiteral converte una stringa Go in un literal VBScript-safe.
func vbsEscapeLiteral(s string) string {
	// `"` -> `""` (escape interno) ; `\n` -> `" & vbCrLf & "` (concat)
	s = strings.ReplaceAll(s, `"`, `""`)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", `" & vbCrLf & "`)
	return s
}
