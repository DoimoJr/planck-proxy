package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// WatchdogEvent e' una riga di watchdog_events serializzata per la UI / API.
type WatchdogEvent struct {
	ID           int64           `json:"id"`
	SessioneID   *int64          `json:"sessioneId,omitempty"`
	Plugin       string          `json:"plugin"`
	IP           string          `json:"ip"`
	NomeStudente string          `json:"nomeStudente,omitempty"`
	TS           int64           `json:"ts"`
	Severity     string          `json:"severity"`
	Payload      json.RawMessage `json:"payload"`
}

// WatchdogPluginConfig e' la riga di watchdog_config per un plugin.
type WatchdogPluginConfig struct {
	Plugin    string          `json:"plugin"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	UpdatedAt int64           `json:"updatedAt"`
}

// SaveWatchdogEvent persiste un evento watchdog. `sessioneID` puo' essere
// 0 (sessione non attiva → NULL nel DB).
func (s *Store) SaveWatchdogEvent(plugin, ip, nomeStudente string, sessioneID int64, ts int64, severity string, payload []byte) (int64, error) {
	if s.disabled {
		return 0, nil
	}
	var sessPtr any
	if sessioneID > 0 {
		sessPtr = sessioneID
	}
	var nomePtr any
	if nomeStudente != "" {
		nomePtr = nomeStudente
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO watchdog_events
		   (sessione_id, plugin, ip, nome_studente, ts, severity, payload_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessPtr, plugin, ip, nomePtr, ts, severity, string(payload),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListaWatchdogEvents ritorna gli ultimi `limit` eventi, opzionalmente
// filtrati per plugin/IP. Ordine cronologico inverso.
func (s *Store) ListaWatchdogEvents(plugin, ip string, limit int) ([]WatchdogEvent, error) {
	if s.disabled {
		return []WatchdogEvent{}, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := `SELECT id, COALESCE(sessione_id, 0), plugin, ip, COALESCE(nome_studente, ''),
	             ts, severity, payload_json
	      FROM watchdog_events`
	args := []any{}
	where := ""
	if plugin != "" {
		where += " AND plugin = ?"
		args = append(args, plugin)
	}
	if ip != "" {
		where += " AND ip = ?"
		args = append(args, ip)
	}
	if where != "" {
		q += " WHERE 1=1" + where
	}
	q += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchdogEvent{}
	for rows.Next() {
		var e WatchdogEvent
		var sessID int64
		var payload string
		if err := rows.Scan(&e.ID, &sessID, &e.Plugin, &e.IP, &e.NomeStudente, &e.TS, &e.Severity, &payload); err != nil {
			return nil, err
		}
		if sessID > 0 {
			e.SessioneID = &sessID
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LoadWatchdogConfig ritorna la config di tutti i plugin presenti in
// watchdog_config. I plugin non ancora configurati (mai abilitati)
// non appaiono — il caller decide cosa fare in quel caso (di solito:
// considera disabled).
func (s *Store) LoadWatchdogConfig() ([]WatchdogPluginConfig, error) {
	if s.disabled {
		return []WatchdogPluginConfig{}, nil
	}
	rows, err := s.db.Query(`SELECT plugin, enabled, config_json, updated_at FROM watchdog_config ORDER BY plugin`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchdogPluginConfig{}
	for rows.Next() {
		var c WatchdogPluginConfig
		var enabled int
		var cfgJSON string
		if err := rows.Scan(&c.Plugin, &enabled, &cfgJSON, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled == 1
		c.Config = json.RawMessage(cfgJSON)
		out = append(out, c)
	}
	return out, rows.Err()
}

// LoadWatchdogPluginConfig legge la config di UN plugin. Se assente
// ritorna (zero, false, nil) e il caller gestisce.
func (s *Store) LoadWatchdogPluginConfig(plugin string) (WatchdogPluginConfig, bool, error) {
	if s.disabled {
		return WatchdogPluginConfig{}, false, nil
	}
	var c WatchdogPluginConfig
	var enabled int
	var cfgJSON string
	err := s.db.QueryRow(
		`SELECT plugin, enabled, config_json, updated_at FROM watchdog_config WHERE plugin = ?`,
		plugin,
	).Scan(&c.Plugin, &enabled, &cfgJSON, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	c.Enabled = enabled == 1
	c.Config = json.RawMessage(cfgJSON)
	return c, true, nil
}

// SaveWatchdogPluginConfig upsert della config di un plugin.
func (s *Store) SaveWatchdogPluginConfig(plugin string, enabled bool, configJSON []byte) error {
	if s.disabled {
		return nil
	}
	if plugin == "" {
		return fmt.Errorf("plugin vuoto")
	}
	var en int
	if enabled {
		en = 1
	}
	if len(configJSON) == 0 {
		configJSON = []byte("{}")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO watchdog_config (plugin, enabled, config_json, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(plugin) DO UPDATE SET
		   enabled = excluded.enabled,
		   config_json = excluded.config_json,
		   updated_at = excluded.updated_at`,
		plugin, en, string(configJSON), time.Now().UnixMilli(),
	)
	return err
}
