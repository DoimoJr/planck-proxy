package builtin

import (
	"fmt"

	"github.com/DoimoJr/planck-proxy/internal/watchdog"
)

// ProcessPlugin rileva il lancio di processi sospetti sul PC studente.
// La denylist di default include: cmd, powershell, regedit, taskmgr, ecc.
// (usabili per aggirare il proxy o accedere a tool di sistema).
type ProcessPlugin struct{}

func (ProcessPlugin) ID() string   { return "process" }
func (ProcessPlugin) Name() string { return "Monitor processi sospetti" }
func (ProcessPlugin) Description() string {
	return "Avvisa quando lo studente avvia un processo nella denylist " +
		"configurata (default: cmd.exe, powershell.exe, regedit.exe, " +
		"taskmgr.exe, gpedit.msc, mmc.exe). Polling ogni 5s, diff vs " +
		"snapshot precedente."
}

// ProcessConfig e' la config persistita per il plugin Process.
type ProcessConfig struct {
	// DenyList sono i nomi processo (case-insensitive, .exe opzionale)
	// che generano un evento al primo rilevamento.
	DenyList []string `json:"denyList"`
}

func (ProcessPlugin) DefaultConfig() any {
	return ProcessConfig{
		DenyList: []string{
			"cmd.exe", "powershell.exe", "powershell_ise.exe", "pwsh.exe",
			"regedit.exe", "taskmgr.exe", "mmc.exe", "gpedit.msc",
			"perfmon.exe", "resmon.exe", "msconfig.exe",
			// Firefox in denylist perche' puo' bypassare il proxy di sistema
			// disattivandolo dalle proprie Preferenze (a differenza di
			// Chrome/Edge che ereditano sempre da Windows). Il policies.json
			// distribuito al setup chiude la falla, ma il watchdog process
			// genera comunque un alert quando lo studente lancia Firefox
			// portable (USB) — quel binario non legge la distribution dir.
			"firefox.exe",
		},
	}
}

// ValidateEvent: payload deve avere {action, name}. `path` e `pid`
// opzionali ma utili per UI/audit.
func (ProcessPlugin) ValidateEvent(payload map[string]any) error {
	action, _ := payload["action"].(string)
	if action != "started" && action != "stopped" {
		return fmt.Errorf("action deve essere 'started' o 'stopped' (got %q)", action)
	}
	if _, ok := payload["name"].(string); !ok {
		return fmt.Errorf("name richiesto (string)")
	}
	return nil
}

func (ProcessPlugin) FormatEvent(payload map[string]any) string {
	action, _ := payload["action"].(string)
	name, _ := payload["name"].(string)
	verb := "avviato"
	if action == "stopped" {
		verb = "terminato"
	}
	return fmt.Sprintf("Processo %s: %s", verb, name)
}

// Severity: "started" e' warning (azione attiva), "stopped" info.
func (ProcessPlugin) Severity(payload map[string]any) watchdog.Severity {
	action, _ := payload["action"].(string)
	if action == "started" {
		return watchdog.SeverityWarning
	}
	return watchdog.SeverityInfo
}
