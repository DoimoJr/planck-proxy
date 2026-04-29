package web

import (
	"net/http"
	"strings"

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
var veyonSymbolicFeatures = map[string]string{
	"screenLock": veyon.FeatureScreenLock,
	"startApp":   veyon.FeatureStartApp,
	"reboot":     veyon.FeatureReboot,
	"powerDown":  veyon.FeaturePowerDown,
	"logoff":     veyon.FeatureLogoff,
	"textMsg":    veyon.FeatureTextMsg,
	"openURL":    veyon.FeatureOpenURL,
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
	if strings.Contains(uuidStr, "-") == false {
		// non assomiglia a un UUID, prova come simbolico
		if mapped, ok := veyonSymbolicFeatures[uuidStr]; ok {
			uuidStr = mapped
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
		Command:     veyon.FeatureCommand(body.Command),
		Arguments:   args,
	}
	if err := a.state.VeyonSendFeature(body.IP, fm); err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "VEYON_SEND")
		return
	}
	writeOK(w, nil)
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
