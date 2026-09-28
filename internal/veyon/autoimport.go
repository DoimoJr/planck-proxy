// Auto-import della master key Veyon al boot.
//
// Veyon Configurator (l'app GUI con cui il docente configura il proprio
// PC come master) salva le chiavi auth in un keystore interno gestito
// dal CLI `veyon-cli`. Il flusso manuale richiederebbe al docente di:
// (1) aprire Veyon Configurator, (2) navigare ad Authentication Keys,
// (3) esportare la master key, (4) caricarla nelle Impostazioni di
// Planck. Tutto evitabile con `veyon-cli authkeys export`.
//
// Questo file:
//   - Localizza l'eseguibile `veyon-cli` nei path standard.
//   - Lista le auth keys note al keystore di Veyon.
//   - Esporta la prima master key (quella che ha sia private che public)
//     in un file temporaneo, ne legge i bytes PEM, e la passa al chiamante.
//
// Errori sono best-effort: se veyon-cli manca o l'export fallisce, il
// chiamante continua col flusso manuale (upload via UI Settings).

package veyon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DoimoJr/planck-proxy/internal/sysutil"
)

// AuthKey rappresenta una entry del keystore Veyon: nome logico
// (es. "master", "teacher") + tipo (private | public).
type AuthKey struct {
	Name string
	Type string
}

// FindCLI cerca l'eseguibile `veyon-cli` nei path standard di Windows
// e Linux (+ macOS via brew tap, se presente). Ritorna stringa vuota
// se non trovato.
//
// Su Windows preferisce le install di Veyon nei Program Files; su Linux
// usa /usr/bin e /usr/local/bin. In ogni caso prova `exec.LookPath`
// per onorare il PATH dell'utente.
func FindCLI() string {
	binName := "veyon-cli"
	if runtime.GOOS == "windows" {
		binName = "veyon-cli.exe"
	}

	// 1) PATH dell'utente.
	if p, err := exec.LookPath(binName); err == nil {
		return p
	}

	// 2) Path standard per OS.
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\Veyon\veyon-cli.exe`,
			`C:\Program Files (x86)\Veyon\veyon-cli.exe`,
		}
	case "darwin":
		candidates = []string{
			"/Applications/Veyon.app/Contents/MacOS/veyon-cli",
			"/usr/local/bin/veyon-cli",
			"/opt/homebrew/bin/veyon-cli",
		}
	default: // linux + altri unix
		candidates = []string{
			"/usr/bin/veyon-cli",
			"/usr/local/bin/veyon-cli",
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// ListAuthKeys esegue `veyon-cli authkeys list` e parsa l'output. Ogni
// riga ha il formato `<nome>/<tipo>` (es. `master/private`,
// `master/public`).
func ListAuthKeys(cliPath string) ([]AuthKey, error) {
	cmd := exec.Command(cliPath, "authkeys", "list")
	sysutil.HideConsoleWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("veyon-cli authkeys list fallito: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	var keys []AuthKey
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			continue
		}
		keys = append(keys, AuthKey{Name: parts[0], Type: parts[1]})
	}
	return keys, nil
}

// ExportPrivateKey scrive la chiave privata di nome `keyName` in
// `destPath` via `veyon-cli authkeys export <name>/private <path>`.
// Sovrascrive il file destinazione se esiste.
func ExportPrivateKey(cliPath, keyName, destPath string) error {
	_ = os.Remove(destPath) // veyon-cli rifiuta di sovrascrivere
	cmd := exec.Command(cliPath, "authkeys", "export", keyName+"/private", destPath)
	sysutil.HideConsoleWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("veyon-cli authkeys export %s: %w (output: %s)", keyName, err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(destPath); err != nil {
		return fmt.Errorf("veyon-cli export ha riportato successo ma il file %s non esiste: %w", destPath, err)
	}
	return nil
}

// AutoImportResult contiene il risultato di AutoImport.
type AutoImportResult struct {
	KeyName  string // nome della chiave importata (es. "master", "teacher")
	PEMBytes []byte // contenuto raw del file PEM esportato
	CLIPath  string // path di veyon-cli usato (per debug log)
}

// AutoImport tenta l'auto-import della prima chiave master Veyon disponibile
// nel keystore. Best-effort: in caso di errore (CLI mancante, nessuna
// chiave, export fallito) ritorna l'errore — il chiamante decide se
// loggare e proseguire silenziosamente.
//
// `dataDir` e' usato per il file temporaneo di export (rimosso prima
// del return — il chiamante e' responsabile di rinominarlo / passarlo
// allo state).
//
// Se ci sono piu' chiavi che hanno entrambi private+public, prende
// quella con nome "master" se esiste, altrimenti la prima che trova.
func AutoImport(dataDir string) (*AutoImportResult, error) {
	cli := FindCLI()
	if cli == "" {
		return nil, fmt.Errorf("veyon-cli non trovato (Veyon Configurator probabilmente non installato)")
	}

	keys, err := ListAuthKeys(cli)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("nessuna chiave nel keystore di Veyon (aprire Veyon Configurator e creare/importare una master key)")
	}

	// Raggruppa per nome: solo le chiavi che hanno entrambi private e
	// public sono utilizzabili come master.
	byName := map[string]map[string]bool{}
	for _, k := range keys {
		if byName[k.Name] == nil {
			byName[k.Name] = map[string]bool{}
		}
		byName[k.Name][k.Type] = true
	}
	var chosen string
	if t := byName["master"]; t["private"] && t["public"] {
		chosen = "master"
	} else {
		for name, t := range byName {
			if t["private"] && t["public"] {
				chosen = name
				break
			}
		}
	}
	if chosen == "" {
		return nil, fmt.Errorf("nessuna chiave master valida (servono sia private che public per lo stesso nome)")
	}

	tmpPath := filepath.Join(dataDir, ".veyon-master-import.pem")
	defer os.Remove(tmpPath)

	if err := ExportPrivateKey(cli, chosen, tmpPath); err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read keyfile esportato: %w", err)
	}

	// Sanity check: il PEM deve essere parseabile.
	if _, err := LoadPrivateKeyPEM(bytes); err != nil {
		return nil, fmt.Errorf("PEM esportato non parseabile: %w", err)
	}

	return &AutoImportResult{
		KeyName:  chosen,
		PEMBytes: bytes,
		CLIPath:  cli,
	}, nil
}
