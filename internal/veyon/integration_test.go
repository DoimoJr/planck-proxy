//go:build integration

// Package veyon integration tests — richiedono un server Veyon vivo su
// localhost:11100. Lanciare con:
//
//	docker run -d --name planck-veyon-test -p 11100:11100 planck-veyon-rig
//	docker cp planck-veyon-test:/export/. test/veyon-rig/keys/
//	go test -tags integration ./internal/veyon/
//
// Senza il tag `integration` questi test non vengono compilati ne'
// eseguiti, quindi `go test ./...` di routine non si rompe se Docker
// e' fermo.
package veyon

import (
	"os"
	"testing"
	"time"
)

const (
	testKeyName  = "planck-test"
	testKeyPath  = "../../test/veyon-rig/keys/planck-test_private.pem"
	testServer   = "localhost:11100"
)

// dialRig apre una connessione contro il Docker rig. Helper condiviso
// dai test integration di questo file.
func dialRig(t *testing.T) *Conn {
	t.Helper()
	pemBytes, err := os.ReadFile(testKeyPath)
	if err != nil {
		t.Fatalf("leggi chiave (rig avviato?): %v", err)
	}
	key, err := LoadPrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("parse PEM: %v", err)
	}
	conn, err := Dial(Config{
		Addr:       testServer,
		KeyName:    testKeyName,
		PrivateKey: key,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return conn
}

func TestIntegrationDialAuth(t *testing.T) {
	conn := dialRig(t)
	defer conn.Close()
	si := conn.ServerInit()
	t.Logf("connessione + auth OK; ServerInit %dx%d %q", si.Width, si.Height, si.Name)
}

func TestIntegrationScreenLock(t *testing.T) {
	conn := dialRig(t)
	defer conn.Close()
	if err := conn.ScreenLock(); err != nil {
		t.Fatalf("ScreenLock: %v", err)
	}
	t.Logf("ScreenLock inviato")
	// Non c'e' modo robusto di asserire che il server l'abbia eseguito
	// (e' headless). Se non si solleva eccezione e la connessione resta
	// aperta, considera success.
	time.Sleep(200 * time.Millisecond)
}

func TestIntegrationStartApp(t *testing.T) {
	conn := dialRig(t)
	defer conn.Close()
	if err := conn.StartApp([]string{"xterm"}); err != nil {
		t.Fatalf("StartApp: %v", err)
	}
	t.Logf("StartApp inviato")
	time.Sleep(200 * time.Millisecond)
}

func TestIntegrationTextMessage(t *testing.T) {
	conn := dialRig(t)
	defer conn.Close()
	if err := conn.TextMessage("Test da Planck integration suite"); err != nil {
		t.Fatalf("TextMessage: %v", err)
	}
	t.Logf("TextMessage inviato")
	time.Sleep(200 * time.Millisecond)
}
