package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrokerSubscribeUnsubscribe(t *testing.T) {
	b := NewBroker()
	if b.Subs() != 0 {
		t.Fatalf("broker iniziale con %d sub, atteso 0", b.Subs())
	}
	ch := b.Subscribe()
	if b.Subs() != 1 {
		t.Errorf("dopo Subscribe, %d sub, atteso 1", b.Subs())
	}
	b.Unsubscribe(ch)
	if b.Subs() != 0 {
		t.Errorf("dopo Unsubscribe, %d sub, atteso 0", b.Subs())
	}
	// Idempotenza: un secondo Unsubscribe non deve panicare.
	b.Unsubscribe(ch)
}

func TestBroadcastReachesAllSubs(t *testing.T) {
	b := NewBroker()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()
	defer b.Unsubscribe(ch1)
	defer b.Unsubscribe(ch2)

	b.Broadcast(map[string]any{"type": "test", "n": 1})

	for i, ch := range []chan []byte{ch1, ch2} {
		select {
		case msg := <-ch:
			str := string(msg)
			if !strings.HasPrefix(str, "data: ") || !strings.HasSuffix(str, "\n\n") {
				t.Errorf("sub %d: payload mal formato: %q", i, str)
			}
			if !strings.Contains(str, `"type":"test"`) {
				t.Errorf("sub %d: payload non contiene il tipo: %q", i, str)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("sub %d: niente messaggio entro 100ms", i)
		}
	}
}

func TestSlowSubscriberDoesntBlock(t *testing.T) {
	b := NewBroker()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Riempio il buffer del subscriber senza svuotarlo.
	for i := 0; i < SubscriberBuffer; i++ {
		b.Broadcast(map[string]any{"i": i})
	}

	// Quando il buffer e' pieno, ulteriori Broadcast devono ritornare subito
	// (drop silenzioso del messaggio per il sub lento). Se questo blocca,
	// il test va in deadlock e fallisce per timeout.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			b.Broadcast(map[string]any{"i": i + 1000})
		}
		close(done)
	}()
	select {
	case <-done:
		// ok, non bloccato
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Broadcast bloccato su subscriber lento (atteso drop, ottenuto block)")
	}
}

func TestHandleStreamHeaders(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// Cancello dopo poco per terminare l'handler.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	b.HandleStream(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, atteso text/event-stream", ct)
	}
	if !strings.Contains(rec.Body.String(), ": connected") {
		t.Errorf("body non contiene preamble ': connected', got: %q", rec.Body.String())
	}
}

func TestHandleStreamReceivesBroadcast(t *testing.T) {
	// Verifica end-to-end: client connesso → broadcast → il body contiene il msg.
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		b.HandleStream(rec, req)
		close(done)
	}()

	// Aspetto che l'handler abbia subscribed (Subs() == 1).
	for i := 0; i < 100 && b.Subs() == 0; i++ {
		time.Sleep(2 * time.Millisecond)
	}
	if b.Subs() != 1 {
		t.Fatalf("client non e' arrivato a Subscribe entro 200ms")
	}

	b.Broadcast(map[string]any{"type": "ping"})

	// Aspetto che il broadcast venga scritto nel body.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.Body.String(), `"type":"ping"`) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done

	if !strings.Contains(rec.Body.String(), `"type":"ping"`) {
		t.Errorf("body non contiene il broadcast: %q", rec.Body.String())
	}
}
