package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ConfigFile rispecchia la struct dell'omonimo tipo in internal/persist:
// usata per Load/Save in modo atomico.
//
// Internamente, ogni campo viene salvato come riga separata in `kv` (con
// JSON encoding). Questo permette in futuro di leggere/scrivere singole
// chiavi senza serializzare/deserializzare l'intero blob.
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
	// VeyonKeyName e' il nome della master key importata dal docente
	// via Settings UI (Phase 3e). Vuoto = Veyon non configurato. La
	// chiave privata vive su disco in `<dataDir>/veyon-master.pem`,
	// non in DB (per separare segreti da config).
	VeyonKeyName string `json:"veyonKeyName"`
	// VeyonPort e' la porta TCP dei veyon-server studente. 0 = default 11100.
	VeyonPort int `json:"veyonPort"`
}

// kvKeys e' la lista dei campi mappata su chiavi `kv`.
var kvKeys = struct {
	Titolo, Classe, Modo, InattivitaSogliaSec,
	ProxyPort, WebPort, AuthEnabled, AuthUser,
	AuthPasswordHash, VeyonKeyName, VeyonPort string
}{
	Titolo:              "titolo",
	Classe:              "classe",
	Modo:                "modo",
	InattivitaSogliaSec: "inattivitaSogliaSec",
	ProxyPort:           "proxyPort",
	WebPort:             "webPort",
	AuthEnabled:         "authEnabled",
	AuthUser:            "authUser",
	AuthPasswordHash:    "authPasswordHash",
	VeyonKeyName:        "veyonKeyName",
	VeyonPort:           "veyonPort",
}

// LoadConfig legge tutti i campi config da kv + dominiIgnorati dalla
// tabella dedicata.
//
// Ritorna (zero, false, nil) se la tabella kv e' vuota (primo boot).
func (s *Store) LoadConfig() (ConfigFile, bool, error) {
	if s.disabled {
		return ConfigFile{}, false, nil
	}
	cfg := ConfigFile{}

	// Conta righe in kv per decidere "exists".
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kv`).Scan(&n); err != nil {
		return cfg, false, err
	}
	if n == 0 {
		return cfg, false, nil
	}

	// Carica i singoli campi.
	cfg.Titolo, _ = s.kvGetString(kvKeys.Titolo)
	cfg.Classe, _ = s.kvGetString(kvKeys.Classe)
	cfg.Modo, _ = s.kvGetString(kvKeys.Modo)
	cfg.InattivitaSogliaSec, _ = s.kvGetInt(kvKeys.InattivitaSogliaSec)
	cfg.ProxyPort, _ = s.kvGetInt(kvKeys.ProxyPort)
	cfg.WebPort, _ = s.kvGetInt(kvKeys.WebPort)
	cfg.AuthEnabled, _ = s.kvGetBool(kvKeys.AuthEnabled)
	cfg.AuthUser, _ = s.kvGetString(kvKeys.AuthUser)
	cfg.AuthPasswordHash, _ = s.kvGetString(kvKeys.AuthPasswordHash)
	cfg.VeyonKeyName, _ = s.kvGetString(kvKeys.VeyonKeyName)
	cfg.VeyonPort, _ = s.kvGetInt(kvKeys.VeyonPort)

	// Domini ignorati dalla tabella dedicata.
	rows, err := s.db.Query(`SELECT dominio FROM domini_ignorati ORDER BY dominio`)
	if err != nil {
		return cfg, true, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return cfg, true, err
		}
		cfg.DominiIgnorati = append(cfg.DominiIgnorati, d)
	}
	return cfg, true, rows.Err()
}

// SaveConfig sovrascrive in blocco la config (kv + domini_ignorati). Atomic.
func (s *Store) SaveConfig(cfg ConfigFile) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	pairs := []struct {
		k string
		v any
	}{
		{kvKeys.Titolo, cfg.Titolo},
		{kvKeys.Classe, cfg.Classe},
		{kvKeys.Modo, cfg.Modo},
		{kvKeys.InattivitaSogliaSec, cfg.InattivitaSogliaSec},
		{kvKeys.ProxyPort, cfg.ProxyPort},
		{kvKeys.WebPort, cfg.WebPort},
		{kvKeys.AuthEnabled, cfg.AuthEnabled},
		{kvKeys.AuthUser, cfg.AuthUser},
		{kvKeys.AuthPasswordHash, cfg.AuthPasswordHash},
		{kvKeys.VeyonKeyName, cfg.VeyonKeyName},
		{kvKeys.VeyonPort, cfg.VeyonPort},
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO kv (key, value, updated_at) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range pairs {
		j, err := json.Marshal(p.v)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(p.k, string(j), now); err != nil {
			return err
		}
	}

	// Domini ignorati: replace-all
	if _, err := tx.Exec(`DELETE FROM domini_ignorati`); err != nil {
		return err
	}
	stmt2, err := tx.Prepare(`INSERT INTO domini_ignorati (dominio) VALUES (?)`)
	if err != nil {
		return err
	}
	defer stmt2.Close()
	for _, d := range cfg.DominiIgnorati {
		if _, err := stmt2.Exec(d); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// kvGetString ritorna il valore JSON-decodato della chiave o "" se assente.
func (s *Store) kvGetString(key string) (string, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var out string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return "", err
	}
	return out, nil
}

func (s *Store) kvGetInt(key string) (int, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var out int
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return 0, err
	}
	return out, nil
}

func (s *Store) kvGetBool(key string) (bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var out bool
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return false, err
	}
	return out, nil
}
