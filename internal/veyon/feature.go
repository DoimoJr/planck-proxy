package veyon

import (
	"fmt"
	"io"

	"github.com/DoimoJr/planck-proxy/internal/veyon/qds"
)

// rfbFeatureMessageType e' il byte RFB extension che Veyon usa per
// incapsulare un FeatureMessage durante una sessione attiva.
//
// Da core/src/FeatureMessage.h:
//
//	static constexpr unsigned char RfbMessageType = 41;
const rfbFeatureMessageType byte = 41

// FeatureCommand e' il sotto-comando di un FeatureMessage (qint32).
//
// Da core/src/FeatureMessage.h enum class Command:
//
//	Default = 0
//	Invalid = -1
//	Init    = -2
//
// I plugin definiscono comandi addizionali nel loro namespace (es.
// ScreenLockCommand::ShowDimmer = 1).
type FeatureCommand int32

const (
	CmdDefault FeatureCommand = 0
	CmdInvalid FeatureCommand = -1
	CmdInit    FeatureCommand = -2
)

// FeatureMessage corrisponde a core/src/FeatureMessage in Veyon.
type FeatureMessage struct {
	FeatureUUID qds.QUuid
	Command     FeatureCommand
	Arguments   qds.VariantMap
}

// SendFeature serializza e invia un FeatureMessage incapsulato come
// RFB extension message (1 byte type=41 + VarMsg).
//
// Da core/src/FeatureMessage::sendAsRfbMessage. Va usato durante una
// sessione RFB attiva (post ServerInit).
func (c *Conn) SendFeature(m FeatureMessage) error {
	if _, err := c.c.Write([]byte{rfbFeatureMessageType}); err != nil {
		return fmt.Errorf("veyon: write RfbFeatureMessage type: %w", err)
	}
	args := m.Arguments
	if args == nil {
		args = qds.VariantMap{}
	}
	return SendVarMsg(c.c, m.FeatureUUID, int32(m.Command), args)
}

// RecvFeature legge un FeatureMessage in arrivo durante una sessione
// RFB. Si aspetta che il prossimo byte sul socket sia 41
// (rfbFeatureMessageType) seguito dal VarMsg.
//
// Errore se il byte non e' 41 (probabilmente e' un altro tipo di
// messaggio RFB tipo FramebufferUpdate — non gestito).
func (c *Conn) RecvFeature() (FeatureMessage, error) {
	var fm FeatureMessage
	var typeByte [1]byte
	if _, err := io.ReadFull(c.c, typeByte[:]); err != nil {
		return fm, fmt.Errorf("veyon: read RFB message type: %w", err)
	}
	if typeByte[0] != rfbFeatureMessageType {
		return fm, fmt.Errorf("veyon: tipo messaggio RFB inatteso 0x%02x (atteso 0x29 FeatureMessage)", typeByte[0])
	}
	dec, _, err := RecvVarMsg(c.c)
	if err != nil {
		return fm, fmt.Errorf("veyon: ricezione FeatureMessage: %w", err)
	}
	uuidAny, err := dec.ReadVariant()
	if err != nil {
		return fm, fmt.Errorf("veyon: parse featureUid: %w", err)
	}
	uuid, ok := uuidAny.(qds.QUuid)
	if !ok {
		return fm, fmt.Errorf("veyon: featureUid type unexpected (%T)", uuidAny)
	}
	cmdAny, err := dec.ReadVariant()
	if err != nil {
		return fm, fmt.Errorf("veyon: parse command: %w", err)
	}
	cmd, ok := cmdAny.(int32)
	if !ok {
		return fm, fmt.Errorf("veyon: command type unexpected (%T)", cmdAny)
	}
	argsAny, err := dec.ReadVariant()
	if err != nil {
		return fm, fmt.Errorf("veyon: parse arguments: %w", err)
	}
	args, ok := argsAny.(qds.VariantMap)
	if !ok && argsAny != nil {
		return fm, fmt.Errorf("veyon: arguments type unexpected (%T)", argsAny)
	}
	fm.FeatureUUID = uuid
	fm.Command = FeatureCommand(cmd)
	fm.Arguments = args
	return fm, nil
}

// ============================================================
// UUID feature note (estratti da plugins/*/CMakeLists.txt e *.cpp del
// progetto Veyon). Se il server e' una versione molto diversa, alcune
// feature potrebbero non essere riconosciute.
// ============================================================

const (
	FeatureScreenLock   = "ccb535a2-1d24-4cc1-a709-8b47d2b2ac79"
	FeatureStartApp     = "da9ca56a-b2ad-4fff-8f8a-929b2927b442" // ex RunProgram
	FeatureReboot       = "4f7d98f0-395a-4fff-b968-e49b8d0f748c"
	FeaturePowerDown    = "6f5a27a0-0e2f-496e-afcc-7aae62eede10"
	FeaturePowerDownNow = "a88039f2-6716-40d8-b4e1-9f5cd48e91ed" // senza countdown
	FeaturePowerOn      = "f483c659-b5e7-4dbc-bd91-2c9403e70ebd" // Wake-on-LAN
	FeatureLogoff       = "7311d43d-ab53-439e-a03a-8cb25f7ed526"
	FeatureTextMsg      = "e75ae9c8-ac17-4d00-8f0d-019348346208"
	FeatureOpenURL      = "8a11a75d-b3db-48b6-b9cb-f8422ddd5b0c"
)

// Comandi specifici di alcuni plugin Veyon. Le chiavi sono integer
// definiti in `enum class FeatureCommand` in plugins/<feature>/*.h —
// auto-incrementati partendo da 0.
const (
	// ScreenLock: enum FeatureCommand { StartLock=0, StopLock=1 }.
	// (StartLock coincide con CmdDefault=0, ma StopLock = 1 e' il
	// command per sbloccare lo schermo.)
	CmdScreenLockStart FeatureCommand = 0
	CmdScreenLockStop  FeatureCommand = 1
)

// uuid e' un helper interno che costruisce qds.QUuid da una stringa
// UUID e fa panic se invalida — sarebbe un bug irreparabile.
func uuid(s string) qds.QUuid {
	u, err := qds.UuidFromString(s)
	if err != nil {
		panic("veyon: UUID hardcoded invalido: " + s + ": " + err.Error())
	}
	return u
}

// ============================================================
// High-level wrappers per i comandi piu' usati
// ============================================================

// ScreenLock attiva il blocco schermo sullo studente (mostra schermo nero).
// Comando StartLock = 0 dell'enum FeatureCommand del plugin ScreenLock.
func (c *Conn) ScreenLock() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureScreenLock),
		Command:     CmdScreenLockStart,
	})
}

// ScreenUnlock rimuove il lock sullo studente. Comando StopLock = 1.
func (c *Conn) ScreenUnlock() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureScreenLock),
		Command:     CmdScreenLockStop,
	})
}

// StartApp esegue uno o piu' programmi sullo studente. I path possono
// essere assoluti (es. "C:\\Windows\\notepad.exe") o nel PATH.
//
// Argomento `Applications` (capitale, da argToString(Argument::Applications)
// in plugins/desktopservices/DesktopServicesFeaturePlugin.h).
func (c *Conn) StartApp(programs []string) error {
	if len(programs) == 0 {
		return fmt.Errorf("veyon: StartApp richiede almeno un programma")
	}
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureStartApp),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"Applications": programs,
		},
	})
}

// Reboot riavvia il PC studente.
func (c *Conn) Reboot() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureReboot),
		Command:     CmdDefault,
	})
}

// PowerDown spegne il PC studente con il countdown standard di Veyon.
// Per spegnimento immediato senza dialog, usa PowerDownNow.
func (c *Conn) PowerDown() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeaturePowerDown),
		Command:     CmdDefault,
	})
}

// PowerDownNow spegne il PC studente immediatamente (no countdown UI).
func (c *Conn) PowerDownNow() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeaturePowerDownNow),
		Command:     CmdDefault,
	})
}

// Logoff disconnette l'utente sul PC studente.
func (c *Conn) Logoff() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureLogoff),
		Command:     CmdDefault,
	})
}

// TextMessage mostra un messaggio modale sul PC studente.
//
// Argomenti (chiavi capitali, da argToString(Argument::Text) in
// plugins/textmessage/TextMessageFeaturePlugin.h):
//   - Text: stringa del messaggio
//   - Icon: int corrispondente a QMessageBox::Icon (1 = Information,
//     2 = Warning, 3 = Critical, 4 = Question)
func (c *Conn) TextMessage(text string) error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureTextMsg),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"Text": text,
			"Icon": int32(1), // QMessageBox::Information
		},
	})
}

// OpenURL apre uno o piu' URL nel browser di default sul PC studente.
//
// Argomento `WebsiteUrls` (capitale, da argToString(Argument::WebsiteUrls)).
func (c *Conn) OpenURL(urls []string) error {
	if len(urls) == 0 {
		return fmt.Errorf("veyon: OpenURL richiede almeno un URL")
	}
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureOpenURL),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"WebsiteUrls": urls,
		},
	})
}

// PowerOn richiede Wake-on-LAN: il PC studente e' spento, niente IP per
// raggiungerlo via Veyon. Implementazione futura con magic packet UDP
// broadcast (porta 9) — richiede MAC address che attualmente non e'
// memorizzato nella mappa studenti. TODO Phase 4.x.
//
// Per ora ritorna un errore esplicativo; il bottone UI puo' rimanere
// disabilitato finche' il MAC non viene aggiunto al record studente.
func (c *Conn) PowerOn() error {
	return fmt.Errorf("veyon: PowerOn (Wake-on-LAN) non ancora implementato — serve MAC address nello studente")
}

