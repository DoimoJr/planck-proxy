package state

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
)

// Soglie heartbeat watchdog (Phase 5.x)
const (
	// HeartbeatTimeout: dopo questo tempo senza heartbeat di un plugin
	// ENABLED, se l'IP e' ancora "alive" (proxy ping), Planck
	// considera lo studente abbia killato il watchdog.
	HeartbeatTimeout = 90 * time.Second
	// HeartbeatCheckInterval: periodo del controllo server-side.
	HeartbeatCheckInterval = 30 * time.Second
)

// SetWatchdogRegistry collega un Registry watchdog allo state. Va
// chiamato a boot da main.go dopo aver registrato i plugin built-in.
// Avvia anche la goroutine di check heartbeat (passa silenziosamente
// se gia' avviata).
func (s *State) SetWatchdogRegistry(r *watchdog.Registry) {
	s.mu.Lock()
	if s.watchdogReg == nil {
		// Avvia check heartbeat solo al primo set (idempotente).
		go s.heartbeatChecker()
	}
	s.watchdogReg = r
	s.mu.Unlock()
}

// RegistraWatchdogHeartbeat aggiorna il timestamp lastSeen di
// (ip, plugin). Chiamato dall'endpoint /api/watchdog/heartbeat
// quando uno script .ps1 sullo studente segnala "sono ancora vivo".
//
// Se questo era un IP "stopped" (abbiamo emesso un evento
// watchdog_stopped in passato e ora il watchdog torna), emette
// un evento "watchdog_resumed".
func (s *State) RegistraWatchdogHeartbeat(ip, plugin string) {
	now := time.Now().UnixMilli()
	wasStopped := false

	s.mu.Lock()
	// Aggiorna lastSeen.
	if s.watchdogHeartbeats[ip] == nil {
		s.watchdogHeartbeats[ip] = map[string]int64{}
	}
	s.watchdogHeartbeats[ip][plugin] = now
	// Se era marcato stopped, sblocca + emetteremo resumed.
	if s.watchdogStoppedAlerted[ip] != nil && s.watchdogStoppedAlerted[ip][plugin] {
		wasStopped = true
		delete(s.watchdogStoppedAlerted[ip], plugin)
	}
	nome := s.studenti[ip]
	sessID := s.sessioneID
	s.mu.Unlock()

	if wasStopped {
		s.emitWatchdogMetaEvent(ip, plugin, nome, sessID, "resumed", now,
			"info", "Watchdog "+plugin+" ricomparso (era silente)")
	}
}

// heartbeatChecker e' una goroutine eseguita dopo SetWatchdogRegistry.
// Ogni HeartbeatCheckInterval, guarda tutte le tuple (ip, plugin) note
// e, per quelle silenti da piu' di HeartbeatTimeout AND con IP che ha
// ricevuto un ping recente nell'aliveMap (= studente online),
// emette un evento "watchdog_stopped" per ciascuna (una sola volta
// per silenziamento, fino al prossimo heartbeat).
func (s *State) heartbeatChecker() {
	ticker := time.NewTicker(HeartbeatCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.checkHeartbeats()
	}
}

// checkHeartbeats e' la logica chiamata dal ticker. Esposta per il test.
func (s *State) checkHeartbeats() {
	now := time.Now().UnixMilli()
	cutoff := now - HeartbeatTimeout.Milliseconds()
	aliveCutoff := now - HeartbeatTimeout.Milliseconds() // stessa soglia per "studente attivo"

	type alert struct {
		ip, plugin, nome string
		sessID, ts       int64
	}
	var alerts []alert

	s.mu.Lock()
	for ip, plugins := range s.watchdogHeartbeats {
		// Skip se l'IP non e' "alive" recente (studente offline ->
		// watchdog assente e' atteso).
		if s.aliveMap[ip] < aliveCutoff {
			continue
		}
		for plugin, lastSeen := range plugins {
			if lastSeen >= cutoff {
				continue // ancora vivo
			}
			// Skip se gia' alertato per questo plugin.
			if s.watchdogStoppedAlerted[ip] != nil && s.watchdogStoppedAlerted[ip][plugin] {
				continue
			}
			// Marca come alertato.
			if s.watchdogStoppedAlerted[ip] == nil {
				s.watchdogStoppedAlerted[ip] = map[string]bool{}
			}
			s.watchdogStoppedAlerted[ip][plugin] = true
			alerts = append(alerts, alert{
				ip:     ip,
				plugin: plugin,
				nome:   s.studenti[ip],
				sessID: s.sessioneID,
				ts:     now,
			})
		}
	}
	s.mu.Unlock()

	for _, a := range alerts {
		s.emitWatchdogMetaEvent(a.ip, a.plugin, a.nome, a.sessID,
			"stopped", a.ts, "warning",
			"Watchdog "+a.plugin+" silente da >"+
				HeartbeatTimeout.String()+" (probabilmente killato)")
	}
}

// emitWatchdogMetaEvent emette un evento "meta" (watchdog stopped/resumed)
// che NON passa per la validation di un plugin specifico ma viene
// persistito + broadcastato come evento normale. Severity preimpostata.
func (s *State) emitWatchdogMetaEvent(ip, plugin, nome string, sessID int64, action string, ts int64, severity, format string) {
	payload := map[string]any{
		"action": action,
		"plugin": plugin,
	}
	payloadJSON, _ := json.Marshal(payload)

	id, err := s.store.SaveWatchdogEvent("watchdog-"+plugin, ip, nome, sessID, ts, severity, payloadJSON)
	if err != nil {
		log.Printf("watchdog: errore save meta evento: %v", err)
	}

	log.Printf("watchdog [META] %s %s: %s", ip, action, format)

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
		Plugin:       "watchdog-" + plugin, // distinguibile da eventi reali del plugin
		IP:           ip,
		NomeStudente: nome,
		TS:           ts,
		Severity:     severity,
		Payload:      payload,
		Format:       format,
	})

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
