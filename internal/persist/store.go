// Package persist gestisce la persistenza file-based (Phase 1.6).
//
// Layout su disco (relativo a DataDir):
//
//	studenti.json                 mappa IP → nome
//	_blocked_domains.txt          blocklist (un dominio per riga)
//	config.json                   titolo, modo, soglia, ports, auth, dominiIgnorati
//	presets/<nome>.json           snapshot blocklist con metadata
//	classi/<classe>--<lab>.json   mappa salvata per coppia classe+lab
//	sessioni/_corrente.ndjson     entries della sessione attiva (append-only)
//	sessioni/<timestamp>.json     sessioni archiviate (snapshot completo)
//
// Tutte le scritture sono protette da sync.Mutex sul Store. Le letture
// non prendono lock (file-based, le concorrenze rimangono serializzate
// sulle scritture).
//
// In Phase 2 (SQLite) tutta questa persistenza migrera' a un singolo
// planck.db, e questo package verra' rimpiazzato da internal/store con
// API simile ma backend SQL. Per Phase 1.6 e' sufficiente il file-based.
package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store rappresenta un'istanza di persistenza ancorata a una directory.
// Concurrent-safe via sync.Mutex.
//
// Se `disabled` e' true (vedi NoOpStore), tutti i metodi sono no-op e
// ritornano errori nil / valori zero. Usato nei test per disaccoppiare
// dallo stato del filesystem.
type Store struct {
	mu       sync.Mutex
	dataDir  string
	disabled bool
}

// New costruisce un Store per la directory data fornita. Crea le
// sotto-directory necessarie (`presets/`, `classi/`, `sessioni/`) se
// mancanti.
func New(dataDir string) (*Store, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("dataDir non valido: %w", err)
	}
	for _, sub := range []string{"", "presets", "classi", "sessioni"} {
		path := filepath.Join(abs, sub)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %q: %w", path, err)
		}
	}
	return &Store{dataDir: abs}, nil
}

// NoOpStore ritorna un Store disabilitato: tutte le operazioni sono no-op,
// utili per test o configurazioni runtime senza persistenza disco.
func NoOpStore() *Store { return &Store{disabled: true} }

// Disabled riporta true se lo store non scrive su disco.
func (s *Store) Disabled() bool { return s.disabled }

// DataDir ritorna il path assoluto della directory dati. Vuoto se NoOp.
func (s *Store) DataDir() string { return s.dataDir }

// pathFor compone un path assoluto rispetto a dataDir.
func (s *Store) pathFor(parts ...string) string {
	return filepath.Join(append([]string{s.dataDir}, parts...)...)
}

// writeJSONFile scrive `data` come JSON pretty-printed in path atomico
// (write to tmp, rename). Idempotente, sostituisce se esiste.
func writeJSONFile(path string, data any) error {
	tmp := path + ".tmp"
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// readJSONFile carica un file JSON in `out`. Se il file non esiste,
// ritorna (false, nil) — il caller usa default. Errore solo per problemi
// di parsing o I/O reale.
func readJSONFile(path string, out any) (exists bool, err error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %q: %w", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return true, fmt.Errorf("parse %q: %w", path, err)
	}
	return true, nil
}

// ============================================================
// studenti.json
// ============================================================

// LoadStudenti carica la mappa dal file. Se non esiste, ritorna mappa vuota.
// Le chiavi che iniziano per "_" sono trattate come commenti (convenzione
// di v1) e droppate.
func (s *Store) LoadStudenti() (map[string]string, error) {
	if s.disabled {
		return map[string]string{}, nil
	}
	var raw map[string]string
	exists, err := readJSONFile(s.pathFor("studenti.json"), &raw)
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if !strings.HasPrefix(k, "_") {
			out[k] = v
		}
	}
	return out, nil
}

// SaveStudenti scrive la mappa su disco (overwrite atomico).
func (s *Store) SaveStudenti(m map[string]string) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.pathFor("studenti.json"), m)
}

// ============================================================
// _blocked_domains.txt (uno per riga)
// ============================================================

// LoadBloccati legge la blocklist da file. Righe vuote / con solo
// whitespace sono ignorate. Restituisce slice vuoto se file non esiste.
func (s *Store) LoadBloccati() ([]string, error) {
	if s.disabled {
		return []string{}, nil
	}
	body, err := os.ReadFile(s.pathFor("_blocked_domains.txt"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read blocklist: %w", err)
	}
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// SaveBloccati scrive la blocklist su disco. Lista ordinata per stabilita'.
func (s *Store) SaveBloccati(list []string) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sorted := make([]string, len(list))
	copy(sorted, list)
	sort.Strings(sorted)
	body := strings.Join(sorted, "\n")
	if body != "" {
		body += "\n"
	}
	tmp := s.pathFor("_blocked_domains.txt.tmp")
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.pathFor("_blocked_domains.txt"))
}

// ============================================================
// config.json
// ============================================================

// ConfigFile e' il payload serializzato di config.json. Usato da state
// per popolare i suoi campi al boot e ri-scriverli ad ogni mutazione
// rilevante (settings, dominiIgnorati).
type ConfigFile struct {
	Titolo              string   `json:"titolo"`
	Classe              string   `json:"classe"`
	Modo                string   `json:"modo"`
	InattivitaSogliaSec int      `json:"inattivitaSogliaSec"`
	ProxyPort           int      `json:"proxyPort"`
	WebPort             int      `json:"webPort"`
	AuthEnabled         bool     `json:"authEnabled"`
	AuthUser            string   `json:"authUser"`
	AuthPasswordHash    string   `json:"authPasswordHash"`
	DominiIgnorati      []string `json:"dominiIgnorati"`
}

// LoadConfig legge config.json. Se non esiste, ritorna (zero, false, nil)
// e il caller usa i default.
func (s *Store) LoadConfig() (cfg ConfigFile, exists bool, err error) {
	if s.disabled {
		return ConfigFile{}, false, nil
	}
	exists, err = readJSONFile(s.pathFor("config.json"), &cfg)
	return cfg, exists, err
}

// SaveConfig sovrascrive config.json.
func (s *Store) SaveConfig(c ConfigFile) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.pathFor("config.json"), c)
}

// ============================================================
// Sanitization
// ============================================================

// sanitizeName valida nomi usati come segmenti di filename: accetta solo
// alfanumerici, underscore, trattino. Restituisce stringa vuota se invalida.
func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			out = append(out, r)
		}
	}
	return string(out)
}

// sanitizeArchiveName accetta anche '.' (per ".json" suffix). Vincolo:
// deve terminare con .json.
func sanitizeArchiveName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			out = append(out, r)
		}
	}
	clean := string(out)
	if strings.HasSuffix(clean, ".json") {
		return clean
	}
	return ""
}

// ErrNomeInvalido viene ritornato quando un nome (preset, classe, lab,
// archivio sessione) non passa la sanitizzazione.
var ErrNomeInvalido = fmt.Errorf("nome invalido (caratteri ammessi: lettere, cifre, _, -)")
