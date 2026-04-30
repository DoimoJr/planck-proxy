// Package builtin contiene i plugin watchdog di default forniti
// con Planck. Ogni plugin va registrato a boot in main.go via
// `state.WatchdogRegistry().Register(builtin.UsbPlugin{})`.
package builtin

import (
	"fmt"

	"github.com/DoimoJr/planck-proxy/internal/watchdog"
)

// UsbPlugin rileva quando uno studente collega un dispositivo USB di
// classe "interessante" (chiavette, telefoni, hard disk esterni). Il
// polling avviene lato studente via un PowerShell script (vedi
// usb_watchdog.ps1) che fa diff vs baseline ogni 5s.
type UsbPlugin struct{}

func (UsbPlugin) ID() string   { return "usb" }
func (UsbPlugin) Name() string { return "Monitor dispositivi USB" }
func (UsbPlugin) Description() string {
	return "Avvisa quando uno studente collega/scollega un dispositivo USB " +
		"di classe non sicura (chiavette, telefoni MTP, hard disk esterni, " +
		"camere). Esclude di default le classi HID, Mouse, Keyboard, Audio " +
		"integrato."
}

// UsbConfig e' la config persistita per il plugin USB.
type UsbConfig struct {
	// IgnoredClasses sono le classi PnP Windows escluse dal monitoring.
	// Default: HID/Mouse/Keyboard/Audio integrati. Modificabile via
	// settings UI per allowlistare device specifici.
	IgnoredClasses []string `json:"ignoredClasses"`

	// AllowVidPid e' un opt-in: VID:PID che il docente vuole considerare
	// "ok" anche se di classe altrimenti allarmante. Es. "1234:5678"
	// per la chiavetta personale dell'insegnante.
	AllowVidPid []string `json:"allowVidPid"`
}

func (UsbPlugin) DefaultConfig() any {
	return UsbConfig{
		IgnoredClasses: []string{
			"HIDClass", "Mouse", "Keyboard",
			"USB",          // root hub
			"Bluetooth",
			"AudioEndpoint", "MEDIA",
			"System", "DiskDrive", // disco interno
			"Battery", "Processor", "Computer",
		},
		AllowVidPid: []string{},
	}
}

// ValidateEvent: il payload deve avere almeno {action, instanceId}.
// `class` e `deviceName` sono opzionali ma utili per la UI.
func (UsbPlugin) ValidateEvent(payload map[string]any) error {
	action, _ := payload["action"].(string)
	if action != "added" && action != "removed" {
		return fmt.Errorf("action deve essere 'added' o 'removed' (got %q)", action)
	}
	if _, ok := payload["instanceId"].(string); !ok {
		return fmt.Errorf("instanceId richiesto (string)")
	}
	return nil
}

func (UsbPlugin) FormatEvent(payload map[string]any) string {
	action, _ := payload["action"].(string)
	name, _ := payload["deviceName"].(string)
	class, _ := payload["class"].(string)
	if name == "" {
		name = payload["instanceId"].(string)
	}
	verb := "collegato"
	if action == "removed" {
		verb = "scollegato"
	}
	if class != "" {
		return fmt.Sprintf("USB %s [%s]: %s", verb, class, name)
	}
	return fmt.Sprintf("USB %s: %s", verb, name)
}

// Severity: "added" e' warning (potenziale rischio); "removed" e' info.
func (UsbPlugin) Severity(payload map[string]any) watchdog.Severity {
	action, _ := payload["action"].(string)
	if action == "added" {
		return watchdog.SeverityWarning
	}
	return watchdog.SeverityInfo
}
