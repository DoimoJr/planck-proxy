// checkstudente: dump dei domini di un IP specifico durante una sessione,
// con classificazione corrente. Identifica candidati AI nei domini "utente".
//
// Usage: go run ./tools/checkstudente <DB> <SESSION_ID> <IP_SUFFIX>
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

var aiHints = []string{
	"gpt", "openai", "anthropic", "claude", "perplexity", "gemini",
	"copilot", "bard", "llama", "mistral", "huggingface", "cohere",
	"stability", "replicate", "deepseek", "groq", "you.com",
	"chatgpt", "phind", "character.ai", "poe.com",
	"meta.ai", "moonshot", "qwen", "tongyi", "doubao",
	"chat.", ".ai/", ".chat", "/chat",
	"writesonic", "jasper.ai", "rytr.me", "sudowrite",
	"neuralwriter", "wordtune", "quillbot", "grammarly",
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "Usage: checkstudente <DB> <SESSION_ID> <IP_SUFFIX>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?mode=ro")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	sessID := os.Args[2]
	ipSuffix := os.Args[3]
	rows, err := db.Query(`SELECT DISTINCT ip FROM entries WHERE sessione_id = ? AND ip LIKE ?`, sessID, "%."+ipSuffix)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var ips []string
	for rows.Next() {
		var ip string
		rows.Scan(&ip)
		ips = append(ips, ip)
	}
	if len(ips) == 0 {
		fmt.Println("Nessun IP che termina in", ipSuffix, "trovato nella sessione")
		return
	}
	fmt.Println("IP trovati:", strings.Join(ips, ", "))
	for _, ip := range ips {
		dRows, _ := db.Query(`SELECT dominio, COUNT(*) AS n FROM entries WHERE sessione_id = ? AND ip = ? GROUP BY dominio ORDER BY n DESC`, sessID, ip)
		fmt.Printf("\n=== %s — domini distinti ===\n", ip)
		var ai []string
		var aiCandidates []string
		var utenti []string
		var sysCnt int
		for dRows.Next() {
			var d string
			var n int
			dRows.Scan(&d, &n)
			tipo := classify.Classifica(d)
			line := fmt.Sprintf("  %5d  %s", n, d)
			switch tipo {
			case classify.TipoAI:
				ai = append(ai, line)
			case classify.TipoSistema:
				sysCnt++
			default:
				dl := strings.ToLower(d)
				match := false
				for _, h := range aiHints {
					if strings.Contains(dl, h) {
						match = true
						break
					}
				}
				if match {
					aiCandidates = append(aiCandidates, line)
				} else {
					utenti = append(utenti, line)
				}
			}
		}
		dRows.Close()
		if len(ai) > 0 {
			fmt.Println("\n[AI gia' riconosciuti]")
			for _, l := range ai {
				fmt.Println(l)
			}
		}
		if len(aiCandidates) > 0 {
			fmt.Println("\n[CANDIDATI AI da rivedere — pattern AI nel nome]")
			for _, l := range aiCandidates {
				fmt.Println(l)
			}
		}
		fmt.Printf("\n[SISTEMA] %d domini (skip)\n", sysCnt)
		fmt.Println("\n[UTENTE — tutto il resto]")
		max := 60
		for _, l := range utenti {
			fmt.Println(l)
			max--
			if max == 0 {
				fmt.Printf("  ... + %d altri\n", len(utenti)-60)
				break
			}
		}
	}
}
