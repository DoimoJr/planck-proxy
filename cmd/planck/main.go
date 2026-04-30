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
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/proxy"
	"github.com/DoimoJr/planck-proxy/internal/scripts"
	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
	"github.com/DoimoJr/planck-proxy/internal/watchdog/builtin"
	"github.com/DoimoJr/planck-proxy/internal/web"
)

const (
	Versione = "2.1.0"
	Fase     = "stable"
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

// openBrowserAppMode lancia il browser in modalita' "app" (finestra senza
// barra URL, senza tab, senza menu) puntato sull'URL fornito. Comportamento
// "single-page-app desktop look" per Planck.
//
// Su Windows preferisce Edge (sempre installato su 10/11), poi Chrome,
// poi fallback alla shell `start <url>` che apre il default browser
// in modalita' tab normale.
//
// Skipped se PLANCK_NO_BROWSER=1 (utile per server headless).
// Skipped silenziosamente se il sub-process fallisce (Planck continua).
func openBrowserAppMode(url string) {
	if os.Getenv("PLANCK_NO_BROWSER") == "1" {
		return
	}

	// Candidati Edge (preferito perche' presente su ogni Win10/11)
	edgePaths := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		edgePaths = append([]string{p}, edgePaths...)
	}
	for _, p := range edgePaths {
		if _, err := os.Stat(p); err == nil {
			if err := exec.Command(p, "--app="+url).Start(); err == nil {
				log.Printf("Aperta finestra Edge in modalita' app su %s", url)
				return
			}
		}
	}

	// Candidati Chrome
	chromePaths := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	if p, err := exec.LookPath("chrome.exe"); err == nil {
		chromePaths = append([]string{p}, chromePaths...)
	}
	for _, p := range chromePaths {
		if _, err := os.Stat(p); err == nil {
			if err := exec.Command(p, "--app="+url).Start(); err == nil {
				log.Printf("Aperta finestra Chrome in modalita' app su %s", url)
				return
			}
		}
	}

	// Fallback: default browser via cmd start (apre come tab normale)
	if err := exec.Command("cmd", "/c", "start", "", url).Start(); err == nil {
		log.Printf("Aperto default browser su %s", url)
		return
	}
	log.Printf("Nessun browser disponibile per aprire %s automaticamente", url)
}

func main() {
	webPort := envOrDefault("PLANCK_WEB_PORT", "9999")
	proxyPort := envOrDefault("PLANCK_PROXY_PORT", "9090")
	dataDir := dataDirDefault()

	// Persistenza: apri SQLite (creandolo se non esiste) e applica le
	// migrations dello schema.
	dbPath := filepath.Join(dataDir, "planck.db")
	st0, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("Errore apertura DB %q: %v", dbPath, err)
	}
	defer st0.Close()

	// One-shot: importa i dati legacy file-based (Phase 1.6) nel DB se
	// presenti. Idempotente via marker in kv.
	if imported, err := st0.MigrateFromFiles(dataDir); err != nil {
		log.Printf("Migrazione v1 -> SQLite fallita: %v", err)
	} else if len(imported) > 0 {
		log.Printf("Migrazione v1 -> SQLite: importati %d file", len(imported))
	}

	// Crash recovery: chiudi forzatamente eventuali sessioni rimaste aperte
	// al crash precedente (sessione_fine NULL).
	if id, inizio, found, err := st0.SessionFindActive(); err != nil {
		log.Printf("Recovery sessione: %v", err)
	} else if found {
		now := time.Now().UTC()
		if err := st0.SessionClose(id, now.Format(time.RFC3339), 0, now.UnixMilli()); err != nil {
			log.Printf("Recovery sessione %d (inizio %s): %v", id, inizio, err)
		} else {
			log.Printf("Recuperata sessione interrotta id=%d (inizio %s)", id, inizio)
		}
	}

	// Wiring: broker (SSE) → state (mutazioni + persistenza).
	broker := web.NewBroker()
	st := state.NewWithStore(broker, st0)

	// Genera proxy_on.bat / proxy_off.bat con IP+porta corretti per la
	// rete corrente. Sovrascrive ogni boot (riflette IP che potrebbe
	// cambiare con DHCP). Override IP via env var PLANCK_LAN_IP se
	// l'auto-detection sbaglia su macchine multi-interfaccia.
	lanIP := os.Getenv("PLANCK_LAN_IP")
	if lanIP == "" {
		lanIP = scripts.LocalLANIP()
	}
	proxyPortInt, _ := strconv.Atoi(proxyPort)
	webPortInt, _ := strconv.Atoi(webPort)
	if onPath, offPath, err := scripts.Generate(dataDir, Versione, lanIP, proxyPortInt, webPortInt); err != nil {
		log.Printf("Generazione script studenti fallita: %v", err)
	} else {
		log.Printf("Script studenti pronti: %s + %s (IP %s:%d, web :%d)", onPath, offPath, lanIP, proxyPortInt, webPortInt)
	}
	// Esponi il LAN IP via state cosi' la UI sa quale IP usare per
	// "Distribuisci proxy" senza dover chiedere ogni volta.
	st.SetLanIP(lanIP)

	// Watchdog plugins (Phase 5): registra i built-in.
	wdReg := watchdog.NewRegistry()
	for _, p := range []watchdog.WatchdogPlugin{
		builtin.UsbPlugin{},
		builtin.ProcessPlugin{},
	} {
		if err := wdReg.Register(p); err != nil {
			log.Printf("Watchdog: registrazione plugin %s: %v", p.ID(), err)
		}
	}
	st.SetWatchdogRegistry(wdReg)

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

	// Apri il browser (Edge in modalita' app) dopo un breve delay,
	// per dare tempo ai server HTTP di completare il bind.
	go func() {
		time.Sleep(400 * time.Millisecond)
		openBrowserAppMode("http://localhost:" + webPort)
	}()

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
