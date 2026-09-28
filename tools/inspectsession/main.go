// inspectsession dump rapido del contenuto di sessioni di un planck.db.
// Helper di debug.
//
// Usage:
//
//	go run ./tools/inspectsession <DB>
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: inspectsession <DB>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?mode=ro")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, sessione_inizio, COALESCE(sessione_fine,'NULL'), COALESCE(durata_sec,0), COALESCE(titolo,''),
		(SELECT COUNT(*) FROM entries WHERE sessione_id = sessioni.id),
		(SELECT COUNT(*) FROM watchdog_events WHERE sessione_id = sessioni.id)
		FROM sessioni ORDER BY id`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	fmt.Println("ID | inizio                | fine                  | durata   | titolo                  | entries | wd")
	fmt.Println("---+----------------------+----------------------+----------+-------------------------+---------+----")
	n := 0
	for rows.Next() {
		var id, durata, ne, nw int64
		var inizio, fine, titolo string
		rows.Scan(&id, &inizio, &fine, &durata, &titolo, &ne, &nw)
		fmt.Printf("%-3d| %-20s | %-20s | %6ds  | %-23q | %7d | %3d\n", id, inizio, fine, durata, titolo, ne, nw)
		n++
	}
	if n == 0 {
		fmt.Println("(nessuna sessione)")
	}
}
