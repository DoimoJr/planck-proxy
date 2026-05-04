package state

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/DoimoJr/planck-proxy/internal/classify"
	"github.com/DoimoJr/planck-proxy/internal/store"
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
	s.persistBloccati(list)
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
	s.persistBloccati(list)
	s.broadcastBlocklist(list)
}

// BlockAllAI aggiunge tutti i domini AI noti (lista classify.AIDomains()) alla blocklist.
func (s *State) BlockAllAI() {
	s.mu.Lock()
	for _, d := range classify.AIDomains() {
		s.bloccati[d] = struct{}{}
	}
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.persistBloccati(list)
	s.broadcastBlocklist(list)
}

// UnblockAllAI rimuove tutti i domini AI dalla blocklist.
func (s *State) UnblockAllAI() {
	s.mu.Lock()
	for _, d := range classify.AIDomains() {
		delete(s.bloccati, d)
	}
	list := s.bloccatiSortedLocked()
	s.mu.Unlock()
	s.persistBloccati(list)
	s.broadcastBlocklist(list)
}

// ClearBlocklist svuota completamente la blocklist.
func (s *State) ClearBlocklist() {
	s.mu.Lock()
	s.bloccati = map[string]struct{}{}
	s.mu.Unlock()
	s.persistBloccati([]string{})
	s.broadcastBlocklist([]string{})
}

// persistBloccati e' un wrapper che serializza la blocklist su disco
// con error logging. Helper per non duplicare la riga dopo ogni mutate.
func (s *State) persistBloccati(list []string) {
	if err := s.store.SaveBloccati(list); err != nil {
		log.Printf("state: errore save blocklist: %v", err)
	}
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
	s.persistStudenti(stud)
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
	s.persistStudenti(stud)
	s.broadcastStudenti(stud)
}

// ClearStudents svuota tutta la mappa.
func (s *State) ClearStudents() {
	s.mu.Lock()
	s.studenti = map[string]string{}
	s.mu.Unlock()
	s.persistStudenti(map[string]string{})
	s.broadcastStudenti(map[string]string{})
}

// SetStudenti sostituisce in blocco la mappa studenti corrente. Usato
// dall'endpoint /api/classi/load (carica una combo salvata).
func (s *State) SetStudenti(stud map[string]string) {
	s.mu.Lock()
	s.studenti = make(map[string]string, len(stud))
	for k, v := range stud {
		s.studenti[k] = v
	}
	cp := s.studentiCopyLocked()
	s.mu.Unlock()
	s.persistStudenti(cp)
	s.broadcastStudenti(cp)
}

// MergeStudentiAuto aggiunge alla mappa studenti gli IP scoperti via
// discovery LAN, SENZA sovrascrivere voci esistenti (preserva i nomi
// reali se l'utente ha rinominato). IP gia' presenti nella mappa vengono
// ignorati. Persiste e broadcasta solo se la mappa e' davvero cambiata.
//
// Il nome assegnato agli IP nuovi e' stringa vuota: la UI mostra l'IP
// stesso come label finche' l'utente non assegna un nome reale.
func (s *State) MergeStudentiAuto(ips []string) {
	if len(ips) == 0 {
		return
	}
	s.mu.Lock()
	added := 0
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if _, exists := s.studenti[ip]; exists {
			continue
		}
		s.studenti[ip] = ""
		added++
	}
	if added == 0 {
		s.mu.Unlock()
		return
	}
	cp := s.studentiCopyLocked()
	s.mu.Unlock()
	s.persistStudenti(cp)
	s.broadcastStudenti(cp)
	log.Printf("discover: aggiunti %d IP nuovi alla mappa studenti", added)
}

func (s *State) persistStudenti(stud map[string]string) {
	if err := s.store.SaveStudenti(stud); err != nil {
		log.Printf("state: errore save studenti: %v", err)
	}
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
	s.saveConfigLocked()
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
	s.saveConfigLocked()
	settings := s.settingsSnapshotLocked()
	s.mu.Unlock()
	s.broadcastSettings(settings)
}

// ============================================================
// Sessione (start/stop)
// ============================================================

// SessionStart apre una nuova sessione: se ce n'e' una attiva la chiude
// in DB, poi crea una nuova riga in `sessioni` con snapshot di studenti
// e bloccati. Azzera il ring buffer in-memory.
//
// Ritorna `(sessioneInizio, archiviata)`. `archiviata` e' il filename-id
// della sessione precedente (vuoto se non c'era).
func (s *State) SessionStart() (sessioneInizio string, archiviata string) {
	s.mu.Lock()

	// Chiudi la precedente in DB se attiva (defensive).
	if s.sessioneAttiva && s.sessioneID > 0 {
		now := time.Now().UTC()
		fine := now.Format(time.RFC3339)
		durata := computeDurata(s.sessioneInizio, now)
		if err := s.store.SessionClose(s.sessioneID, fine, durata, now.UnixMilli()); err != nil {
			log.Printf("state: errore close prev session: %v", err)
		}
		archiviata = sessionFilename(s.sessioneID, s.sessioneInizio)
	}

	// Apri nuova sessione in DB.
	now := time.Now().UTC()
	meta := store.SessionMeta{
		SessioneInizio: now.Format(time.RFC3339),
		Classe:         s.classe,
		Lab:            "",
		Titolo:         s.titolo,
		Modo:           s.modo,
		Studenti:       s.studentiCopyLocked(),
		Bloccati:       s.bloccatiSortedLocked(),
	}
	id, err := s.store.SessionStart(meta)
	if err != nil {
		log.Printf("state: errore start session: %v", err)
	}

	s.storia = make([]Entry, 0, 256)
	s.sessioneInizio = meta.SessioneInizio
	s.sessioneFineISO = ""
	s.sessioneAttiva = true
	s.sessioneID = id
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

// SessionStop chiude la sessione corrente in DB. Il ring buffer in-memory
// NON viene azzerato: resta visibile in UI fino a un nuovo SessionStart.
//
// Ritorna `(archiviata, sessioneFineISO)`. No-op se la sessione non e' attiva.
func (s *State) SessionStop() (archiviata, fineISO string) {
	s.mu.Lock()
	if !s.sessioneAttiva {
		s.mu.Unlock()
		return "", ""
	}
	now := time.Now().UTC()
	fine := now.Format(time.RFC3339)
	durata := computeDurata(s.sessioneInizio, now)
	if s.sessioneID > 0 {
		if err := s.store.SessionClose(s.sessioneID, fine, durata, now.UnixMilli()); err != nil {
			log.Printf("state: errore close session: %v", err)
		}
		archiviata = sessionFilename(s.sessioneID, s.sessioneInizio)
	}
	s.sessioneAttiva = false
	s.sessioneFineISO = fine
	s.sessioneID = 0
	s.mu.Unlock()
	s.broadcastSessionState()
	return archiviata, fine
}

// ArchiviaCorrente esegue un "checkpoint": chiude la sessione corrente
// in DB e ne apre una nuova con gli stessi metadata. Ritorna il filename-id
// della sessione chiusa, o "" se non c'e' nulla da archiviare.
func (s *State) ArchiviaCorrente() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.archiveLocked()
}

// archiveLocked esegue la rotazione (close + open new) della sessione
// corrente. La sessione resta attiva con un nuovo ID.
//
// **Deve essere chiamato col lock gia' tenuto.**
func (s *State) archiveLocked() string {
	if s.store.Disabled() || !s.sessioneAttiva || s.sessioneID == 0 {
		return ""
	}
	now := time.Now().UTC()
	fine := now.Format(time.RFC3339)
	durata := computeDurata(s.sessioneInizio, now)
	closedID := s.sessioneID
	closedInizio := s.sessioneInizio
	if err := s.store.SessionClose(closedID, fine, durata, now.UnixMilli()); err != nil {
		log.Printf("state: errore close session (archive): %v", err)
		return ""
	}

	meta := store.SessionMeta{
		SessioneInizio: fine,
		Classe:         s.classe,
		Lab:            "",
		Titolo:         s.titolo,
		Modo:           s.modo,
		Studenti:       s.studentiCopyLocked(),
		Bloccati:       s.bloccatiSortedLocked(),
	}
	id, err := s.store.SessionStart(meta)
	if err != nil {
		log.Printf("state: errore start session post-archive: %v", err)
		s.sessioneAttiva = false
		s.sessioneID = 0
		return sessionFilename(closedID, closedInizio)
	}
	s.sessioneInizio = fine
	s.sessioneID = id
	s.storia = s.storia[:0]
	return sessionFilename(closedID, closedInizio)
}

// computeDurata ritorna i secondi tra `inizio` (RFC3339) e `now`.
// Ritorna 0 se inizio e' invalido o la differenza e' negativa.
func computeDurata(inizio string, now time.Time) int64 {
	t, err := time.Parse(time.RFC3339, inizio)
	if err != nil {
		return 0
	}
	d := int64(now.Sub(t).Seconds())
	if d < 0 {
		return 0
	}
	return d
}

// sessionFilename costruisce l'id-stringa "<id>-<inizio>.json" usato
// dagli endpoint API per retro-compatibilita' col modello v1 file-based.
func sessionFilename(id int64, inizio string) string {
	clean := strings.NewReplacer(":", "-", "T", "-", ".", "-").Replace(inizio)
	if len(clean) > 19 {
		clean = clean[:19]
	}
	return fmt.Sprintf("%d-%s.json", id, clean)
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
		s.saveConfigLocked()
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
