package state

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
)

// SetWatchdogRegistry collega un Registry watchdog allo state. Va
// chiamato a boot da main.go dopo aver registrato i plugin built-in.
func (s *State) SetWatchdogRegistry(r *watchdog.Registry) {
	s.mu.Lock()
	s.watchdogReg = r
	s.mu.Unlock()
}

// WatchdogRegistry ritorna il Registry corrente (nil se non settato).
func (s *State) WatchdogRegistry() *watchdog.Registry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.watchdogReg
}

// RegistraWatchdogEvent valida + persiste + broadcasta un evento
// watchdog ricevuto via /api/watchdog/event. Il timestamp e' assegnato
// dal server (per coerenza con l'orologio Planck).
//
// Errore se il plugin e' sconosciuto o il payload non passa la validation.
func (s *State) RegistraWatchdogEvent(plugin, ip string, payload map[string]any) error {
	reg := s.WatchdogRegistry()
	if reg == nil {
		return fmt.Errorf("watchdog: registry non inizializzato")
	}
	p, ok := reg.Get(plugin)
	if !ok {
		return fmt.Errorf("watchdog: plugin %q sconosciuto", plugin)
	}
	if err := p.ValidateEvent(payload); err != nil {
		return fmt.Errorf("watchdog: %s evento invalido: %w", plugin, err)
	}

	severity := string(p.Severity(payload))
	now := time.Now().UnixMilli()

	s.mu.RLock()
	sessID := s.sessioneID
	nome := s.studenti[ip]
	s.mu.RUnlock()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("watchdog: marshal payload: %w", err)
	}

	id, err := s.store.SaveWatchdogEvent(plugin, ip, nome, sessID, now, severity, payloadJSON)
	if err != nil {
		log.Printf("watchdog: errore save evento: %v", err)
		// continua comunque col broadcast SSE
	}

	// Log human-readable.
	log.Printf("watchdog [%s] %s %s%s: %s",
		plugin, ip,
		condStr(nome != "", nome+" ", ""),
		severity,
		p.FormatEvent(payload))

	// SSE broadcast: la UI mostra il badge sulla card studente + evento
	// nel pannello "Eventi watchdog".
	s.broker.Broadcast(struct {
		Type         string         `json:"type"`
		ID           int64          `json:"id"`
		Plugin       string         `json:"plugin"`
		IP           string         `json:"ip"`
		NomeStudente string         `json:"nomeStudente,omitempty"`
		TS           int64          `json:"ts"`
		Severity     string         `json:"severity"`
		Payload      map[string]any `json:"payload"`
		Format       string         `json:"format"`
	}{
		Type:         "watchdog",
		ID:           id,
		Plugin:       plugin,
		IP:           ip,
		NomeStudente: nome,
		TS:           now,
		Severity:     severity,
		Payload:      payload,
		Format:       p.FormatEvent(payload),
	})

	return nil
}

// ListaWatchdogEvents ritorna gli ultimi N eventi watchdog (filtri
// opzionali plugin/ip). Per la UI tab "Eventi" e per /api/watchdog/events.
func (s *State) ListaWatchdogEvents(plugin, ip string, limit int) ([]store.WatchdogEvent, error) {
	return s.store.ListaWatchdogEvents(plugin, ip, limit)
}

// LoadWatchdogConfig ritorna lo stato di tutti i plugin: combinando
// quelli registrati (Registry) con la loro config persistita (DB).
// Plugin senza riga in watchdog_config sono ritornati come disabled
// con DefaultConfig().
func (s *State) LoadWatchdogConfig() ([]WatchdogConfigEntry, error) {
	reg := s.WatchdogRegistry()
	if reg == nil {
		return nil, fmt.Errorf("watchdog: registry non inizializzato")
	}

	persisted, err := s.store.LoadWatchdogConfig()
	if err != nil {
		return nil, err
	}
	persistedMap := make(map[string]store.WatchdogPluginConfig, len(persisted))
	for _, c := range persisted {
		persistedMap[c.Plugin] = c
	}

	plugins := reg.List()
	out := make([]WatchdogConfigEntry, 0, len(plugins))
	for _, p := range plugins {
		entry := WatchdogConfigEntry{
			ID:          p.ID(),
			Name:        p.Name(),
			Description: p.Description(),
			Enabled:     false,
		}
		if persistedCfg, ok := persistedMap[p.ID()]; ok {
			entry.Enabled = persistedCfg.Enabled
			entry.Config = persistedCfg.Config
		} else {
			defaultCfg, _ := json.Marshal(p.DefaultConfig())
			entry.Config = defaultCfg
		}
		out = append(out, entry)
	}
	return out, nil
}

// SaveWatchdogPluginConfig persiste enable/disable + config di un plugin.
// Errore se il plugin non e' registrato.
func (s *State) SaveWatchdogPluginConfig(plugin string, enabled bool, configJSON []byte) error {
	reg := s.WatchdogRegistry()
	if reg == nil {
		return fmt.Errorf("watchdog: registry non inizializzato")
	}
	if _, ok := reg.Get(plugin); !ok {
		return fmt.Errorf("watchdog: plugin %q sconosciuto", plugin)
	}
	return s.store.SaveWatchdogPluginConfig(plugin, enabled, configJSON)
}

// WatchdogConfigEntry e' la rappresentazione di un plugin per /api/watchdog/config.
type WatchdogConfigEntry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
}

// condStr e' un piccolo helper "ternary".
func condStr(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}
