// Planck Proxy v2 — entry point del binario.
//
// Avvia in parallelo:
//   - il proxy HTTP/HTTPS (default :9090) che instrada il traffico studenti
//   - il web server (default :9999) che serve UI + API REST
//
// In Phase 1.2 il proxy fa solo forwarding + watchdog (niente blocchi,
// niente persistenza, niente broadcast SSE). Lo state condiviso tra i due
// server arrivera' in Phase 1.3.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/DoimoJr/planck-proxy/internal/classify"
	"github.com/DoimoJr/planck-proxy/internal/proxy"
	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/web"
)

const Versione = "2.0.0-phase1"

const indexHTML = `<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<title>Planck Proxy v2 — Phase 1</title>
<style>
  body { font-family: 'Segoe UI', Tahoma, sans-serif; padding: 40px; max-width: 720px; margin: 0 auto; background: #1a1d23; color: #e0e0e0; line-height: 1.6; }
  h1 { color: #b77dd4; margin-bottom: 8px; }
  .tag { display: inline-block; background: #252a33; padding: 3px 10px; border-radius: 4px; font-size: 0.85em; color: #888; }
  code { background: #252a33; padding: 2px 6px; border-radius: 3px; font-family: Consolas, monospace; }
  .status { color: #27ae60; font-weight: 600; }
  ul { padding-left: 20px; }
  a { color: #b77dd4; }
</style>
</head>
<body>
<h1>Planck Proxy v2</h1>
<p class="tag">Phase 1.3 — state condiviso + SSE</p>

<p class="status">Backend Go in ascolto.</p>

<p>Proxy HTTP/HTTPS attivo, state condiviso in piedi, broadcast SSE su
<code>/api/stream</code>. Le API REST complete arrivano in 1.4, la UI
completa in 1.7.</p>

<p>Per testare lo stream SSE:</p>
<pre>curl -N http://localhost:9999/api/stream</pre>
<p>Mentre lo stream e' aperto, ogni richiesta proxata produce un messaggio
<code>traffic</code>; ogni ping <code>/_alive</code> produce un messaggio
<code>alive</code>.</p>

<h3>Endpoint web (porta 9999)</h3>
<ul>
<li><code>GET /</code> &mdash; questa pagina</li>
<li><code>GET /api/version</code> &mdash; versione corrente in JSON</li>
<li><code>GET /api/classifica?dominio=X</code> &mdash; classifica un dominio (smoke test, sara' rimosso in 1.4)</li>
<li><code>GET /api/stream</code> &mdash; SSE: messaggi <code>traffic</code> e <code>alive</code> in tempo reale</li>
</ul>

<h3>Proxy (porta 9090)</h3>
<ul>
<li><code>GET http://example.com/...</code> &mdash; HTTP forwarding (URL assoluto)</li>
<li><code>CONNECT example.com:443</code> &mdash; HTTPS tunneling</li>
<li><code>GET /_alive</code> (diretto al proxy) &mdash; watchdog keepalive</li>
</ul>
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

// versionHandler espone la versione del binario in JSON.
func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	fmt.Fprintf(w, `{"version":"%s","stack":"go","fase":"1.3"}`, Versione)
}

// classificaHandler espone la classificazione di un dominio passato come
// query param. Smoke test integrato (verra' rimosso in Phase 1.4).
func classificaHandler(w http.ResponseWriter, r *http.Request) {
	dominio := r.URL.Query().Get("dominio")
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if dominio == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"ok":false,"error":"parametro 'dominio' richiesto"}`)
		return
	}
	tipo := classify.Classifica(dominio)
	fmt.Fprintf(w, `{"ok":true,"dominio":%q,"tipo":%q}`, dominio, tipo)
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

	// Wiring: broker (SSE) → state (registra eventi + broadcasta) → proxy (chiama state)
	broker := web.NewBroker()
	st := state.New(broker)
	proxySrv := proxy.New(":"+proxyPort, st)

	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/version", versionHandler)
	mux.HandleFunc("/api/classifica", classificaHandler)
	mux.HandleFunc("/api/stream", broker.HandleStream)

	log.Printf("Planck Proxy v%s", Versione)
	log.Printf("Web:   http://localhost:%s", webPort)
	log.Printf("Proxy: http://localhost:%s", proxyPort)
	log.Printf("In ascolto...")

	// Lancio i due server in parallelo. Se uno dei due fallisce all'avvio
	// (es. porta occupata), l'intero processo termina.
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
