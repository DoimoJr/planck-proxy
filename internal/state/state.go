// Package state contiene lo stato condiviso tra proxy, web API e SSE
// broadcaster. Tutte le operazioni sono concurrent-safe via sync.RWMutex.
//
// In Phase 1.3 lo state copre solo monitor + watchdog: ring buffer del
// traffico, aliveMap, e dei flag stub per sessione/pausa/modo che le API
// in 1.4 inizieranno a mutare. La persistenza su disco arriva in 1.6.
//
// Architectural note: l'interfaccia Broker e' definita qui (non importata
// da web) per evitare cicli di import (state -> web -> state).
package state

import (
	"log"
	"sync"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// Entry rappresenta una richiesta loggata dal proxy.
//
// `Ora` e' la stringa "YYYY-MM-DD HH:MM:SS" UTC pensata per display
// umano nel UI (retrocompatibile col formato v1). `TS` e' unix ms per
// query temporali rapide (sara' usato in Phase 4 con SQLite).
type Entry struct {
	Ora     string        `json:"ora"`
	TS      int64         `json:"ts"`
	IP      string        `json:"ip"`
	Metodo  string        `json:"metodo"` // GET, POST, ..., HTTPS (per CONNECT)
	Dominio string        `json:"dominio"`
	Tipo    classify.Tipo `json:"tipo"`
	Blocked bool          `json:"blocked"`
}

// Broker e' l'interfaccia minima che State usa per emettere eventi SSE.
// Implementata da internal/web.Broker. Definita qui per disaccoppiamento.
type Broker interface {
	Broadcast(msg any)
}

// MaxStoria e' il cap del ring buffer in RAM.
//
// In Phase 1.3 e' lo stesso valore di v1 (5000). In Phase 1.6 si scendera'
// a ~1000-2000 quando la persistenza passera' a NDJSON e questo buffer
// servira' solo per l'idratazione UI (non per l'archivio).
const MaxStoria = 5000

// State e' lo stato condiviso del processo Planck.
//
// Tutte le mutazioni e i read sono protetti da `mu`. Le snapshot ritornate
// sono copie indipendenti: il caller puo' usarle senza tenere il lock.
type State struct {
	mu     sync.RWMutex
	broker Broker

	// Ring buffer del traffico (capped a MaxStoria).
	storia []Entry

	// Watchdog: ip → ts unix ms dell'ultimo ping ricevuto.
	aliveMap map[string]int64

	// Sessione lifecycle (stub in 1.3, lifecycle completo in 1.4-1.6).
	// In 1.3 questi campi esistono ma non vengono mutati: il monitor live
	// e' sempre attivo a prescindere (vedi SPEC §3.2).
	sessioneAttiva  bool
	sessioneInizio  string
	sessioneFineISO string

	// Flag per le mutazioni 1.4 (stub).
	pausato     bool
	deadlineISO string
	modo        string // "blocklist" | "allowlist"
}

// New costruisce uno State agganciato a un Broker per il broadcast SSE.
func New(broker Broker) *State {
	return &State{
		broker:   broker,
		storia:   make([]Entry, 0, 256),
		aliveMap: make(map[string]int64),
		modo:     "blocklist",
	}
}

// RegistraTraffic accoda una nuova Entry al ring buffer e la broadcasta
// a tutti i client SSE come messaggio `{type:"traffic", entry:...}`.
//
// Il monitor e' SEMPRE attivo: la registrazione non e' gated dalla sessione
// (vedi SPEC §3.2 — separazione Monitor/Sessione).
//
// `blocked` indica se il proxy ha respinto la richiesta con 403. In Phase
// 1.3 e' sempre false (i blocchi arriveranno in 1.4).
func (s *State) RegistraTraffic(ip, metodo, dominio string, blocked bool, tipo classify.Tipo) {
	now := time.Now().UTC()
	entry := Entry{
		Ora:     now.Format("2006-01-02 15:04:05"),
		TS:      now.UnixMilli(),
		IP:      ip,
		Metodo:  metodo,
		Dominio: dominio,
		Tipo:    tipo,
		Blocked: blocked,
	}

	s.mu.Lock()
	s.storia = append(s.storia, entry)
	if len(s.storia) > MaxStoria {
		// Ring buffer drop-oldest: ricreiamo lo slice con l'ultima finestra
		// per liberare la backing array dopo molti append (evita aggregare
		// referenze a entries vecchie indefinitamente).
		s.storia = append(s.storia[:0:0], s.storia[len(s.storia)-MaxStoria:]...)
	}
	s.mu.Unlock()

	// Log su stdout (formato compatibile v1) + broadcast SSE.
	bm := ""
	if blocked {
		bm = " [BLOCKED]"
	}
	log.Printf("%s [%s] %s %s (%s)%s", now.Format("15:04:05"), ip, metodo, dominio, tipo, bm)

	s.broker.Broadcast(struct {
		Type  string `json:"type"`
		Entry Entry  `json:"entry"`
	}{Type: "traffic", Entry: entry})
}

// RegistraAlive aggiorna aliveMap[ip] al timestamp corrente e broadcasta
// `{type:"alive", ip, ts}`. Indipendente dalla sessione.
func (s *State) RegistraAlive(ip string) {
	ts := time.Now().UnixMilli()

	s.mu.Lock()
	s.aliveMap[ip] = ts
	s.mu.Unlock()

	log.Printf("%s [%s] alive", time.Now().UTC().Format("15:04:05"), ip)

	s.broker.Broadcast(struct {
		Type string `json:"type"`
		IP   string `json:"ip"`
		TS   int64  `json:"ts"`
	}{Type: "alive", IP: ip, TS: ts})
}

// SnapshotStoria ritorna una copia indipendente del ring buffer.
// Usata da /api/history per l'idratazione UI (Phase 1.4).
func (s *State) SnapshotStoria() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, len(s.storia))
	copy(out, s.storia)
	return out
}

// SnapshotAlive ritorna una copia indipendente di aliveMap.
func (s *State) SnapshotAlive() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int64, len(s.aliveMap))
	for k, v := range s.aliveMap {
		out[k] = v
	}
	return out
}

// SessioneStato ritorna lo stato sessione corrente in modo atomico.
// Usato dalle API in 1.4 per popolare /api/session/status.
func (s *State) SessioneStato() (attiva bool, inizio, fine string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessioneAttiva, s.sessioneInizio, s.sessioneFineISO
}
