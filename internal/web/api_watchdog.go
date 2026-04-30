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
// rispettivi script PowerShell con IP/porta + config del plugin
// (denylist/allowlist) sostituiti dal template. Senza auth (stesso
// modello dei .vbs che gli studenti scaricano).

// Config typed (rispecchia builtin/usb.go e builtin/process.go).
type usbPluginConfig struct {
	IgnoredClasses []string `json:"ignoredClasses"`
	AllowVidPid    []string `json:"allowVidPid"`
}
type processPluginConfig struct {
	DenyList []string `json:"denyList"`
}

func (a *API) handleScriptWatchdogUsb(w http.ResponseWriter, r *http.Request) {
	cfg, ip, port, ok := a.prepareWatchdogScript(w, r, "usb", "usb_watchdog.ps1")
	if !ok {
		return
	}
	var c usbPluginConfig
	_ = json.Unmarshal(cfg, &c)
	_, _ = w.Write([]byte(scripts.WatchdogUsbScript(ip, port, c.IgnoredClasses, c.AllowVidPid)))
}

func (a *API) handleScriptWatchdogProcess(w http.ResponseWriter, r *http.Request) {
	cfg, ip, port, ok := a.prepareWatchdogScript(w, r, "process", "process_watchdog.ps1")
	if !ok {
		return
	}
	var c processPluginConfig
	_ = json.Unmarshal(cfg, &c)
	_, _ = w.Write([]byte(scripts.WatchdogProcessScript(ip, port, c.DenyList)))
}

// prepareWatchdogScript fa i check comuni a tutti gli endpoint .ps1:
// metodo, plugin enabled (404 altrimenti), risolve IP+port. Ritorna
// la config raw JSON del plugin (da deserializzare in tipo specifico)
// + IP + port + ok=true. Se ok=false, ha gia' scritto la risposta.
func (a *API) prepareWatchdogScript(w http.ResponseWriter, r *http.Request, pluginID, filename string) ([]byte, string, int, bool) {
	if !requireMethod(w, r, http.MethodGet) {
		return nil, "", 0, false
	}

	cfgs, err := a.state.LoadWatchdogConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return nil, "", 0, false
	}
	var rawCfg []byte
	enabled := false
	for _, c := range cfgs {
		if c.ID == pluginID {
			enabled = c.Enabled
			rawCfg = []byte(c.Config)
			break
		}
	}
	if !enabled {
		http.Error(w, "watchdog plugin "+pluginID+" non abilitato", http.StatusNotFound)
		return nil, "", 0, false
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
	return rawCfg, ip, port, true
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
