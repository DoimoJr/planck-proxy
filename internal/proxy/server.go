// Package proxy implementa il server proxy HTTP/HTTPS che instrada il
// traffico dei PC studenti.
//
// Comportamento:
//   - HTTP forwarding: i browser configurati col proxy mandano richieste
//     con URL assoluto ("GET http://example.com/path"). Il proxy parsa,
//     classifica il dominio, fa il forwarding all'origin con net/http.Client.
//     Nessun MITM, nessuna ispezione del body.
//   - HTTPS via CONNECT: il browser apre un tunnel TCP al proxy con metodo
//     CONNECT (es. "CONNECT example.com:443"). Il proxy classifica
//     l'hostname, apre TCP verso l'origin, hijacka la connessione client e
//     copia bytes bidirezionali fino a EOF. Niente decrypt, niente cert.
//   - /_alive (watchdog): richiesta diretta al proxy (non proxata) dallo
//     script proxy_on.bat sui PC studenti. Per ora ritorna solo "ok"; in
//     Phase 1.3 aggiornera' la state.aliveMap e fara' broadcast SSE.
//
// In Phase 1.2 il proxy NON applica blocchi: tutto il traffico passa.
// I blocchi arriveranno in Phase 1.4 quando avremo le API e lo state
// condiviso (in 1.3).
package proxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// Recorder e' l'interfaccia che il proxy usa per:
//   - Registrare gli eventi di traffico e watchdog (Phase 1.3)
//   - Decidere se un dominio va bloccato con 403 (Phase 1.5)
//
// Tipicamente implementata da *state.State, ma astratta per semplificare
// i test (vedi server_test.go).
type Recorder interface {
	RegistraTraffic(ip, metodo, dominio string, blocked bool, tipo classify.Tipo)
	RegistraAlive(ip string)
	DominioBloccato(dominio, clientIP string) bool
}

// paginaBloccata e' l'HTML servito allo studente sui domini bloccati.
// In Phase 1.7 verra' embedded da public/blocked.html; per ora inline.
const paginaBloccata = `<!DOCTYPE html><html lang="it"><head><meta charset="UTF-8">` +
	`<title>Accesso bloccato</title>` +
	`<style>body{font-family:'Segoe UI',sans-serif;text-align:center;padding:80px;background:#1a1d23;color:#e0e0e0}` +
	`h1{color:#e74c3c;font-size:2em}p{color:#888;margin-top:20px}</style></head>` +
	`<body><h1>Accesso bloccato dal docente</h1>` +
	`<p>Il dominio richiesto non e' consentito durante questa sessione.</p></body></html>`

// Server e' il proxy HTTP/HTTPS in ascolto su una porta.
type Server struct {
	addr     string
	srv      *http.Server
	recorder Recorder
}

// New costruisce un nuovo proxy server in ascolto su addr (es. ":9090")
// che registra gli eventi su `recorder`.
// Il server non viene avviato finche' non si chiama Start().
func New(addr string, recorder Recorder) *Server {
	s := &Server{addr: addr, recorder: recorder}
	s.srv = &http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(s.handle),
		// ReadTimeout / WriteTimeout intentionally NOT set: i tunnel HTTPS
		// CONNECT possono restare aperti a lungo (es. WebSocket sopra TLS),
		// e v1 non aveva timeout sui connect socket.
	}
	return s
}

// Start avvia il listener (chiamata bloccante). Tipico uso: in goroutine.
// Ritorna http.ErrServerClosed se chiuso via Stop, altri errori per fallimenti.
func (s *Server) Start() error {
	log.Printf("Proxy in ascolto su %s", s.addr)
	return s.srv.ListenAndServe()
}

// Stop chiude il proxy in modo immediato (no graceful — connessioni in corso
// vengono chiuse). Per Phase 1.2 e' sufficiente; in fasi successive si potra'
// passare a srv.Shutdown(ctx) con un context di timeout.
func (s *Server) Stop() error {
	return s.srv.Close()
}

// handle e' il dispatcher principale. Distingue tre casi:
//   - r.Method == "CONNECT" → tunneling HTTPS (handleConnect)
//   - r.URL.Scheme == ""    → richiesta diretta al proxy (solo /_alive accettato)
//   - r.URL.Scheme == "http" → proxy HTTP forwarding (handleHTTP)
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	ipClient := getClientIP(r.RemoteAddr)

	if r.Method == http.MethodConnect {
		s.handleConnect(w, r, ipClient)
		return
	}

	// Richiesta non-proxy diretta al proxy port
	if r.URL.Scheme == "" {
		if r.URL.Path == "/_alive" {
			s.handleAlive(w, r, ipClient)
			return
		}
		http.Error(w, "Il proxy accetta solo richieste in forwarding o GET /_alive",
			http.StatusBadRequest)
		return
	}

	s.handleHTTP(w, r, ipClient)
}

// rispondiBloccato scrive una risposta 403 con la pagina HTML di blocco.
// Usata per le richieste HTTP normali (per HTTPS si scrive direttamente
// sul socket prima del tunneling — vedi handleConnect).
func rispondiBloccato(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(paginaBloccata))
}

// handleAlive risponde al ping watchdog dello studente.
// Aggiorna aliveMap dello state e broadcasta `{type:"alive",...}` via SSE.
func (s *Server) handleAlive(w http.ResponseWriter, r *http.Request, ip string) {
	s.recorder.RegistraAlive(ip)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", "2")
	w.Header().Set("Connection", "close")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleHTTP forwarda una richiesta HTTP normale (URL assoluto stile proxy).
// Costruisce una nuova richiesta verso l'origin con http.Client, copia
// header e body, ritorna la response al client. Registra l'evento sullo
// state (logged + broadcast SSE).
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request, ip string) {
	dominio := r.URL.Hostname()
	if dominio == "" {
		http.Error(w, "Hostname mancante nell'URL", http.StatusBadRequest)
		return
	}

	tipo := classify.Classifica(dominio)

	// Check blocco (Phase 1.5): se il dominio e' bloccato, registra come
	// blocked=true e rispondi con la pagina 403, niente forwarding.
	if s.recorder.DominioBloccato(dominio, ip) {
		s.recorder.RegistraTraffic(ip, r.Method, dominio, true, tipo)
		rispondiBloccato(w)
		return
	}
	s.recorder.RegistraTraffic(ip, r.Method, dominio, false, tipo)

	// Header pulito: rimuovo gli hop-by-hop "proxy-*"
	headers := make(http.Header, len(r.Header))
	for k, vv := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "proxy-") {
			continue
		}
		headers[k] = vv
	}
	headers.Set("Connection", "close")

	fwReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, "Errore costruzione richiesta: "+err.Error(), http.StatusBadGateway)
		return
	}
	fwReq.Header = headers

	client := &http.Client{
		Timeout: 15 * time.Second,
		// Niente redirect-follow automatico: il browser deve vedere la 30x.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(fwReq)
	if err != nil {
		http.Error(w, "Impossibile raggiungere "+dominio+": "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleConnect implementa il tunneling HTTPS. Il browser ha gia' mandato
// "CONNECT host:port HTTP/1.1"; noi rispondiamo "200 Connection Established"
// e poi ci limitiamo a copiare i byte tra client <-> origin in entrambe le
// direzioni.
//
// Nessuna ispezione del payload: i bytes sono TLS opachi.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request, ip string) {
	host := r.URL.Host // formato "hostname:port"
	dominio, _, err := net.SplitHostPort(host)
	if err != nil {
		// Fallback: usa l'intero host come dominio (caso anomalo, di solito CONNECT include la porta)
		dominio = host
	}

	tipo := classify.Classifica(dominio)

	// Check blocco (Phase 1.5): per HTTPS rispondiamo "HTTP/1.1 403" sul
	// client socket prima dell'handshake TLS. Il browser mostrera' un
	// errore di connessione (non puo' renderizzare la pagina HTML perche'
	// si aspettava TLS), ma il blocco e' effettivo e visibile in UI.
	if s.recorder.DominioBloccato(dominio, ip) {
		s.recorder.RegistraTraffic(ip, "HTTPS", dominio, true, tipo)
		_, _ = w.Write([]byte("HTTP/1.1 403 Forbidden\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"Connection: close\r\n\r\n"))
		_, _ = w.Write([]byte(paginaBloccata))
		return
	}
	s.recorder.RegistraTraffic(ip, "HTTPS", dominio, false, tipo)

	target, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		http.Error(w, "Impossibile raggiungere "+host+": "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = target.Close()
		http.Error(w, "Hijacking non supportato dal server HTTP", http.StatusInternalServerError)
		return
	}
	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		_ = target.Close()
		log.Printf("Errore hijacking: %v", err)
		return
	}

	// Notifico al client che il tunnel e' pronto.
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = clientConn.Close()
		_ = target.Close()
		return
	}

	// Bidirectional copy. Quando una direzione si chiude, chiude anche l'altra
	// connessione cosi' la goroutine corrispondente esce.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// bufrw.Reader puo' contenere bytes gia' letti dall'http server prima
		// dell'hijack — drainage trasparente.
		_, _ = io.Copy(target, bufrw)
		_ = target.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, target)
		_ = clientConn.Close()
	}()
	wg.Wait()
}

// getClientIP estrae l'IPv4 da un RemoteAddr "ip:port", rimuovendo il prefisso
// IPv6-mapped "::ffff:" che Go aggiunge per socket dual-stack su Linux/Windows.
func getClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Fallback: nessuna porta -> usa la stringa cosi' com'e'
		host = remoteAddr
	}
	return strings.TrimPrefix(host, "::ffff:")
}
