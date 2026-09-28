package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestConfronta(t *testing.T) {
	casi := []struct {
		a, b string
		vuoi int
	}{
		{"2.9.28", "2.9.25", 1},
		{"2.9.25", "2.9.28", -1},
		{"2.9.28", "2.9.28", 0},
		{"v2.9.28", "2.9.28", 0}, // la "v" del tag non conta
		{"2.10.0", "2.9.28", 1},  // confronto numerico, non lessicografico
		{"2.9.9", "2.9.13", -1},  // idem
		{"3.0.0", "2.9.28", 1},
		{"2.0.0", "2.0.0-alpha.5", 1}, // release > pre-release
		{"2.0.0-alpha.5", "2.0.0", -1},
		{"2.0.0-alpha.5", "2.0.0-alpha.4", 1},
		{"2.9", "2.9.0", 0}, // segmenti mancanti valgono 0
	}
	for _, c := range casi {
		if got := Confronta(c.a, c.b); got != c.vuoi {
			t.Errorf("Confronta(%q, %q) = %d, atteso %d", c.a, c.b, got, c.vuoi)
		}
	}
}

// serverFinto simula la risposta di GitHub per l'ultima release.
func serverFinto(t *testing.T, tag string, assetName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{
			"tag_name": tag,
			"html_url": "https://esempio.test/release",
			"body":     "note della release",
			"assets": []map[string]any{
				{"name": assetName, "size": 1234, "browser_download_url": "https://esempio.test/" + assetName},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestCheckTrovaAggiornamento(t *testing.T) {
	srv := serverFinto(t, "v2.9.99", NomeAsset())
	defer srv.Close()
	t.Setenv("PLANCK_UPDATE_API", srv.URL)

	info, err := Check(context.Background(), "2.9.28")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !info.Disponibile {
		t.Errorf("Disponibile = false, atteso true (2.9.99 > 2.9.28)")
	}
	if info.Ultima != "2.9.99" {
		t.Errorf("Ultima = %q, atteso 2.9.99 (senza la v)", info.Ultima)
	}
	if info.AssetByte != 1234 {
		t.Errorf("AssetByte = %d, atteso 1234", info.AssetByte)
	}
}

func TestCheckGiaAggiornato(t *testing.T) {
	srv := serverFinto(t, "v2.9.28", NomeAsset())
	defer srv.Close()
	t.Setenv("PLANCK_UPDATE_API", srv.URL)

	info, err := Check(context.Background(), "2.9.28")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.Disponibile {
		t.Errorf("Disponibile = true con versioni pari, atteso false")
	}
}

// Una release piu' recente ma senza asset per questa piattaforma non e'
// installabile: Disponibile deve restare false.
func TestCheckSenzaAssetPerLaPiattaforma(t *testing.T) {
	srv := serverFinto(t, "v9.9.9", "planck-qualcosaltro.bin")
	defer srv.Close()
	t.Setenv("PLANCK_UPDATE_API", srv.URL)

	info, err := Check(context.Background(), "2.9.28")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.Disponibile {
		t.Errorf("Disponibile = true senza asset per %s, atteso false", runtime.GOOS)
	}
}

func TestVerificaScartaDownloadRotti(t *testing.T) {
	buono := eseguibileFinto()
	if err := verifica(buono, int64(len(buono))); err != nil {
		t.Errorf("verifica su binario valido: %v", err)
	}
	if err := verifica(nil, 0); err == nil {
		t.Errorf("download vuoto accettato")
	}
	if err := verifica(buono, int64(len(buono))+1); err == nil {
		t.Errorf("download troncato accettato (dimensione diversa da quella dichiarata)")
	}
	if err := verifica([]byte("<!DOCTYPE html><html>errore</html>"), 0); err == nil {
		t.Errorf("pagina HTML accettata come eseguibile")
	}
}

// eseguibileFinto produce dei magic bytes validi per la piattaforma su
// cui gira il test, cosi' il test passa sia su macOS sia su Windows.
func eseguibileFinto() []byte {
	switch runtime.GOOS {
	case "windows":
		return append([]byte{'M', 'Z'}, make([]byte, 100)...)
	case "darwin":
		return append([]byte{0xcf, 0xfa, 0xed, 0xfe}, make([]byte, 100)...)
	default:
		return append([]byte{0x7f, 'E', 'L', 'F'}, make([]byte, 100)...)
	}
}
