package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// utf8BOM e' la sequenza di byte UTF-8 BOM. Notepad su Windows aggiunge
// il BOM ai file `Save As UTF-8`, e il decoder JSON di Go non lo skippa.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// MigrateFromFiles importa i dati persistiti dal layout file-based di
// Phase 1.6 (`config.json`, `studenti.json`, `_blocked_domains.txt`,
// `presets/`, `classi/`, `sessioni/`) nel database SQLite, rinominando
// poi i file legacy come `*.v1.bak` cosi' al prossimo boot non vengono
// re-importati.
//
// E' una funzione **idempotente per data**: se chiamata su un DB gia'
// popolato, sovrascrive le righe matchate (INSERT OR REPLACE su molte
// tabelle), e i file rinominati `.v1.bak` non vengono toccati.
//
// Best-effort: errori su singoli file sono loggati ma non bloccano il
// processo. Il caller deve loggare a sua volta il risultato globale.
func (s *Store) MigrateFromFiles(filesDir string) (imported []string, err error) {
	if s.disabled {
		return nil, nil
	}

	// Marker per non ri-eseguire l'import (idempotenza forte): se la
	// chiave kv "migrated_from_files" e' impostata, skip.
	if v, _ := s.kvGetString("migrated_from_files"); v != "" {
		return nil, nil
	}

	// 1) config.json
	if cfg, ok := readJSONFile[v1Config](filepath.Join(filesDir, "config.json")); ok {
		if err := s.SaveConfig(ConfigFile{
			Titolo:              cfg.Titolo,
			Classe:              cfg.Classe,
			Modo:                cfg.Modo,
			InattivitaSogliaSec: cfg.InattivitaSogliaSec,
			ProxyPort:           cfg.ProxyPort,
			WebPort:             cfg.WebPort,
			AuthEnabled:         cfg.AuthEnabled,
			AuthUser:            cfg.AuthUser,
			AuthPasswordHash:    cfg.AuthPasswordHash,
			DominiIgnorati:      cfg.DominiIgnorati,
		}); err != nil {
			log.Printf("migrate v1 config: %v", err)
		} else {
			imported = append(imported, "config.json")
			renameToBak(filepath.Join(filesDir, "config.json"))
		}
	}

	// 2) studenti.json — rimosso in v2.6.0 (mappa studenti non e' piu'
	//    persistita: rigenerata ad ogni boot dal /24 corrente).

	// 3) _blocked_domains.txt
	if body, err := os.ReadFile(filepath.Join(filesDir, "_blocked_domains.txt")); err == nil {
		body = bytes.TrimPrefix(body, utf8BOM)
		var lines []string
		for _, l := range strings.Split(string(body), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, l)
			}
		}
		if err := s.SaveBloccati(lines); err != nil {
			log.Printf("migrate v1 bloccati: %v", err)
		} else {
			imported = append(imported, "_blocked_domains.txt")
			renameToBak(filepath.Join(filesDir, "_blocked_domains.txt"))
		}
	}

	// 4) presets/*.json
	presetEntries, _ := os.ReadDir(filepath.Join(filesDir, "presets"))
	for _, e := range presetEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(filesDir, "presets", e.Name())
		if p, ok := readJSONFile[v1Preset](path); ok {
			if err := s.SavePreset(PresetFile{
				Nome:        p.Nome,
				Descrizione: p.Descrizione,
				Domini:      p.Domini,
				CreatedAt:   p.CreatedAt,
			}); err != nil {
				log.Printf("migrate v1 preset %s: %v", e.Name(), err)
			} else {
				imported = append(imported, "presets/"+e.Name())
				renameToBak(path)
			}
		}
	}

	// 5) classi/*.json — rimosso in v2.6.0 (concetto "classe/laboratorio
	//    salvato" eliminato: ogni boot e' un laboratorio diverso).

	// 6) sessioni/*.json (snapshot archiviati)
	// Il formato v1 ha entries come array di oggetti completi; li importiamo
	// preservando il timestamp originale.
	sessEntries, _ := os.ReadDir(filepath.Join(filesDir, "sessioni"))
	sort.Slice(sessEntries, func(i, j int) bool {
		return sessEntries[i].Name() < sessEntries[j].Name()
	})
	for _, e := range sessEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(filesDir, "sessioni", e.Name())
		if a, ok := readJSONFile[v1Archive](path); ok {
			if err := s.importV1Archive(a); err != nil {
				log.Printf("migrate v1 sessione %s: %v", e.Name(), err)
			} else {
				imported = append(imported, "sessioni/"+e.Name())
				renameToBak(path)
			}
		}
	}

	// Marca la migrazione come completata.
	if err := s.kvSetString("migrated_from_files", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return imported, fmt.Errorf("set migration marker: %w", err)
	}
	return imported, nil
}

// importV1Archive inserisce una sessione archiviata (header + entries) nel DB.
func (s *Store) importV1Archive(a v1Archive) error {
	// Apri la sessione con metadata snapshot.
	id, err := s.SessionStart(SessionMeta{
		SessioneInizio: a.SessioneInizio,
		Titolo:         a.Titolo,
		Classe:         a.Classe,
		Modo:           a.Modo,
		Studenti:       a.Studenti,
		Bloccati:       a.Bloccati,
	})
	if err != nil {
		return fmt.Errorf("session start: %w", err)
	}
	// Inserisci ogni entry preservando i campi.
	for _, raw := range a.Entries {
		var e v1Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		if err := s.SessionAppendEntry(id, e.TS, e.Ora, e.IP, "", e.Metodo, e.Dominio, e.Tipo, e.Blocked, false); err != nil {
			return fmt.Errorf("append entry: %w", err)
		}
	}
	// Chiudi la sessione con SessioneFineISO se presente, altrimenti usa Inizio.
	fineISO := a.SessioneFineISO
	if fineISO == "" {
		fineISO = a.EsportatoAlle
	}
	if fineISO == "" {
		fineISO = a.SessioneInizio
	}
	if err := s.SessionClose(id, fineISO, 0, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("session close: %w", err)
	}
	return nil
}

// kvSetString imposta una stringa nella tabella kv (helper interno).
func (s *Store) kvSetString(key, value string) error {
	j, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO kv (key, value, updated_at) VALUES (?, ?, ?)`,
		key, string(j), time.Now().UnixMilli(),
	)
	return err
}

// readJSONFile e' un helper generico che legge + decodifica un file JSON.
// Ritorna (zero, false) se il file non esiste o non e' parseabile.
//
// Tollera il BOM UTF-8 che Notepad e altri editor Windows aggiungono ai
// file salvati come "UTF-8".
func readJSONFile[T any](path string) (T, bool) {
	var zero T
	body, err := os.ReadFile(path)
	if err != nil {
		return zero, false
	}
	body = bytes.TrimPrefix(body, utf8BOM)
	if err := json.Unmarshal(body, &zero); err != nil {
		return zero, false
	}
	return zero, true
}

// renameToBak rinomina path → path+".v1.bak". Best-effort, errore loggato.
func renameToBak(path string) {
	if err := os.Rename(path, path+".v1.bak"); err != nil {
		log.Printf("rename %s: %v", path, err)
	}
}

// ============================================================
// Tipi v1 file-based (per il decode al volo)
// ============================================================

type v1Config struct {
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

type v1Preset struct {
	Nome        string   `json:"nome"`
	Descrizione string   `json:"descrizione"`
	Domini      []string `json:"domini"`
	CreatedAt   int64    `json:"createdAt"`
}

type v1Archive struct {
	SessioneInizio  string            `json:"sessioneInizio"`
	SessioneFineISO string            `json:"sessioneFineISO"`
	EsportatoAlle   string            `json:"esportatoAlle"`
	Titolo          string            `json:"titolo"`
	Classe          string            `json:"classe"`
	Modo            string            `json:"modo"`
	Studenti        map[string]string `json:"studenti"`
	Bloccati        []string          `json:"bloccati"`
	Entries         []json.RawMessage `json:"entries"`
}

type v1Entry struct {
	Ora     string `json:"ora"`
	TS      int64  `json:"ts"`
	IP      string `json:"ip"`
	Metodo  string `json:"metodo"`
	Dominio string `json:"dominio"`
	Tipo    string `json:"tipo"`
	Blocked bool   `json:"blocked"`
}
