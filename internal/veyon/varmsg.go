// Package veyon implementa il client per il protocollo Veyon Master ↔
// veyon-server: handshake RFB v3.8, security type custom Veyon (0x28),
// auth handshake KeyFile (RSA), e invio comandi feature.
//
// Reference (sorgente Veyon, GPL-2.0, letto come specifica del protocollo):
//   - core/src/RfbVeyonAuth.h          → security type ID, auth method enum
//   - core/src/VeyonConnection.cpp     → flow auth client-side
//   - core/src/VariantArrayMessage.cpp → framing [u32 BE length][N×QVariant]
//   - core/src/VariantStream.cpp       → QDataStream::Qt_5_5
//
// Il payload binario di ogni messaggio Veyon e' QVariant-serialized via
// internal/veyon/qds (Phase 3a). VarMsg incapsula il framing.
package veyon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/DoimoJr/planck-proxy/internal/veyon/qds"
)

// MaxMessageSize e' il cap del payload VariantArrayMessage. Veyon enforce
// lo stesso limite lato server (vedi VariantArrayMessage::MaxMessageSize).
// 16 MB e' largamente sufficiente; serve solo a respingere stream
// malformati che dichiarino lunghezze assurde.
const MaxMessageSize = 16 << 20

// SendVarMsg serializza una sequenza di QVariant in un VariantArrayMessage:
//
//	[u32 BE length][concat(QVariant_1, QVariant_2, ...)]
//
// Replicat l'API di Veyon `VariantArrayMessage::send()`: il caller
// passa i valori in ordine e SendVarMsg si occupa di lunghezza + frame.
func SendVarMsg(w io.Writer, values ...any) error {
	var payload bytes.Buffer
	enc := qds.NewEncoder(&payload)
	for i, v := range values {
		if err := enc.WriteVariant(v); err != nil {
			return fmt.Errorf("varmsg: encode variant %d: %w", i, err)
		}
	}
	if payload.Len() > MaxMessageSize {
		return fmt.Errorf("varmsg: payload troppo grande (%d > %d)", payload.Len(), MaxMessageSize)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(payload.Len()))
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("varmsg: write header: %w", err)
	}
	if _, err := w.Write(payload.Bytes()); err != nil {
		return fmt.Errorf("varmsg: write payload: %w", err)
	}
	return nil
}

// RecvVarMsg legge un VariantArrayMessage e ritorna un Decoder posizionato
// sul payload. Il caller chiama Decoder.ReadVariant() N volte secondo lo
// schema atteso del messaggio.
//
// **Salta automaticamente i VariantArrayMessage vuoti** (header con
// length=0 e nessun payload). Veyon invia messaggi vuoti come marker
// di transizione di stato — il client deve consumarli e proseguire al
// successivo (vedi `setState(Authenticating); VariantArrayMessage(m_socket).send();`
// in core/src/VncServerProtocol.cpp).
//
// Ritorna anche il payload raw (utile per debug/log/test).
func RecvVarMsg(r io.Reader) (*qds.Decoder, []byte, error) {
	for {
		dec, payload, err := recvOne(r)
		if err != nil {
			return nil, nil, err
		}
		if len(payload) > 0 {
			return dec, payload, nil
		}
		// Empty VarMsg: state-transition marker. Salta e leggi la prossima.
	}
}

// recvOne legge una singola VarMsg senza skip. Esposto per i casi rari
// in cui il caller vuole vedere anche le VarMsg vuote.
func recvOne(r io.Reader) (*qds.Decoder, []byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, nil, fmt.Errorf("varmsg: read header: %w", err)
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > MaxMessageSize {
		return nil, nil, fmt.Errorf("varmsg: payload dichiarato troppo grande (%d > %d)", n, MaxMessageSize)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, nil, fmt.Errorf("varmsg: read payload (%d byte): %w", n, err)
	}
	return qds.NewDecoder(bytes.NewReader(payload)), payload, nil
}
