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
)

const (
	testKeyName  = "planck-test"
	testKeyPath  = "../../test/veyon-rig/keys/planck-test_private.pem"
	testServer   = "localhost:11100"
)

func TestIntegrationDialAuth(t *testing.T) {
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
		Username:   "",
		PrivateKey: key,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	t.Logf("connessione + auth OK contro %s", testServer)
}
