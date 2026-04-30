package veyon

import (
	"bytes"
	"fmt"
	"io"
)

// Reference RFB protocol v3.8: https://datatracker.ietf.org/doc/html/rfc6143
//
// Veyon usa RFB v3.8 come trasporto e ci innesta una security type
// custom (0x28) con flow di auth proprio.

// rfbProtocolVersion e' il greeting fisso scambiato in entrambi i sensi.
const rfbProtocolVersion = "RFB 003.008\n"

// SecTypeVeyon e' il security type ID custom di Veyon (RfbVeyonAuth.h):
//
//	static constexpr char rfbSecTypeVeyon = 40;
const SecTypeVeyon byte = 40

// Esiti SecurityResult RFB v3.8 (sezione 7.1.3 RFC 6143).
const (
	rfbSecurityResultOK     = 0
	rfbSecurityResultFailed = 1
)

// rfbHandshakeVersion esegue lo scambio di greeting "RFB 003.008\n" in
// entrambi i sensi. La conn viene scritta esattamente con la stessa
// stringa che il server invia, come da spec RFB.
//
// Errore se il server invia una versione che non riconosciamo (Veyon
// e' fissato su 3.8).
func rfbHandshakeVersion(rw io.ReadWriter) error {
	greeting := make([]byte, 12)
	if _, err := io.ReadFull(rw, greeting); err != nil {
		return fmt.Errorf("rfb: lettura greeting: %w", err)
	}
	if !bytes.Equal(greeting, []byte(rfbProtocolVersion)) {
		return fmt.Errorf("rfb: greeting inatteso %q (atteso %q)", greeting, rfbProtocolVersion)
	}
	if _, err := rw.Write([]byte(rfbProtocolVersion)); err != nil {
		return fmt.Errorf("rfb: write greeting: %w", err)
	}
	return nil
}

// rfbSelectSecurityType legge la lista dei security types proposta dal
// server e ne sceglie uno. Cerca SecTypeVeyon (0x28); se assente,
// errore.
//
// Wire format (RFC 6143 §7.1.2): server invia
//
//	[u8 N] [u8 type_1] ... [u8 type_N]
//
// Caso speciale N=0: il server ha rifiutato la connessione e segue
// [u32 reasonLength][reasonString].
//
// Il client risponde con [u8 chosenType].
func rfbSelectSecurityType(rw io.ReadWriter, want byte) error {
	var n [1]byte
	if _, err := io.ReadFull(rw, n[:]); err != nil {
		return fmt.Errorf("rfb: read security types count: %w", err)
	}
	if n[0] == 0 {
		// Server ha rifiutato: leggi il reason e propagalo come errore.
		var lenBuf [4]byte
		if _, err := io.ReadFull(rw, lenBuf[:]); err != nil {
			return fmt.Errorf("rfb: read reason length: %w", err)
		}
		reasonLen := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])
		if reasonLen > MaxMessageSize {
			return fmt.Errorf("rfb: reason length non valida (%d)", reasonLen)
		}
		reason := make([]byte, reasonLen)
		_, _ = io.ReadFull(rw, reason)
		return fmt.Errorf("rfb: server ha rifiutato la connessione: %s", reason)
	}
	types := make([]byte, n[0])
	if _, err := io.ReadFull(rw, types); err != nil {
		return fmt.Errorf("rfb: read security types: %w", err)
	}
	if !bytes.Contains(types, []byte{want}) {
		return fmt.Errorf("rfb: security type 0x%02x non offerto dal server (offerti: %v)", want, types)
	}
	if _, err := rw.Write([]byte{want}); err != nil {
		return fmt.Errorf("rfb: write chosen type: %w", err)
	}
	return nil
}

// ServerInit e' la struttura RFB ServerInit (RFC 6143 §7.3.2).
//
// Veyon non popola davvero il framebuffer (e' un control plane), ma lo
// invia comunque per restare RFB-compatible. I valori non sono significativi
// per Planck — vengono letti e poi ignorati.
type ServerInit struct {
	Width       uint16
	Height      uint16
	PixelFormat [16]byte
	Name        string
}

// rfbSendClientInit scrive il ClientInit message (RFC 6143 §7.3.1):
// 1 byte shared-flag. Veyon vuole sempre shared=1.
func rfbSendClientInit(w io.Writer, shared bool) error {
	var b byte
	if shared {
		b = 1
	}
	_, err := w.Write([]byte{b})
	return err
}

// rfbReadServerInit legge il ServerInit message (RFC 6143 §7.3.2):
//
//	[u16 width][u16 height][16 byte pixelFormat][u32 nameLen][nameLen byte name]
func rfbReadServerInit(r io.Reader) (ServerInit, error) {
	var hdr [24]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return ServerInit{}, fmt.Errorf("rfb: read ServerInit header: %w", err)
	}
	si := ServerInit{
		Width:  uint16(hdr[0])<<8 | uint16(hdr[1]),
		Height: uint16(hdr[2])<<8 | uint16(hdr[3]),
	}
	copy(si.PixelFormat[:], hdr[4:20])
	nameLen := uint32(hdr[20])<<24 | uint32(hdr[21])<<16 | uint32(hdr[22])<<8 | uint32(hdr[23])
	if nameLen > MaxMessageSize {
		return si, fmt.Errorf("rfb: ServerInit name length non valida (%d)", nameLen)
	}
	if nameLen > 0 {
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(r, name); err != nil {
			return si, fmt.Errorf("rfb: read ServerInit name: %w", err)
		}
		si.Name = string(name)
	}
	return si, nil
}

// rfbReadSecurityResult legge il SecurityResult finale (4 byte big-endian,
// 0=OK, 1=failed). Per RFB v3.8 in caso di failed segue una reason
// string [u32 length][bytes].
func rfbReadSecurityResult(r io.Reader) error {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return fmt.Errorf("rfb: read security result: %w", err)
	}
	result := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	if result == rfbSecurityResultOK {
		return nil
	}
	// failed: leggi reason
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		// alcune versioni non mandano il reason; ritorna errore generico
		return fmt.Errorf("rfb: security result fallito (codice %d)", result)
	}
	reasonLen := uint32(lenBuf[0])<<24 | uint32(lenBuf[1])<<16 | uint32(lenBuf[2])<<8 | uint32(lenBuf[3])
	if reasonLen > MaxMessageSize {
		return fmt.Errorf("rfb: security failed (codice %d), reason length invalida", result)
	}
	reason := make([]byte, reasonLen)
	_, _ = io.ReadFull(r, reason)
	return fmt.Errorf("rfb: security failed: %s", reason)
}
