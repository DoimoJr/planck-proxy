// Package state contiene lo stato condiviso tra proxy, web API e SSE
// broadcaster. Tutte le operazioni sono concurrent-safe via sync.RWMutex.
//
// Architettura del package:
//   - state.go      → tipi base (State, Entry, Combo), New, RegistraTraffic, RegistraAlive
//   - snapshots.go  → tipi Snapshot* + metodi *Data() per le API GET (Phase 1.4)
//   - mutations.go  → mutazioni runtime (block, pausa, deadline, sessione, ...) (Phase 1.5)
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
type Entry struct {
	Ora     string        `json:"ora"`
	TS      int64         `json:"ts"`
	IP      string        `json:"ip"`
	Metodo  string        `json:"metodo"`
	Dominio string        `json:"dominio"`
	Tipo    classify.Tipo `json:"tipo"`
	Blocked bool          `json:"blocked"`
}

// Combo identifica una mappa salvata di studenti per coppia (classe, lab).
// Stub in 1.4-1.5; la persistenza su disco arriva in Phase 1.6.
type Combo struct {
	Classe string `json:"classe"`
	Lab    string `json:"lab"`
	File   string `json:"file"`
}

// Broker e' l'interfaccia minima che State usa per emettere eventi SSE.
type Broker interface {
	Broadcast(msg any)
}

// MaxStoria e' il cap del ring buffer in RAM.
const MaxStoria = 5000

// dominiIgnoratiDefault e' la lista minima di domini necessari per il
// funzionamento di base dello studente (localhost, wpad, captive portal,
// OCSP). Senza questi, in modalita' allowlist o pausa, il browser
// studente si pianta.
var dominiIgnoratiDefault = []string{
	"localhost",
	"127.0.0.1",
	"ocsp.digicert.com",
	"ctldl.windowsupdate.com",
	"settings-win.data.microsoft.com",
	"wpad",
	"dns.msftncsi.com",
	"www.msftconnecttest.com",
	"login.live.com",
	"activity.windows.com",
	"edge.microsoft.com",
	"msedge.api.cdp.microsoft.com",
}

// State e' lo stato condiviso del processo Planck.
type State struct {
	mu     sync.RWMutex
	broker Broker

	// --- Config (settabile via /api/settings/update; persistita in 1.6) ---
	titolo              string
	classe              string
	modo                string // "blocklist" | "allowlist"
	inattivitaSogliaSec int
	proxyPort           int // boot-only
	webPort             int // boot-only
	authEnabled         bool
	authUser            string
	authPasswordHash    string // hash bcrypt; vuoto = nessuna password impostata

	// --- Liste ---
	bloccati       map[string]struct{}
	dominiIgnorati []string
	studenti       map[string]string

	// --- Runtime traffico ---
	storia   []Entry
	aliveMap map[string]int64

	// --- Sessione ---
	sessioneAttiva  bool
	sessioneInizio  string // RFC3339
	sessioneFineISO string

	// --- Stato globale runtime ---
	pausato       bool
	deadlineISO   string
	deadlineTimer *time.Timer
}

// New costruisce uno State con default sensati.
func New(broker Broker) *State {
	ignorati := make([]string, len(dominiIgnoratiDefault))
	copy(ignorati, dominiIgnoratiDefault)

	return &State{
		broker:              broker,
		titolo:              "Planck Proxy",
		classe:              "",
		modo:                "blocklist",
		inattivitaSogliaSec: 180,
		proxyPort:           9090,
		webPort:             9999,
		authEnabled:         false,
		authUser:            "docente",
		authPasswordHash:    "",
		bloccati:            map[string]struct{}{},
		dominiIgnorati:      ignorati,
		studenti:            map[string]string{},
		storia:              make([]Entry, 0, 256),
		aliveMap:            map[string]int64{},
	}
}

// ============================================================
// Mutazioni runtime di base (chiamate dal proxy)
// ============================================================

// RegistraTraffic accoda una nuova Entry al ring buffer e la broadcasta.
// Sempre attivo (Monitor sempre on, vedi SPEC §3.2).
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
		s.storia = append(s.storia[:0:0], s.storia[len(s.storia)-MaxStoria:]...)
	}
	s.mu.Unlock()

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

// RegistraAlive aggiorna aliveMap[ip] e broadcasta `{type:"alive",ip,ts}`.
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
