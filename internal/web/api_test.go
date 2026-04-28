package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DoimoJr/planck-proxy/internal/classify"
	"github.com/DoimoJr/planck-proxy/internal/state"
)

// helper: costruisce API + state nuovi per i test (auth disabilitata di default).
func newTestAPI() (*API, *state.State) {
	b := NewBroker()
	s := state.New(b)
	return NewAPI(s, b, "test-version", "1.5-test"), s
}

// post invia un POST con body JSON e ritorna response recorder.
func post(api *API, path string, body any, handler http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	bodyStr := ""
	if body != nil {
		b, _ := json.Marshal(body)
		bodyStr = string(b)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/json")
	handler(rec, req)
	return rec
}

// ============================================================
// GET (Phase 1.4 — verifica regressione)
// ============================================================

func TestVersion(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	api.handleVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body non JSON: %v", err)
	}
	if got["version"] != "test-version" || got["fase"] != "1.5-test" {
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
}

func TestConfig(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	api.handleConfig(rec, req)

	var got state.ConfigSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Modo != "blocklist" || len(got.DominiAI) < 50 {
		t.Errorf("payload non plausibile: %+v", got)
	}
}

func TestHistory(t *testing.T) {
	api, s := newTestAPI()
	s.RegistraTraffic("ip1", "GET", "x.com", false, classify.TipoUtente)
	s.RegistraAlive("ip2")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	api.handleHistory(rec, req)

	var got state.HistorySnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 || len(got.Alive) != 1 {
		t.Errorf("snapshot non plausibile: %+v", got)
	}
}

func TestSettingsPasswordMasked(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	api.handleSettings(rec, req)

	if !strings.Contains(rec.Body.String(), `"password":""`) {
		t.Errorf("password non mascherata: %s", rec.Body.String())
	}
}

// ============================================================
// POST mutations (Phase 1.5)
// ============================================================

func TestBlockUnblockFlow(t *testing.T) {
	api, s := newTestAPI()

	// Block
	rec := post(api, "/api/block", map[string]any{"dominio": "instagram.com"}, api.handleBlock)
	if rec.Code != http.StatusOK {
		t.Fatalf("block status = %d", rec.Code)
	}
	if !s.DominioBloccato("www.instagram.com") {
		t.Errorf("dopo block, www.instagram.com dovrebbe essere bloccato")
	}

	// Unblock
	rec = post(api, "/api/unblock", map[string]any{"dominio": "instagram.com"}, api.handleUnblock)
	if rec.Code != http.StatusOK {
		t.Fatalf("unblock status = %d", rec.Code)
	}
	if s.DominioBloccato("www.instagram.com") {
		t.Errorf("dopo unblock, non dovrebbe essere bloccato")
	}
}

func TestBlockBadBody(t *testing.T) {
	api, _ := newTestAPI()
	rec := post(api, "/api/block", map[string]any{}, api.handleBlock)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, atteso 400 per body vuoto", rec.Code)
	}
}

func TestPauseToggle(t *testing.T) {
	api, _ := newTestAPI()
	rec := post(api, "/api/pause/toggle", nil, api.handlePauseToggle)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["pausato"] != true {
		t.Errorf("dopo toggle iniziale, pausato = %v, atteso true", got["pausato"])
	}
	// Toggle again
	rec = post(api, "/api/pause/toggle", nil, api.handlePauseToggle)
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["pausato"] != false {
		t.Errorf("dopo secondo toggle, pausato = %v, atteso false", got["pausato"])
	}
}

func TestSessionStartStop(t *testing.T) {
	api, s := newTestAPI()

	// Start
	rec := post(api, "/api/session/start", nil, api.handleSessionStart)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d", rec.Code)
	}
	attiva, inizio, _ := s.SessioneStato()
	if !attiva || inizio == "" {
		t.Errorf("dopo start, attiva=%v inizio=%q", attiva, inizio)
	}

	// Stop
	rec = post(api, "/api/session/stop", nil, api.handleSessionStop)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d", rec.Code)
	}
	attiva, _, fine := s.SessioneStato()
	if attiva {
		t.Errorf("dopo stop, attiva = true (atteso false)")
	}
	if fine == "" {
		t.Errorf("dopo stop, sessioneFineISO vuoto")
	}
}

func TestDeadlineSet(t *testing.T) {
	api, _ := newTestAPI()
	rec := post(api, "/api/deadline/set", map[string]any{"time": "23:59"}, api.handleDeadlineSet)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if iso, _ := got["deadlineISO"].(string); iso == "" {
		t.Errorf("deadlineISO mancante: %v", got)
	}
}

func TestDeadlineSetBadFormat(t *testing.T) {
	api, _ := newTestAPI()
	rec := post(api, "/api/deadline/set", map[string]any{"time": "non-orario"}, api.handleDeadlineSet)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, atteso 400 per formato invalido", rec.Code)
	}
}

func TestSettingsUpdateMixed(t *testing.T) {
	api, s := newTestAPI()
	rec := post(api, "/api/settings/update", map[string]any{
		"titolo":   "Verifica 4DII",
		"modo":     "allowlist",
		"chiaveSconosciuta": "x",        // rejected
		"inattivitaSogliaSec": 120,      // updated
		"proxy.port": 9091,              // richiedeRiavvio
	}, api.handleSettingsUpdate)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)

	updated, _ := got["updated"].([]any)
	rejected, _ := got["rejected"].([]any)
	riavvio, _ := got["richiedeRiavvio"].([]any)

	if len(updated) != 4 {
		t.Errorf("updated count = %d, atteso 4: %v", len(updated), updated)
	}
	if len(rejected) != 1 {
		t.Errorf("rejected count = %d, atteso 1 (chiaveSconosciuta): %v", len(rejected), rejected)
	}
	if len(riavvio) != 1 {
		t.Errorf("richiedeRiavvio count = %d, atteso 1 (proxy.port): %v", len(riavvio), riavvio)
	}

	// Verifica che lo state sia stato effettivamente mutato
	cfg := s.ConfigSnapshotData()
	if cfg.Titolo != "Verifica 4DII" || cfg.Modo != "allowlist" || cfg.InattivitaSogliaSec != 120 {
		t.Errorf("state non mutato: %+v", cfg)
	}
}

func TestStudentCRUD(t *testing.T) {
	api, _ := newTestAPI()

	// Add
	post(api, "/api/students/set", map[string]any{"ip": "192.168.1.50", "nome": "Mario"}, api.handleStudentSet)
	post(api, "/api/students/set", map[string]any{"ip": "192.168.1.51", "nome": "Luca"}, api.handleStudentSet)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	api.handleConfig(rec, req)
	var cfg state.ConfigSnapshot
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if cfg.Studenti["192.168.1.50"] != "Mario" || cfg.Studenti["192.168.1.51"] != "Luca" {
		t.Errorf("studenti dopo SET: %+v", cfg.Studenti)
	}

	// Delete
	post(api, "/api/students/delete", map[string]any{"ip": "192.168.1.50"}, api.handleStudentDelete)
	rec = httptest.NewRecorder()
	api.handleConfig(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if _, ok := cfg.Studenti["192.168.1.50"]; ok {
		t.Errorf("dopo delete, Mario ancora presente")
	}

	// Clear
	post(api, "/api/students/clear", nil, api.handleStudentClear)
	rec = httptest.NewRecorder()
	api.handleConfig(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	json.Unmarshal(rec.Body.Bytes(), &cfg)
	if len(cfg.Studenti) != 0 {
		t.Errorf("dopo clear, studenti = %d, atteso 0", len(cfg.Studenti))
	}
}

func TestNotImplementedStub(t *testing.T) {
	api, _ := newTestAPI()
	rec := post(api, "/api/preset/save", map[string]any{"nome": "x"}, api.handleNotImplemented)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, atteso 501", rec.Code)
	}
}

func TestSessioniStubVuoto(t *testing.T) {
	api, _ := newTestAPI()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessioni", nil)
	api.handleSessioni(rec, req)
	if !strings.Contains(rec.Body.String(), `"sessioni":[]`) {
		t.Errorf("payload sessioni non vuoto: %s", rec.Body.String())
	}
}
