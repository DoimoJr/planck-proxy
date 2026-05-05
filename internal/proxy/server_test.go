package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// stubRecorder e' una Recorder no-op che registra le chiamate per
// assertion in test, senza dipendenza da internal/state.
type stubRecorder struct {
	mu          sync.Mutex
	traffic     []trafficCall
	aliveCount  int
	aliveLastIP string
	// blocca: lista di stringhe che, se contained nel dominio, fanno
	// ritornare true a DominioBloccato. Vuoto = niente bloccato.
	blocca []string
}

type trafficCall struct {
	IP, Metodo, Dominio string
	Blocked             bool
	Tipo                classify.Tipo
}

func (s *stubRecorder) RegistraTraffic(ip, metodo, dominio string, blocked bool, tipo classify.Tipo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traffic = append(s.traffic, trafficCall{ip, metodo, dominio, blocked, tipo})
}

func (s *stubRecorder) RegistraAlive(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliveCount++
	s.aliveLastIP = ip
}

func (s *stubRecorder) DominioBloccato(dominio, _ string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.blocca {
		if b != "" && contains(dominio, b) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func (s *stubRecorder) trafficCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.traffic)
}

// TestGetClientIP copre i formati di RemoteAddr che vediamo in pratica:
// IPv4 con porta, IPv6 mapped (formato canonico Go con brackets),
// IPv6 puro, fallback senza porta.
func TestGetClientIP(t *testing.T) {
	casi := map[string]string{
		"192.168.1.50:54321":         "192.168.1.50",
		"127.0.0.1:9999":             "127.0.0.1",
		"[::ffff:192.168.1.50]:1234": "192.168.1.50", // canonico per IPv4-mapped
		"[::1]:8080":                 "::1",
		"hostonly":                   "hostonly",
	}
	for in, atteso := range casi {
		if got := getClientIP(in); got != atteso {
			t.Errorf("getClientIP(%q) = %q, atteso %q", in, got, atteso)
		}
	}
}

// TestHandleAlive verifica che l'endpoint watchdog risponda 200 + "ok"
// e che chiami recorder.RegistraAlive con l'IP del client.
func TestHandleAlive(t *testing.T) {
	rec := &stubRecorder{}
	s := New(":0", rec)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_alive", nil)
	req.RemoteAddr = "192.168.1.50:54321"

	s.handle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, atteso 200", w.Code)
	}
	if got := w.Body.String(); got != "ok" {
		t.Errorf("body = %q, atteso \"ok\"", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, atteso text/plain*", ct)
	}
	if rec.aliveCount != 1 {
		t.Errorf("recorder.aliveCount = %d, atteso 1", rec.aliveCount)
	}
	if rec.aliveLastIP != "192.168.1.50" {
		t.Errorf("recorder.aliveLastIP = %q, atteso 192.168.1.50", rec.aliveLastIP)
	}
}

// TestHandleDirectNonAlive verifica che richieste dirette al proxy diverse
// da /_alive vengano respinte con 400, senza chiamare il recorder.
func TestHandleDirectNonAlive(t *testing.T) {
	rec := &stubRecorder{}
	s := New(":0", rec)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/qualcosa-di-non-proxy", nil)
	req.RemoteAddr = "192.168.1.50:54321"

	s.handle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, atteso 400 per richiesta non-proxy", w.Code)
	}
	if rec.trafficCount() != 0 || rec.aliveCount != 0 {
		t.Errorf("recorder non doveva essere chiamato (traffic=%d, alive=%d)",
			rec.trafficCount(), rec.aliveCount)
	}
}

// TestHandleHTTPMissingHostname verifica che una richiesta HTTP forwarding
// senza hostname (degenere) ritorni 400, senza chiamare il recorder.
func TestHandleHTTPMissingHostname(t *testing.T) {
	rec := &stubRecorder{}
	s := New(":0", rec)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http:///path", nil)
	req.RemoteAddr = "192.168.1.50:54321"

	s.handleHTTP(w, req, "192.168.1.50")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, atteso 400 per hostname mancante", w.Code)
	}
	if rec.trafficCount() != 0 {
		t.Errorf("recorder.trafficCount = %d, atteso 0 (hostname invalido)", rec.trafficCount())
	}
}

// TestHandleHTTPBlocked verifica che una richiesta verso un dominio
// considerato "bloccato" dal recorder ritorni 403 + pagina HTML, e che
// il recorder venga chiamato con blocked=true.
func TestHandleHTTPBlocked(t *testing.T) {
	rec := &stubRecorder{blocca: []string{"instagram"}}
	s := New(":0", rec)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://www.instagram.com/", nil)
	req.RemoteAddr = "192.168.1.50:54321"

	s.handleHTTP(w, req, "192.168.1.50")

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, atteso 403", w.Code)
	}
	if rec.trafficCount() != 1 {
		t.Fatalf("trafficCount = %d, atteso 1", rec.trafficCount())
	}
	if !rec.traffic[0].Blocked {
		t.Errorf("traffic[0].Blocked = false, atteso true")
	}
}
