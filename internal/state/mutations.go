package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/classify"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================
// Mutazioni Phase 1.5
// ============================================================

// ============================================================
// Blocklist
// ============================================================

// Block aggiunge un dominio alla blocklist e broadcasta `{type:"blocklist"}`.
// Idempotente: aggiungere un dominio gia' presente non e' un errore.
func (s *State) Block(dominio string) {
	d := strings.ToLower(strings.TrimSpace(dominio))
	if d == "" {
		return
	}
	s.mu.Lock()
	s.bloccati[d] = struct{}{}
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.broadcastBlocklist(list)
}

// Unblock rimuove un dominio dalla blocklist.
// Idempotente: rimuovere un dominio non presente non e' un errore.
func (s *State) Unblock(dominio string) {
	d := strings.ToLower(strings.TrimSpace(dominio))
	if d == "" {
		return
	}
	s.mu.Lock()
	delete(s.bloccati, d)
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.broadcastBlocklist(list)
}

// BlockAllAI aggiunge tutti i domini AI noti (lista classify.DominiAI) alla blocklist.
func (s *State) BlockAllAI() {
	s.mu.Lock()
	for _, d := range classify.DominiAI {
		s.bloccati[d] = struct{}{}
	}
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.broadcastBlocklist(list)
}

// UnblockAllAI rimuove tutti i domini AI dalla blocklist.
func (s *State) UnblockAllAI() {
	s.mu.Lock()
	for _, d := range classify.DominiAI {
		delete(s.bloccati, d)
	}
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.broadcastBlocklist(list)
}

// ClearBlocklist svuota completamente la blocklist.
func (s *State) ClearBlocklist() {
	s.mu.Lock()
	s.bloccati = map[string]struct{}{}
	s.mu.Unlock()
	s.broadcastBlocklist([]string{})
}

func (s *State) broadcastBlocklist(list []string) {
	s.broker.Broadcast(struct {
		Type string   `json:"type"`
		List []string `json:"list"`
	}{Type: "blocklist", List: list})
}

// ============================================================
// Logica di blocco (chiamata dal proxy per ogni richiesta)
// ============================================================

// DominioBloccato decide se il proxy deve respingere il dominio con 403.
//
// Logica (vedi SPEC §3.4):
//  1. Se `dominio` matcha `dominiIgnorati` (substring) → false (passa sempre)
//  2. Se `pausato` → true (blocca tutto tranne ignorati)
//  3. Modo `allowlist`: blocca se NON matcha la blocklist
//  4. Modo `blocklist` (default): blocca se matcha la blocklist
//
// Match case-insensitive, sostringa.
func (s *State) DominioBloccato(dominio string) bool {
	d := strings.ToLower(dominio)
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Ignorati passano sempre.
	for _, ig := range s.dominiIgnorati {
		if strings.Contains(d, strings.ToLower(ig)) {
			return false
		}
	}
	// 2. Pausa globale: blocca tutto.
	if s.pausato {
		return true
	}
	// 3+4. Match con la blocklist.
	matchInLista := false
	for bl := range s.bloccati {
		if strings.Contains(d, bl) {
			matchInLista = true
			break
		}
	}
	if s.modo == "allowlist" {
		return !matchInLista
	}
	return matchInLista // default: blocklist
}

// ============================================================
// Pausa globale
// ============================================================

// SetPausa imposta esplicitamente lo stato di pausa. Idempotente.
// Ritorna lo stato finale (eq. al parametro).
func (s *State) SetPausa(p bool) bool {
	s.mu.Lock()
	s.pausato = p
	s.mu.Unlock()
	s.broadcastPausa(p)
	return p
}

// TogglePausa inverte lo stato di pausa. Ritorna il nuovo stato.
func (s *State) TogglePausa() bool {
	s.mu.Lock()
	s.pausato = !s.pausato
	v := s.pausato
	s.mu.Unlock()
	s.broadcastPausa(v)
	return v
}

func (s *State) broadcastPausa(p bool) {
	s.broker.Broadcast(struct {
		Type    string `json:"type"`
		Pausato bool   `json:"pausato"`
	}{Type: "pausa", Pausato: p})
}

// ============================================================
// Deadline / countdown
// ============================================================

// SetDeadline programma una scadenza assoluta dato l'orario "HH:MM" (locale).
// Se l'orario e' gia' passato per oggi, viene risolto a domani.
//
// Schedula un time.AfterFunc che broadcasta `{type:"deadline-reached"}`
// allo scadere. Cancella eventuale timer precedente.
//
// Ritorna l'ISO 8601 calcolato.
func (s *State) SetDeadline(timeStr string) (string, error) {
	var h, m int
	if _, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m); err != nil {
		return "", fmt.Errorf("formato HH:MM richiesto, ottenuto %q", timeStr)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return "", fmt.Errorf("ora invalida: %02d:%02d", h, m)
	}

	now := time.Now()
	target := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}

	s.mu.Lock()
	if s.deadlineTimer != nil {
		s.deadlineTimer.Stop()
	}
	s.deadlineISO = target.UTC().Format(time.RFC3339)
	delay := time.Until(target)
	deadlineISO := s.deadlineISO
	s.deadlineTimer = time.AfterFunc(delay, func() {
		s.broker.Broadcast(struct {
			Type        string `json:"type"`
			DeadlineISO string `json:"deadlineISO"`
		}{Type: "deadline-reached", DeadlineISO: deadlineISO})
	})
	s.mu.Unlock()

	s.broadcastDeadline(deadlineISO)
	return deadlineISO, nil
}

// ClearDeadline annulla la scadenza programmata e cancella il timer.
func (s *State) ClearDeadline() {
	s.mu.Lock()
	if s.deadlineTimer != nil {
		s.deadlineTimer.Stop()
		s.deadlineTimer = nil
	}
	s.deadlineISO = ""
	s.mu.Unlock()
	s.broadcastDeadline("")
}

func (s *State) broadcastDeadline(iso string) {
	s.broker.Broadcast(struct {
		Type        string `json:"type"`
		DeadlineISO string `json:"deadlineISO"`
	}{Type: "deadline", DeadlineISO: iso})
}

// ============================================================
// Studenti (mappa IP → nome)
// ============================================================

// SetStudent aggiunge o aggiorna una voce IP→nome.
func (s *State) SetStudent(ip, nome string) {
	ip = strings.TrimSpace(ip)
	nome = strings.TrimSpace(nome)
	if ip == "" || nome == "" {
		return
	}
	s.mu.Lock()
	s.studenti[ip] = nome
	stud := s.studentiCopyLocked()
	s.mu.Unlock()
	s.broadcastStudenti(stud)
}

// DeleteStudent rimuove una voce dalla mappa.
func (s *State) DeleteStudent(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	s.mu.Lock()
	delete(s.studenti, ip)
	stud := s.studentiCopyLocked()
	s.mu.Unlock()
	s.broadcastStudenti(stud)
}

// ClearStudents svuota tutta la mappa.
func (s *State) ClearStudents() {
	s.mu.Lock()
	s.studenti = map[string]string{}
	s.mu.Unlock()
	s.broadcastStudenti(map[string]string{})
}

func (s *State) studentiCopyLocked() map[string]string {
	out := make(map[string]string, len(s.studenti))
	for k, v := range s.studenti {
		out[k] = v
	}
	return out
}

func (s *State) broadcastStudenti(stud map[string]string) {
	s.broker.Broadcast(struct {
		Type     string            `json:"type"`
		Studenti map[string]string `json:"studenti"`
	}{Type: "studenti", Studenti: stud})
}

// ============================================================
// Domini ignorati
// ============================================================

// AddIgnorato aggiunge un dominio alla lista ignorati (no duplicati).
func (s *State) AddIgnorato(dominio string) {
	d := strings.TrimSpace(dominio)
	if d == "" {
		return
	}
	s.mu.Lock()
	for _, x := range s.dominiIgnorati {
		if x == d {
			s.mu.Unlock()
			return // gia' presente
		}
	}
	s.dominiIgnorati = append(s.dominiIgnorati, d)
	settings := s.settingsSnapshotLocked()
	s.mu.Unlock()
	s.broadcastSettings(settings)
}

// RemoveIgnorato rimuove un dominio dalla lista ignorati.
func (s *State) RemoveIgnorato(dominio string) {
	d := strings.TrimSpace(dominio)
	if d == "" {
		return
	}
	s.mu.Lock()
	out := s.dominiIgnorati[:0]
	for _, x := range s.dominiIgnorati {
		if x != d {
			out = append(out, x)
		}
	}
	s.dominiIgnorati = out
	settings := s.settingsSnapshotLocked()
	s.mu.Unlock()
	s.broadcastSettings(settings)
}

// ============================================================
// Sessione (start/stop)
// ============================================================

// SessionStart apre una nuova sessione di registrazione:
// archivia la precedente (stub in 1.5, NDJSON+JSON in 1.6),
// azzera il ring buffer, imposta sessioneInizio = now.
//
// Ritorna `(sessioneInizio, archiviata)`. `archiviata` e' il nome del
// file archivio prodotto (vuoto in 1.5; popolato in 1.6).
func (s *State) SessionStart() (sessioneInizio string, archiviata string) {
	s.mu.Lock()

	// Archivia la precedente se aveva dati (stub Phase 1.5)
	archiviata = s.archiveLocked()

	s.storia = make([]Entry, 0, 256)
	s.sessioneInizio = time.Now().UTC().Format(time.RFC3339)
	s.sessioneFineISO = ""
	s.sessioneAttiva = true
	inizio := s.sessioneInizio
	s.mu.Unlock()

	// Reset notifica al client di pulire i suoi buffer.
	s.broker.Broadcast(struct {
		Type           string `json:"type"`
		SessioneInizio string `json:"sessioneInizio"`
	}{Type: "reset", SessioneInizio: inizio})
	s.broadcastSessionState()
	return inizio, archiviata
}

// SessionStop chiude la sessione corrente:
// archivia il buffer (stub in 1.5), imposta sessioneFineISO = now.
// Il buffer NON viene azzerato: resta visibile in UI per consultazione
// finche' non si avvia una nuova sessione.
//
// Ritorna `(archiviata, sessioneFineISO)`. No-op se la sessione non e' attiva.
func (s *State) SessionStop() (archiviata, fineISO string) {
	s.mu.Lock()
	if !s.sessioneAttiva {
		s.mu.Unlock()
		return "", ""
	}
	archiviata = s.archiveLocked()
	s.sessioneAttiva = false
	s.sessioneFineISO = time.Now().UTC().Format(time.RFC3339)
	fine := s.sessioneFineISO
	s.mu.Unlock()
	s.broadcastSessionState()
	return archiviata, fine
}

// archiveLocked scrive la sessione corrente su disco.
// **Stub in Phase 1.5**: ritorna sempre "" (nessun archivio).
// In Phase 1.6 leggera' NDJSON e produrra' un JSON snapshot in sessioni/.
func (s *State) archiveLocked() string {
	// TODO Phase 1.6: serialize NDJSON + write sessioni/<timestamp>.json
	return ""
}

func (s *State) broadcastSessionState() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.broker.Broadcast(struct {
		Type            string `json:"type"`
		SessioneAttiva  bool   `json:"sessioneAttiva"`
		SessioneInizio  string `json:"sessioneInizio,omitempty"`
		SessioneFineISO string `json:"sessioneFineISO,omitempty"`
	}{
		Type:            "session-state",
		SessioneAttiva:  s.sessioneAttiva,
		SessioneInizio:  s.sessioneInizio,
		SessioneFineISO: s.sessioneFineISO,
	})
}

// ============================================================
// Settings update (multi-key validato)
// ============================================================

// UpdateSettings applica un set di coppie key→value alla config in modo
// atomico. Chiavi non riconosciute o con valori invalidi finiscono in
// `rejected`. Chiavi che richiedono restart (ports, auth.*) finiscono in
// `richiedeRiavvio` per far accendere il banner UI.
//
// Le keys sono dotted-path: "proxy.port", "web.auth.password", ecc.
// Valori numerici arrivano come float64 dal JSON decode (vedi toInt).
func (s *State) UpdateSettings(updates map[string]any) (updated, rejected, richiedeRiavvio []string) {
	s.mu.Lock()

	for key, val := range updates {
		switch key {
		case "titolo":
			if v, ok := val.(string); ok && len(v) <= 200 {
				s.titolo = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "classe":
			if v, ok := val.(string); ok && len(v) <= 100 {
				s.classe = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "modo":
			v, ok := val.(string)
			if ok && (v == "blocklist" || v == "allowlist") {
				s.modo = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "inattivitaSogliaSec":
			if v, ok := toInt(val); ok && v >= 10 && v <= 3600 {
				s.inattivitaSogliaSec = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "proxy.port":
			if v, ok := toInt(val); ok && v >= 1024 && v <= 65535 {
				s.proxyPort = v
				updated = append(updated, key)
				richiedeRiavvio = append(richiedeRiavvio, key)
			} else {
				rejected = append(rejected, key)
			}
		case "web.port":
			if v, ok := toInt(val); ok && v >= 1024 && v <= 65535 {
				s.webPort = v
				updated = append(updated, key)
				richiedeRiavvio = append(richiedeRiavvio, key)
			} else {
				rejected = append(rejected, key)
			}
		case "web.auth.enabled":
			if v, ok := val.(bool); ok {
				s.authEnabled = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "web.auth.user":
			if v, ok := val.(string); ok && len(v) > 0 && len(v) < 100 {
				s.authUser = v
				updated = append(updated, key)
			} else {
				rejected = append(rejected, key)
			}
		case "web.auth.password":
			if v, ok := val.(string); ok && len(v) > 0 && len(v) < 200 {
				if hash, err := bcrypt.GenerateFromPassword([]byte(v), bcrypt.DefaultCost); err == nil {
					s.authPasswordHash = string(hash)
					updated = append(updated, key)
				} else {
					rejected = append(rejected, key)
				}
			} else {
				rejected = append(rejected, key)
			}
		default:
			rejected = append(rejected, key)
		}
	}

	var settings SettingsSnapshot
	if len(updated) > 0 {
		settings = s.settingsSnapshotLocked()
	}
	s.mu.Unlock()

	if len(updated) > 0 {
		s.broadcastSettings(settings)
	}
	return updated, rejected, richiedeRiavvio
}

// toInt converte un valore JSON (di solito float64) in int validando che
// non ci siano frazioni. Accetta anche int nativo per robustezza.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	case int:
		return x, true
	}
	return 0, false
}

func (s *State) broadcastSettings(settings SettingsSnapshot) {
	s.broker.Broadcast(struct {
		Type     string           `json:"type"`
		Settings SettingsSnapshot `json:"settings"`
	}{Type: "settings", Settings: settings})
}

// ============================================================
// Auth info (per il middleware HTTP Basic)
// ============================================================

// AuthInfo ritorna le credenziali correnti in modo atomico.
// Se `enabled` e' false, il middleware non deve richiedere autenticazione.
// Se `passwordHash` e' vuoto ma `enabled` e' true, ogni request va respinta
// con 401 (l'utente ha abilitato auth ma non ha settato una password).
func (s *State) AuthInfo() (enabled bool, user, passwordHash string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authEnabled, s.authUser, s.authPasswordHash
}
