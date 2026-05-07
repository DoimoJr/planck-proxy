package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DoimoJr/planck-proxy/internal/scripts"
	"github.com/DoimoJr/planck-proxy/internal/veyon"
	"github.com/DoimoJr/planck-proxy/internal/veyon/qds"
)

// ============================================================
// Veyon API (Phase 3e)
// ============================================================
//
// Tutti gli endpoint sono dietro l'auth Basic come gli altri.
// Lo stato Veyon (configured, keyName, port) e' incluso in
// /api/veyon/status; la configurazione passa per /configure
// (POST con PEM master key) e /clear.
//
// I comandi feature passano per /feature: il body specifica IP
// studente, UUID feature, command, e arguments come JSON map.
// La conn sottostante usa pattern dial-send-close.

func (a *API) handleVeyonStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, a.state.VeyonStatusData())
}

type veyonConfigureBody struct {
	KeyName       string `json:"keyName"`
	PrivateKeyPEM string `json:"privateKeyPEM"`
}

func (a *API) handleVeyonConfigure(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body veyonConfigureBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Body deve essere {keyName, privateKeyPEM}", "BAD_BODY")
		return
	}
	if body.KeyName == "" || body.PrivateKeyPEM == "" {
		writeError(w, http.StatusBadRequest, "keyName e privateKeyPEM richiesti", "BAD_BODY")
		return
	}
	if err := a.state.VeyonConfigure(body.KeyName, []byte(body.PrivateKeyPEM)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_KEY")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": a.state.VeyonStatusData(),
	})
}

func (a *API) handleVeyonClear(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := a.state.VeyonClear(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeOK(w, nil)
}

type veyonTestBody struct {
	IP string `json:"ip"`
}

func (a *API) handleVeyonTest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body veyonTestBody
	if err := decodeJSONBody(r, &body); err != nil || body.IP == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {ip}", "BAD_BODY")
		return
	}
	if err := a.state.VeyonTest(body.IP); err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "VEYON_DIAL")
		return
	}
	writeOK(w, nil)
}

type veyonFeatureBody struct {
	IP        string                 `json:"ip"`
	Feature   string                 `json:"feature"`           // UUID o nome simbolico
	Command   int32                  `json:"command,omitempty"` // 0 = Default
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// veyonSymbolicFeatures mappa nomi simbolici comodi → UUID.
// Qualunque chiamata con `feature` non in questa mappa viene
// interpretata come UUID raw.
//
// Per ScreenLock il command field discrimina lock/unlock (0/1):
// usiamo due alias simbolici "screenLock" e "screenUnlock" per
// non costringere il client a sapere i numeri.
var veyonSymbolicFeatures = map[string]string{
	"screenLock":   veyon.FeatureScreenLock,
	"screenUnlock": veyon.FeatureScreenLock, // stesso UUID, command=1 dal client
	"startApp":     veyon.FeatureStartApp,
	"reboot":       veyon.FeatureReboot,
	"powerDown":    veyon.FeaturePowerDown,
	"powerDownNow": veyon.FeaturePowerDownNow,
	"logoff":       veyon.FeatureLogoff,
	"textMsg":      veyon.FeatureTextMsg,
	"openURL":      veyon.FeatureOpenURL,
}

// veyonSymbolicCommands mappa nomi simbolici → command number per le
// feature dove il default (0) non e' sufficiente. Solo screenUnlock
// per ora.
var veyonSymbolicCommands = map[string]int32{
	"screenUnlock": 1, // ScreenLock::FeatureCommand::StopLock
}

func (a *API) handleVeyonFeature(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body veyonFeatureBody
	if err := decodeJSONBody(r, &body); err != nil || body.IP == "" || body.Feature == "" {
		writeError(w, http.StatusBadRequest, "Body deve essere {ip, feature, [command], [arguments]}", "BAD_BODY")
		return
	}

	// Risolvi nome simbolico se presente.
	uuidStr := body.Feature
	cmd := body.Command
	if strings.Contains(uuidStr, "-") == false {
		// non assomiglia a un UUID, prova come simbolico
		if mapped, ok := veyonSymbolicFeatures[body.Feature]; ok {
			uuidStr = mapped
			// Se il simbolico ha un command default associato, usalo
			// (override del campo `command` del body, che spesso e' 0).
			if c, ok := veyonSymbolicCommands[body.Feature]; ok && body.Command == 0 {
				cmd = c
			}
		} else {
			writeError(w, http.StatusBadRequest, "feature sconosciuta: "+body.Feature, "BAD_FEATURE")
			return
		}
	}
	uuid, err := qds.UuidFromString(uuidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "UUID feature invalido: "+err.Error(), "BAD_FEATURE")
		return
	}

	// Converti gli argomenti JSON in qds.VariantMap. JSON gia' usa string keys e
	// any values, quindi e' una conversione di alias di tipo.
	args := qds.VariantMap{}
	for k, v := range body.Arguments {
		args[k] = jsonToVariant(v)
	}

	fm := veyon.FeatureMessage{
		FeatureUUID: uuid,
		Command:     veyon.FeatureCommand(cmd),
		Arguments:   args,
	}
	if err := a.state.VeyonSendFeature(body.IP, fm); err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "VEYON_SEND")
		return
	}
	writeOK(w, nil)
}

// distributeBatBody e' il body di /api/veyon/distribuisci-proxy + /disinstalla-proxy.
// Il caller passa la lista di IP target; il server legge il bat appropriato
// dal data dir e lo manda via FileTransfer con OpenFileInApplication=true.
type distributeBatBody struct {
	IPs []string `json:"ips"`
}

// handleVeyonDistribuisciProxy invia proxy_on.vbs ai target via FileTransfer
// con flag "open in app" → wscript.exe lo esegue, attivando il proxy lato
// studente. wscript e' subsystem GUI -> nessuna finestra/console visibile
// (niente flash, niente prompt).
func (a *API) handleVeyonDistribuisciProxy(w http.ResponseWriter, r *http.Request) {
	a.distributeBat(w, r, "proxy_on.vbs")
}

// handleVeyonDisinstallaProxy invia proxy_off.vbs ai target.
// Prima della distribuzione, marca gli IP come "proxy rimosso" cosi'
// checkHeartbeats non emette alert spuri "watchdog stopped" durante
// e dopo la rimozione (il watchdog DEVE morire, l'utente l'ha chiesto).
func (a *API) handleVeyonDisinstallaProxy(w http.ResponseWriter, r *http.Request) {
	// Per pre-marcare gli IP come "proxy rimosso" senza consumare il body
	// che distributeBat legge di nuovo, leggo i bytes e poi reinietto.
	if r.Method == http.MethodPost && r.Body != nil {
		raw, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err == nil {
			var body distributeBatBody
			if jerr := json.Unmarshal(raw, &body); jerr == nil && len(body.IPs) > 0 {
				a.state.MarkProxyRemoved(body.IPs)
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			r.ContentLength = int64(len(raw))
		}
	}
	a.distributeBat(w, r, "proxy_off.vbs")
}

// distributeBat e' il flow comune che legge il file dal data dir, e
// delega a distributeBytes per la trasmissione.
func (a *API) distributeBat(w http.ResponseWriter, r *http.Request, filename string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body distributeBatBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Body deve essere {ips:[...]}", "BAD_BODY")
		return
	}
	if len(body.IPs) == 0 {
		writeError(w, http.StatusBadRequest, "Lista IPs vuota", "BAD_BODY")
		return
	}

	// Legge il bat dal data dir (path: <dataDir>/<filename>).
	dataDir := a.state.Store().DataDir()
	path := filepath.Join(dataDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lettura "+filename+": "+err.Error(), "FILE_READ")
		return
	}

	a.distributeBytes(w, body.IPs, filename, content)
}

// distributeBytes invia content come file `filename` a tutti gli IPs
// in parallelo via Veyon FileTransfer (OpenFileInApplication=true,
// overwrite=true). Risponde con il riassunto ok/fail per ogni target.
// Estratto da distributeBat per riusare il flow con contenuto generato
// in-memory (es. firefox_lockdown.vbs).
func (a *API) distributeBytes(w http.ResponseWriter, ips []string, filename string, content []byte) {
	// Invia in parallelo, raccogli risultati.
	type result struct {
		IP    string `json:"ip"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	results := make([]result, len(ips))
	done := make(chan int, len(ips))
	for i, ip := range ips {
		go func(i int, ip string) {
			err := a.state.VeyonSendFile(ip, filename, content, true, true)
			r := result{IP: ip, OK: err == nil}
			if err != nil {
				r.Error = err.Error()
			}
			results[i] = r
			done <- i
		}(i, ip)
	}
	for range ips {
		<-done
	}

	ok, fail := 0, 0
	for _, r := range results {
		if r.OK {
			ok++
		} else {
			fail++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      fail == 0,
		"total":   len(ips),
		"success": ok,
		"failed":  fail,
		"results": results,
	})
}

// handleVeyonDistribuisciFirefoxLockdown genera al volo un VBS silent
// (no MsgBox di feedback) che scrive policies.json nelle distribution
// dir di Firefox e lo distribuisce ai target via Veyon FileTransfer.
// One-time per PC: dopo la prima esecuzione il policies resta finche'
// qualcuno non lo cancella.
func (a *API) handleVeyonDistribuisciFirefoxLockdown(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body distributeBatBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Body deve essere {ips:[...]}", "BAD_BODY")
		return
	}
	if len(body.IPs) == 0 {
		writeError(w, http.StatusBadRequest, "Lista IPs vuota", "BAD_BODY")
		return
	}
	content := []byte(scripts.FirefoxLockdownVBS(true))
	a.distributeBytes(w, body.IPs, "firefox_lockdown.vbs", content)
}

// jsonToVariant converte un valore decodato da encoding/json in un tipo
// compatibile con qds.WriteVariant.
//
// Mapping:
//
//	bool          -> bool
//	float64       -> int32 (se intero senza frazione) o int64 (se grosso)
//	string        -> string
//	[]any         -> qds.VariantList o []string (se omogeneo)
//	map[string]any -> qds.VariantMap (ricorsivo)
//	nil           -> nil
func jsonToVariant(v any) any {
	switch x := v.(type) {
	case bool, string, nil:
		return x
	case float64:
		// JSON numero. Se ha frazione → double; altrimenti int.
		if x == float64(int64(x)) {
			i := int64(x)
			if i >= -2147483648 && i <= 2147483647 {
				return int32(i)
			}
			return i
		}
		return x
	case []any:
		// Se tutti string -> []string (QStringList), altrimenti VariantList.
		allStr := true
		for _, e := range x {
			if _, ok := e.(string); !ok {
				allStr = false
				break
			}
		}
		if allStr {
			out := make([]string, len(x))
			for i, e := range x {
				out[i] = e.(string)
			}
			return out
		}
		out := make(qds.VariantList, len(x))
		for i, e := range x {
			out[i] = jsonToVariant(e)
		}
		return out
	case map[string]any:
		out := qds.VariantMap{}
		for k, val := range x {
			out[k] = jsonToVariant(val)
		}
		return out
	}
	return v
}
