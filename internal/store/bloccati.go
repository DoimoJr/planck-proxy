package store

import (
	"sort"
	"time"
)

// LoadBloccati ritorna la blocklist ordinata.
func (s *Store) LoadBloccati() ([]string, error) {
	if s.disabled {
		return []string{}, nil
	}
	rows, err := s.db.Query(`SELECT dominio FROM bloccati ORDER BY dominio`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveBloccati sostituisce in blocco la blocklist. Atomic.
func (s *Store) SaveBloccati(list []string) error {
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
	if _, err := tx.Exec(`DELETE FROM bloccati`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO bloccati (dominio, added_at) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UnixMilli()
	sorted := append([]string{}, list...)
	sort.Strings(sorted)
	for _, d := range sorted {
		if _, err := stmt.Exec(d, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadBloccatiPerIp ritorna la mappa ip → []domini ordinati.
func (s *Store) LoadBloccatiPerIp() (map[string][]string, error) {
	out := map[string][]string{}
	if s.disabled {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT ip, dominio FROM bloccati_per_ip ORDER BY ip, dominio`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ip, d string
		if err := rows.Scan(&ip, &d); err != nil {
			return nil, err
		}
		out[ip] = append(out[ip], d)
	}
	return out, rows.Err()
}

// SaveBloccatiPerIp sostituisce in blocco l'intera mappa. Atomic.
func (s *Store) SaveBloccatiPerIp(perIp map[string][]string) error {
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
	if _, err := tx.Exec(`DELETE FROM bloccati_per_ip`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO bloccati_per_ip (ip, dominio, added_at) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UnixMilli()
	ips := make([]string, 0, len(perIp))
	for ip := range perIp {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	for _, ip := range ips {
		doms := append([]string{}, perIp[ip]...)
		sort.Strings(doms)
		for _, d := range doms {
			if _, err := stmt.Exec(ip, d, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
