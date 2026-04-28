// API HTTP REST: in Phase 1.4 solo GET endpoint read-only per servire UI
// e idratare lo state al primo load. Le mutazioni (POST) arriveranno in 1.5.
//
// Convenzioni:
//   - GET: ritornano il payload direttamente (no wrapping {ok,data})
//   - In errore: writeError → {ok:false, error:"...", code:"..."}
//   - Method check: requireMethod risponde 405 se metodo non consentito
package web

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/DoimoJr/planck-proxy/internal/state"
)

// API raggruppa state + broker e i suoi handler HTTP.
// Una sola istanza per processo: viene costruita in main e i suoi
// handler vengono registrati via Register().
type API struct {
	state   *state.State
	broker  *Broker
	version string
	fase    string
}

// NewAPI costruisce l'oggetto API con le dipendenze necessarie.
//
//	version e' la versione del binario (es. "2.0.0-phase1")
//	fase    e' l'identificatore di fase corrente (es. "1.4")
//
// Entrambe finiscono nel payload di /api/version.
func NewAPI(s *state.State, b *Broker, version, fase string) *API {
	return &API{state: s, broker: b, version: version, fase: fase}
}

// Register monta tutti gli handler dell'API sul mux fornito.
// In Phase 1.4 sono tutti GET; le mutazioni arriveranno in 1.5.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/version", a.handleVersion)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/history", a.handleHistory)
	mux.HandleFunc("/api/session/status", a.handleSessionStatus)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/sessioni", a.handleSessioni)
	mux.HandleFunc("/api/presets", a.handlePresets)
	mux.HandleFunc("/api/classi", a.handleClassi)
	mux.HandleFunc("/api/stream", a.broker.HandleStream)
}

// ============================================================
// Helpers
// ============================================================

// writeJSON serializza data come JSON con status code, settando il
// Content-Type. Errori di Encode sono loggati ma non propagati (non si puo'
// piu' cambiare la response a quel punto).
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("API: errore encode JSON: %v", err)
	}
}

// writeError ritorna {ok:false, error, code} con lo status indicato.
// Shape standard per gli errori (vedi SPEC §5.1).
func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
		"code":  code,
	})
}

// requireMethod scrive 405 se il metodo non e' quello atteso, e ritorna
// false. Il caller deve fare `if !requireMethod(...) { return }`.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "Metodo non consentito", "METHOD_NOT_ALLOWED")
		return false
	}
	return true
}

// ============================================================
// Handler
// ============================================================

// handleVersion espone metadata del binario.
func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": a.version,
		"stack":   "go",
		"fase":    a.fase,
	})
}

// handleConfig ritorna i dati di boot per il client (titolo, modo, liste
// di classificazione AI/sistema, mappa studenti, presets, classi).
func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.ConfigSnapshotData())
}

// handleHistory ritorna lo snapshot completo dello stato per idratazione UI:
// entries del ring buffer, blocklist, sessione, deadline, alive map.
func (a *API) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.HistorySnapshotData())
}

// handleSessionStatus ritorna lo stato della sessione corrente (attiva/ferma,
// inizio, fine, durata in secondi, numero richieste registrate, ecc.).
func (a *API) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.SessionStatusData())
}

// handleSettings ritorna l'intera config corrente (porte, auth, modo,
// titolo, classe, soglia, dominiIgnorati). Password mai serializzata
// (vedi SPEC §7.2.4).
func (a *API) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.SettingsSnapshotData())
}

// handleSessioni ritorna la lista delle sessioni archiviate sul disco.
// Phase 1.4: stub vuoto (la persistenza arriva in 1.6 col layer file/SQLite).
func (a *API) handleSessioni(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessioni": []string{}})
}

// handlePresets ritorna la lista dei preset blocklist salvati.
// Phase 1.4: stub vuoto.
func (a *API) handlePresets(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": []string{}})
}

// handleClassi ritorna la lista delle combinazioni (classe, lab) salvate.
// Phase 1.4: stub vuoto.
func (a *API) handleClassi(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"classi": []state.Combo{}})
}
