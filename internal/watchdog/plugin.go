// Package watchdog implementa il framework di plugin per il monitoraggio
// remoto del PC studente (oltre al traffico web). I plugin built-in
// monitorano: dispositivi USB, processi sospetti, cambi di interfaccia
// di rete. L'architettura permette di aggiungere nuovi plugin sia
// server-side (validazione + UI) che client-side (script PowerShell che
// gira sullo studente).
//
// # Modello
//
// Un plugin ha tre componenti:
//
//   - server-side Go: implementa WatchdogPlugin, registrato a boot in main.go
//   - script PowerShell lato studente: serve eventi periodicamente via HTTP
//     POST a /api/watchdog/event
//   - UI Planck: card configurazione + visualizzazione eventi
//
// # Wire format eventi
//
//	POST /api/watchdog/event
//	{
//	    "plugin": "usb",
//	    "ts": 1745920000123,        // ms epoch, opzionale (server fa now())
//	    "payload": { ... plugin-specific }
//	}
//
// # Trust model
//
// Stesso di /_alive: niente auth sull'endpoint /event (LAN trust).
// L'endpoint /config richiede auth (HTTP Basic come gli altri) perche'
// modifica config persistente.
package watchdog

import (
	"fmt"
	"sort"
	"sync"
)

// Severity classifica l'urgenza visiva di un evento. La UI usa questo
// per scegliere il colore del badge (info=grigio, warning=arancione,
// critical=rosso) e l'eventuale beep/notifica.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// WatchdogPlugin e' l'interfaccia che ogni plugin built-in deve
// implementare. Pure: nessuno stato, niente goroutines. Lo stato
// (eventi) vive in internal/state/watchdog.go.
type WatchdogPlugin interface {
	// ID e' lo slug stabile del plugin, lo stesso che appare nei
	// log file Veyon worker e che l'API usa per identificarlo.
	// Es. "usb", "process".
	ID() string

	// Name e' il nome human-readable mostrato nella UI Settings.
	Name() string

	// Description e' un paragrafo che spiega cosa il plugin rileva
	// e quali sono i casi d'uso.
	Description() string

	// DefaultConfig ritorna il valore default della config
	// JSON-serializzabile del plugin (es. denylist process, allowlist
	// USB VID:PID). Usato come baseline al primo enable.
	DefaultConfig() any

	// ValidateEvent ispeziona il payload di un evento in arrivo per
	// verificare che abbia i campi attesi. Errore = evento scartato.
	ValidateEvent(payload map[string]any) error

	// FormatEvent ritorna una stringa human-readable per la UI
	// (lista eventi, tooltip card, log).
	FormatEvent(payload map[string]any) string

	// Severity classifica un evento. La maggior parte dei plugin
	// ritornano una severity costante, ma alcuni potrebbero variare
	// in base al payload (es. processo "cmd.exe" = info, "regedit.exe"
	// = critical).
	Severity(payload map[string]any) Severity
}

// Registry e' il container concurrent-safe dei plugin registrati.
// I plugin vengono aggiunti UNA volta a boot (in main.go), poi sola
// lettura.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]WatchdogPlugin
}

// NewRegistry costruisce un Registry vuoto.
func NewRegistry() *Registry {
	return &Registry{plugins: map[string]WatchdogPlugin{}}
}

// Register aggiunge un plugin al registro. Errore se l'ID e' gia'
// preso.
func (r *Registry) Register(p WatchdogPlugin) error {
	if p == nil {
		return fmt.Errorf("watchdog: plugin nil")
	}
	id := p.ID()
	if id == "" {
		return fmt.Errorf("watchdog: plugin ID vuoto")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[id]; exists {
		return fmt.Errorf("watchdog: plugin %q gia' registrato", id)
	}
	r.plugins[id] = p
	return nil
}

// Get ritorna un plugin per ID, o (nil, false) se non esiste.
func (r *Registry) Get(id string) (WatchdogPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// List ritorna tutti i plugin registrati, ordinati per ID per stabilita'
// in UI.
func (r *Registry) List() []WatchdogPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WatchdogPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return out
}
