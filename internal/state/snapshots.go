package state

import (
	"sort"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// ============================================================
// Snapshot per le API GET (Phase 1.4)
// ============================================================

// ConfigSnapshot e' il payload di /api/config: dati richiesti dal client
// al boot per l'idratazione iniziale.
type ConfigSnapshot struct {
	Titolo              string            `json:"titolo"`
	Classe              string            `json:"classe"`
	Modo                string            `json:"modo"`
	InattivitaSogliaSec int               `json:"inattivitaSogliaSec"`
	DominiAI            []string          `json:"dominiAI"`
	PatternSistema      []string          `json:"patternSistema"`
	Studenti            map[string]string `json:"studenti"`
	Presets             []string          `json:"presets"`
	// LanIP e' l'IP del PC docente che gli studenti usano per raggiungere
	// Planck (set a boot via PLANCK_LAN_IP o auto-detected). Stesso valore
	// embeddato nel proxy_on.bat. La UI lo usa per "Distribuisci" senza
	// dover chiedere all'utente.
	LanIP string `json:"lanIP"`
}

// ConfigSnapshotData ritorna il payload per /api/config.
// In Phase 1.6 legge presets e classi dal Store (su disco).
func (s *State) ConfigSnapshotData() ConfigSnapshot {
	s.mu.RLock()
	studCopy := make(map[string]string, len(s.studenti))
	for k, v := range s.studenti {
		studCopy[k] = v
	}
	snap := ConfigSnapshot{
		Titolo:              s.titolo,
		Classe:              s.classe,
		Modo:                s.modo,
		InattivitaSogliaSec: s.inattivitaSogliaSec,
		DominiAI:            classify.AIDomains(),
		PatternSistema:      classify.PatternSistema,
		Studenti:            studCopy,
		Presets:             []string{},
		LanIP:               s.lanIP,
	}
	s.mu.RUnlock()

	// Letture disco fuori dal lock (le file ops sono lente).
	if presets, err := s.store.ListaPresets(); err == nil {
		snap.Presets = presets
	}
	return snap
}

// HistorySnapshot e' il payload di /api/history per l'idratazione UI.
type HistorySnapshot struct {
	Entries         []Entry             `json:"entries"`
	Bloccati        []string            `json:"bloccati"`
	BlocchiPerIp    map[string][]string `json:"blocchiPerIp"`
	SessioneAttiva  bool                `json:"sessioneAttiva"`
	SessioneInizio  string              `json:"sessioneInizio,omitempty"`
	SessioneFineISO string              `json:"sessioneFineISO,omitempty"`
	Pausato         bool                `json:"pausato"`
	DeadlineISO     string              `json:"deadlineISO,omitempty"`
	Alive           map[string]int64    `json:"alive"`
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
		BlocchiPerIp:    s.blocchiPerIpSnapshotLocked(),
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
// Calcola la durata: attiva → (now - inizio); ferma con `sessioneFineISO`
// → (fine - inizio); altrimenti 0.
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

// SettingsSnapshot e' il payload di /api/settings con password mascherata.
type SettingsSnapshot struct {
	Proxy               ProxySettings `json:"proxy"`
	Web                 WebSettings   `json:"web"`
	Modo                string        `json:"modo"`
	Titolo              string        `json:"titolo"`
	Classe              string        `json:"classe"`
	InattivitaSogliaSec int           `json:"inattivitaSogliaSec"`
	DominiIgnorati      []string      `json:"dominiIgnorati"`
	DiscoverVeyonOnly   bool          `json:"discoverVeyonOnly"`
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
	Password    string `json:"password"`    // sempre vuota in output
	PasswordSet bool   `json:"passwordSet"` // true se hash != ""
}

// SettingsSnapshotData ritorna il payload di /api/settings.
func (s *State) SettingsSnapshotData() SettingsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settingsSnapshotLocked()
}

// settingsSnapshotLocked costruisce il SettingsSnapshot col lock gia' tenuto.
// Estratta come helper perche' usata sia da SettingsSnapshotData (lock proprio)
// sia da UpdateSettings (broadcast post-mutation con lock gia' tenuto).
func (s *State) settingsSnapshotLocked() SettingsSnapshot {
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
		DiscoverVeyonOnly:   s.discoverVeyonOnly,
	}
}

// ============================================================
// Snapshot di base
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

// SessioneStato ritorna lo stato sessione in modo atomico.
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
