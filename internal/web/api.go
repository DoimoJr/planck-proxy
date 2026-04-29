// API HTTP REST.
//
// In Phase 1.4 sono stati introdotti gli endpoint GET di lettura.
// In Phase 1.5 si aggiungono le mutazioni POST + il middleware HTTP Basic.
// La persistenza disco arriva in Phase 1.6.
//
// Convenzioni:
//   - GET ritornano il payload direttamente
//   - POST ritornano `{ok:true, ...}` con eventuali campi extra
//   - Errori sempre `{ok:false, error, code}` con HTTP status appropriato
//   - Method check: requireMethod risponde 405 se metodo non consentito
//   - Tutti gli endpoint /api/* sono coperti da RequireAuth (no-op se
//     auth disabilitata in state)
package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/state"
	"github.com/DoimoJr/planck-proxy/internal/store"
)

// API raggruppa state + broker e i suoi handler HTTP.
type API struct {
	state   *state.State
	broker  *Broker
	version string
	fase    string
}

// NewAPI costruisce l'oggetto API con le dipendenze necessarie.
func NewAPI(s *state.State, b *Broker, version, fase string) *API {
	return &API{state: s, broker: b, version: version, fase: fase}
}

// Register monta tutti gli handler dell'API sul mux fornito.
// Tutti gli endpoint /api/* passano attraverso il middleware RequireAuth.
// I file statici della UI (root "/") usano lo stesso middleware: il
// browser ricevera' un challenge HTTP Basic prima di poter caricare
// la dashboard se l'auth e' abilitata.
//
// /api/stream e' un caso particolare: gestito dal broker, ma sempre
// dietro auth.
func (a *API) Register(mux *http.ServeMux) {
	auth := func(h http.HandlerFunc) http.HandlerFunc { return RequireAuth(a.state, h) }

	// Root catch-all → file statici embeddati (index.html, css, js).
	// http.FileServer serve "/" come index.html; gli altri path matchano
	// i file in public/. Il mux instrada qui tutto cio' che non matcha
	// /api/*.
	staticH := StaticHandler()
	mux.Handle("/", auth(func(w http.ResponseWriter, r *http.Request) {
		staticH.ServeHTTP(w, r)
	}))

	// Read-only (Phase 1.4)
	mux.HandleFunc("/api/version", auth(a.handleVersion))
	mux.HandleFunc("/api/config", auth(a.handleConfig))
	mux.HandleFunc("/api/history", auth(a.handleHistory))
	mux.HandleFunc("/api/session/status", auth(a.handleSessionStatus))
	mux.HandleFunc("/api/settings", auth(a.handleSettings))
	mux.HandleFunc("/api/sessioni", auth(a.handleSessioni))
	mux.HandleFunc("/api/presets", auth(a.handlePresets))
	mux.HandleFunc("/api/classi", auth(a.handleClassi))
	mux.HandleFunc("/api/stream", auth(a.broker.HandleStream))

	// Mutations (Phase 1.5)
	mux.HandleFunc("/api/block", auth(a.handleBlock))
	mux.HandleFunc("/api/unblock", auth(a.handleUnblock))
	mux.HandleFunc("/api/block-all-ai", auth(a.handleBlockAllAI))
	mux.HandleFunc("/api/unblock-all-ai", auth(a.handleUnblockAllAI))
	mux.HandleFunc("/api/clear-blocklist", auth(a.handleClearBlocklist))

	mux.HandleFunc("/api/session/start", auth(a.handleSessionStart))
	mux.HandleFunc("/api/session/stop", auth(a.handleSessionStop))

	mux.HandleFunc("/api/pause/toggle", auth(a.handlePauseToggle))
	mux.HandleFunc("/api/pause/on", auth(a.handlePauseOn))
	mux.HandleFunc("/api/pause/off", auth(a.handlePauseOff))

	mux.HandleFunc("/api/deadline/set", auth(a.handleDeadlineSet))
	mux.HandleFunc("/api/deadline/clear", auth(a.handleDeadlineClear))

	mux.HandleFunc("/api/settings/update", auth(a.handleSettingsUpdate))
	mux.HandleFunc("/api/settings/ignorati/add", auth(a.handleIgnoratiAdd))
	mux.HandleFunc("/api/settings/ignorati/remove", auth(a.handleIgnoratiRemove))

	mux.HandleFunc("/api/students/set", auth(a.handleStudentSet))
	mux.HandleFunc("/api/students/delete", auth(a.handleStudentDelete))
	mux.HandleFunc("/api/students/clear", auth(a.handleStudentClear))

	// Download script studenti (Phase 1.7)
	mux.HandleFunc("/api/scripts/proxy_on.bat", auth(a.handleScriptProxyOn))
	mux.HandleFunc("/api/scripts/proxy_off.bat", auth(a.handleScriptProxyOff))

	// Shutdown (Phase 1.7+): consente di spegnere il server dalla UI
	mux.HandleFunc("/api/shutdown", auth(a.handleShutdown))

	// Persistence-backed (Phase 1.6)
	mux.HandleFunc("/api/preset/save", auth(a.handlePresetSave))
	mux.HandleFunc("/api/preset/load", auth(a.handlePresetLoad))
	mux.HandleFunc("/api/preset/delete", auth(a.handlePresetDelete))
	mux.HandleFunc("/api/classi/save", auth(a.handleClassiSave))
	mux.HandleFunc("/api/classi/load", auth(a.handleClassiLoad))
	mux.HandleFunc("/api/classi/delete", auth(a.handleClassiDelete))
	mux.HandleFunc("/api/sessioni/archivia", auth(a.handleSessioniArchivia))
	mux.HandleFunc("/api/sessioni/load", auth(a.handleSessioniLoad))
	mux.HandleFunc("/api/sessioni/delete", auth(a.handleSessioniDelete))
}

// ============================================================
// Helpers
// ============================================================

// writeJSON serializza data come JSON con status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("API: errore encode JSON: %v", err)
	}
}

// writeError ritorna {ok:false, error, code} con lo status indicato.
func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
		"code":  code,
	})
}

// writeOK ritorna {ok:true, ...extra} con status 200.
func writeOK(w http.ResponseWriter, extra map[string]any) {
	body := map[string]any{"ok": true}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, http.StatusOK, body)
}

// requireMethod scrive 405 se il metodo non e' quello atteso.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "Metodo non consentito", "METHOD_NOT_ALLOWED")
		return false
	}
	return true
}

// decodeJSONBody legge fino a 1 MB di body e lo deserializza in v.
// Anti-flood guard.
func decodeJSONBody(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

// ============================================================
// GET handlers (Phase 1.4)
// ============================================================

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

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.ConfigSnapshotData())
}

func (a *API) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.HistorySnapshotData())
}

func (a *API) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.SessionStatusData())
}

func (a *API) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.SettingsSnapshotData())
}

func (a *API) handleSessioni(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	lista, err := a.state.Store().SessionListFilenames()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Errore lettura archivio: "+err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessioni": lista})
}

func (a *API) handlePresets(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	lista, err := a.state.Store().ListaPresets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Errore lettura presets: "+err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": lista})
}

func (a *API) handleClassi(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	combo, err := a.state.Store().ListaClassi()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Errore lettura classi: "+err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"classi": combo})
}

// ============================================================
// POST handlers — Blocklist
// ============================================================

type dominioBody struct {
	Dominio string `json:"dominio"`
}

func (a *API) handleBlock(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body dominioBody
	if err := decodeJSONBody(r, &body); err != nil || body.Dominio == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {dominio: string}", "BAD_BODY")
		return
	}
	a.state.Block(body.Dominio)
	writeOK(w, nil)
}

func (a *API) handleUnblock(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body dominioBody
	if err := decodeJSONBody(r, &body); err != nil || body.Dominio == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {dominio: string}", "BAD_BODY")
		return
	}
	a.state.Unblock(body.Dominio)
	writeOK(w, nil)
}

func (a *API) handleBlockAllAI(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.BlockAllAI()
	writeOK(w, nil)
}

func (a *API) handleUnblockAllAI(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.UnblockAllAI()
	writeOK(w, nil)
}

func (a *API) handleClearBlocklist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.ClearBlocklist()
	writeOK(w, nil)
}

// ============================================================
// POST handlers — Sessione
// ============================================================

func (a *API) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	inizio, archiviata := a.state.SessionStart()
	writeOK(w, map[string]any{
		"sessioneInizio": inizio,
		"archiviata":     archiviata, // "" in Phase 1.5 (no persistence)
	})
}

func (a *API) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	archiviata, fine := a.state.SessionStop()
	writeOK(w, map[string]any{
		"sessioneFineISO": fine,
		"archiviata":      archiviata,
	})
}

// ============================================================
// POST handlers — Pausa
// ============================================================

func (a *API) handlePauseToggle(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	p := a.state.TogglePausa()
	writeOK(w, map[string]any{"pausato": p})
}

func (a *API) handlePauseOn(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.SetPausa(true)
	writeOK(w, map[string]any{"pausato": true})
}

func (a *API) handlePauseOff(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.SetPausa(false)
	writeOK(w, map[string]any{"pausato": false})
}

// ============================================================
// POST handlers — Deadline
// ============================================================

type deadlineBody struct {
	Time string `json:"time"`
}

func (a *API) handleDeadlineSet(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body deadlineBody
	if err := decodeJSONBody(r, &body); err != nil || body.Time == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {time: \"HH:MM\"}", "BAD_BODY")
		return
	}
	iso, err := a.state.SetDeadline(body.Time)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_TIME")
		return
	}
	writeOK(w, map[string]any{"deadlineISO": iso})
}

func (a *API) handleDeadlineClear(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.ClearDeadline()
	writeOK(w, nil)
}

// ============================================================
// POST handlers — Settings
// ============================================================

func (a *API) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var updates map[string]any
	if err := decodeJSONBody(r, &updates); err != nil {
		writeError(w, http.StatusBadRequest, "Body deve essere oggetto JSON", "BAD_BODY")
		return
	}
	updated, rejected, riavvio := a.state.UpdateSettings(updates)
	writeOK(w, map[string]any{
		"updated":         updated,
		"rejected":        rejected,
		"richiedeRiavvio": riavvio,
		"settings":        a.state.SettingsSnapshotData(),
	})
}

func (a *API) handleIgnoratiAdd(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body dominioBody
	if err := decodeJSONBody(r, &body); err != nil || body.Dominio == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {dominio: string}", "BAD_BODY")
		return
	}
	a.state.AddIgnorato(body.Dominio)
	writeOK(w, nil)
}

func (a *API) handleIgnoratiRemove(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body dominioBody
	if err := decodeJSONBody(r, &body); err != nil || body.Dominio == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {dominio: string}", "BAD_BODY")
		return
	}
	a.state.RemoveIgnorato(body.Dominio)
	writeOK(w, nil)
}

// ============================================================
// POST handlers — Studenti
// ============================================================

type studentBody struct {
	IP   string `json:"ip"`
	Nome string `json:"nome"`
}

func (a *API) handleStudentSet(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body studentBody
	if err := decodeJSONBody(r, &body); err != nil || body.IP == "" || body.Nome == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {ip, nome}", "BAD_BODY")
		return
	}
	a.state.SetStudent(body.IP, body.Nome)
	writeOK(w, nil)
}

func (a *API) handleStudentDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body studentBody
	if err := decodeJSONBody(r, &body); err != nil || body.IP == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {ip}", "BAD_BODY")
		return
	}
	a.state.DeleteStudent(body.IP)
	writeOK(w, nil)
}

func (a *API) handleStudentClear(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	a.state.ClearStudents()
	writeOK(w, nil)
}

// ============================================================
// Shutdown (Phase 1.7+)
// ============================================================

// handleShutdown spegne il binario via os.Exit(0) dopo aver risposto al
// client. Risponde subito, fa exit dopo 200ms in goroutine cosi' la
// risposta HTTP arriva al browser (che chiude la finestra app
// automaticamente quando perde la connessione).
//
// Note: os.Exit non chiama defers / hook. Va bene per Phase 1: il
// principale "cleanup" e' la rotazione del NDJSON al successivo boot
// via RecoverNDJSONIfAny. Per Phase 8 (release) si potra' aggiungere
// un graceful shutdown reale.
func (a *API) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	writeOK(w, nil)
	go func() {
		time.Sleep(200 * time.Millisecond)
		log.Println("Shutdown via API")
		os.Exit(0)
	}()
}

// ============================================================
// Download script studenti (Phase 1.7)
// ============================================================

func (a *API) handleScriptProxyOn(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a.serveScriptDownload(w, "proxy_on.bat")
}

func (a *API) handleScriptProxyOff(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	a.serveScriptDownload(w, "proxy_off.bat")
}

// serveScriptDownload manda il file .bat come download (Content-Disposition
// attachment) leggendolo dalla data dir.
func (a *API) serveScriptDownload(w http.ResponseWriter, filename string) {
	dataDir := a.state.Store().DataDir()
	if dataDir == "" {
		writeError(w, http.StatusInternalServerError, "DataDir non configurata", "NO_DATADIR")
		return
	}
	body, err := os.ReadFile(filepath.Join(dataDir, filename))
	if err != nil {
		writeError(w, http.StatusNotFound, "Script non trovato: "+err.Error(), "NOT_FOUND")
		return
	}
	w.Header().Set("Content-Type", "application/x-bat; charset=UTF-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

// ============================================================
// POST handlers — Preset (Phase 1.6, persistence-backed)
// ============================================================

type presetSaveBody struct {
	Nome        string `json:"nome"`
	Descrizione string `json:"descrizione"`
}

type nomeBody struct {
	Nome string `json:"nome"`
}

func (a *API) handlePresetSave(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body presetSaveBody
	if err := decodeJSONBody(r, &body); err != nil || body.Nome == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {nome}", "BAD_BODY")
		return
	}
	// Snapshot della blocklist corrente
	bloccatiSnap := a.state.HistorySnapshotData().Bloccati
	p := store.PresetFile{
		Nome:        body.Nome,
		Descrizione: body.Descrizione,
		Domini:      bloccatiSnap,
		CreatedAt:   time.Now().UnixMilli(),
	}
	if err := a.state.Store().SavePreset(p); err != nil {
		if err == store.ErrNomeInvalido {
			writeError(w, http.StatusBadRequest, err.Error(), "BAD_NAME")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, map[string]any{"nome": body.Nome})
}

func (a *API) handlePresetLoad(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body nomeBody
	if err := decodeJSONBody(r, &body); err != nil || body.Nome == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {nome}", "BAD_BODY")
		return
	}
	p, err := a.state.Store().LoadPreset(body.Nome)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
		return
	}
	// Sostituisce la blocklist corrente con quella del preset.
	a.state.ClearBlocklist()
	for _, d := range p.Domini {
		a.state.Block(d)
	}
	writeOK(w, map[string]any{"caricati": len(p.Domini)})
}

func (a *API) handlePresetDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body nomeBody
	if err := decodeJSONBody(r, &body); err != nil || body.Nome == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {nome}", "BAD_BODY")
		return
	}
	if err := a.state.Store().DeletePreset(body.Nome); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, nil)
}

// ============================================================
// POST handlers — Classi (Phase 1.6, persistence-backed)
// ============================================================

type classeBody struct {
	Classe string `json:"classe"`
	Lab    string `json:"lab"`
}

func (a *API) handleClassiSave(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body classeBody
	if err := decodeJSONBody(r, &body); err != nil || body.Classe == "" || body.Lab == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {classe, lab}", "BAD_BODY")
		return
	}
	// Salva la mappa studenti corrente come snapshot per quella combo.
	cfg := a.state.ConfigSnapshotData()
	c := store.ClasseFile{
		Classe:    body.Classe,
		Lab:       body.Lab,
		Mappa:     cfg.Studenti,
		UpdatedAt: time.Now().UnixMilli(),
	}
	if err := a.state.Store().SaveClasse(c); err != nil {
		if err == store.ErrNomeInvalido {
			writeError(w, http.StatusBadRequest, err.Error(), "BAD_NAME")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, map[string]any{"classe": body.Classe, "lab": body.Lab})
}

func (a *API) handleClassiLoad(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body classeBody
	if err := decodeJSONBody(r, &body); err != nil || body.Classe == "" || body.Lab == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {classe, lab}", "BAD_BODY")
		return
	}
	c, err := a.state.Store().LoadClasse(body.Classe, body.Lab)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
		return
	}
	a.state.SetStudenti(c.Mappa)
	writeOK(w, map[string]any{"caricati": len(c.Mappa)})
}

func (a *API) handleClassiDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body classeBody
	if err := decodeJSONBody(r, &body); err != nil || body.Classe == "" || body.Lab == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {classe, lab}", "BAD_BODY")
		return
	}
	if err := a.state.Store().DeleteClasse(body.Classe, body.Lab); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, nil)
}

// ============================================================
// POST handlers — Sessioni archivio (Phase 1.6)
// ============================================================

// handleSessioniArchivia forza l'archivio della sessione corrente senza
// fermarla. Utile come "checkpoint" durante una sessione lunga.
func (a *API) handleSessioniArchivia(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	// La logica di archivio vive nello state (legge NDJSON + scrive snapshot).
	// Esponiamo un wrapper read-only che chiama internamente.
	fn := a.state.ArchiviaCorrente()
	if fn == "" {
		writeError(w, http.StatusBadRequest,
			"Niente da archiviare (sessione non avviata o buffer vuoto)",
			"NOTHING_TO_ARCHIVE")
		return
	}
	writeOK(w, map[string]any{"archiviata": fn})
}

type filenameBody struct {
	Filename string `json:"filename"`
}

// handleSessioniLoad carica una sessione archiviata dall'id-stringa
// "<id>-<inizio>.json" prodotto da SessionListFilenames.
// Body: {filename:"12-2026-04-22-10-23-45.json"}.
func (a *API) handleSessioniLoad(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body filenameBody
	if err := decodeJSONBody(r, &body); err != nil || body.Filename == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {filename}", "BAD_BODY")
		return
	}
	id, err := store.ParseSessionFilename(body.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_BODY")
		return
	}
	archive, err := a.state.Store().SessionLoad(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, archive)
}

// handleSessioniDelete elimina una sessione archiviata.
// Body: {filename:"<id>-<inizio>.json"}
func (a *API) handleSessioniDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body filenameBody
	if err := decodeJSONBody(r, &body); err != nil || body.Filename == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {filename}", "BAD_BODY")
		return
	}
	if !strings.HasSuffix(body.Filename, ".json") {
		writeError(w, http.StatusBadRequest, "Filename deve terminare con .json", "BAD_BODY")
		return
	}
	id, err := store.ParseSessionFilename(body.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_BODY")
		return
	}
	if err := a.state.Store().SessionDelete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, nil)
}
