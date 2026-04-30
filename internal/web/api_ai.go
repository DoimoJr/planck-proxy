package web

import "net/http"

// handleAIStatus ritorna lo stato corrente della lista AI: count,
// source (embedded/cache/remote), timestamp, URL remoto opzionale.
func (a *API) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	l := a.state.CurrentAIList()
	if l == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"count": 0, "source": "", "updatedAt": "",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":     l.Count,
		"source":    string(l.Source),
		"updatedAt": l.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"url":       l.URL,
	})
}

// handleAIRefresh forza un fetch dalla URL remota (default
// `classify.AIDomainsURL`). Salva la cache + promuove. Ritorna lo
// status risultante (anche su errore: lascia la lista corrente
// inalterata e ritorna l'errore col last-known-good).
func (a *API) handleAIRefresh(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	l, err := a.state.RefreshAIListNow()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":     false,
			"error":  err.Error(),
			"code":   "REFRESH_FAILED",
			"count":  l.Count,
			"source": string(l.Source),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"count":     l.Count,
		"source":    string(l.Source),
		"updatedAt": l.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
