// importsession copia le sessioni archiviate da un planck.db sorgente
// (es. quello di scuola in D:\proxy-old\) verso il planck.db destinazione
// del repo corrente. Le sessioni vengono RIMAPPATE su nuovi id (cosi' non
// collidono con quelle esistenti); entries + watchdog_events seguono.
//
// Usage:
//
//	go run ./tools/importsession <SOURCE_DB> <DEST_DB>
//
// Esempio:
//
//	go run ./tools/importsession D:/proxy-old/planck.db C:/Dev/consegna_portable/planck.db
//
// Sicurezza: il SOURCE viene aperto in read-only, il DEST in write. Il
// tool NON tocca le sessioni gia' presenti nel destinazione.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: importsession <SOURCE_DB> <DEST_DB>")
		os.Exit(2)
	}
	srcPath := os.Args[1]
	dstPath := os.Args[2]

	src, err := sql.Open("sqlite", "file:"+srcPath+"?mode=ro&_pragma=journal_mode(wal)")
	if err != nil {
		log.Fatalf("apri source %s: %v", srcPath, err)
	}
	defer src.Close()

	dst, err := sql.Open("sqlite", "file:"+dstPath)
	if err != nil {
		log.Fatalf("apri dest %s: %v", dstPath, err)
	}
	defer dst.Close()

	// Tutte le sessioni del source (anche quelle con sessione_fine NULL,
	// es. crash o "planck.exe chiuso senza Stop"). Per quelle si stima la
	// durata dall'ultima entry e si chiude col timestamp dell'ultima.
	rows, err := src.Query(`SELECT id, sessione_inizio, COALESCE(sessione_fine, ''), COALESCE(durata_sec, 0),
		COALESCE(classe, ''), COALESCE(lab, ''), COALESCE(titolo, ''), modo,
		studenti_snapshot, bloccati_snapshot, archiviata_at
		FROM sessioni ORDER BY id`)
	if err != nil {
		log.Fatalf("query sessioni: %v", err)
	}
	defer rows.Close()

	importate := 0
	totEntries := 0
	totWdEvents := 0
	for rows.Next() {
		var oldID, durata, archiviataAt int64
		var inizio, fine, classe, lab, titolo, modo, stud, blocc string
		if err := rows.Scan(&oldID, &inizio, &fine, &durata, &classe, &lab, &titolo, &modo, &stud, &blocc, &archiviataAt); err != nil {
			log.Printf("skip riga sessione: %v", err)
			continue
		}
		// Sessione orfana (chiusura mai avvenuta): chiudila a importazione,
		// stimando durata + fine dall'ultima entry.
		if fine == "" || durata == 0 {
			var lastTs int64
			var lastOra string
			if err := src.QueryRow(`SELECT COALESCE(MAX(ts),0), COALESCE(MAX(ora),'') FROM entries WHERE sessione_id = ?`, oldID).Scan(&lastTs, &lastOra); err == nil && lastTs > 0 {
				if fine == "" && lastOra != "" {
					// ora e' "YYYY-MM-DD HH:MM:SS" UTC; convertila in RFC3339 minimal.
					fine = strings.Replace(lastOra, " ", "T", 1) + "Z"
				}
				if durata == 0 {
					if t0, err := time.Parse(time.RFC3339, inizio); err == nil {
						durata = int64((time.UnixMilli(lastTs).Sub(t0)).Seconds())
						if durata < 0 {
							durata = 0
						}
					}
				}
				if archiviataAt == 0 {
					archiviataAt = lastTs
				}
				log.Printf("[orfana] sessione %d: stima fine=%s durata=%ds da ultima entry", oldID, fine, durata)
			}
		}
		res, err := dst.Exec(`INSERT INTO sessioni
			(sessione_inizio, sessione_fine, durata_sec, classe, lab, titolo, modo,
			 studenti_snapshot, bloccati_snapshot, archiviata_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			inizio, fine, durata, classe, lab, titolo, modo, stud, blocc, archiviataAt)
		if err != nil {
			log.Printf("insert sessione (oldID=%d) fallito: %v", oldID, err)
			continue
		}
		newID, _ := res.LastInsertId()
		log.Printf("Sessione %d → %d  inizio=%s  durata=%ds  titolo=%q", oldID, newID, inizio, durata, titolo)

		// Entries di quella sessione.
		eRows, err := src.Query(`SELECT ora, ts, ip, nome_studente, metodo, dominio, tipo, blocked, flagged
			FROM entries WHERE sessione_id = ?`, oldID)
		nE := 0
		if err == nil {
			for eRows.Next() {
				var ora, ip, metodo, dominio, tipo string
				var nomeStud sql.NullString
				var ts int64
				var blocked, flagged int
				if err := eRows.Scan(&ora, &ts, &ip, &nomeStud, &metodo, &dominio, &tipo, &blocked, &flagged); err != nil {
					continue
				}
				_, err := dst.Exec(`INSERT INTO entries
					(sessione_id, ora, ts, ip, nome_studente, metodo, dominio, tipo, blocked, flagged)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					newID, ora, ts, ip, nomeStud, metodo, dominio, tipo, blocked, flagged)
				if err == nil {
					nE++
				}
			}
			eRows.Close()
		}
		log.Printf("  entries: %d", nE)
		totEntries += nE

		// Watchdog events di quella sessione (table aggiunta in migration v2).
		wRows, err := src.Query(`SELECT plugin, ip, nome_studente, ts, severity, payload_json
			FROM watchdog_events WHERE sessione_id = ?`, oldID)
		nW := 0
		if err == nil {
			for wRows.Next() {
				var plugin, ip, severity, payload string
				var nomeStud sql.NullString
				var ts int64
				if err := wRows.Scan(&plugin, &ip, &nomeStud, &ts, &severity, &payload); err != nil {
					continue
				}
				_, err := dst.Exec(`INSERT INTO watchdog_events
					(sessione_id, plugin, ip, nome_studente, ts, severity, payload_json)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					newID, plugin, ip, nomeStud, ts, severity, payload)
				if err == nil {
					nW++
				}
			}
			wRows.Close()
		}
		log.Printf("  watchdog events: %d", nW)
		totWdEvents += nW
		importate++
	}

	fmt.Fprintf(os.Stderr, "\n=== Import completato ===\n")
	fmt.Fprintf(os.Stderr, "Sessioni:        %d\n", importate)
	fmt.Fprintf(os.Stderr, "Entries totali:  %d\n", totEntries)
	fmt.Fprintf(os.Stderr, "Watchdog events: %d\n", totWdEvents)
}
