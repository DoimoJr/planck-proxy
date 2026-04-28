// Planck Proxy v2 — entry point del binario.
//
// Avvia in parallelo:
//   - il proxy HTTP/HTTPS (default :9090) che instrada il traffico studenti
//   - il web server (default :9999) che serve UI + API REST + SSE
//
// Wiring: broker (SSE) -> state (mutazioni e snapshot) -> proxy (eventi)
//                                                     -> api (handler GET)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/DoimoJr/planck-proxy/internal/persist"
	"github.com/DoimoJr/planck-proxy/internal/proxy"
	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/web"
)

const (
	Versione = "2.0.0-phase1"
	Fase     = "1.7"
)

// dataDirDefault risolve la directory dati: env var PLANCK_DATA_DIR
// override, altrimenti la cartella dell'eseguibile, altrimenti CWD.
func dataDirDefault() string {
	if d := os.Getenv("PLANCK_DATA_DIR"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// envOrDefault legge una env var o ritorna il default fornito.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	webPort := envOrDefault("PLANCK_WEB_PORT", "9999")
	proxyPort := envOrDefault("PLANCK_PROXY_PORT", "9090")
	dataDir := dataDirDefault()

	// Persistenza: crea le sotto-directory necessarie e carica i file
	// esistenti (config.json, studenti.json, _blocked_domains.txt).
	store, err := persist.New(dataDir)
	if err != nil {
		log.Fatalf("Errore inizializzazione persistenza in %q: %v", dataDir, err)
	}

	// Wiring: broker (SSE) → state (mutazioni + persistenza)
	broker := web.NewBroker()
	st := state.NewWithStore(broker, store)

	// Crash recovery: se al boot esiste un NDJSON con dati, prova ad
	// archiviarlo come "recovered-<inizioISO>.json".
	if recovered, err := store.RecoverNDJSONIfAny(persist.ArchiveFile{
		SessioneInizio: "recovered-" + Versione,
		EsportatoAlle:  "boot",
		Titolo:         "Sessione recuperata da NDJSON residuo",
	}); err != nil {
		log.Printf("Recovery NDJSON fallita: %v", err)
	} else if recovered != "" {
		log.Printf("Sessione interrotta recuperata in archivio: %s", recovered)
	}

	// Proxy: registra eventi sullo state
	proxySrv := proxy.New(":"+proxyPort, st)

	// API HTTP: handler GET registrati su mux + /api/stream del broker
	api := web.NewAPI(st, broker, Versione, Fase)
	mux := http.NewServeMux()
	api.Register(mux) // monta /api/* + root "/" → static files embeddati

	log.Printf("Planck Proxy v%s (fase %s)", Versione, Fase)
	log.Printf("Web:   http://localhost:%s", webPort)
	log.Printf("Proxy: http://localhost:%s", proxyPort)
	log.Printf("Data:  %s", dataDir)
	log.Printf("In ascolto...")

	// Lancio i due server in parallelo. Se uno fallisce (es. porta occupata),
	// l'intero processo termina via log.Fatalf.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := http.ListenAndServe(":"+webPort, mux); err != nil {
			log.Fatalf("Errore avvio server web: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := proxySrv.Start(); err != nil {
			log.Fatalf("Errore avvio proxy: %v", err)
		}
	}()

	wg.Wait()
}
