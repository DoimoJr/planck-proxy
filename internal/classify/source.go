package classify

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// AIDomainsURL e' l'URL di default da cui Planck pulla la lista AI
// aggiornata. Punta al raw GitHub del repo: chiunque puo' aprire una
// PR sul branch main per aggiungere domini, e tutti i Planck installati
// in giro la prendono al prossimo refresh.
const AIDomainsURL = "https://raw.githubusercontent.com/DoimoJr/planck-proxy/main/data/ai-domains.txt"

//go:embed embedded_ai_domains.txt
var embeddedAIDomains string

// AISource indica da dove la lista corrente arriva.
type AISource string

const (
	AISourceEmbedded AISource = "embedded" // built-in, dal binario stesso
	AISourceCache    AISource = "cache"    // dal file in dataDir
	AISourceRemote   AISource = "remote"   // appena scaricata
)

// AIList e' lo stato corrente della lista AI: domini + metadata.
// Pubblicato come atomic.Pointer per letture senza lock dai chiamanti
// hot-path (Classifica).
type AIList struct {
	Domains   []string  `json:"-"`
	Count     int       `json:"count"`
	Source    AISource  `json:"source"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url,omitempty"`
}

var (
	aiListPtr atomic.Pointer[AIList]
	aiMu      sync.Mutex // serialize gli update
)

func init() {
	// All'init carica la lista embedded come fallback iniziale.
	domains := parseAIList(embeddedAIDomains)
	aiListPtr.Store(&AIList{
		Domains:   domains,
		Count:     len(domains),
		Source:    AISourceEmbedded,
		UpdatedAt: time.Now(),
	})
}

// CurrentAIList ritorna la lista corrente (snapshot, non muta).
func CurrentAIList() *AIList {
	return aiListPtr.Load()
}

// AIDomains ritorna lo slice dei domini AI correnti. Sostituisce la
// vecchia variabile package-level `DominiAI`.
func AIDomains() []string {
	l := aiListPtr.Load()
	if l == nil {
		return nil
	}
	return l.Domains
}

// LoadAICache prova a caricare la lista dal file di cache nel data
// dir. Se ok, la promuove come lista corrente. No-op silenzioso se
// il file non esiste.
func LoadAICache(dataDir string) error {
	path := filepath.Join(dataDir, "ai-domains-cache.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		return err // probabilmente file non esiste
	}
	domains := parseAIList(string(body))
	if len(domains) == 0 {
		return fmt.Errorf("cache vuota")
	}
	stat, _ := os.Stat(path)
	mtime := time.Now()
	if stat != nil {
		mtime = stat.ModTime()
	}
	aiMu.Lock()
	defer aiMu.Unlock()
	aiListPtr.Store(&AIList{
		Domains:   domains,
		Count:     len(domains),
		Source:    AISourceCache,
		UpdatedAt: mtime,
	})
	return nil
}

// RefreshAIList scarica la lista da `url` (timeout 10s), la valida,
// la salva nella cache `<dataDir>/ai-domains-cache.txt` e la
// promuove come lista corrente. Se il fetch fallisce o ritorna
// roba assurda, lascia la lista corrente inalterata e ritorna errore.
func RefreshAIList(url, dataDir string) error {
	if url == "" {
		url = AIDomainsURL
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	domains := parseAIList(string(body))
	if len(domains) < 10 {
		// Sanity check: una lista con < 10 domini e' probabilmente
		// corrotta / un errore HTML dietro un proxy / un 404 page.
		return fmt.Errorf("lista AI sospetta: %d domini", len(domains))
	}

	aiMu.Lock()
	defer aiMu.Unlock()

	// Salva cache solo se dataDir e' settato (caso non-NoOp).
	if dataDir != "" {
		path := filepath.Join(dataDir, "ai-domains-cache.txt")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			// Cache write failure non e' fatale: prosegui col promote.
			fmt.Printf("classify: warning cache write %s: %v\n", path, err)
		}
	}

	aiListPtr.Store(&AIList{
		Domains:   domains,
		Count:     len(domains),
		Source:    AISourceRemote,
		UpdatedAt: time.Now(),
		URL:       url,
	})
	return nil
}

// parseAIList parsa il formato di ai-domains.txt:
//   - una entry (dominio) per riga
//   - righe vuote ignorate
//   - righe che iniziano con `#` ignorate (commenti / sezioni)
//   - whitespace tagliato
//
// Ritorna la lista deduplicata, ordine preservato dalla prima
// occorrenza.
func parseAIList(content string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ToLower(line)
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}
