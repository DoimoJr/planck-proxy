package web

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/DoimoJr/planck-proxy/internal/scripts"
)

// ============================================================
// Watchdog API (Phase 5)
// ============================================================

// handleWatchdogPlugins ritorna la lista plugin disponibili (registrati
// dal binario al boot) + stato enabled/disabled e config.
func (a *API) handleWatchdogPlugins(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	cfg, err := a.state.LoadWatchdogConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": cfg})
}

type watchdogConfigBody struct {
	Plugin  string          `json:"plugin"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// handleWatchdogConfig accetta enable/disable + config di UN plugin.
func (a *API) handleWatchdogConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body watchdogConfigBody
	if err := decodeJSONBody(r, &body); err != nil || body.Plugin == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {plugin, enabled, [config]}", "BAD_BODY")
		return
	}
	cfg := []byte(body.Config)
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}
	if err := a.state.SaveWatchdogPluginConfig(body.Plugin, body.Enabled, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_PLUGIN")
		return
	}
	writeOK(w, nil)
}

type watchdogEventBody struct {
	Plugin  string         `json:"plugin"`
	Payload map[string]any `json:"payload"`
}

// handleWatchdogEvent riceve eventi dai watchdog scripts che girano sui
// PC studenti. Niente auth Basic (stesso trust model di /_alive — LAN).
// Lo studente IP viene preso da r.RemoteAddr (X-Forwarded-For ignorato).
func (a *API) handleWatchdogEvent(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body watchdogEventBody
	if err := decodeJSONBody(r, &body); err != nil || body.Plugin == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {plugin, payload}", "BAD_BODY")
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Strip IPv6 mapped IPv4 prefix.
	ip = strings.TrimPrefix(ip, "::ffff:")

	if err := a.state.RegistraWatchdogEvent(body.Plugin, ip, body.Payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_EVENT")
		return
	}
	writeOK(w, nil)
}

// handleWatchdogEvents lista gli ultimi N eventi per il pannello UI.
// Query params opzionali: plugin, ip, limit (default 100, max 1000).
func (a *API) handleWatchdogEvents(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	plugin := r.URL.Query().Get("plugin")
	ip := r.URL.Query().Get("ip")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		// Parsing minimale, non e' critico
		var n int
		_, _ = formatInt(&n, l)
		if n > 0 {
			limit = n
		}
	}
	events, err := a.state.ListaWatchdogEvents(plugin, ip, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// handleScriptWatchdogUsb e handleScriptWatchdogProcess servono i
// rispettivi script PowerShell con IP/porta del docente sostituiti.
// Senza auth (stesso modello dei .bat che gli studenti scaricano).
func (a *API) handleScriptWatchdogUsb(w http.ResponseWriter, r *http.Request) {
	a.serveWatchdogScript(w, r, "usb_watchdog.ps1", scripts.WatchdogUsbScript)
}
func (a *API) handleScriptWatchdogProcess(w http.ResponseWriter, r *http.Request) {
	a.serveWatchdogScript(w, r, "process_watchdog.ps1", scripts.WatchdogProcessScript)
}

func (a *API) serveWatchdogScript(w http.ResponseWriter, r *http.Request, filename string, gen func(string, int) string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	ip := a.state.LanIP()
	port := 9999
	if h := r.Host; h != "" {
		if _, p, err := net.SplitHostPort(h); err == nil {
			var n int
			_, _ = formatInt(&n, p)
			if n > 0 {
				port = n
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(gen(ip, port)))
}

// formatInt e' un mini-helper int-parse senza errori. Usato per limit/port
// dove "non valido" e' OK e cade su default.
func formatInt(out *int, s string) (int, error) {
	*out = 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		*out = *out*10 + int(c-'0')
	}
	return *out, nil
}
