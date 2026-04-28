package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// helper: nuovo Store su una tempdir isolata di testing.
func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNoOpStore(t *testing.T) {
	s := NoOpStore()
	if !s.Disabled() {
		t.Errorf("NoOp non disabled")
	}
	// Tutte le operazioni devono essere safe (no error)
	if err := s.SaveStudenti(map[string]string{"x": "y"}); err != nil {
		t.Errorf("NoOp SaveStudenti: %v", err)
	}
	if err := s.NDJSONAppend([]byte(`{"a":1}`)); err != nil {
		t.Errorf("NoOp NDJSONAppend: %v", err)
	}
}

func TestStudentiRoundtrip(t *testing.T) {
	s := tempStore(t)
	in := map[string]string{"192.168.1.50": "Mario", "192.168.1.51": "Luca"}
	if err := s.SaveStudenti(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := s.LoadStudenti()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out) != 2 || out["192.168.1.50"] != "Mario" || out["192.168.1.51"] != "Luca" {
		t.Errorf("roundtrip mismatch: %+v", out)
	}
}

func TestStudentiDropsCommentKeys(t *testing.T) {
	s := tempStore(t)
	// Scrivo direttamente il file con chiavi "_commento" come fa v1
	raw := map[string]string{
		"_esempio": "questo e' un commento",
		"192.168.1.50": "Mario",
	}
	body, _ := json.Marshal(raw)
	if err := writeRaw(filepath.Join(s.DataDir(), "studenti.json"), body); err != nil {
		t.Fatal(err)
	}
	out, _ := s.LoadStudenti()
	if _, ok := out["_esempio"]; ok {
		t.Errorf("chiave _esempio non droppata")
	}
	if out["192.168.1.50"] != "Mario" {
		t.Errorf("Mario perso: %+v", out)
	}
}

func TestBloccatiRoundtrip(t *testing.T) {
	s := tempStore(t)
	if err := s.SaveBloccati([]string{"chatgpt.com", "claude.ai", "openai.com"}); err != nil {
		t.Fatal(err)
	}
	out, err := s.LoadBloccati()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Errorf("len = %d, atteso 3", len(out))
	}
	// Sorted at write time
	expected := []string{"chatgpt.com", "claude.ai", "openai.com"}
	for i, want := range expected {
		if out[i] != want {
			t.Errorf("out[%d] = %q, atteso %q", i, out[i], want)
		}
	}
}

func TestConfigRoundtrip(t *testing.T) {
	s := tempStore(t)
	in := ConfigFile{
		Titolo:              "Verifica 4DII",
		Modo:                "blocklist",
		InattivitaSogliaSec: 120,
		ProxyPort:           9090,
		WebPort:             9999,
		AuthEnabled:         true,
		AuthUser:            "docente",
		AuthPasswordHash:    "$2a$10$fake",
		DominiIgnorati:      []string{"localhost", "127.0.0.1"},
	}
	if err := s.SaveConfig(in); err != nil {
		t.Fatal(err)
	}
	out, exists, err := s.LoadConfig()
	if err != nil || !exists {
		t.Fatalf("load: err=%v exists=%v", err, exists)
	}
	if out.Titolo != in.Titolo || out.Modo != in.Modo || out.AuthPasswordHash != in.AuthPasswordHash {
		t.Errorf("mismatch: in=%+v out=%+v", in, out)
	}
}

func TestConfigLoadMissing(t *testing.T) {
	s := tempStore(t)
	cfg, exists, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if exists {
		t.Errorf("file mancante segnato come exists=true")
	}
	if cfg.Titolo != "" {
		t.Errorf("default config non zero-value: %+v", cfg)
	}
}

func TestPresetCRUD(t *testing.T) {
	s := tempStore(t)
	p := PresetFile{
		Nome:        "verifica-prog",
		Descrizione: "blocchi tipici della verifica di programmazione",
		Domini:      []string{"chatgpt.com", "stackoverflow.com"},
		CreatedAt:   1714305600000,
	}
	if err := s.SavePreset(p); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.LoadPreset("verifica-prog")
	if err != nil || got.Nome != "verifica-prog" {
		t.Errorf("load: got=%+v err=%v", got, err)
	}
	lista, _ := s.ListaPresets()
	if len(lista) != 1 || lista[0] != "verifica-prog" {
		t.Errorf("lista = %v", lista)
	}
	if err := s.DeletePreset("verifica-prog"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	lista, _ = s.ListaPresets()
	if len(lista) != 0 {
		t.Errorf("lista dopo delete = %v", lista)
	}
}

func TestPresetNomeInvalido(t *testing.T) {
	// Nome che dopo sanitizzazione e' vuoto (solo caratteri non ammessi)
	// → ErrNomeInvalido.
	s := tempStore(t)
	err := s.SavePreset(PresetFile{Nome: "../@/"})
	if err != ErrNomeInvalido {
		t.Errorf("err = %v, atteso ErrNomeInvalido", err)
	}
}

func TestPresetNomeStrappo(t *testing.T) {
	// Nome con caratteri non ammessi mischiati a validi: i caratteri
	// invalidi vengono strippati, il resto viene salvato (path traversal
	// non possibile perche' "/" e "." sono droppati). Comportamento v1.
	s := tempStore(t)
	if err := s.SavePreset(PresetFile{Nome: "../etc/passwd"}); err != nil {
		t.Errorf("save: %v", err)
	}
	// "../etc/passwd" -> "etcpasswd" dopo sanitizzazione
	lista, _ := s.ListaPresets()
	if len(lista) != 1 || lista[0] != "etcpasswd" {
		t.Errorf("lista = %v, atteso [etcpasswd]", lista)
	}
}

func TestClassiCRUD(t *testing.T) {
	s := tempStore(t)
	c := ClasseFile{
		Classe: "4dii",
		Lab:    "lab2",
		Mappa:  map[string]string{"192.168.1.50": "Mario"},
	}
	if err := s.SaveClasse(c); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.LoadClasse("4dii", "lab2")
	if err != nil || got.Classe != "4dii" || got.Lab != "lab2" {
		t.Errorf("load: got=%+v err=%v", got, err)
	}
	lista, _ := s.ListaClassi()
	if len(lista) != 1 || lista[0].Classe != "4dii" || lista[0].Lab != "lab2" {
		t.Errorf("lista = %+v", lista)
	}
	if err := s.DeleteClasse("4dii", "lab2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestNDJSONAppendAndReset(t *testing.T) {
	s := tempStore(t)
	if err := s.NDJSONAppend([]byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.NDJSONAppend([]byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	exists, size, _ := s.NDJSONExists()
	if !exists || size == 0 {
		t.Errorf("dopo append, exists=%v size=%d", exists, size)
	}
	entries, _ := s.NDJSONReadAll()
	if len(entries) != 2 {
		t.Errorf("entries = %d, atteso 2", len(entries))
	}
	if err := s.NDJSONReset(); err != nil {
		t.Fatal(err)
	}
	_, size, _ = s.NDJSONExists()
	if size != 0 {
		t.Errorf("dopo reset, size = %d, atteso 0", size)
	}
}

func TestArchiveCRUD(t *testing.T) {
	s := tempStore(t)
	a := ArchiveFile{
		SessioneInizio:  "2026-04-22T10:23:45Z",
		SessioneFineISO: "2026-04-22T11:30:00Z",
		EsportatoAlle:   "2026-04-22T11:30:01Z",
		Titolo:          "Verifica",
		Modo:            "blocklist",
		Studenti:        map[string]string{"ip": "Mario"},
		Bloccati:        []string{"chatgpt.com"},
	}
	fn, err := s.SaveArchive(a)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if fn != "2026-04-22-10-23-45.json" {
		t.Errorf("filename = %q, atteso 2026-04-22-10-23-45.json", fn)
	}
	got, err := s.LoadArchive(fn)
	if err != nil || got.SessioneInizio != a.SessioneInizio {
		t.Errorf("load: got=%+v err=%v", got, err)
	}
	lista, _ := s.ListaSessioni()
	if len(lista) != 1 || lista[0] != fn {
		t.Errorf("lista = %v", lista)
	}
	if err := s.DeleteArchive(fn); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// writeRaw scrive bytes direttamente su disco (helper test).
func writeRaw(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
