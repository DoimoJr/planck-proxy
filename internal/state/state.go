// Package state contiene lo stato condiviso tra proxy, web API e SSE
// broadcaster. Tutte le operazioni sono concurrent-safe via sync.RWMutex.
//
// In Phase 1.3 lo state copre monitor + watchdog: ring buffer del traffico,
// aliveMap.
// In Phase 1.4 si aggiungono i campi config (titolo, modo, ports, auth,
// dominiIgnorati, studenti, ecc.) + le funzioni Snapshot* per servire le
// API GET di lettura.
//
// In Phase 1.5 arriveranno le mutazioni; in 1.6 la persistenza su disco.
//
// Architectural note: l'interfaccia Broker e' definita qui (non importata
// da web) per evitare cicli di import (state -> web -> state).
package state

import (
	"log"
	"sort"
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
// In 1.4 e' un oggetto stub (la persistenza su disco arriva in 1.6).
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
// funzionamento di base dello studente (localhost, wpad, captive portal
// detection, OCSP). Senza questi, in modalita' allowlist o pausa,
// il browser studente si pianta.
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

	// --- Config (settabile via /api/settings/update in 1.5, persistita in 1.6) ---
	titolo              string
	classe              string
	modo                string // "blocklist" | "allowlist"
	inattivitaSogliaSec int
	proxyPort           int // boot-only (richiede restart se cambiato)
	webPort             int // boot-only
	authEnabled         bool
	authUser            string
	authPasswordHash    string // hash bcrypt; vuoto = nessuna password impostata

	// --- Liste ---
	bloccati       map[string]struct{}
	dominiIgnorati []string // ordered slice (non set) per output stabile
	studenti       map[string]string

	// --- Runtime traffico ---
	storia   []Entry
	aliveMap map[string]int64

	// --- Sessione (lifecycle completo in 1.4-1.6) ---
	sessioneAttiva  bool
	sessioneInizio  string // RFC3339
	sessioneFineISO string

	// --- Stato globale runtime ---
	pausato     bool
	deadlineISO string
}

// New costruisce uno State con default sensati.
// I default vengono usati al primo boot quando non c'e' ancora persistenza.
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
// Mutazioni runtime (chiamate dal proxy)
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

// ============================================================
// Snapshot per le API GET (Phase 1.4)
// ============================================================

// ConfigSnapshot e' il payload di /api/config: dati richiesti dal client al boot
// per l'idratazione iniziale. Include le liste DominiAI/PatternSistema esposte
// via classify, e gli stub di sessioniArchivio/presets/classi (vuoti in 1.4,
// popolati in 1.6 dalla lettura disco).
type ConfigSnapshot struct {
	Titolo              string            `json:"titolo"`
	Classe              string            `json:"classe"`
	Modo                string            `json:"modo"`
	InattivitaSogliaSec int               `json:"inattivitaSogliaSec"`
	DominiAI            []string          `json:"dominiAI"`
	PatternSistema      []string          `json:"patternSistema"`
	Studenti            map[string]string `json:"studenti"`
	Presets             []string          `json:"presets"`
	Classi              []Combo           `json:"classi"`
}

// ConfigSnapshotData ritorna il payload per /api/config.
func (s *State) ConfigSnapshotData() ConfigSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	studCopy := make(map[string]string, len(s.studenti))
	for k, v := range s.studenti {
		studCopy[k] = v
	}
	return ConfigSnapshot{
		Titolo:              s.titolo,
		Classe:              s.classe,
		Modo:                s.modo,
		InattivitaSogliaSec: s.inattivitaSogliaSec,
		DominiAI:            classify.DominiAI,
		PatternSistema:      classify.PatternSistema,
		Studenti:            studCopy,
		Presets:             []string{}, // populated in 1.6
		Classi:              []Combo{},  // populated in 1.6
	}
}

// HistorySnapshot e' il payload di /api/history: stato corrente per l'idratazione
// dopo la connessione del client (entries, alive map, blocklist, sessione, deadline).
type HistorySnapshot struct {
	Entries         []Entry          `json:"entries"`
	Bloccati        []string         `json:"bloccati"`
	SessioneAttiva  bool             `json:"sessioneAttiva"`
	SessioneInizio  string           `json:"sessioneInizio,omitempty"`
	SessioneFineISO string           `json:"sessioneFineISO,omitempty"`
	Pausato         bool             `json:"pausato"`
	DeadlineISO     string           `json:"deadlineISO,omitempty"`
	Alive           map[string]int64 `json:"alive"`
}

// HistorySnapshotData ritorna il payload per /api/history.
func (s *State) HistorySnapshotData() HistorySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	storiaCopy := make([]Entry, len(s.storia))
	copy(storiaCopy, s.storia)
	aliveCopy := make(map[string]int64, len(s.aliveMap))
	for k, v := range s.aliveMap {
		aliveCopy[k] = v
	}
	return HistorySnapshot{
		Entries:         storiaCopy,
		Bloccati:        s.bloccatiSortedLocked(),
		SessioneAttiva:  s.sessioneAttiva,
		SessioneInizio:  s.sessioneInizio,
		SessioneFineISO: s.sessioneFineISO,
		Pausato:         s.pausato,
		DeadlineISO:     s.deadlineISO,
		Alive:           aliveCopy,
	}
}

// SessionStatus e' il payload di /api/session/status.
type SessionStatus struct {
	SessioneAttiva  bool     `json:"sessioneAttiva"`
	SessioneInizio  string   `json:"sessioneInizio,omitempty"`
	SessioneFineISO string   `json:"sessioneFineISO,omitempty"`
	DurataSec       int64    `json:"durataSec"`
	Richieste       int      `json:"richieste"`
	Bloccati        []string `json:"bloccati"`
	Pausato         bool     `json:"pausato"`
	DeadlineISO     string   `json:"deadlineISO,omitempty"`
}

// SessionStatusData ritorna il payload per /api/session/status.
// Calcola la durata: se sessione attiva, e' (now - inizio); se ferma con
// `sessioneFineISO`, e' (fine - inizio); altrimenti 0.
func (s *State) SessionStatusData() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var durataSec int64 = 0
	if s.sessioneInizio != "" {
		if inizio, err := time.Parse(time.RFC3339, s.sessioneInizio); err == nil {
			var fine time.Time
			switch {
			case s.sessioneAttiva:
				fine = time.Now().UTC()
			case s.sessioneFineISO != "":
				if t, err := time.Parse(time.RFC3339, s.sessioneFineISO); err == nil {
					fine = t
				} else {
					fine = inizio
				}
			default:
				fine = inizio
			}
			d := fine.Sub(inizio)
			if d < 0 {
				d = 0
			}
			durataSec = int64(d.Seconds())
		}
	}

	return SessionStatus{
		SessioneAttiva:  s.sessioneAttiva,
		SessioneInizio:  s.sessioneInizio,
		SessioneFineISO: s.sessioneFineISO,
		DurataSec:       durataSec,
		Richieste:       len(s.storia),
		Bloccati:        s.bloccatiSortedLocked(),
		Pausato:         s.pausato,
		DeadlineISO:     s.deadlineISO,
	}
}

// SettingsSnapshot e' il payload di /api/settings: tutti i campi settabili
// dalla UI nella tab Impostazioni. La password reale non esce mai
// (sostituita da `passwordSet: bool`, vedi SPEC §7.2.4).
type SettingsSnapshot struct {
	Proxy               ProxySettings `json:"proxy"`
	Web                 WebSettings   `json:"web"`
	Modo                string        `json:"modo"`
	Titolo              string        `json:"titolo"`
	Classe              string        `json:"classe"`
	InattivitaSogliaSec int           `json:"inattivitaSogliaSec"`
	DominiIgnorati      []string      `json:"dominiIgnorati"`
}

type ProxySettings struct {
	Port int `json:"port"`
}

type WebSettings struct {
	Port int     `json:"port"`
	Auth WebAuth `json:"auth"`
}

type WebAuth struct {
	Enabled     bool   `json:"enabled"`
	User        string `json:"user"`
	Password    string `json:"password"`    // sempre stringa vuota in output
	PasswordSet bool   `json:"passwordSet"` // true se hash != ""
}

// SettingsSnapshotData ritorna il payload per /api/settings con la password
// mascherata (mai serializzata).
func (s *State) SettingsSnapshotData() SettingsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ignoratiCopy := make([]string, len(s.dominiIgnorati))
	copy(ignoratiCopy, s.dominiIgnorati)
	return SettingsSnapshot{
		Proxy: ProxySettings{Port: s.proxyPort},
		Web: WebSettings{
			Port: s.webPort,
			Auth: WebAuth{
				Enabled:     s.authEnabled,
				User:        s.authUser,
				Password:    "",
				PasswordSet: s.authPasswordHash != "",
			},
		},
		Modo:                s.modo,
		Titolo:              s.titolo,
		Classe:              s.classe,
		InattivitaSogliaSec: s.inattivitaSogliaSec,
		DominiIgnorati:      ignoratiCopy,
	}
}

// ============================================================
// Snapshot di base (gia' usate da 1.3)
// ============================================================

// SnapshotStoria ritorna una copia indipendente del ring buffer.
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
func (s *State) SessioneStato() (attiva bool, inizio, fine string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessioneAttiva, s.sessioneInizio, s.sessioneFineISO
}

// bloccatiSortedLocked ritorna la blocklist come slice ordinato.
// **Deve essere chiamato col lock gia' tenuto.**
func (s *State) bloccatiSortedLocked() []string {
	out := make([]string, 0, len(s.bloccati))
	for d := range s.bloccati {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
