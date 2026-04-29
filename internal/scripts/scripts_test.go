package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	on, off, err := Generate(dir, "2.0.0-test", "192.168.1.100", 9090, 9999)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if filepath.Base(on) != "proxy_on.bat" || filepath.Base(off) != "proxy_off.bat" {
		t.Errorf("path inattesi: on=%s off=%s", on, off)
	}

	onContent, _ := os.ReadFile(on)
	body := string(onContent)
	if !strings.Contains(body, "set IP_PROF=192.168.1.100") {
		t.Errorf("proxy_on.bat: IP non sostituito\n%s", body)
	}
	if !strings.Contains(body, "set PORTA=9090") {
		t.Errorf("proxy_on.bat: porta non sostituita\n%s", body)
	}
	if strings.Contains(body, "__IP_DOCENTE__") || strings.Contains(body, "__PORTA_PROXY__") {
		t.Errorf("proxy_on.bat: segnaposti non sostituiti")
	}

	offContent, _ := os.ReadFile(off)
	if !strings.Contains(string(offContent), "Disattiva proxy") &&
		!strings.Contains(string(offContent), "Disattiva il proxy") {
		t.Errorf("proxy_off.bat: contenuto inatteso\n%s", offContent)
	}
}

func TestGenerateRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Generate(dir, "v", "", 9090, 9999); err == nil {
		t.Errorf("ipDocente vuoto: atteso errore")
	}
	if _, _, err := Generate(dir, "v", "1.2.3.4", 0, 9999); err == nil {
		t.Errorf("porta 0: atteso errore")
	}
	if _, _, err := Generate(dir, "v", "1.2.3.4", 99999, 9999); err == nil {
		t.Errorf("porta 99999: atteso errore")
	}
}

func TestLocalLANIP(t *testing.T) {
	// Test best-effort: deve ritornare qualcosa di non vuoto.
	// Ambiente CI potrebbe avere solo loopback → torna 127.0.0.1.
	ip := LocalLANIP()
	if ip == "" {
		t.Errorf("LocalLANIP() ha ritornato stringa vuota")
	}
	t.Logf("LocalLANIP detected: %s", ip)
}
