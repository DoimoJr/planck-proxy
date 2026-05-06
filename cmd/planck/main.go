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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/discover"
	"github.com/DoimoJr/planck-proxy/internal/proxy"
	"github.com/DoimoJr/planck-proxy/internal/scripts"
	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/store"
	"github.com/DoimoJr/planck-proxy/internal/sysutil"
	"github.com/DoimoJr/planck-proxy/internal/watchdog"
	"github.com/DoimoJr/planck-proxy/internal/watchdog/builtin"
	"github.com/DoimoJr/planck-proxy/internal/web"
)

const (
	Versione = "2.9.8"
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

	// Pulisci i singleton lock files lasciati da una precedente istanza
	// di Edge/Chrome che usava lo stesso --user-data-dir. Senza, Edge
	// "si attacca" a un'istanza esistente fantasma → cmd.Wait() ritorna
	// in pochi millisecondi → Planck fa os.Exit(0) prima ancora che la
	// finestra carichi → "pagina non raggiungibile" al primo avvio.
	// Sintomatico nei log: <50ms tra "Browser avviato" e "Browser chiuso".
	for _, lockName := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie", "lockfile"} {
		_ = os.Remove(filepath.Join(profileDir, lockName))
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cmd := exec.Command(p, "--app="+url, "--user-data-dir="+profileDir,
			"--no-first-run", "--no-default-browser-check")
		sysutil.HideConsoleWindow(cmd)
		startedAt := time.Now()
		if err := cmd.Start(); err != nil {
			log.Printf("Browser %s: errore avvio (%v), provo prossimo", p, err)
			continue
		}
		log.Printf("Browser avviato (%s, PID %d). Chiusura finestra → shutdown del server.", filepath.Base(p), cmd.Process.Pid)
		_ = cmd.Wait()
		elapsed := time.Since(startedAt)
		// Se Wait ritorna in <2s e' quasi certamente un caso "attached
		// to existing instance" — la finestra non e' nostra. Non
		// auto-spegnere Planck: l'utente la chiude dalla UI o killa
		// dal Task Manager. Logghiamo + restiamo vivi (return, niente
		// os.Exit). Il `wg.Wait()` in main mantiene il processo up.
		if elapsed < 2*time.Second {
			log.Printf("Browser exit prematuro (%v) — probabilmente attached to existing instance. Planck resta attivo, apri manualmente %s o killa il processo.", elapsed, url)
			return
		}
		log.Println("Browser chiuso, spengo Planck.")
		os.Exit(0)
	}

	log.Printf("Nessun browser app-mode trovato. Apri manualmente %s e termina Planck dalla UI quando hai finito.", url)
}

func main() {
	// Nascondi subito la finestra cmd allocata da Windows. Il binario
	// e' compilato come console subsystem (NO -H=windowsgui) per evitare
	// false positive Defender, ma l'utente vede solo un flash di ~50ms
	// prima dell'hide. Pareggio accettabile: console subsystem non
	// triggera SmartScreen/Defender come la GUI subsystem.
	sysutil.HideOwnConsole()

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

	// Single-instance: se al boot esiste un planck.pid lasciato da un'istanza
	// precedente (es. l'utente ha chiuso la finestra browser ma il processo
	// e' rimasto orfano per il caso "attached to existing instance"), killa
	// il vecchio PID e attendi che il kernel rilasci la porta. Niente piu'
	// "porta gia' in uso" al secondo avvio.
	pidPath := filepath.Join(dataDir, "planck.pid")
	if data, err := os.ReadFile(pidPath); err == nil {
		if oldPID, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && oldPID > 0 && oldPID != os.Getpid() {
			if proc, err := os.FindProcess(oldPID); err == nil {
				if err := proc.Kill(); err == nil {
					log.Printf("[boot] killed stale planck process pid=%d", oldPID)
					time.Sleep(800 * time.Millisecond) // attendi rilascio porta TCP
				}
			}
		}
		_ = os.Remove(pidPath)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		log.Printf("[boot] non riesco a scrivere planck.pid: %v", err)
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

	// Mappa studenti: scan TCP dell'intera SUBNET del docente (con la mask
	// vera dell'interfaccia di rete, non solo /24 hardcoded) per scoprire
	// i PC vivi (Veyon :11100 / SMB :445 / RPC :135). Il binario e'
	// portatile, quindi ad ogni boot lo scan rivaluta la LAN corrente.
	// La grid Live mostra solo IP che hanno risposto.
	//
	// Subnet detection: legge la maschera dell'interfaccia che ha lanIP.
	// Se la subnet e' enorme (> /22 = 1024 host), ricade su /24 attorno
	// a lanIP per evitare scan massivi.
	//
	// Avvio: scan iniziale sincrono + loop periodico ogni 30s.
	if sub := discover.LocalSubnet(lanIP); sub != nil {
		ones, _ := sub.Mask.Size()
		log.Printf("discover: subnet rilevata %s/%d (interfaccia col docente %s)", sub.IP, ones, lanIP)
	} else {
		log.Printf("discover: impossibile rilevare la subnet di %s, fallback /24", lanIP)
	}
	// Discover mode: setting persistito in state (toggle UI Impostazioni).
	// Default true (lab scolastici); env PLANCK_DISCOVER_VEYON_ONLY=0/1
	// override del valore persistito al boot.
	if v := os.Getenv("PLANCK_DISCOVER_VEYON_ONLY"); v == "0" || v == "1" {
		_, _, _ = st.UpdateSettings(map[string]any{"discoverVeyonOnly": v == "1"})
	}
	scanAndApply := func() {
		veyonOnly := st.DiscoverVeyonOnly()
		ips := discover.ScanSubnet(lanIP, 300*time.Millisecond, veyonOnly)
		if len(ips) > 0 {
			st.SetStudentiIPs(ips)
			log.Printf("discover: scan subnet di %s → %d PC vivi", lanIP, len(ips))
		} else {
			// Nessun PC trovato dallo scan: fallback su range .1-.30 cosi' la
			// grid mostra comunque le card placeholder (utile in setup nuovo
			// dove i PC studente non sono ancora accesi).
			fallback := discover.DefaultRange(lanIP)
			if len(fallback) > 0 {
				st.SetStudentiIPs(fallback)
				log.Printf("discover: scan vuoto, fallback range .%d-.%d (%d IP)",
					discover.DefaultFirst, discover.DefaultLast, len(fallback))
			}
		}
	}
	scanAndApply()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			scanAndApply()
		}
	}()

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

	// Bind ESPLICITO del listener web prima di lanciare browser/Serve:
	// `net.Listen` ritorna quando il TCP socket e' gia' in stato LISTEN,
	// quindi connessioni successive entrano nella backlog del kernel
	// anche prima che `http.Serve` cominci ad Accept. Cosi' Edge puo'
	// fare GET subito senza beccare "pagina non raggiungibile".
	log.Printf("[boot] bind web :%s ...", webPort)
	webListener, err := net.Listen("tcp", ":"+webPort)
	if err != nil {
		log.Fatalf("Errore bind porta web :%s: %v", webPort, err)
	}
	log.Printf("[boot] web bind OK")

	// Lancio i due server in parallelo. Se uno fallisce (es. porta occupata),
	// l'intero processo termina via log.Fatalf.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		log.Printf("[boot] http.Serve web :%s starting", webPort)
		if err := http.Serve(webListener, mux); err != nil {
			log.Fatalf("Errore avvio server web: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		log.Printf("[boot] proxy :%s starting", proxyPort)
		if err := proxySrv.Start(); err != nil {
			log.Fatalf("Errore avvio proxy: %v", err)
		}
	}()

	// Apri il browser DOPO che ENTRAMBE le porte sono pronte:
	// - :9999 (web) → la pagina UI
	// - :9090 (proxy) → se il PC docente ha proxy_on.bat attivo, Edge
	//   passa per 127.0.0.1:9090 anche per localhost:9999. Se quel
	//   listener non e' ancora up al primo GET, Edge mostra "pagina
	//   non raggiungibile". Aspettiamo TCP listen su entrambe le porte
	//   + un GET reale a /api/health prima di lanciare la finestra.
	go func() {
		webURL := "http://localhost:" + webPort
		log.Printf("[boot] waiting proxy :%s ...", proxyPort)
		okProxy := sysutil.WaitForPort("127.0.0.1:"+proxyPort, 15*time.Second)
		log.Printf("[boot] proxy :%s ready=%v", proxyPort, okProxy)
		log.Printf("[boot] waiting web /api/health ...")
		okWeb := sysutil.WaitForHTTP(webURL+"/api/health", 15*time.Second)
		log.Printf("[boot] web /api/health ready=%v", okWeb)
		// Margine extra: Edge a freddo crea il profilo --user-data-dir
		// e puo' fare la prima GET prima di settare la connessione.
		time.Sleep(500 * time.Millisecond)
		log.Printf("[boot] launching browser at %s", webURL)
		profileDir := filepath.Join(dataDir, ".planck-browser-profile")
		openBrowserAndWaitForClose(webURL, profileDir)
	}()

	wg.Wait()
}
