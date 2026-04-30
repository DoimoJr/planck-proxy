package state

import (
	"log"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// LoadAICacheAtBoot prova a caricare la cache AI dal data dir. Se la
// cache non c'e' o e' corrotta, classify resta sulla lista embedded.
// Best-effort: errori solo loggati, non fatali.
func (s *State) LoadAICacheAtBoot() {
	dataDir := s.store.DataDir()
	if dataDir == "" {
		return
	}
	if err := classify.LoadAICache(dataDir); err == nil {
		l := classify.CurrentAIList()
		log.Printf("classify: lista AI caricata dalla cache (%d domini, %s)",
			l.Count, l.UpdatedAt.Format("2006-01-02 15:04"))
	}
}

// RefreshAIListNow scarica la lista da remote (URL hardcoded
// classify.AIDomainsURL) + aggiorna cache. Sincrono per il caller —
// usato da `/api/ai/refresh`.
func (s *State) RefreshAIListNow() (*classify.AIList, error) {
	dataDir := s.store.DataDir()
	if err := classify.RefreshAIList("", dataDir); err != nil {
		return classify.CurrentAIList(), err
	}
	l := classify.CurrentAIList()
	log.Printf("classify: lista AI aggiornata da remote (%d domini)", l.Count)
	// Broadcast settings update cosi' la UI ricarica i contatori.
	s.broker.Broadcast(struct {
		Type   string `json:"type"`
		Source string `json:"source"`
		Count  int    `json:"count"`
		TS     int64  `json:"ts"`
	}{
		Type:   "ai-list",
		Source: string(l.Source),
		Count:  l.Count,
		TS:     l.UpdatedAt.UnixMilli(),
	})
	return l, nil
}

// CurrentAIList passthrough per la UI status.
func (s *State) CurrentAIList() *classify.AIList { return classify.CurrentAIList() }
