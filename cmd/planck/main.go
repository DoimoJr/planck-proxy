// Planck Proxy v2 — Phase 0 skeleton
//
// Entry point del binario. In Phase 0 e' uno stub: avvia un web server
// minimale su :9999 che serve una pagina di benvenuto. La logica reale
// (proxy, classificazione, sessioni, Veyon) arriva nelle fasi successive
// secondo SPEC.md sezione 8.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

const Versione = "2.0.0-phase0"

const indexHTML = `<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<title>Planck Proxy v2 — Phase 0</title>
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
<p class="tag">Phase 0 — skeleton</p>

<p class="status">Backend Go in ascolto.</p>

<p>Questo e' lo scheletro iniziale del rewrite v2 (vedi <code>SPEC.md</code>
nella radice del repo). Le funzionalita' reali (proxy, classificazione,
sessioni, integrazione Veyon, dashboard) arriveranno con le fasi
successive.</p>

<h3>Fasi successive</h3>
<ul>
<li>Phase 1: porting backend + Monitor sempre attivo</li>
<li>Phase 2: persistenza SQLite</li>
<li>Phase 3-4: integrazione Veyon</li>
<li>Phase 8: release v2.0.0</li>
</ul>

<h3>Endpoint disponibili</h3>
<ul>
<li><code>GET /</code> &mdash; questa pagina</li>
<li><code>GET /api/version</code> &mdash; versione corrente in JSON</li>
<li><code>GET /api/classifica?dominio=X</code> &mdash; classifica un dominio (smoke test, sara' rimosso in 1.4)</li>
</ul>
</body>
</html>`

// indexHandler serve la pagina HTML di benvenuto del Phase 0.
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
	fmt.Fprintf(w, `{"version":"%s","stack":"go","fase":"1"}`, Versione)
}

// classificaHandler espone la classificazione di un dominio passato come
// query param. Smoke test integrato per verificare che il package classify
// funzioni end-to-end (sara' rimosso in Phase 1.4 quando l'API completa
// sara' in piedi).
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/version", versionHandler)
	mux.HandleFunc("/api/classifica", classificaHandler)

	log.Printf("Planck Proxy v%s", Versione)
	log.Printf("Web: http://localhost:%s", webPort)
	log.Printf("In ascolto...")

	addr := ":" + webPort
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Errore avvio server web: %v", err)
	}
}
