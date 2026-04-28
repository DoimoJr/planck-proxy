package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DoimoJr/planck-proxy/internal/classify"
	"github.com/DoimoJr/planck-proxy/internal/state"
)

// helper: costruisce API con state nuovo + broker per i test.
func newTestAPI() (*API, *state.State) {
	b := NewBroker()
	s := state.New(b)
	return NewAPI(s, b, "test-version", "1.4-test"), s
}

func TestVersion(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	api.handleVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, atteso 200", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body non e' JSON valido: %v\n%s", err, rec.Body.String())
	}
	if got["version"] != "test-version" || got["stack"] != "go" || got["fase"] != "1.4-test" {
		t.Errorf("payload sbagliato: %+v", got)
	}
}

func TestVersionMethodNotAllowed(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/version", nil)
	api.handleVersion(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, atteso 405", rec.Code)
	}
	if a := rec.Header().Get("Allow"); a != http.MethodGet {
		t.Errorf("Allow header = %q, atteso GET", a)
	}
}

func TestConfig(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	api.handleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got state.ConfigSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if got.Modo != "blocklist" {
		t.Errorf("modo default = %q", got.Modo)
	}
	if len(got.DominiAI) < 50 {
		t.Errorf("DominiAI = %d, atteso >=50", len(got.DominiAI))
	}
}

func TestHistory(t *testing.T) {
	api, s := newTestAPI()
	s.RegistraTraffic("ip1", "GET", "x.com", false, classify.TipoUtente)
	s.RegistraAlive("ip2")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	api.handleHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got state.HistorySnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if len(got.Entries) != 1 {
		t.Errorf("entries = %d, atteso 1", len(got.Entries))
	}
	if len(got.Alive) != 1 {
		t.Errorf("alive = %d, atteso 1", len(got.Alive))
	}
	if got.SessioneAttiva {
		t.Errorf("SessioneAttiva = true, atteso false (default)")
	}
}

func TestSessionStatus(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/session/status", nil)
	api.handleSessionStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got state.SessionStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if got.SessioneAttiva {
		t.Errorf("default SessioneAttiva = true, atteso false")
	}
	if got.DurataSec != 0 {
		t.Errorf("durata sessione mai avviata = %d, atteso 0", got.DurataSec)
	}
}

func TestSettingsPasswordMasked(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	api.handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Verifica che la chiave "password" abbia valore vuoto nel JSON serializzato.
	body := rec.Body.String()
	if !contains(body, `"password":""`) {
		t.Errorf("settings JSON non mascherato; body=%s", body)
	}
}

func TestSessioniStubVuoto(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessioni", nil)
	api.handleSessioni(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !contains(rec.Body.String(), `"sessioni":[]`) {
		t.Errorf("payload non e' lista vuota: %s", rec.Body.String())
	}
}

// contains e' un helper minimo per evitare di importare strings nel test.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
