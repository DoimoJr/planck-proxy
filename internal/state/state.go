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
	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
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
	store  *store.Store // mai nil: e' NoOpStore se persistenza disabilitata

	// --- Config (settabile via /api/settings/update; persistita in config.json) ---
	titolo              string
	classe              string
	modo                string // "blocklist" | "allowlist"
	inattivitaSogliaSec int
	proxyPort           int // boot-only
	webPort             int // boot-only
	authEnabled         bool
	authUser            string
	authPasswordHash    string // hash bcrypt; vuoto = nessuna password impostata

	// --- Veyon (Phase 3e) ---
	veyonKeyName string // nome master key (vuoto = Veyon non configurato)
	veyonPort    int    // 0 = default 11100

	// --- Network info esposta in /api/config per "Distribuisci proxy" ---
	lanIP string // IP LAN del docente (auto-detected o env PLANCK_LAN_IP)

	// --- Watchdog plugin registry (Phase 5) ---
	watchdogReg *watchdog.Registry
	// watchdogHeartbeats[ip][pluginID] = ms epoch del piu' recente
	// heartbeat ricevuto. Usato per detectare il "watchdog killato"
	// dallo studente (Phase 5.x).
	watchdogHeartbeats map[string]map[string]int64
	// watchdogStoppedAlerted[ip][pluginID] = true quando abbiamo
	// gia' emesso l'evento "stopped" e stiamo aspettando un
	// nuovo heartbeat per "resumed" (evita flooding).
	watchdogStoppedAlerted map[string]map[string]bool

	// --- Liste ---
	bloccati       map[string]struct{}
	// blocchiPerIp[ip] = set di domini bloccati SOLO per quell'IP
	// (additivi rispetto alla blocklist globale). Persistito su DB.
	blocchiPerIp   map[string]map[string]struct{}
	dominiIgnorati []string
	studenti       map[string]string

	// --- Runtime traffico ---
	storia   []Entry
	aliveMap map[string]int64

	// --- Sessione ---
	sessioneAttiva  bool
	sessioneID      int64  // id riga in store.sessioni quando attiva, 0 se ferma
	sessioneInizio  string // RFC3339
	sessioneFineISO string

	// --- Stato globale runtime ---
	pausato       bool
	deadlineISO   string
	deadlineTimer *time.Timer
}

// New costruisce uno State con default sensati e store NoOp (in-memory).
// Per persistenza usa NewWithStore.
func New(broker Broker) *State {
	return NewWithStore(broker, store.NoOpStore())
}

// NewWithStore costruisce uno State con un Store SQLite.
// Carica dal DB: config (titolo, modo, soglia, porte, auth, ignorati) e
// blocklist. NON carica piu' la mappa studenti ne' la chiave Veyon: sono
// rigenerate ad ogni boot (il binario e' portatile tra laboratori → ogni
// avvio rigenera lo stato dipendente dalla LAN corrente).
func NewWithStore(broker Broker, st *store.Store) *State {
	ignorati := make([]string, len(dominiIgnoratiDefault))
	copy(ignorati, dominiIgnoratiDefault)

	s := &State{
		broker:              broker,
		store:               st,
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
		blocchiPerIp:        map[string]map[string]struct{}{},
		dominiIgnorati:      ignorati,
		studenti:            map[string]string{},
		storia:              make([]Entry, 0, 256),
		aliveMap:               map[string]int64{},
		watchdogHeartbeats:     map[string]map[string]int64{},
		watchdogStoppedAlerted: map[string]map[string]bool{},
	}

	// Carica config persistita (se esiste).
	if cfg, exists, err := st.LoadConfig(); err == nil && exists {
		if cfg.Titolo != "" {
			s.titolo = cfg.Titolo
		}
		s.classe = cfg.Classe
		if cfg.Modo == "blocklist" || cfg.Modo == "allowlist" {
			s.modo = cfg.Modo
		}
		if cfg.InattivitaSogliaSec > 0 {
			s.inattivitaSogliaSec = cfg.InattivitaSogliaSec
		}
		if cfg.ProxyPort > 0 {
			s.proxyPort = cfg.ProxyPort
		}
		if cfg.WebPort > 0 {
			s.webPort = cfg.WebPort
		}
		s.authEnabled = cfg.AuthEnabled
		if cfg.AuthUser != "" {
			s.authUser = cfg.AuthUser
		}
		s.authPasswordHash = cfg.AuthPasswordHash
		if len(cfg.DominiIgnorati) > 0 {
			s.dominiIgnorati = cfg.DominiIgnorati
		}
		// veyonKeyName / studenti NON caricati da DB: rigenerati ad ogni
		// boot (chiave via veyon-cli del PC corrente; mappa via range
		// /24 del LAN IP corrente).
		s.veyonPort = cfg.VeyonPort
	} else if err != nil {
		log.Printf("state: errore lettura config: %v", err)
	}

	// Carica blocklist.
	if blocs, err := st.LoadBloccati(); err == nil {
		for _, d := range blocs {
			s.bloccati[d] = struct{}{}
		}
	} else {
		log.Printf("state: errore lettura blocklist: %v", err)
	}

	// Carica blocchi per-IP.
	if perIp, err := st.LoadBloccatiPerIp(); err == nil {
		for ip, doms := range perIp {
			set := make(map[string]struct{}, len(doms))
			for _, d := range doms {
				set[d] = struct{}{}
			}
			s.blocchiPerIp[ip] = set
		}
	} else {
		log.Printf("state: errore lettura bloccati_per_ip: %v", err)
	}

	return s
}

// saveConfigLocked serializza i campi config correnti su disco.
// **Deve essere chiamato col lock gia' tenuto** (uso da UpdateSettings,
// AddIgnorato, ecc.).
func (s *State) saveConfigLocked() {
	if s.store.Disabled() {
		return
	}
	cfg := store.ConfigFile{
		Titolo:              s.titolo,
		Classe:              s.classe,
		Modo:                s.modo,
		InattivitaSogliaSec: s.inattivitaSogliaSec,
		ProxyPort:           s.proxyPort,
		WebPort:             s.webPort,
		AuthEnabled:         s.authEnabled,
		AuthUser:            s.authUser,
		AuthPasswordHash:    s.authPasswordHash,
		DominiIgnorati:      append([]string{}, s.dominiIgnorati...),
		VeyonPort:           s.veyonPort,
	}
	if err := s.store.SaveConfig(cfg); err != nil {
		log.Printf("state: errore save config: %v", err)
	}
}

// Store esposto per i handler API che hanno bisogno di operare direttamente
// sul DB (es. CRUD presets, listing sessioni).
func (s *State) Store() *store.Store { return s.store }

// SetLanIP imposta il LAN IP del docente, settato a boot da main.go (auto-
// detected o env PLANCK_LAN_IP). Esposto via /api/config per la UI.
func (s *State) SetLanIP(ip string) {
	s.mu.Lock()
	s.lanIP = ip
	s.mu.Unlock()
}

// LanIP ritorna il LAN IP configurato.
func (s *State) LanIP() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lanIP
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
	persistTraffic := s.sessioneAttiva
	sessID := s.sessioneID
	nome := s.studenti[ip]
	s.mu.Unlock()

	bm := ""
	if blocked {
		bm = " [BLOCKED]"
	}
	log.Printf("%s [%s] %s %s (%s)%s", now.Format("15:04:05"), ip, metodo, dominio, tipo, bm)

	// Persistenza: se sessione attiva, append entry come riga in entries.
	// Solo se tipo != sistema (vedi SPEC §3.2: il rumore non finisce
	// nel session log per non gonfiare).
	if persistTraffic && sessID > 0 && tipo != classify.TipoSistema {
		if err := s.store.SessionAppendEntry(
			sessID, entry.TS, entry.Ora, entry.IP, nome,
			entry.Metodo, entry.Dominio, string(entry.Tipo),
			entry.Blocked, false,
		); err != nil {
			log.Printf("state: errore append entry: %v", err)
		}
	}

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
