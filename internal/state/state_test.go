package state

import (
	"sync"
	"testing"

	"github.com/DoimoJr/planck-proxy/internal/classify"
)

// mockBroker registra le chiamate Broadcast in memoria per ispezione in test.
type mockBroker struct {
	mu   sync.Mutex
	msgs []any
}

func (m *mockBroker) Broadcast(msg any) {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
}

func (m *mockBroker) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.msgs)
}

func TestRegistraTraffic(t *testing.T) {
	b := &mockBroker{}
	s := New(b)

	s.RegistraTraffic("192.168.1.50", "GET", "example.com", false, classify.TipoUtente)

	storia := s.SnapshotStoria()
	if len(storia) != 1 {
		t.Fatalf("storia ha %d entry, atteso 1", len(storia))
	}
	e := storia[0]
	if e.IP != "192.168.1.50" || e.Dominio != "example.com" || e.Tipo != classify.TipoUtente {
		t.Errorf("entry sbagliata: %+v", e)
	}
	if e.Ora == "" || e.TS == 0 {
		t.Errorf("Ora/TS non popolati: %+v", e)
	}
	if b.Count() != 1 {
		t.Errorf("broker ha ricevuto %d messaggi, atteso 1", b.Count())
	}
}

func TestRegistraAlive(t *testing.T) {
	b := &mockBroker{}
	s := New(b)

	s.RegistraAlive("192.168.1.50")
	s.RegistraAlive("192.168.1.51")
	s.RegistraAlive("192.168.1.50") // overwrite

	alive := s.SnapshotAlive()
	if len(alive) != 2 {
		t.Errorf("aliveMap ha %d ip, atteso 2", len(alive))
	}
	if alive["192.168.1.50"] == 0 {
		t.Errorf("ip 192.168.1.50 mancante o ts a zero")
	}
	if b.Count() != 3 {
		t.Errorf("broker ha ricevuto %d messaggi, atteso 3", b.Count())
	}
}

func TestRingBufferCap(t *testing.T) {
	b := &mockBroker{}
	s := New(b)
	for i := 0; i < MaxStoria+50; i++ {
		s.RegistraTraffic("ip", "GET", "x.com", false, classify.TipoUtente)
	}
	storia := s.SnapshotStoria()
	if len(storia) != MaxStoria {
		t.Errorf("ring buffer non capped: %d entry (atteso %d)", len(storia), MaxStoria)
	}
}

func TestSnapshotIndipendente(t *testing.T) {
	// Modificare la slice ritornata da SnapshotStoria NON deve alterare lo state.
	b := &mockBroker{}
	s := New(b)
	s.RegistraTraffic("ip", "GET", "x.com", false, classify.TipoUtente)

	snap := s.SnapshotStoria()
	if len(snap) != 1 {
		t.Fatalf("snap iniziale len=%d", len(snap))
	}
	snap[0].Dominio = "alterato.com"

	snap2 := s.SnapshotStoria()
	if snap2[0].Dominio != "x.com" {
		t.Errorf("snap2[0].Dominio = %q, atteso x.com (snapshot non indipendente)", snap2[0].Dominio)
	}
}

func TestConfigSnapshot(t *testing.T) {
	b := &mockBroker{}
	s := New(b)

	cfg := s.ConfigSnapshotData()
	if cfg.Modo != "blocklist" {
		t.Errorf("modo default = %q, atteso blocklist", cfg.Modo)
	}
	if cfg.InattivitaSogliaSec != 180 {
		t.Errorf("inattivitaSogliaSec default = %d, atteso 180", cfg.InattivitaSogliaSec)
	}
	if len(cfg.DominiAI) < 50 {
		t.Errorf("DominiAI nel snapshot = %d, atteso >= 50", len(cfg.DominiAI))
	}
	if cfg.Studenti == nil {
		t.Errorf("Studenti deve essere mappa vuota, non nil")
	}
}

// TestSetStudentiIPs copre cio' che ha sostituito le API
// /api/students/{set,delete,clear}, rimosse in v2.6.0 insieme al setup
// manuale della classe: la mappa non e' piu' editabile ne' persistita, viene
// rigenerata a ogni boot dal /24 del PC docente corrente.
func TestSetStudentiIPs(t *testing.T) {
	b := &mockBroker{}
	s := New(b)

	s.SetStudentiIPs([]string{"192.168.1.1", " 192.168.1.2 ", "", "   "})

	got := s.ConfigSnapshotData().Studenti
	if len(got) != 2 {
		t.Fatalf("studenti = %d, attesi 2 (vuoti e spazi scartati): %+v", len(got), got)
	}
	// Gli IP sono trimmati e il nome e' sempre vuoto: la UI mostra l'IP.
	for _, ip := range []string{"192.168.1.1", "192.168.1.2"} {
		nome, ok := got[ip]
		if !ok {
			t.Errorf("IP %q assente dalla mappa: %+v", ip, got)
		}
		if nome != "" {
			t.Errorf("nome per %q = %q, atteso vuoto", ip, nome)
		}
	}

	// Sostituzione in blocco: la chiamata successiva non fa merge.
	s.SetStudentiIPs([]string{"10.0.0.7"})
	got = s.ConfigSnapshotData().Studenti
	if len(got) != 1 {
		t.Errorf("dopo sostituzione studenti = %d, atteso 1: %+v", len(got), got)
	}
	if _, ok := got["192.168.1.1"]; ok {
		t.Errorf("IP della lista precedente ancora presente: %+v", got)
	}

	if b.Count() == 0 {
		t.Errorf("SetStudentiIPs deve broadcastare la nuova mappa")
	}
}

func TestHistorySnapshot(t *testing.T) {
	b := &mockBroker{}
	s := New(b)
	s.RegistraTraffic("ip1", "GET", "x.com", false, classify.TipoUtente)
	s.RegistraAlive("ip2")

	h := s.HistorySnapshotData()
	if len(h.Entries) != 1 {
		t.Errorf("Entries len = %d, atteso 1", len(h.Entries))
	}
	if len(h.Alive) != 1 || h.Alive["ip2"] == 0 {
		t.Errorf("Alive snapshot non corretta: %+v", h.Alive)
	}
	if h.Bloccati == nil {
		t.Errorf("Bloccati deve essere slice vuoto, non nil")
	}
}

func TestSettingsSnapshotPasswordMasked(t *testing.T) {
	b := &mockBroker{}
	s := New(b)
	// Forzo manualmente un hash per simulare auth con password impostata.
	s.authPasswordHash = "$2a$10$some-fake-bcrypt-hash"

	settings := s.SettingsSnapshotData()
	if settings.Web.Auth.Password != "" {
		t.Errorf("Password leakata: %q (atteso stringa vuota)", settings.Web.Auth.Password)
	}
	if !settings.Web.Auth.PasswordSet {
		t.Errorf("PasswordSet = false ma hash e' impostato")
	}
}

func TestSessionStatusDurata(t *testing.T) {
	b := &mockBroker{}
	s := New(b)

	// Sessione mai avviata -> durata 0
	if got := s.SessionStatusData(); got.DurataSec != 0 {
		t.Errorf("durata sessione mai avviata = %d, atteso 0", got.DurataSec)
	}

	// Sessione "ferma" con inizio + fine impostati a mano
	s.sessioneInizio = "2026-04-22T10:00:00Z"
	s.sessioneFineISO = "2026-04-22T11:30:00Z"
	s.sessioneAttiva = false

	got := s.SessionStatusData()
	if got.DurataSec != 5400 { // 1h 30m
		t.Errorf("durata = %d, atteso 5400 (1h30m)", got.DurataSec)
	}
}

func TestConcorrenza(t *testing.T) {
	// 100 goroutine x 100 registrazioni: verifica che il count totale sia
	// coerente e nessuna race rompa il ring buffer.
	b := &mockBroker{}
	s := New(b)

	const goroutines = 100
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				s.RegistraTraffic("ip", "GET", "x.com", false, classify.TipoUtente)
			}
		}()
	}
	wg.Wait()

	expected := goroutines * perGoroutine
	if expected > MaxStoria {
		expected = MaxStoria
	}
	if got := len(s.SnapshotStoria()); got != expected {
		t.Errorf("dopo concorrenza, storia = %d, atteso %d", got, expected)
	}
	// Il broker conta tutti i broadcast (non capped come la storia).
	if got := b.Count(); got != goroutines*perGoroutine {
		t.Errorf("broker count = %d, atteso %d", got, goroutines*perGoroutine)
	}
}
