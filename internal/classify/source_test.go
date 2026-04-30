package classify

import (
	"os"
	"testing"
)

// TestEmbeddedMatchesDataFile assicura che il file embedded
// (`embedded_ai_domains.txt` dentro questo package) e quello
// canonico in `data/ai-domains.txt` (servito via raw GitHub URL e
// usato dalla `RefreshAIList`) siano sempre identici.
//
// Senza questo test, un PR potrebbe aggiornare solo `data/` o solo
// `embedded_*` e i due divergerebbero silenziosamente.
//
// Per aggiornare la lista, modifica `data/ai-domains.txt` e poi:
//
//	cp data/ai-domains.txt internal/classify/embedded_ai_domains.txt
func TestEmbeddedMatchesDataFile(t *testing.T) {
	canonical, err := os.ReadFile("../../data/ai-domains.txt")
	if err != nil {
		t.Skipf("data/ai-domains.txt non leggibile (probabilmente run isolato): %v", err)
	}
	if string(canonical) != embeddedAIDomains {
		t.Fatalf("embedded_ai_domains.txt e' divergente da data/ai-domains.txt.\n" +
			"Per riallineare: cp data/ai-domains.txt internal/classify/embedded_ai_domains.txt")
	}
}

// TestParseAIList sanity check sul parser.
func TestParseAIList(t *testing.T) {
	in := `# header
# === sezione ===
foo.com
  bar.com

# commento
foo.com
BAZ.com
`
	got := parseAIList(in)
	want := []string{"foo.com", "bar.com", "baz.com"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d (%v)", len(got), len(want), got)
	}
	for i, d := range want {
		if got[i] != d {
			t.Errorf("[%d] got %q want %q", i, got[i], d)
		}
	}
}
