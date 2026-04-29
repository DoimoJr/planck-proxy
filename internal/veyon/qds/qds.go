// Package qds implementa un sotto-insieme dell'encoder/decoder Qt
// QDataStream in Go puro, sufficiente per il protocollo Veyon
// (Master <-> veyon-server) di Phase 3.
//
// Sorgente di verita': il file core/src/VariantStream.cpp di Veyon
// imposta `setVersion(QDataStream::Qt_5_5)` (= versione 16). I tipi
// serializzati sono solo quelli usati da Veyon:
//
//	Bool, Int (qint32), LongLong (qint64), Double,
//	QByteArray, QString, QStringList, QUuid, QRect,
//	QVariantList, QVariantMap, QVariant
//
// Niente altro e' implementato di proposito (no QDate, no QHash, no
// QPoint, ecc.) per tenere la superficie minima e auditabile.
//
// # Convenzioni QDataStream
//
//   - Byte order: big-endian (default Qt)
//   - Float precision: double (default da Qt 4.6)
//   - QString: quint32 lunghezza in BYTE (non in caratteri) + UTF-16 BE.
//     Lunghezza 0xFFFFFFFF indica una QString null (diversa dalla stringa
//     vuota, che ha lunghezza 0).
//   - QByteArray: quint32 lunghezza + raw bytes. 0xFFFFFFFF = null.
//   - QVariant: quint32 typeId + quint8 isNull + payload type-dipendente.
//   - QVariantMap: itera per chiavi *ordinate* (QMap di Qt e' sorted).
//
// # API
//
// Tipo principali:
//
//   - Encoder: scrive su un io.Writer (di solito *bytes.Buffer).
//   - Decoder: legge da un io.Reader (di solito *bytes.Reader).
//
// I metodi ritornano errore solo per IO; le validazioni semantiche
// (size cap, recursion depth) le fa il caller, in linea col mirror
// VariantStream.cpp di Veyon (checkByteArray/checkString/...).
package qds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"unicode/utf16"
)

// QtNullLength e' il sentinel quint32 0xFFFFFFFF che QDataStream usa
// per indicare una QString o QByteArray null (distinta dalla stringa
// vuota a lunghezza 0).
const QtNullLength uint32 = 0xFFFFFFFF

// MaxReasonableSize e' un cap di sicurezza usato dai metodi di lettura
// per rifiutare allocazioni grandi a fronte di stream malformati. 16 MB
// e' largamente sufficiente per ogni messaggio Veyon ragionevole.
const MaxReasonableSize uint32 = 16 << 20

// QUuid rappresenta un Qt QUuid serializzato come 16 byte:
//
//	data1 (quint32 BE) | data2 (quint16 BE) | data3 (quint16 BE) | data4 [8 byte raw]
type QUuid [16]byte

// QRect rappresenta un Qt QRect: 4 qint32 (x, y, width, height).
type QRect struct {
	X, Y, Width, Height int32
}

// Encoder serializza tipi Qt verso un io.Writer in formato QDataStream.
type Encoder struct {
	w io.Writer
}

// NewEncoder crea un Encoder che scrive su w.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// Decoder de-serializza tipi Qt da un io.Reader in formato QDataStream.
type Decoder struct {
	r io.Reader
}

// NewDecoder crea un Decoder che legge da r.
func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: r} }

// ============================================================
// Primitives
// ============================================================

// WriteBool scrive un Qt bool come singolo byte (0 o 1).
func (e *Encoder) WriteBool(v bool) error {
	var b byte
	if v {
		b = 1
	}
	_, err := e.w.Write([]byte{b})
	return err
}

// ReadBool legge un singolo byte e lo interpreta come bool.
func (d *Decoder) ReadBool() (bool, error) {
	var buf [1]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return false, err
	}
	return buf[0] != 0, nil
}

// WriteUint8 scrive un quint8.
func (e *Encoder) WriteUint8(v uint8) error {
	_, err := e.w.Write([]byte{v})
	return err
}

// ReadUint8 legge un quint8.
func (d *Decoder) ReadUint8() (uint8, error) {
	var buf [1]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// WriteInt32 scrive un qint32 (signed) big-endian.
func (e *Encoder) WriteInt32(v int32) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(v))
	_, err := e.w.Write(buf[:])
	return err
}

// ReadInt32 legge un qint32.
func (d *Decoder) ReadInt32() (int32, error) {
	v, err := d.ReadUint32()
	return int32(v), err
}

// WriteUint32 scrive un quint32 big-endian.
func (e *Encoder) WriteUint32(v uint32) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	_, err := e.w.Write(buf[:])
	return err
}

// ReadUint32 legge un quint32.
func (d *Decoder) ReadUint32() (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

// WriteInt64 scrive un qint64 (signed) big-endian.
func (e *Encoder) WriteInt64(v int64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	_, err := e.w.Write(buf[:])
	return err
}

// ReadInt64 legge un qint64.
func (d *Decoder) ReadInt64() (int64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(buf[:])), nil
}

// WriteUint16 scrive un quint16 big-endian.
func (e *Encoder) WriteUint16(v uint16) error {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], v)
	_, err := e.w.Write(buf[:])
	return err
}

// ReadUint16 legge un quint16.
func (d *Decoder) ReadUint16() (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

// WriteDouble scrive un double IEEE 754 big-endian.
//
// Qt 4.6+ usa di default DoublePrecision per i floating point.
func (e *Encoder) WriteDouble(v float64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
	_, err := e.w.Write(buf[:])
	return err
}

// ReadDouble legge un double IEEE 754.
func (d *Decoder) ReadDouble() (float64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.BigEndian.Uint64(buf[:])), nil
}

// ============================================================
// QByteArray
// ============================================================

// WriteByteArray scrive un Qt QByteArray (quint32 lunghezza + raw bytes).
// Per scrivere un QByteArray null (diverso dall'array vuoto), usare
// WriteNullByteArray.
func (e *Encoder) WriteByteArray(b []byte) error {
	if err := e.WriteUint32(uint32(len(b))); err != nil {
		return err
	}
	_, err := e.w.Write(b)
	return err
}

// WriteNullByteArray scrive il sentinel di QByteArray null
// (lunghezza = 0xFFFFFFFF, niente payload). Diverso da scrivere
// un array vuoto.
func (e *Encoder) WriteNullByteArray() error {
	return e.WriteUint32(QtNullLength)
}

// ReadByteArray legge un QByteArray. Ritorna (nil, nil) se il sentinel
// indica null. Errore se la dimensione supera MaxReasonableSize.
func (d *Decoder) ReadByteArray() ([]byte, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	if n == QtNullLength {
		return nil, nil
	}
	if n > MaxReasonableSize {
		return nil, fmt.Errorf("qds: QByteArray troppo grande (%d byte > cap %d)", n, MaxReasonableSize)
	}
	out := make([]byte, n)
	if _, err := io.ReadFull(d.r, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ============================================================
// QString
// ============================================================

// WriteString scrive una Qt QString come quint32 (lunghezza in BYTE,
// non in code unit) + UTF-16 BE. Per scrivere una QString null, usare
// WriteNullString.
func (e *Encoder) WriteString(s string) error {
	if s == "" {
		// Qt distingue stringa vuota (len=0) da null (len=0xFFFFFFFF).
		// Una stringa Go "" mappa su QString vuota.
		return e.WriteUint32(0)
	}
	codeUnits := utf16.Encode([]rune(s))
	byteLen := uint32(len(codeUnits) * 2)
	if err := e.WriteUint32(byteLen); err != nil {
		return err
	}
	buf := make([]byte, byteLen)
	for i, cu := range codeUnits {
		binary.BigEndian.PutUint16(buf[i*2:], cu)
	}
	_, err := e.w.Write(buf)
	return err
}

// WriteNullString scrive il sentinel di QString null (0xFFFFFFFF).
func (e *Encoder) WriteNullString() error {
	return e.WriteUint32(QtNullLength)
}

// ReadString legge una QString. Ritorna ("", nil) sia per stringa vuota
// (len=0) sia per null (len=0xFFFFFFFF) — la distinzione e' raramente
// significativa per Veyon e l'API resta semplice.
func (d *Decoder) ReadString() (string, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return "", err
	}
	if n == QtNullLength || n == 0 {
		return "", nil
	}
	if n%2 != 0 {
		return "", fmt.Errorf("qds: QString lunghezza dispari (%d) — non e' UTF-16", n)
	}
	if n > MaxReasonableSize {
		return "", fmt.Errorf("qds: QString troppo grande (%d byte > cap %d)", n, MaxReasonableSize)
	}
	raw := make([]byte, n)
	if _, err := io.ReadFull(d.r, raw); err != nil {
		return "", err
	}
	codeUnits := make([]uint16, n/2)
	for i := range codeUnits {
		codeUnits[i] = binary.BigEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(codeUnits)), nil
}

// ============================================================
// QStringList
// ============================================================

// WriteStringList scrive un Qt QStringList: quint32 count + N × QString.
func (e *Encoder) WriteStringList(list []string) error {
	if err := e.WriteUint32(uint32(len(list))); err != nil {
		return err
	}
	for _, s := range list {
		if err := e.WriteString(s); err != nil {
			return err
		}
	}
	return nil
}

// ReadStringList legge un QStringList.
func (d *Decoder) ReadStringList() ([]string, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	if n > MaxReasonableSize {
		return nil, fmt.Errorf("qds: QStringList troppo grande (%d > cap %d)", n, MaxReasonableSize)
	}
	out := make([]string, n)
	for i := range out {
		s, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// ============================================================
// QUuid
// ============================================================

// WriteUuid scrive un QUuid come 16 byte raw (data1+data2+data3+data4
// gia' big-endian dato che sono i 16 byte canonici).
//
// Nota: Qt's QDataStream<<QUuid scrive in realta' quint32 + quint16 +
// quint16 + 8 byte raw, ma il risultato e' identico ai 16 byte di un
// UUID standard in network byte order.
func (e *Encoder) WriteUuid(u QUuid) error {
	_, err := e.w.Write(u[:])
	return err
}

// ReadUuid legge un QUuid (16 byte).
func (d *Decoder) ReadUuid() (QUuid, error) {
	var u QUuid
	_, err := io.ReadFull(d.r, u[:])
	return u, err
}

// UuidFromString costruisce un QUuid da una stringa nel formato canonico
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" (con o senza graffe).
func UuidFromString(s string) (QUuid, error) {
	var u QUuid
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == '{' || c == '}' || c == ' ' {
			continue
		}
		clean = append(clean, c)
	}
	if len(clean) != 32 {
		return u, fmt.Errorf("qds: UUID lunghezza invalida: %q", s)
	}
	for i := 0; i < 16; i++ {
		hi, err := hexNibble(clean[i*2])
		if err != nil {
			return u, err
		}
		lo, err := hexNibble(clean[i*2+1])
		if err != nil {
			return u, err
		}
		u[i] = hi<<4 | lo
	}
	return u, nil
}

// String formatta un QUuid nel formato canonico "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx".
func (u QUuid) String() string {
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hex[u[i]>>4]
		out[pos+1] = hex[u[i]&0xF]
		pos += 2
	}
	return string(out)
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("qds: carattere hex non valido: %q", c)
}

// ============================================================
// QRect
// ============================================================

// WriteRect scrive un QRect come 4 qint32 (x, y, width, height).
func (e *Encoder) WriteRect(r QRect) error {
	for _, v := range []int32{r.X, r.Y, r.Width, r.Height} {
		if err := e.WriteInt32(v); err != nil {
			return err
		}
	}
	return nil
}

// ReadRect legge un QRect.
func (d *Decoder) ReadRect() (QRect, error) {
	var r QRect
	for _, p := range []*int32{&r.X, &r.Y, &r.Width, &r.Height} {
		v, err := d.ReadInt32()
		if err != nil {
			return r, err
		}
		*p = v
	}
	return r, nil
}

// ============================================================
// QVariantMap helpers (key sorting)
// ============================================================

// sortedKeys ritorna le chiavi di m in ordine lessicografico, come
// fa la serializzazione di QMap di Qt. Necessario per garantire
// byte-equality coi messaggi prodotti da un client Qt.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ErrUnsupportedType e' ritornato da WriteVariant quando il valore Go
// passato non e' uno dei tipi mappati su QVariant.
var ErrUnsupportedType = errors.New("qds: tipo Go non supportato come QVariant")
