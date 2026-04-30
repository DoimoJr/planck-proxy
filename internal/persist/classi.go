package persist

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ClasseFile e' il payload di un file combo (classe, lab).
type ClasseFile struct {
	Classe    string            `json:"classe"`
	Lab       string            `json:"lab"`
	Mappa     map[string]string `json:"mappa"`
	UpdatedAt int64             `json:"updatedAt"`
}

// ComboInfo descrive una combo (classe, lab) salvata, nei termini esposti
// dall'API /api/classi.
type ComboInfo struct {
	Classe string `json:"classe"`
	Lab    string `json:"lab"`
	File   string `json:"file"`
}

// classeFilename costruisce il filename "<classe>--<lab>.json" sanitizzato.
func classeFilename(classe, lab string) (string, error) {
	c := sanitizeName(classe)
	l := sanitizeName(lab)
	if c == "" || l == "" {
		return "", ErrNomeInvalido
	}
	return c + "--" + l + ".json", nil
}

// parseComboFilename estrae (classe, lab) da un filename "<classe>--<lab>.json".
// Ritorna "" "" se il formato non e' riconosciuto.
func parseComboFilename(filename string) (classe, lab string) {
	stem := strings.TrimSuffix(filename, ".json")
	idx := strings.Index(stem, "--")
	if idx < 0 {
		return "", ""
	}
	return stem[:idx], stem[idx+2:]
}

// LoadClasse legge una combo da disco.
func (s *Store) LoadClasse(classe, lab string) (ClasseFile, error) {
	if s.disabled {
		return ClasseFile{}, ErrNomeInvalido
	}
	fn, err := classeFilename(classe, lab)
	if err != nil {
		return ClasseFile{}, err
	}
	var c ClasseFile
	exists, err := readJSONFile(s.pathFor("classi", fn), &c)
	if err != nil {
		return ClasseFile{}, err
	}
	if !exists {
		return ClasseFile{}, fmt.Errorf("combo %q/%q non trovata", classe, lab)
	}
	return c, nil
}

// SaveClasse scrive una combo. Idempotente.
func (s *Store) SaveClasse(c ClasseFile) error {
	if s.disabled {
		return nil
	}
	fn, err := classeFilename(c.Classe, c.Lab)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.pathFor("classi", fn), c)
}

// DeleteClasse rimuove il file combo. No-op se non esiste.
func (s *Store) DeleteClasse(classe, lab string) error {
	if s.disabled {
		return nil
	}
	fn, err := classeFilename(classe, lab)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err = os.Remove(s.pathFor("classi", fn))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListaClassi scansiona la directory classi/ e ritorna la lista delle combo
// disponibili, ordinate per (classe, lab).
func (s *Store) ListaClassi() ([]ComboInfo, error) {
	if s.disabled {
		return []ComboInfo{}, nil
	}
	entries, err := os.ReadDir(s.pathFor("classi"))
	if os.IsNotExist(err) {
		return []ComboInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]ComboInfo, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		classe, lab := parseComboFilename(name)
		if classe == "" || lab == "" {
			continue
		}
		out = append(out, ComboInfo{Classe: classe, Lab: lab, File: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Classe != out[j].Classe {
			return out[i].Classe < out[j].Classe
		}
		return out[i].Lab < out[j].Lab
	})
	return out, nil
}
