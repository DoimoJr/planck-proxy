package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// PresetFile rispecchia la struct corrispondente in internal/persist.
type PresetFile struct {
	Nome        string   `json:"nome"`
	Descrizione string   `json:"descrizione"`
	Domini      []string `json:"domini"`
	CreatedAt   int64    `json:"createdAt"`
}

// LoadPreset carica un preset per nome.
func (s *Store) LoadPreset(nome string) (PresetFile, error) {
	if s.disabled {
		return PresetFile{}, fmt.Errorf("store disabled")
	}
	safe := sanitizeName(nome)
	if safe == "" {
		return PresetFile{}, ErrNomeInvalido
	}
	var p PresetFile
	var dominiJSON string
	err := s.db.QueryRow(
		`SELECT nome, COALESCE(descrizione, ''), domini, created_at FROM presets WHERE nome = ?`,
		safe,
	).Scan(&p.Nome, &p.Descrizione, &dominiJSON, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("preset %q non trovato", nome)
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(dominiJSON), &p.Domini); err != nil {
		return p, fmt.Errorf("parse domini: %w", err)
	}
	return p, nil
}

// SavePreset crea/aggiorna un preset (overwrite).
func (s *Store) SavePreset(p PresetFile) error {
	if s.disabled {
		return nil
	}
	safe := sanitizeName(p.Nome)
	if safe == "" {
		return ErrNomeInvalido
	}
	dominiJSON, err := json.Marshal(p.Domini)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO presets (nome, descrizione, domini, created_at) VALUES (?, ?, ?, ?)`,
		safe, p.Descrizione, string(dominiJSON), p.CreatedAt,
	)
	return err
}

// DeletePreset rimuove un preset. No-op se non esiste.
func (s *Store) DeletePreset(nome string) error {
	if s.disabled {
		return nil
	}
	safe := sanitizeName(nome)
	if safe == "" {
		return ErrNomeInvalido
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM presets WHERE nome = ?`, safe)
	return err
}

// ListaPresets ritorna i nomi dei preset disponibili, ordinati.
func (s *Store) ListaPresets() ([]string, error) {
	if s.disabled {
		return []string{}, nil
	}
	rows, err := s.db.Query(`SELECT nome FROM presets ORDER BY nome`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
