// analizzasessione classifica TUTTI i domini visti in una sessione del DB
// usando classify.Classifica, e propone candidati per:
//   - lista AI: domini classificati "utente" che hanno pattern tipici AI
//   - lista sistema: domini "utente" che hanno pattern tipici di telemetria/CDN/ad
//
// Usage:
//
//	go run ./tools/analizzasessione <DB> <SESSION_ID>
//
// Output stdout: 3 sezioni (AI rilevati / Sistema rilevati / Utente — top
// con count >= 5). L'utente legge l'output e decide cosa aggiungere alle
// liste in classify/domains.go.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

type domStat struct {
	Dominio string
	Count   int
	Tipo    classify.Tipo
}

// Pattern euristici per suggerire candidati:
//   - AI: nomi che contengono token tipici di servizi AI/LLM
//   - Sistema: telemetria, beacon, ad tech, CDN
var (
	aiHints = []string{
		"gpt", "openai", "anthropic", "claude", "perplexity", "gemini",
		"copilot", "bard", "llama", "mistral", "huggingface", "cohere",
		"stability", "replicate", "deepseek", "groq", "you.com",
		"chatgpt", "phind", "character.ai", "poe.com", "kimi",
		"wenxin", "tongyi", "doubao", "tencent.ai",
	}
	systemHints = []string{
		"telemetry", "beacon", "metrics", "analytics", "tracker",
		"tracking", "sentry", "cdn-", "fwd.", "log.", "logger",
		"-cdn", "events.", "stats.", "rum.", "perf.",
		"clients.", "client-", "settings-services", "push-services",
		"diagnostics", "crash.", "ads.", "ad-", "doubleclick",
		"-prod-cdn", "-edge", "edge-", "wpad",
	}
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: analizzasessione <DB> <SESSION_ID>")
		os.Exit(2)
	}
	dbPath := os.Args[1]
	sessID, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		log.Fatalf("session id non valido: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=journal_mode(wal)")
	if err != nil {
		log.Fatalf("apri db: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT dominio, COUNT(*) AS n FROM entries
		WHERE sessione_id = ? GROUP BY dominio ORDER BY n DESC`, sessID)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var stats []domStat
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			continue
		}
		stats = append(stats, domStat{
			Dominio: d,
			Count:   n,
			Tipo:    classify.Classifica(d),
		})
	}
	if len(stats) == 0 {
		fmt.Fprintln(os.Stderr, "Nessun dato per session_id =", sessID)
		os.Exit(1)
	}

	// Buckets.
	var ai, sys, utenti []domStat
	for _, s := range stats {
		switch s.Tipo {
		case classify.TipoAI:
			ai = append(ai, s)
		case classify.TipoSistema:
			sys = append(sys, s)
		default:
			utenti = append(utenti, s)
		}
	}

	// Top utenti: candidati per nuova classificazione AI/sistema.
	var aiCandidates, sysCandidates []domStat
	for _, s := range utenti {
		dl := strings.ToLower(s.Dominio)
		isAi := containsAny(dl, aiHints)
		isSys := containsAny(dl, systemHints) || hasManySubdomains(dl)
		if isAi {
			aiCandidates = append(aiCandidates, s)
		} else if isSys {
			sysCandidates = append(sysCandidates, s)
		}
	}

	fmt.Printf("=== Sessione %d — %d domini distinti, %d entries totali ===\n\n",
		sessID, len(stats), totalCount(stats))

	printGroup("[GIA' AI]    ", ai, 50)
	printGroup("[GIA' SISTEMA]", sys, 30)

	fmt.Println("\n=== CANDIDATI da rivedere ===")
	printGroup("[NUOVO AI?]    ", aiCandidates, 40)
	printGroup("[NUOVO SISTEMA?]", sysCandidates, 40)

	fmt.Println("\n=== TOP UTENTE (resto, primi 30 per request count) ===")
	sort.Slice(utenti, func(i, j int) bool { return utenti[i].Count > utenti[j].Count })
	skip := mapDomains(append(aiCandidates, sysCandidates...))
	limit := 30
	for _, s := range utenti {
		if skip[s.Dominio] {
			continue
		}
		fmt.Printf("  %5d  %s\n", s.Count, s.Dominio)
		limit--
		if limit == 0 {
			break
		}
	}
}

func printGroup(label string, list []domStat, max int) {
	if len(list) == 0 {
		return
	}
	fmt.Printf("\n--- %s — %d domini ---\n", label, len(list))
	for i, s := range list {
		if i >= max {
			fmt.Printf("  ... + %d altri\n", len(list)-max)
			break
		}
		fmt.Printf("  %5d  %s\n", s.Count, s.Dominio)
	}
}

func containsAny(s string, hints []string) bool {
	for _, h := range hints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

func hasManySubdomains(d string) bool {
	// Heuristic: 4+ punti spesso = sottodomini di servizio (telemetry, push, etc.)
	return strings.Count(d, ".") >= 4
}

func totalCount(list []domStat) int {
	n := 0
	for _, s := range list {
		n += s.Count
	}
	return n
}

func mapDomains(list []domStat) map[string]bool {
	m := map[string]bool{}
	for _, s := range list {
		m[s.Dominio] = true
	}
	return m
}
