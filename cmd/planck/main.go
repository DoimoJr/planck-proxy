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

	"github.com/DoimoJr/planck-proxy/internal/discover"
	"github.com/DoimoJr/planck-proxy/internal/proxy"
	"github.com/DoimoJr/planck-proxy/internal/scripts"
	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
	"github.com/DoimoJr/planck-proxy/internal/watchdog/builtin"
	"github.com/DoimoJr/planck-proxy/internal/web"
)

const (
	Versione = "2.8.0"
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

// openBrowserAndWaitForClose lancia Edge (o Chrome) in modalita' app
// (finestra senza barra URL/tab/menu) puntato sull'URL fornito, ASPETTA
// che la finestra venga chiusa, poi termina il processo Planck con
// os.Exit(0). Risultato: chiudere la finestra dell'app = spegnere il
// server. Lifecycle "single-process" trasparente per l'utente.
//
// Trick chiave: `--user-data-dir=<path>` forza una nuova istanza
// isolata di Edge/Chrome, cosi' il sub-process e' dedicato alla nostra
// finestra (e non si attacca a un'istanza esistente del browser, dove
// `cmd.Wait()` ritornerebbe subito lasciando la finestra orfana).
//
// Su Windows preferisce Edge (sempre installato su 10/11), poi Chrome.
// Niente browser → log warning + return (Planck resta vivo, l'utente
// lo killa manualmente da Task Manager o spegne dalla UI).
//
// Skipped se PLANCK_NO_BROWSER=1: in modalita' headless niente browser
// e niente auto-shutdown.
func openBrowserAndWaitForClose(url, profileDir string) {
	if os.Getenv("PLANCK_NO_BROWSER") == "1" {
		return
	}

	// Candidati: Edge prima (sempre presente su Win10/11), poi Chrome.
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		candidates = append([]string{p}, candidates...)
	}
	if p, err := exec.LookPath("chrome.exe"); err == nil {
		candidates = append(candidates, p)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cmd := exec.Command(p, "--app="+url, "--user-data-dir="+profileDir)
		if err := cmd.Start(); err != nil {
			log.Printf("Browser %s: errore avvio (%v), provo prossimo", p, err)
			continue
		}
		log.Printf("Browser avviato (%s, PID %d). Chiusura finestra → shutdown del server.", filepath.Base(p), cmd.Process.Pid)
		_ = cmd.Wait()
		log.Println("Browser chiuso, spengo Planck.")
		os.Exit(0)
	}

	log.Printf("Nessun browser app-mode trovato. Apri manualmente %s e termina Planck dalla UI quando hai finito.", url)
}

func main() {
	webPort := envOrDefault("PLANCK_WEB_PORT", "9999")
	proxyPort := envOrDefault("PLANCK_PROXY_PORT", "9090")
	dataDir := dataDirDefault()

	// Redirect log a file: con subsystem GUI (Windows) il binario non ha
	// console attaccata e i log su stderr vanno persi. Scriviamo in
	// `planck.log` accanto al DB, troncato ad ogni boot (file di sessione
	// corrente, comodo per debug post-mortem).
	logPath := filepath.Join(dataDir, "planck.log")
	if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		// Niente defer Close: il processo termina con os.Exit, l'OS chiude.
	}

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

	// Veyon: reset stato precedente + auto-import dal veyon-cli del PC
	// docente corrente. Il binario Planck e' portatile su chiavetta
	// (l'utente lo porta in piu' laboratori), quindi ad ogni avvio
	// dobbiamo prendere la master key del laboratorio CORRENTE — mai
	// fidarsi di quella precedente residua su disco.
	_ = st.VeyonClear()
	if name, err := st.AutoImportVeyonKey(); err != nil {
		log.Printf("Veyon auto-import non riuscito (non critico, sara' disabled per questa sessione): %v", err)
	} else if name != "" {
		log.Printf("Veyon: master key '%s' importata da veyon-cli del laboratorio corrente", name)
	}

	// Mappa studenti: range fisso .1-.30 del /24 del docente. In-memory
	// only, rigenerato ad ogni boot (binario portatile → ogni laboratorio
	// ha il suo /24, niente residui dal lab precedente).
	if ips := discover.DefaultRange(lanIP); len(ips) > 0 {
		st.SetStudentiIPs(ips)
		log.Printf("discover: mappa studenti col range .%d-.%d del /24 di %s (%d IP)",
			discover.DefaultFirst, discover.DefaultLast, lanIP, len(ips))
	}

	// Watchdog plugins (Phase 5): registra i built-in.
	wdReg := watchdog.NewRegistry()
	for _, p := range []watchdog.WatchdogPlugin{
		builtin.UsbPlugin{},
		builtin.ProcessPlugin{},
		builtin.NetworkPlugin{},
	} {
		if err := wdReg.Register(p); err != nil {
			log.Printf("Watchdog: registrazione plugin %s: %v", p.ID(), err)
		}
	}
	st.SetWatchdogRegistry(wdReg)

	// Lista AI (Phase 6): tenta cache locale al boot, poi async tenta
	// fetch remote. Niente blocchi sul boot — se non c'e' internet,
	// classify resta sulla lista embedded (~129 domini canonici).
	st.LoadAICacheAtBoot()
	go func() {
		if _, err := st.RefreshAIListNow(); err != nil {
			log.Printf("classify: refresh AI list a boot fallito (uso cache/embedded): %v", err)
		}
	}()

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
	// per dare tempo ai server HTTP di completare il bind. La funzione
	// e' bloccante: aspetta che la finestra browser venga chiusa, poi
	// chiama os.Exit(0). Lifecycle "single-process": chiudi la finestra
	// = spegni Planck.
	go func() {
		time.Sleep(400 * time.Millisecond)
		profileDir := filepath.Join(dataDir, ".planck-browser-profile")
		openBrowserAndWaitForClose("http://localhost:"+webPort, profileDir)
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
