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

' Self-elevate via UAC: se non gia' admin, riavvia con verb "runas".
' Il flag "/elevated" segnala alla seconda invocazione di non rilanciare.
Set args = WScript.Arguments
Dim alreadyElevated : alreadyElevated = False
For i = 0 To args.Count - 1
    If LCase(args(i)) = "/elevated" Then alreadyElevated = True
Next

If Not alreadyElevated Then
    Dim shellApp : Set shellApp = CreateObject("Shell.Application")
    shellApp.ShellExecute "wscript.exe", _
        Chr(34) & WScript.ScriptFullName & Chr(34) & " /elevated", _
        "", "runas", 1
    WScript.Quit 0
End If

' --- da qui in poi gira come admin ---

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

If writtenCount > 0 Then
    MsgBox "Planck: lockdown Firefox completato (" & writtenCount & " installazione/i).", _
        64, "Planck Proxy"
Else
    MsgBox "Planck: nessuna installazione Firefox trovata in Program Files.", _
        48, "Planck Proxy"
End If

WScript.Quit 0
`

// FirefoxLockdownVBS ritorna il VBS pronto per la distribuzione, con il
// policies.json incorporato come literal VBScript (escape `"` -> `""`,
// newline -> `" & vbCrLf & "`).
func FirefoxLockdownVBS() string {
	encoded := vbsEscapeLiteral(FirefoxPoliciesJSON)
	return strings.ReplaceAll(firefoxPoliciesDeployTemplate, "__POLICIES_JSON__", encoded)
}

// vbsEscapeLiteral converte una stringa Go in un literal VBScript-safe.
func vbsEscapeLiteral(s string) string {
	// `"` -> `""` (escape interno) ; `\n` -> `" & vbCrLf & "` (concat)
	s = strings.ReplaceAll(s, `"`, `""`)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", `" & vbCrLf & "`)
	return s
}
