// Package web ospita il server HTTP che serve la dashboard, le API REST e
// lo stream SSE. In Phase 1.3 contiene solo il Broker SSE; i handler delle
// API arriveranno in 1.4.
package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SubscriberBuffer e' la capacita' del canale per ogni subscriber SSE.
// Con burst tipici (~100 msg/s a regime, picchi ~500/s durante una verifica
// con tutti gli studenti attivi) 256 messaggi danno margine sufficiente
// per assorbire ritardi temporanei del client. Subscriber che riempiono
// il buffer perdono i messaggi successivi (vedi Broadcast).
const SubscriberBuffer = 256

// HeartbeatInterval e' la cadenza dei messaggi `: hb` keepalive. Servono
// a evitare che proxy/reverse-proxy timeoutino la connessione SSE come
// idle. L'EventSource lato browser ignora i commenti SSE (linee che
// iniziano con `:`).
const HeartbeatInterval = 20 * time.Second

// Broker distribuisce messaggi SSE a tutti i client connessi al stream.
// Concurrent-safe: Subscribe / Unsubscribe / Broadcast possono essere
// chiamati da goroutine diverse.
type Broker struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

// NewBroker crea un Broker pronto all'uso.
func NewBroker() *Broker {
	return &Broker{subs: make(map[chan []byte]struct{})}
}

// Subscribe registra un nuovo client e ritorna il suo canale buffered.
// Il caller deve invocare Unsubscribe quando il client si disconnette
// (tipicamente in `defer` dentro l'handler HTTP, vedi HandleStream).
func (b *Broker) Subscribe() chan []byte {
	ch := make(chan []byte, SubscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe rimuove e chiude il canale.
// Idempotente: chiamare due volte sullo stesso canale e' safe.
func (b *Broker) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Broadcast serializza msg come JSON, lo wrappa nel formato SSE
// (`data: <json>\n\n`) e lo invia a tutti i subscriber.
//
// **Subscriber lenti vengono skippati**: se il canale e' pieno, il
// messaggio viene scartato per quel sub specifico (`select default`).
// Questa scelta evita che un singolo client lento blocchi tutti gli altri.
// Il client perdera' un evento ma riceve i successivi.
func (b *Broker) Broadcast(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("SSE broadcast: errore marshal: %v", err)
		return
	}
	payload := append([]byte("data: "), append(data, '\n', '\n')...)

	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- payload:
		default:
			// Sub lento: drop. Niente lock-step col piu' lento del gruppo.
		}
	}
}

// Subs ritorna il numero di subscriber attualmente connessi.
// Utile per /api/health o test.
func (b *Broker) Subs() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// HandleStream e' l'handler HTTP per /api/stream.
//
// Tiene aperta la connessione, scrive ogni messaggio ricevuto dal canale
// del subscriber, e manda un heartbeat ogni HeartbeatInterval per evitare
// timeout di proxy intermedi. Termina quando il client chiude la connessione
// (rilevato via `r.Context().Done()`).
func (b *Broker) HandleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming non supportato", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Disabilita il buffering di nginx/altri reverse proxy.
	w.Header().Set("X-Accel-Buffering", "no")

	// Preamble per chiarire che la connessione e' viva.
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	heartbeat := time.NewTicker(HeartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnesso.
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": hb\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
