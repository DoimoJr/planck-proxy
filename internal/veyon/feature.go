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
	FeatureScreenLock = "ccb535a2-1d24-4cc1-a709-8b47d2b2ac79"
	FeatureStartApp   = "da9ca56a-b2ad-4fff-8f8a-929b2927b442" // ex RunProgram
	FeatureReboot     = "4f7d98f0-395a-4fff-b968-e49b8d0f748c"
	FeaturePowerDown  = "6f5a27a0-0e2f-496e-afcc-7aae62eede10"
	FeatureLogoff     = "7311d43d-ab53-439e-a03a-8cb25f7ed526"
	FeatureTextMsg    = "e75ae9c8-ac17-4d00-8f0d-019348346208"
	FeatureOpenURL    = "8a11a75d-b3db-48b6-b9cb-f8422ddd5b0c"
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

// ScreenLock attiva il blocco schermo sullo studente.
//
// Nota: Veyon ScreenLock e' edge-triggered come "start". Per sbloccare
// occorre inviare la stessa feature con un command custom dipendente
// dal plugin (variabile fra release Veyon — quando serve, decoderemo
// dal sorgente del plugin specifico).
func (c *Conn) ScreenLock() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureScreenLock),
		Command:     CmdDefault,
	})
}

// StartApp esegue uno o piu' programmi sullo studente. I path possono
// essere assoluti (es. "C:\\Windows\\notepad.exe") o nel PATH.
func (c *Conn) StartApp(programs []string) error {
	if len(programs) == 0 {
		return fmt.Errorf("veyon: StartApp richiede almeno un programma")
	}
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureStartApp),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"applications": programs,
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

// PowerDown spegne il PC studente.
func (c *Conn) PowerDown() error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeaturePowerDown),
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
func (c *Conn) TextMessage(text string) error {
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureTextMsg),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"text": text,
		},
	})
}

// OpenURL apre uno o piu' URL nel browser di default sul PC studente.
func (c *Conn) OpenURL(urls []string) error {
	if len(urls) == 0 {
		return fmt.Errorf("veyon: OpenURL richiede almeno un URL")
	}
	return c.SendFeature(FeatureMessage{
		FeatureUUID: uuid(FeatureOpenURL),
		Command:     CmdDefault,
		Arguments: qds.VariantMap{
			"websiteUrls": urls,
		},
	})
}

