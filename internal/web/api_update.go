package web

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/update"
)

// statoUpdate tiene l'esito dell'ultimo controllo aggiornamenti.
//
// Serve a due cose: mostrare in UI l'esito senza rifare la chiamata a
// GitHub a ogni render, e dare a /api/update/apply l'URL dell'asset
// senza doverlo ricevere dal client (che potrebbe passarne uno
// arbitrario: l'unico URL installabile e' quello che abbiamo appena
// letto dalla release ufficiale).
type statoUpdate struct {
	mu      sync.Mutex
	info    *update.Info
	quando  time.Time
	inCorso bool
	riavvia func()
}

// SetRiavvio registra la funzione che respawna il binario dopo un
// aggiornamento. La fornisce main, che e' l'unico a sapere come
// rilanciare (e cosa passare al nuovo processo, vedi l'adozione del
// PID del browser). Senza, /api/update/apply installa ma non riavvia.
func (a *API) SetRiavvio(f func()) {
	a.update.mu.Lock()
	defer a.update.mu.Unlock()
	a.update.riavvia = f
}

// handleUpdateCheck interroga GitHub e memorizza l'esito.
func (a *API) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	info, err := update.Check(r.Context(), a.version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Controllo non riuscito: "+err.Error(), "UPDATE_CHECK_FAILED")
		return
	}
	a.update.mu.Lock()
	a.update.info = info
	a.update.quando = time.Now()
	a.update.mu.Unlock()

	writeJSON(w, http.StatusOK, info)
}

// handleUpdateApply scarica, installa e riavvia.
//
// Rifiuta a sessione attiva: il riavvio stacca il proxy per un paio di
// secondi e i browser degli studenti darebbero errore a meta' verifica.
// Il vincolo sta qui e non solo nella UI, cosi' vale anche per una
// chiamata diretta all'API.
func (a *API) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if a.state.SessionStatusData().SessioneAttiva {
		writeError(w, http.StatusConflict,
			"C'e' una sessione in corso: fermala prima di aggiornare, il riavvio interrompe il proxy per qualche secondo.",
			"SESSIONE_ATTIVA")
		return
	}

	a.update.mu.Lock()
	if a.update.inCorso {
		a.update.mu.Unlock()
		writeError(w, http.StatusConflict, "Aggiornamento gia' in corso.", "UPDATE_IN_CORSO")
		return
	}
	info := a.update.info
	riavvia := a.update.riavvia
	a.update.mu.Unlock()

	// Nessun check precedente in memoria (es. dopo un restart): rifallo
	// al volo invece di costringere l'utente a premere prima "Controlla".
	if info == nil {
		var err error
		info, err = update.Check(r.Context(), a.version)
		if err != nil {
			writeError(w, http.StatusBadGateway, "Controllo non riuscito: "+err.Error(), "UPDATE_CHECK_FAILED")
			return
		}
	}
	if !info.Disponibile {
		writeError(w, http.StatusBadRequest, "Nessun aggiornamento disponibile.", "NESSUN_AGGIORNAMENTO")
		return
	}

	a.update.mu.Lock()
	a.update.inCorso = true
	a.update.mu.Unlock()

	// Il download puo' durare minuti su linea lenta: usiamo un context
	// scollegato dalla richiesta HTTP, che nel frattempo ha gia'
	// risposto. Il client segue l'avanzamento via SSE.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	a.broker.Broadcast(map[string]any{"type": "update-stato", "fase": "download", "versione": info.Ultima})
	writeOK(w, map[string]any{"versione": info.Ultima})

	go func() {
		defer cancel()
		if err := update.Applica(ctx, info); err != nil {
			log.Printf("Update fallito: %v", err)
			a.update.mu.Lock()
			a.update.inCorso = false
			a.update.mu.Unlock()
			a.broker.Broadcast(map[string]any{"type": "update-stato", "fase": "errore", "errore": err.Error()})
			return
		}
		log.Printf("Update a v%s installato, riavvio.", info.Ultima)
		a.broker.Broadcast(map[string]any{"type": "update-stato", "fase": "riavvio", "versione": info.Ultima})
		// Lascia il tempo al frame SSE di raggiungere il browser prima
		// che il processo sparisca.
		time.Sleep(500 * time.Millisecond)
		if riavvia != nil {
			riavvia()
		}
	}()
}

// ControllaInBackground fa un check silenzioso e, se trova una versione
// nuova, la annuncia via SSE perche' la UI mostri il badge. Chiamata da
// main al boot: un errore di rete non deve disturbare nessuno, quindi
// viene solo loggato.
func (a *API) ControllaInBackground() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := update.Check(ctx, a.version)
		if err != nil {
			log.Printf("Controllo aggiornamenti al boot non riuscito: %v", err)
			return
		}
		a.update.mu.Lock()
		a.update.info = info
		a.update.quando = time.Now()
		a.update.mu.Unlock()
		if info.Disponibile {
			log.Printf("Aggiornamento disponibile: v%s (in esecuzione v%s)", info.Ultima, info.Corrente)
			a.broker.Broadcast(map[string]any{"type": "update-disponibile", "info": info})
		}
	}()
}
