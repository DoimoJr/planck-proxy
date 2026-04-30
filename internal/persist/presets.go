package persist

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// PresetFile e' il payload di un file preset.
type PresetFile struct {
	Nome        string   `json:"nome"`
	Descrizione string   `json:"descrizione"`
	Domini      []string `json:"domini"`
	CreatedAt   int64    `json:"createdAt"`
}

// LoadPreset legge un preset da disco.
func (s *Store) LoadPreset(nome string) (PresetFile, error) {
	if s.disabled {
		return PresetFile{}, ErrNomeInvalido
	}
	safe := sanitizeName(nome)
	if safe == "" {
		return PresetFile{}, ErrNomeInvalido
	}
	var p PresetFile
	exists, err := readJSONFile(s.pathFor("presets", safe+".json"), &p)
	if err != nil {
		return PresetFile{}, err
	}
	if !exists {
		return PresetFile{}, fmt.Errorf("preset %q non trovato", nome)
	}
	return p, nil
}

// SavePreset scrive un preset su disco. Idempotente (overwrite).
func (s *Store) SavePreset(p PresetFile) error {
	if s.disabled {
		return nil
	}
	safe := sanitizeName(p.Nome)
	if safe == "" {
		return ErrNomeInvalido
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.pathFor("presets", safe+".json"), p)
}

// DeletePreset rimuove il file preset. No-op se non esiste.
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
	err := os.Remove(s.pathFor("presets", safe+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListaPresets ritorna i nomi dei preset disponibili (senza estensione),
// ordinati alfabeticamente.
func (s *Store) ListaPresets() ([]string, error) {
	if s.disabled {
		return []string{}, nil
	}
	entries, err := os.ReadDir(s.pathFor("presets"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".json") {
			out = append(out, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(out)
	return out, nil
}
