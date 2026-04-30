package builtin

import (
	"fmt"

	"github.com/DoimoJr/planck-proxy/internal/watchdog"
)

// NetworkPlugin rileva la comparsa di nuove interfacce di rete sullo
// studente: tipicamente segnale che si sta provando ad aggirare il
// proxy via tethering 4G, hotspot USB, o connessione VPN.
//
// Policy di default: il plugin fa baseline al boot dello studente di
// tutte le interfacce "Up" presenti, e segnala come warning ogni nuova
// interfaccia che appare Up successivamente. Se il pattern del nome
// corrisponde a una "denylist" (Cellular, VPN, TAP, ecc.) la severity
// diventa critical.
type NetworkPlugin struct{}

func (NetworkPlugin) ID() string   { return "network" }
func (NetworkPlugin) Name() string { return "Monitor interfacce di rete" }
func (NetworkPlugin) Description() string {
	return "Avvisa quando appare una nuova interfaccia di rete sullo " +
		"studente (es. tethering USB da telefono, hotspot Wi-Fi, " +
		"client VPN, dongle 4G). Filtra le interfacce gia' presenti " +
		"al boot per non far rumore."
}

// NetworkConfig: pattern (substring case-insensitive) che, quando
// matchati nell'InterfaceDescription, alzano la severity a critical.
type NetworkConfig struct {
	// SuspiciousPatterns sono substring (case-insensitive) cercate nel
	// nome dell'interfaccia per alzare la severity. Ogni nuova
	// interfaccia non in baseline e' comunque warning.
	SuspiciousPatterns []string `json:"suspiciousPatterns"`
	// IgnorePatterns sono substring per le interfacce da SKIPPARE
	// completamente (mai allarmare). Utile per device sempre presenti
	// che cambiano descrizione (es. update driver).
	IgnorePatterns []string `json:"ignorePatterns"`
}

func (NetworkPlugin) DefaultConfig() any {
	return NetworkConfig{
		SuspiciousPatterns: []string{
			"VPN", "TAP", "TUN", "WireGuard", "OpenVPN",
			"Cellular", "Mobile", "Broadband",
			"USB", "Hotspot", "Tether",
			"PAN", // Bluetooth PAN
		},
		IgnorePatterns: []string{
			"Loopback",
			"Hyper-V",
			"VirtualBox Host-Only", // dev a casa, non da rimuovere ma neppure da segnalare
		},
	}
}

// ValidateEvent: payload deve avere {action, name}. Optional:
// description, mediaType, mac.
func (NetworkPlugin) ValidateEvent(payload map[string]any) error {
	action, _ := payload["action"].(string)
	if action != "added" && action != "removed" {
		return fmt.Errorf("action deve essere 'added' o 'removed' (got %q)", action)
	}
	if _, ok := payload["name"].(string); !ok {
		return fmt.Errorf("name richiesto (string)")
	}
	return nil
}

func (NetworkPlugin) FormatEvent(payload map[string]any) string {
	action, _ := payload["action"].(string)
	name, _ := payload["name"].(string)
	desc, _ := payload["description"].(string)
	verb := "comparsa"
	if action == "removed" {
		verb = "scomparsa"
	}
	if desc != "" && desc != name {
		return fmt.Sprintf("Interfaccia %s: %s (%s)", verb, name, desc)
	}
	return fmt.Sprintf("Interfaccia %s: %s", verb, name)
}

// Severity: "added" critical se il nome match una pattern sospetta,
// altrimenti warning. "removed" e' info (interfaccia scomparsa, di
// solito non e' un problema).
func (NetworkPlugin) Severity(payload map[string]any) watchdog.Severity {
	action, _ := payload["action"].(string)
	if action == "removed" {
		return watchdog.SeverityInfo
	}
	suspicious, _ := payload["suspicious"].(bool)
	if suspicious {
		return watchdog.SeverityCritical
	}
	return watchdog.SeverityWarning
}
