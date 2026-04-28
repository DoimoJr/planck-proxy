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
	"fmt"
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
	Fase     = "1.6"
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

const indexHTML = `<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<title>Planck Proxy v2 — Phase 1.6</title>
<style>
  body { font-family: 'Segoe UI', Tahoma, sans-serif; padding: 40px; max-width: 760px; margin: 0 auto; background: #1a1d23; color: #e0e0e0; line-height: 1.6; }
  h1 { color: #b77dd4; margin-bottom: 8px; }
  .tag { display: inline-block; background: #252a33; padding: 3px 10px; border-radius: 4px; font-size: 0.85em; color: #888; }
  code { background: #252a33; padding: 2px 6px; border-radius: 3px; font-family: Consolas, monospace; }
  pre { background: #252a33; padding: 10px 12px; border-radius: 4px; overflow-x: auto; }
  .status { color: #27ae60; font-weight: 600; }
  ul { padding-left: 20px; }
</style>
</head>
<body>
<h1>Planck Proxy v2</h1>
<p class="tag">Phase 1.6 — persistenza file-based + recovery NDJSON</p>

<p class="status">Backend Go in ascolto.</p>

<p>Proxy attivo con applicazione blocchi (403 su domini in blocklist o
in modo allowlist non-matching, dominiIgnorati passano sempre, pausa
globale blocca tutto). API REST GET+POST disponibili, auth HTTP Basic
opzionale (default off; abilitabile via /api/settings/update).
La persistenza disco arriva in 1.6, la UI completa in 1.7.</p>

<h3>Endpoint web (porta 9999)</h3>
<ul>
<li><code>GET /api/version</code> — metadata binario</li>
<li><code>GET /api/config</code> — boot data: titolo, modo, liste AI/sistema, mappa studenti</li>
<li><code>GET /api/history</code> — snapshot completo per idratazione UI</li>
<li><code>GET /api/session/status</code> — stato sessione + durata calcolata</li>
<li><code>GET /api/settings</code> — config completa (password mascherata)</li>
<li><code>GET /api/sessioni</code> — archivio sessioni (vuoto in 1.4)</li>
<li><code>GET /api/presets</code> — preset blocklist (vuoto in 1.4)</li>
<li><code>GET /api/classi</code> — combo classe+lab (vuoto in 1.4)</li>
<li><code>GET /api/stream</code> — SSE: <code>traffic</code> + <code>alive</code> in tempo reale</li>
</ul>

<h3>Proxy (porta 9090)</h3>
<ul>
<li>HTTP forwarding via URL assoluto + HTTPS via CONNECT</li>
<li><code>GET /_alive</code> watchdog keepalive</li>
</ul>

<h3>Smoke test</h3>
<pre>curl http://localhost:9999/api/config | head -c 500
curl http://localhost:9999/api/history
curl -N http://localhost:9999/api/stream &amp;
curl -x http://localhost:9090 http://example.com/</pre>
</body>
</html>`

// indexHandler serve la pagina HTML di benvenuto.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	fmt.Fprint(w, indexHTML)
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
	mux.HandleFunc("/", indexHandler)
	api.Register(mux)

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
