// analizzaeventi: dump degli eventi watchdog di una sessione, raggruppati
// per plugin + severity + payload, per capire cosa e' rumore vs azione
// rilevante.
//
// Usage: go run ./tools/analizzaeventi <DB> <SESSION_ID>
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type eventoRow struct {
	Plugin   string
	IP       string
	Nome     string
	TS       int64
	Severity string
	Payload  map[string]any
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: analizzaeventi <DB> <SESSION_ID>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?mode=ro")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT plugin, ip, COALESCE(nome_studente, ''), ts, severity, payload_json
		FROM watchdog_events WHERE sessione_id = ? ORDER BY ts ASC`, os.Args[2])
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var all []eventoRow
	for rows.Next() {
		var ev eventoRow
		var payloadJSON string
		rows.Scan(&ev.Plugin, &ev.IP, &ev.Nome, &ev.TS, &ev.Severity, &payloadJSON)
		_ = json.Unmarshal([]byte(payloadJSON), &ev.Payload)
		all = append(all, ev)
	}
	if len(all) == 0 {
		fmt.Println("Nessun evento.")
		return
	}

	// 1) Aggregato per plugin + severity
	bySev := map[string]map[string]int{}
	for _, e := range all {
		if bySev[e.Plugin] == nil {
			bySev[e.Plugin] = map[string]int{}
		}
		bySev[e.Plugin][e.Severity]++
	}
	fmt.Printf("=== %d eventi totali nella sessione ===\n\n", len(all))
	fmt.Println("BREAKDOWN per plugin + severity:")
	plugins := keys(bySev)
	sort.Strings(plugins)
	for _, p := range plugins {
		fmt.Printf("  %-10s ", p)
		sevs := keys(bySev[p])
		sort.Strings(sevs)
		for _, s := range sevs {
			fmt.Printf(" %s=%d", s, bySev[p][s])
		}
		fmt.Println()
	}

	// 2) Aggregato per "tipo" di evento (payload[event] tipico)
	byType := map[string]int{}
	byTypeFirst := map[string]eventoRow{}
	for _, e := range all {
		t := signature(e)
		byType[t]++
		if _, ok := byTypeFirst[t]; !ok {
			byTypeFirst[t] = e
		}
	}
	type tk struct{ key string; n int }
	var tks []tk
	for k, n := range byType {
		tks = append(tks, tk{k, n})
	}
	sort.Slice(tks, func(i, j int) bool { return tks[i].n > tks[j].n })

	fmt.Println("\nTIPI DI EVENTO (signature plugin/severity/key payload):")
	for _, t := range tks {
		first := byTypeFirst[t.key]
		var sample string
		if b, err := json.Marshal(first.Payload); err == nil {
			sample = string(b)
			if len(sample) > 100 {
				sample = sample[:97] + "..."
			}
		}
		fmt.Printf("  [%4d] %s\n         sample: %s\n", t.n, t.key, sample)
	}

	// 3) Per IP: count eventi
	byIP := map[string]int{}
	byIPName := map[string]string{}
	for _, e := range all {
		byIP[e.IP]++
		if e.Nome != "" {
			byIPName[e.IP] = e.Nome
		}
	}
	type ipk struct {
		ip   string
		n    int
		nome string
	}
	var ipks []ipk
	for ip, n := range byIP {
		ipks = append(ipks, ipk{ip, n, byIPName[ip]})
	}
	sort.Slice(ipks, func(i, j int) bool { return ipks[i].n > ipks[j].n })
	fmt.Println("\nPER IP:")
	for _, k := range ipks {
		fmt.Printf("  %5d  %s  %s\n", k.n, k.ip, k.nome)
	}

	// 4) Severity warning/critical
	fmt.Println("\nEVENTI 'warning' / 'critical' (potenzialmente azione studente):")
	wc := 0
	for _, e := range all {
		if e.Severity == "warning" || e.Severity == "critical" {
			b, _ := json.Marshal(e.Payload)
			payload := string(b)
			if len(payload) > 80 {
				payload = payload[:77] + "..."
			}
			fmt.Printf("  [%s] %s %s | %s\n", e.Severity, e.IP, e.Plugin, payload)
			wc++
			if wc >= 30 {
				fmt.Println("  ... (limit 30)")
				break
			}
		}
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// signature crea una chiave "plugin/severity/keys-del-payload" per
// raggruppare eventi simili. Es: usb/info/{event,vendor} | process/warning/{name,pid}.
func signature(e eventoRow) string {
	var ks []string
	for k := range e.Payload {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return fmt.Sprintf("%s/%s/{%s}", e.Plugin, e.Severity, strings.Join(ks, ","))
}
