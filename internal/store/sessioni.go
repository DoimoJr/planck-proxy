package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SessionMeta sono i metadati di apertura sessione (tutto eccetto
// l'array entries, che cresce nel tempo).
type SessionMeta struct {
	ID              int64             `json:"id"`
	SessioneInizio  string            `json:"sessioneInizio"`
	SessioneFineISO string            `json:"sessioneFineISO,omitempty"`
	DurataSec       int64             `json:"durataSec,omitempty"`
	Classe          string            `json:"classe"`
	Lab             string            `json:"lab"`
	Titolo          string            `json:"titolo"`
	Modo            string            `json:"modo"`
	Studenti        map[string]string `json:"studenti"`
	Bloccati        []string          `json:"bloccati"`
	ArchiviataAt    int64             `json:"archiviataAt"`
}

// SessionWithEntries e' SessionMeta + le entries della sessione.
type SessionWithEntries struct {
	SessionMeta
	Entries []json.RawMessage `json:"entries"`
}

// SessionStart apre una nuova riga nella tabella sessioni e ritorna l'id.
// `studentiSnap` e `bloccatiSnap` sono fotografati al momento dell'avvio.
func (s *Store) SessionStart(meta SessionMeta) (int64, error) {
	if s.disabled {
		return 0, nil
	}
	studJSON, _ := json.Marshal(meta.Studenti)
	bloccJSON, _ := json.Marshal(meta.Bloccati)
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO sessioni (sessione_inizio, sessione_fine, durata_sec, classe, lab, titolo, modo, studenti_snapshot, bloccati_snapshot, archiviata_at)
		 VALUES (?, NULL, NULL, ?, ?, ?, ?, ?, ?, 0)`,
		meta.SessioneInizio, meta.Classe, meta.Lab, meta.Titolo, meta.Modo,
		string(studJSON), string(bloccJSON),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SessionAppendEntry inserisce una entry per la sessione attiva.
func (s *Store) SessionAppendEntry(sessionID int64, ts int64, ora, ip, nomeStudente, metodo, dominio, tipo string, blocked, flagged bool) error {
	if s.disabled {
		return nil
	}
	var nomePtr any
	if nomeStudente != "" {
		nomePtr = nomeStudente
	}
	var b, f int
	if blocked {
		b = 1
	}
	if flagged {
		f = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO entries (sessione_id, ora, ts, ip, nome_studente, metodo, dominio, tipo, blocked, flagged)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, ora, ts, ip, nomePtr, metodo, dominio, tipo, b, f,
	)
	return err
}

// SessionClose imposta sessione_fine + durata_sec + archiviata_at sulla riga.
func (s *Store) SessionClose(sessionID int64, fineISO string, durataSec int64, archiviataAt int64) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE sessioni SET sessione_fine = ?, durata_sec = ?, archiviata_at = ? WHERE id = ?`,
		fineISO, durataSec, archiviataAt, sessionID,
	)
	return err
}

// SessionRename aggiorna il titolo (nome custom) di una sessione esistente.
// Usato dopo Stop quando l'utente da' un nome alla sessione appena archiviata.
func (s *Store) SessionRename(sessionID int64, titolo string) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE sessioni SET titolo = ? WHERE id = ?`,
		titolo, sessionID,
	)
	return err
}

// SessionFindActive cerca una sessione attiva (sessione_fine NULL).
// Usato al boot per crash recovery: se ne esiste una, il chiamante puo'
// decidere di chiuderla forzatamente come "recovered".
func (s *Store) SessionFindActive() (id int64, inizio string, found bool, err error) {
	if s.disabled {
		return 0, "", false, nil
	}
	row := s.db.QueryRow(
		`SELECT id, sessione_inizio FROM sessioni WHERE sessione_fine IS NULL ORDER BY id DESC LIMIT 1`,
	)
	if err := row.Scan(&id, &inizio); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, nil
		}
		return 0, "", false, err
	}
	return id, inizio, true, nil
}

// SessionList ritorna le sessioni archiviate (sessione_fine NOT NULL),
// ordinate dalla piu' recente.
//
// Il campo `File` (presente in `SessionMeta` come parte dell'ID-stringa)
// non e' incluso qui — il caller deve usare `ID` numerico per fare load/delete.
func (s *Store) SessionList() ([]SessionMeta, error) {
	if s.disabled {
		return []SessionMeta{}, nil
	}
	rows, err := s.db.Query(
		`SELECT id, sessione_inizio, COALESCE(sessione_fine, ''), COALESCE(durata_sec, 0),
		        classe, lab, COALESCE(titolo, ''), modo,
		        studenti_snapshot, bloccati_snapshot, archiviata_at
		 FROM sessioni
		 WHERE sessione_fine IS NOT NULL
		 ORDER BY sessione_inizio DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionMeta{}
	for rows.Next() {
		var m SessionMeta
		var studJSON, bloccJSON string
		if err := rows.Scan(
			&m.ID, &m.SessioneInizio, &m.SessioneFineISO, &m.DurataSec,
			&m.Classe, &m.Lab, &m.Titolo, &m.Modo,
			&studJSON, &bloccJSON, &m.ArchiviataAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(studJSON), &m.Studenti)
		_ = json.Unmarshal([]byte(bloccJSON), &m.Bloccati)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SessionLoad ritorna i metadati + tutte le entries di una sessione.
func (s *Store) SessionLoad(sessionID int64) (SessionWithEntries, error) {
	out := SessionWithEntries{}
	if s.disabled {
		return out, fmt.Errorf("store disabled")
	}

	row := s.db.QueryRow(
		`SELECT id, sessione_inizio, COALESCE(sessione_fine, ''), COALESCE(durata_sec, 0),
		        classe, lab, COALESCE(titolo, ''), modo,
		        studenti_snapshot, bloccati_snapshot, archiviata_at
		 FROM sessioni WHERE id = ?`,
		sessionID,
	)
	var studJSON, bloccJSON string
	if err := row.Scan(
		&out.ID, &out.SessioneInizio, &out.SessioneFineISO, &out.DurataSec,
		&out.Classe, &out.Lab, &out.Titolo, &out.Modo,
		&studJSON, &bloccJSON, &out.ArchiviataAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, fmt.Errorf("sessione %d non trovata", sessionID)
		}
		return out, err
	}
	_ = json.Unmarshal([]byte(studJSON), &out.Studenti)
	_ = json.Unmarshal([]byte(bloccJSON), &out.Bloccati)

	rows, err := s.db.Query(
		`SELECT ora, ts, ip, COALESCE(nome_studente, ''), metodo, dominio, tipo, blocked, flagged
		 FROM entries WHERE sessione_id = ? ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Entries = []json.RawMessage{}
	for rows.Next() {
		var ora, ip, nome, metodo, dominio, tipo string
		var ts int64
		var blocked, flagged int
		if err := rows.Scan(&ora, &ts, &ip, &nome, &metodo, &dominio, &tipo, &blocked, &flagged); err != nil {
			return out, err
		}
		entry := map[string]any{
			"ora":     ora,
			"ts":      ts,
			"ip":      ip,
			"metodo":  metodo,
			"dominio": dominio,
			"tipo":    tipo,
			"blocked": blocked == 1,
		}
		if flagged == 1 {
			entry["flagged"] = true
		}
		if nome != "" {
			entry["nome"] = nome
		}
		raw, _ := json.Marshal(entry)
		out.Entries = append(out.Entries, raw)
	}
	return out, rows.Err()
}

// SessionDelete elimina una sessione (e le sue entries via CASCADE).
func (s *Store) SessionDelete(sessionID int64) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessioni WHERE id = ?`, sessionID)
	return err
}

// SessionListFilenames ritorna gli "id-string" delle sessioni archiviate
// nella forma `<id>-<inizio>.json` per compatibilita' con l'API
// /api/sessioni che storicamente ritornava una lista di filename.
//
// Il client puo' decodare l'id estraendo la parte prima del primo "-".
func (s *Store) SessionListFilenames() ([]string, error) {
	list, err := s.SessionList()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(list))
	for i, m := range list {
		// Es. "12-2026-04-29-10-23-45.json"
		clean := strings.NewReplacer(":", "-", "T", "-", ".", "-").Replace(m.SessioneInizio)
		if len(clean) > 19 {
			clean = clean[:19]
		}
		out[i] = fmt.Sprintf("%d-%s.json", m.ID, clean)
	}
	return out, nil
}

// ParseSessionFilename estrae l'id numerico da un filename SessionListFilenames-style.
func ParseSessionFilename(filename string) (int64, error) {
	idx := strings.Index(filename, "-")
	if idx < 0 {
		return 0, fmt.Errorf("filename mal formato: %q", filename)
	}
	var id int64
	if _, err := fmt.Sscanf(filename[:idx], "%d", &id); err != nil {
		return 0, fmt.Errorf("id non numerico in %q: %w", filename, err)
	}
	return id, nil
}
