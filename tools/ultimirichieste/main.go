// ultimirichieste mostra le ultime N richieste di un IP in una sessione.
//
// Usage: go run ./tools/ultimirichieste <DB> <SESSION_ID> <IP_SUFFIX> [N]
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: ultimirichieste <DB> <SESSION_ID> <IP_SUFFIX> [N]")
		os.Exit(2)
	}
	n := 20
	if len(os.Args) >= 5 {
		if v, err := strconv.Atoi(os.Args[4]); err == nil {
			n = v
		}
	}
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?mode=ro")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT ora, ip, metodo, dominio, tipo, blocked
		FROM entries
		WHERE sessione_id = ? AND ip LIKE ?
		ORDER BY ts DESC LIMIT ?`,
		os.Args[2], "%."+os.Args[3], n)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	fmt.Printf("%-19s | %-15s | %-6s | %-50s | %-7s | %s\n", "ora", "ip", "metodo", "dominio", "tipo", "blk")
	fmt.Println("---")
	for rows.Next() {
		var ora, ip, metodo, dominio, tipo string
		var blocked int
		rows.Scan(&ora, &ip, &metodo, &dominio, &tipo, &blocked)
		blk := ""
		if blocked == 1 {
			blk = "✓"
		}
		fmt.Printf("%-19s | %-15s | %-6s | %-50s | %-7s | %s\n", ora, ip, metodo, dominio, tipo, blk)
	}
}
