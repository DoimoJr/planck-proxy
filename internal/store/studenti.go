package store

// LoadStudenti carica la mappa IP→nome dalla tabella studenti_correnti.
// Se il DB e' vuoto, ritorna mappa vuota (non e' un errore).
func (s *Store) LoadStudenti() (map[string]string, error) {
	if s.disabled {
		return map[string]string{}, nil
	}
	rows, err := s.db.Query(`SELECT ip, nome FROM studenti_correnti`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ip, nome string
		if err := rows.Scan(&ip, &nome); err != nil {
			return nil, err
		}
		out[ip] = nome
	}
	return out, rows.Err()
}

// SaveStudenti sostituisce in blocco la mappa attiva. Atomic via transaction.
func (s *Store) SaveStudenti(m map[string]string) error {
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
	if _, err := tx.Exec(`DELETE FROM studenti_correnti`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO studenti_correnti (ip, nome) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for ip, nome := range m {
		if _, err := stmt.Exec(ip, nome); err != nil {
			return err
		}
	}
	return tx.Commit()
}
