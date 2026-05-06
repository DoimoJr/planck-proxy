// rmsession elimina una sessione (entries + watchdog_events tramite
// CASCADE) da un planck.db. Usa: go run ./tools/rmsession <DB> <ID>
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: rmsession <DB> <SESSION_ID>")
		os.Exit(2)
	}
	id, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		log.Fatalf("id non valido: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+os.Args[1])
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		log.Fatalf("fk on: %v", err)
	}
	res, err := db.Exec(`DELETE FROM sessioni WHERE id = ?`, id)
	if err != nil {
		log.Fatalf("delete: %v", err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("Eliminate %d sessione (id=%d). Le entries + watchdog_events sono state cancellate via CASCADE.\n", n, id)
}
