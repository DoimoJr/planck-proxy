package persist

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ndjsonFilename e' il file della sessione corrente (append-only).
const ndjsonFilename = "_corrente.ndjson"

// ArchiveFile e' il payload completo di una sessione archiviata su disco.
type ArchiveFile struct {
	SessioneInizio  string            `json:"sessioneInizio"`
	SessioneFineISO string            `json:"sessioneFineISO,omitempty"`
	EsportatoAlle   string            `json:"esportatoAlle"`
	Titolo          string            `json:"titolo"`
	Classe          string            `json:"classe"`
	Modo            string            `json:"modo"`
	Studenti        map[string]string `json:"studenti"`
	Bloccati        []string          `json:"bloccati"`
	Entries         []json.RawMessage `json:"entries"`
}

// ============================================================
// NDJSON sessione corrente
// ============================================================

// NDJSONAppend appende `entryJSON` come una nuova riga al file della
// sessione corrente. Il caller deve passare gia' marshalato (una entry
// JSON completa, senza newline). Il newline viene aggiunto qui.
func (s *Store) NDJSONAppend(entryJSON []byte) error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.pathFor("sessioni", ndjsonFilename),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(entryJSON); err != nil {
		return err
	}
	if len(entryJSON) == 0 || entryJSON[len(entryJSON)-1] != '\n' {
		_, err = f.Write([]byte{'\n'})
	}
	return err
}

// NDJSONReset tronca il file NDJSON (lo svuota). Usato a SessionStart.
// Non lo elimina: lo lascia esistere come file vuoto.
func (s *Store) NDJSONReset() error {
	if s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.pathFor("sessioni", ndjsonFilename),
		os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// NDJSONExists verifica se il file della sessione corrente esiste e ha
// contenuto. Usato al boot per detecting una sessione interrotta da crash.
func (s *Store) NDJSONExists() (exists bool, size int64, err error) {
	if s.disabled {
		return false, 0, nil
	}
	info, err := os.Stat(s.pathFor("sessioni", ndjsonFilename))
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, info.Size(), nil
}

// NDJSONReadAll legge tutte le entries dal file NDJSON corrente.
// Ogni riga viene ritornata come json.RawMessage (non parsato — il
// caller decide se decodificare). Righe vuote ignorate.
func (s *Store) NDJSONReadAll() ([]json.RawMessage, error) {
	if s.disabled {
		return nil, nil
	}
	f, err := os.Open(s.pathFor("sessioni", ndjsonFilename))
	if os.IsNotExist(err) {
		return []json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	out := []json.RawMessage{}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// copy: scanner.Bytes() e' valido fino al prossimo Scan
		buf := make([]byte, len(line))
		copy(buf, line)
		out = append(out, buf)
	}
	return out, scanner.Err()
}

// ============================================================
// Archivio sessioni
// ============================================================

// archiveFilenameFromInizio costruisce il filename di archivio dato un
// timestamp ISO 8601 di inizio sessione.
//
//	"2026-04-22T10:23:45Z" -> "2026-04-22-10-23-45.json"
func archiveFilenameFromInizio(iso string) string {
	clean := strings.NewReplacer(":", "-", "T", "-", ".", "-").Replace(iso)
	if len(clean) > 19 {
		clean = clean[:19]
	}
	return clean + ".json"
}

// SaveArchive scrive una sessione archiviata. Il filename viene calcolato
// automaticamente da `a.SessioneInizio`. Ritorna il filename usato.
func (s *Store) SaveArchive(a ArchiveFile) (string, error) {
	if s.disabled {
		return "", nil
	}
	if a.SessioneInizio == "" {
		return "", fmt.Errorf("SessioneInizio richiesta")
	}
	fn := archiveFilenameFromInizio(a.SessioneInizio)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeJSONFile(s.pathFor("sessioni", fn), a); err != nil {
		return "", err
	}
	return fn, nil
}

// LoadArchive carica una sessione archiviata dal nome file.
func (s *Store) LoadArchive(filename string) (ArchiveFile, error) {
	if s.disabled {
		return ArchiveFile{}, ErrNomeInvalido
	}
	safe := sanitizeArchiveName(filename)
	if safe == "" {
		return ArchiveFile{}, ErrNomeInvalido
	}
	var a ArchiveFile
	exists, err := readJSONFile(s.pathFor("sessioni", safe), &a)
	if err != nil {
		return ArchiveFile{}, err
	}
	if !exists {
		return ArchiveFile{}, fmt.Errorf("archivio %q non trovato", filename)
	}
	return a, nil
}

// DeleteArchive rimuove una sessione archiviata.
func (s *Store) DeleteArchive(filename string) error {
	if s.disabled {
		return nil
	}
	safe := sanitizeArchiveName(filename)
	if safe == "" {
		return ErrNomeInvalido
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.pathFor("sessioni", safe))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListaSessioni ritorna i filename delle sessioni archiviate, ordinati
// dal piu' recente al meno recente (lessicograficamente desc grazie al
// formato YYYY-MM-DD-HH-MM-SS).
func (s *Store) ListaSessioni() ([]string, error) {
	if s.disabled {
		return []string{}, nil
	}
	entries, err := os.ReadDir(s.pathFor("sessioni"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == ndjsonFilename || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

// ============================================================
// Crash recovery (Phase 1.6 minimale)
// ============================================================

// RecoverNDJSONIfAny controlla se al boot esiste un NDJSON con contenuto.
// Se si', lo legge, costruisce un ArchiveFile usando i meta forniti dal
// caller, lo salva come "recovered-<timestamp>.json", e tronca il NDJSON.
//
// Caller fornisce metadata (titolo/classe/modo/studenti/bloccati) della
// configurazione corrente; per le entries usa il contenuto del NDJSON.
//
// Ritorna il filename dell'archivio prodotto, oppure "" se niente da
// recuperare.
func (s *Store) RecoverNDJSONIfAny(meta ArchiveFile) (string, error) {
	if s.disabled {
		return "", nil
	}
	exists, size, err := s.NDJSONExists()
	if err != nil || !exists || size == 0 {
		return "", err
	}
	entries, err := s.NDJSONReadAll()
	if err != nil {
		return "", fmt.Errorf("read ndjson: %w", err)
	}
	if len(entries) == 0 {
		_ = s.NDJSONReset()
		return "", nil
	}
	meta.Entries = entries
	if meta.SessioneInizio == "" {
		// Niente data di inizio nei meta → usiamo la prima entry (se decodabile)
		// per ricavare il timestamp; fallback "recovered-<bootTime>".
		meta.SessioneInizio = "recovered-bootTime"
	}
	fn, err := s.SaveArchive(meta)
	if err != nil {
		return "", err
	}
	if err := s.NDJSONReset(); err != nil {
		// Archivio gia' scritto: il reset fallito e' un warning, non bloccante
		return fn, fmt.Errorf("ndjson reset (archivio gia' salvato): %w", err)
	}
	return fn, nil
}

