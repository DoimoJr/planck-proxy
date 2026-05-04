package state

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/veyon"
)

// veyonKeyFilename e' il nome del file dove la master key privata
// viene persistita su disco accanto a planck.db. Il file e' creato con
// permessi 0600 al primo upload.
const veyonKeyFilename = "veyon-master.pem"

// veyonKeyPath ritorna il path assoluto del file della master key.
func (s *State) veyonKeyPath() string {
	dir := s.store.DataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, veyonKeyFilename)
}

// VeyonStatus rappresenta lo stato di configurazione Veyon esposto via API.
type VeyonStatus struct {
	Configured bool   `json:"configured"`
	KeyName    string `json:"keyName"`
	Port       int    `json:"port"`
}

// VeyonStatusData ritorna lo stato corrente per /api/veyon/status.
func (s *State) VeyonStatusData() VeyonStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	port := s.veyonPort
	if port == 0 {
		port = veyon.DefaultPort
	}
	configured := s.veyonKeyName != "" && fileExists(s.veyonKeyPath())
	return VeyonStatus{
		Configured: configured,
		KeyName:    s.veyonKeyName,
		Port:       port,
	}
}

// VeyonConfigure persiste master keyfile (PEM) + keyName sul disco.
// Sovrascrive eventuale configurazione precedente.
func (s *State) VeyonConfigure(keyName string, privateKeyPEM []byte) error {
	if keyName == "" {
		return fmt.Errorf("veyon: keyName richiesto")
	}
	// Valida che il PEM sia parseabile prima di scriverlo su disco.
	if _, err := veyon.LoadPrivateKeyPEM(privateKeyPEM); err != nil {
		return fmt.Errorf("veyon: PEM invalido: %w", err)
	}
	path := s.veyonKeyPath()
	if path == "" {
		return fmt.Errorf("veyon: dataDir non disponibile (NoOp store?)")
	}
	if err := os.WriteFile(path, privateKeyPEM, 0o600); err != nil {
		return fmt.Errorf("veyon: write keyfile: %w", err)
	}

	s.mu.Lock()
	s.veyonKeyName = keyName
	s.saveConfigLocked()
	settings := s.settingsSnapshotLocked()
	s.mu.Unlock()

	s.broadcastSettings(settings)
	return nil
}

// AutoImportVeyonKey tenta l'auto-import della master key Veyon via
// `veyon-cli authkeys export` al boot. No-op se Veyon e' gia' configurato
// (l'utente ha gia' caricato una chiave manualmente) o se veyon-cli non e'
// presente sulla macchina del docente.
//
// Errore non-nil indica solo che l'auto-import non e' andato a buon fine:
// il chiamante puo' loggare e continuare — il flusso manuale via
// /api/veyon/configure resta sempre disponibile.
func (s *State) AutoImportVeyonKey() (string, error) {
	s.mu.RLock()
	already := s.veyonKeyName != "" && fileExists(s.veyonKeyPath())
	s.mu.RUnlock()
	if already {
		return "", nil
	}
	dir := s.store.DataDir()
	if dir == "" {
		return "", fmt.Errorf("dataDir non disponibile (NoOp store?)")
	}
	res, err := veyon.AutoImport(dir)
	if err != nil {
		return "", err
	}
	if err := s.VeyonConfigure(res.KeyName, res.PEMBytes); err != nil {
		return "", fmt.Errorf("configure dopo auto-import: %w", err)
	}
	return res.KeyName, nil
}

// VeyonClear rimuove la configurazione Veyon (file su disco + keyName).
func (s *State) VeyonClear() error {
	path := s.veyonKeyPath()
	if path != "" {
		_ = os.Remove(path) // best effort: file potrebbe non esistere
	}
	s.mu.Lock()
	s.veyonKeyName = ""
	s.saveConfigLocked()
	settings := s.settingsSnapshotLocked()
	s.mu.Unlock()
	s.broadcastSettings(settings)
	return nil
}

// VeyonSendFeature apre una connessione one-shot al PC studente con IP `ip`,
// invia il FeatureMessage, chiude. Pattern dial-send-close: niente pool
// di connessioni in Phase 3e (overhead ~500ms/comando, accettabile per
// click manuali UI).
//
// Errore se Veyon non e' configurato o se il dial/auth fallisce.
func (s *State) VeyonSendFeature(ip string, fm veyon.FeatureMessage) error {
	conn, err := s.veyonDial(ip)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.SendFeature(fm)
}

// VeyonSendFile invia un file via FileTransfer feature al PC studente.
// Se openInApp=true il file viene aperto col programma associato
// (utile per .bat → cmd.exe lo esegue).
func (s *State) VeyonSendFile(ip, filename string, content []byte, openInApp, overwrite bool) error {
	conn, err := s.veyonDial(ip)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.SendFile(filename, content, openInApp, overwrite)
}

// VeyonTest tenta una connessione + auth verso `ip` e poi chiude. Usato
// dall'endpoint /api/veyon/test per validare la configurazione contro
// uno specifico studente.
func (s *State) VeyonTest(ip string) error {
	conn, err := s.veyonDial(ip)
	if err != nil {
		return err
	}
	return conn.Close()
}

// veyonDial leggi keyfile + keyName e fa il Dial. Helper interno.
func (s *State) veyonDial(ip string) (*veyon.Conn, error) {
	s.mu.RLock()
	keyName := s.veyonKeyName
	port := s.veyonPort
	s.mu.RUnlock()
	if keyName == "" {
		return nil, fmt.Errorf("veyon: non configurato (manca master key)")
	}
	if port == 0 {
		port = veyon.DefaultPort
	}

	keyPath := s.veyonKeyPath()
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("veyon: read keyfile %s: %w", keyPath, err)
	}
	key, err := veyon.LoadPrivateKeyPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("veyon: parse keyfile: %w", err)
	}
	return veyon.Dial(veyon.Config{
		Addr:        fmt.Sprintf("%s:%d", ip, port),
		KeyName:     keyName,
		PrivateKey:  key,
		DialTimeout: 5 * time.Second,
	})
}

// veyonPrivateKey carica la chiave privata dal disco. Usato dai test.
func (s *State) veyonPrivateKey() (*rsa.PrivateKey, error) {
	path := s.veyonKeyPath()
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return veyon.LoadPrivateKeyPEM(pemBytes)
}

// fileExists ritorna true se path indica un file regolare leggibile.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
