package web

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DoimoJr/planck-proxy/internal/state"
)

func TestRequireAuthDisabled(t *testing.T) {
	// Default state: auth disabilitata. Il middleware deve chiamare next senza check.
	s := state.New(NewBroker())
	called := false
	h := RequireAuth(s, func(w http.ResponseWriter, r *http.Request) { called = true })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	h(rec, req)

	if !called {
		t.Errorf("next non chiamato (auth disabled, atteso passthrough)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, atteso 200 default", rec.Code)
	}
}

func TestRequireAuthEnabledNoPasswordSet(t *testing.T) {
	// Auth abilitata ma nessuna password impostata: ogni request deve essere 401.
	s := state.New(NewBroker())
	s.UpdateSettings(map[string]any{"web.auth.enabled": true})

	called := false
	h := RequireAuth(s, func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	h(rec, req)

	if called {
		t.Errorf("next non doveva essere chiamato (no password set)")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, atteso 401", rec.Code)
	}
}

func TestRequireAuthSuccess(t *testing.T) {
	s := state.New(NewBroker())
	s.UpdateSettings(map[string]any{
		"web.auth.enabled":  true,
		"web.auth.user":     "docente",
		"web.auth.password": "segretissima",
	})

	called := false
	h := RequireAuth(s, func(w http.ResponseWriter, r *http.Request) { called = true })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	creds := base64.StdEncoding.EncodeToString([]byte("docente:segretissima"))
	req.Header.Set("Authorization", "Basic "+creds)
	h(rec, req)

	if !called {
		t.Errorf("next non chiamato (credenziali corrette)")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, atteso 200", rec.Code)
	}
}

func TestRequireAuthWrongPassword(t *testing.T) {
	s := state.New(NewBroker())
	s.UpdateSettings(map[string]any{
		"web.auth.enabled":  true,
		"web.auth.user":     "docente",
		"web.auth.password": "segretissima",
	})

	h := RequireAuth(s, func(w http.ResponseWriter, r *http.Request) {})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	creds := base64.StdEncoding.EncodeToString([]byte("docente:wrong"))
	req.Header.Set("Authorization", "Basic "+creds)
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, atteso 401 (password errata)", rec.Code)
	}
}

func TestRequireAuthMissingHeader(t *testing.T) {
	s := state.New(NewBroker())
	s.UpdateSettings(map[string]any{
		"web.auth.enabled":  true,
		"web.auth.user":     "docente",
		"web.auth.password": "segretissima",
	})

	h := RequireAuth(s, func(w http.ResponseWriter, r *http.Request) {})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, atteso 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("WWW-Authenticate mancante")
	}
}
