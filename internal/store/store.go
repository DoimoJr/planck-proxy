// Package store implementa la persistenza in SQLite per Planck v2 (Phase 2).
//
// Sostituisce il package internal/persist file-based introdotto in Phase 1.6.
// Lo schema vive in un singolo file `planck.db` accanto al binario; tutte
// le mutazioni passano attraverso gli stessi metodi (LoadStudenti,
// SaveStudenti, ecc.) ma il backend e' SQL.
//
// Dipendenza: modernc.org/sqlite (pure Go, niente CGO — mantiene il binario
// Planck portatile e cross-compilabile facilmente).
//
// Architettura:
//   - store.go     → Store + Open + migrations runner + helpers
//   - schema.go    → SQL embedded come stringa (singola migration v1)
//   - studenti.go  → CRUD mappa studenti correnti
//   - bloccati.go  → CRUD blocklist
//   - config.go    → kv config (load/save campo per campo)
//   - presets.go   → CRUD preset blocklist
//   - classi.go    → CRUD combo classe+lab
//   - sessioni.go  → lifecycle sessione + entries + archivio
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // driver "sqlite"
)

// Store rappresenta una connessione al DB SQLite di Planck.
// Concurrent-safe: i metodi sono protetti da sync.Mutex per le scritture
// (SQLite serializza gia' le scritture col WAL, ma il mutex protegge
// pattern read-then-write atomici).
type Store struct {
	db       *sql.DB
	mu       sync.Mutex
	disabled bool
	dbPath   string
}

// Open apre (o crea, se non esiste) il database SQLite a `dbPath`.
// Esegue le migrations dello schema fino alla versione corrente.
//
// Se il file non esiste, viene creato con permessi user-only.
// Si abilita journal_mode=WAL per concorrenza letture/scritture e
// foreign_keys=ON per garantire i CASCADE su DELETE.
func Open(dbPath string) (*Store, error) {
	// Connection string: enable WAL via _pragma in DSN.
	// modernc.org/sqlite usa il prefisso "file:" e accetta query ?_pragma=...
	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// modernc.org/sqlite e' pure Go ma non gestisce bene la concorrenza con
	// MaxOpenConns alto su una connessione SQLite. Limito a 1 per write,
	// uso default per read. Best practice: 1 conn aperta + WAL e' sufficiente.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	s := &Store{db: db, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return s, nil
}

// NoOpStore ritorna uno Store disabilitato: tutti i metodi sono no-op
// (ritornano valori zero / nil error). Usato nei test che non vogliono
// dipendere da SQLite.
func NoOpStore() *Store { return &Store{disabled: true} }

// Disabled indica se lo store e' un NoOp.
func (s *Store) Disabled() bool { return s.disabled }

// Close chiude la connessione SQLite. Da chiamare a fine processo
// per garantire il flush del WAL.
func (s *Store) Close() error {
	if s.disabled || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path ritorna il path assoluto del file DB. Vuoto se NoOp.
func (s *Store) Path() string { return s.dbPath }

// DataDir ritorna la directory dei dati (cartella che contiene il DB).
// Usata dagli endpoint che servono file accessori (es. script studenti
// generati al boot in cmd/planck/main.go). Vuoto se NoOp.
func (s *Store) DataDir() string {
	if s.dbPath == "" {
		return ""
	}
	return filepath.Dir(s.dbPath)
}

// migrate applica tutte le migration sql in sequenza, partendo dalla
// versione attuale registrata in `schema_version`. Le migration sono
// definite in schema.go come slice ordinata.
func (s *Store) migrate() error {
	// Crea la tabella schema_version se manca (non e' parte della migration v1
	// per evitare il chicken-and-egg).
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}

	for _, m := range allMigrations {
		if m.Version <= current {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx v%d: %w", m.Version, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply v%d (%s): %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record v%d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit v%d: %w", m.Version, err)
		}
	}
	return nil
}
