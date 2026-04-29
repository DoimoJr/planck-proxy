package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ClasseFile rispecchia la struct in internal/persist.
type ClasseFile struct {
	Classe    string            `json:"classe"`
	Lab       string            `json:"lab"`
	Mappa     map[string]string `json:"mappa"`
	UpdatedAt int64             `json:"updatedAt"`
}

// ComboInfo descrive una combo come restituita da ListaClassi (per /api/classi).
type ComboInfo struct {
	Classe string `json:"classe"`
	Lab    string `json:"lab"`
	File   string `json:"file"`
}

// LoadClasse carica una combo per (classe, lab).
func (s *Store) LoadClasse(classe, lab string) (ClasseFile, error) {
	if s.disabled {
		return ClasseFile{}, ErrNomeInvalido
	}
	c := sanitizeName(classe)
	l := sanitizeName(lab)
	if c == "" || l == "" {
		return ClasseFile{}, ErrNomeInvalido
	}
	var cf ClasseFile
	var mappaJSON string
	err := s.db.QueryRow(
		`SELECT classe, lab, mappa, updated_at FROM combo WHERE classe = ? AND lab = ?`,
		c, l,
	).Scan(&cf.Classe, &cf.Lab, &mappaJSON, &cf.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return cf, fmt.Errorf("combo %q/%q non trovata", classe, lab)
	}
	if err != nil {
		return cf, err
	}
	if err := json.Unmarshal([]byte(mappaJSON), &cf.Mappa); err != nil {
		return cf, fmt.Errorf("parse mappa: %w", err)
	}
	return cf, nil
}

// SaveClasse crea/aggiorna una combo.
func (s *Store) SaveClasse(cf ClasseFile) error {
	if s.disabled {
		return nil
	}
	c := sanitizeName(cf.Classe)
	l := sanitizeName(cf.Lab)
	if c == "" || l == "" {
		return ErrNomeInvalido
	}
	mappaJSON, err := json.Marshal(cf.Mappa)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if cf.UpdatedAt == 0 {
		cf.UpdatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO combo (classe, lab, mappa, updated_at) VALUES (?, ?, ?, ?)`,
		c, l, string(mappaJSON), cf.UpdatedAt,
	)
	return err
}

// DeleteClasse rimuove una combo. No-op se non esiste.
func (s *Store) DeleteClasse(classe, lab string) error {
	if s.disabled {
		return nil
	}
	c := sanitizeName(classe)
	l := sanitizeName(lab)
	if c == "" || l == "" {
		return ErrNomeInvalido
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM combo WHERE classe = ? AND lab = ?`, c, l)
	return err
}

// ListaClassi elenca tutte le combo, ordinate per (classe, lab).
// Il campo `File` e' settato a una rappresentazione testuale solo per
// retro-compatibilita' con l'API GET /api/classi (UI usa file come id).
func (s *Store) ListaClassi() ([]ComboInfo, error) {
	if s.disabled {
		return []ComboInfo{}, nil
	}
	rows, err := s.db.Query(`SELECT classe, lab FROM combo ORDER BY classe, lab`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ComboInfo{}
	for rows.Next() {
		var c, l string
		if err := rows.Scan(&c, &l); err != nil {
			return nil, err
		}
		out = append(out, ComboInfo{Classe: c, Lab: l, File: c + "--" + l + ".json"})
	}
	return out, rows.Err()
}
