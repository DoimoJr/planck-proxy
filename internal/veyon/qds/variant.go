package qds

import "fmt"

// QMetaType IDs di Qt 5 stabili attraverso le release. Solo quelli
// usati da Veyon sono dichiarati: la lista deve combaciare con
// `VariantStream::checkVariant` in core/src/VariantStream.cpp.
const (
	TypeBool         uint32 = 1
	TypeInt          uint32 = 2
	TypeLongLong     uint32 = 4
	TypeDouble       uint32 = 6
	TypeVariantMap   uint32 = 8
	TypeVariantList  uint32 = 9
	TypeString       uint32 = 10
	TypeStringList   uint32 = 11
	TypeByteArray    uint32 = 12
	TypeRect         uint32 = 19
	TypeUuid         uint32 = 30
)

// VariantMap e' l'alias Go di Qt's QVariantMap. La chiave e' sempre
// una QString; il valore puo' essere uno dei tipi supportati (vedi
// WriteVariant).
type VariantMap = map[string]any

// VariantList e' l'alias Go di Qt's QVariantList.
type VariantList = []any

// WriteVariant serializza v come QVariant: quint32 typeId + quint8
// isNull + payload type-dipendente. Tipi Go mappati:
//
//	bool          -> Bool
//	int32         -> Int
//	int           -> Int (con cast a int32)
//	int64         -> LongLong
//	float64       -> Double
//	string        -> QString
//	[]byte        -> QByteArray
//	[]string      -> QStringList
//	QUuid         -> QUuid
//	QRect         -> QRect
//	VariantList   -> QVariantList
//	VariantMap    -> QVariantMap
//	nil           -> QString null (typeId=QString, isNull=1) — convenzione Veyon
//
// Tipi non supportati ritornano ErrUnsupportedType.
func (e *Encoder) WriteVariant(v any) error {
	switch x := v.(type) {
	case nil:
		if err := e.WriteUint32(TypeString); err != nil {
			return err
		}
		return e.WriteUint8(1) // isNull
	case bool:
		if err := e.writeVariantHeader(TypeBool); err != nil {
			return err
		}
		return e.WriteBool(x)
	case int32:
		if err := e.writeVariantHeader(TypeInt); err != nil {
			return err
		}
		return e.WriteInt32(x)
	case int:
		if err := e.writeVariantHeader(TypeInt); err != nil {
			return err
		}
		return e.WriteInt32(int32(x))
	case int64:
		if err := e.writeVariantHeader(TypeLongLong); err != nil {
			return err
		}
		return e.WriteInt64(x)
	case float64:
		if err := e.writeVariantHeader(TypeDouble); err != nil {
			return err
		}
		return e.WriteDouble(x)
	case string:
		if err := e.writeVariantHeader(TypeString); err != nil {
			return err
		}
		return e.WriteString(x)
	case []byte:
		if err := e.writeVariantHeader(TypeByteArray); err != nil {
			return err
		}
		return e.WriteByteArray(x)
	case []string:
		if err := e.writeVariantHeader(TypeStringList); err != nil {
			return err
		}
		return e.WriteStringList(x)
	case QUuid:
		if err := e.writeVariantHeader(TypeUuid); err != nil {
			return err
		}
		return e.WriteUuid(x)
	case QRect:
		if err := e.writeVariantHeader(TypeRect); err != nil {
			return err
		}
		return e.WriteRect(x)
	case VariantList:
		if err := e.writeVariantHeader(TypeVariantList); err != nil {
			return err
		}
		return e.WriteVariantList(x)
	case VariantMap:
		if err := e.writeVariantHeader(TypeVariantMap); err != nil {
			return err
		}
		return e.WriteVariantMap(x)
	}
	return fmt.Errorf("%w: %T", ErrUnsupportedType, v)
}

func (e *Encoder) writeVariantHeader(typeId uint32) error {
	if err := e.WriteUint32(typeId); err != nil {
		return err
	}
	return e.WriteUint8(0) // isNull = false
}

// ReadVariant legge un QVariant e ritorna il valore decodato come tipo
// Go corrispondente. typeId sconosciuto -> errore.
//
// Mapping tipo Go ritornato:
//
//	Bool          -> bool
//	Int           -> int32
//	LongLong      -> int64
//	Double        -> float64
//	QString       -> string
//	QByteArray    -> []byte
//	QStringList   -> []string
//	QUuid         -> QUuid
//	QRect         -> QRect
//	QVariantList  -> VariantList
//	QVariantMap   -> VariantMap
//	(qualsiasi con isNull=1) -> nil
func (d *Decoder) ReadVariant() (any, error) {
	typeId, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	isNull, err := d.ReadUint8()
	if err != nil {
		return nil, err
	}
	if isNull == 1 {
		// Per Qt il payload non viene comunque saltato — un null QVariant
		// non ha bytes successivi. Confermato dalla read-back di Veyon.
		return nil, nil
	}
	switch typeId {
	case TypeBool:
		return d.ReadBool()
	case TypeInt:
		return d.ReadInt32()
	case TypeLongLong:
		return d.ReadInt64()
	case TypeDouble:
		return d.ReadDouble()
	case TypeString:
		return d.ReadString()
	case TypeByteArray:
		return d.ReadByteArray()
	case TypeStringList:
		return d.ReadStringList()
	case TypeUuid:
		return d.ReadUuid()
	case TypeRect:
		return d.ReadRect()
	case TypeVariantList:
		return d.ReadVariantList()
	case TypeVariantMap:
		return d.ReadVariantMap()
	}
	return nil, fmt.Errorf("qds: QVariant typeId %d non supportato", typeId)
}

// WriteVariantList scrive un QVariantList: quint32 count + N × QVariant.
func (e *Encoder) WriteVariantList(list VariantList) error {
	if err := e.WriteUint32(uint32(len(list))); err != nil {
		return err
	}
	for _, v := range list {
		if err := e.WriteVariant(v); err != nil {
			return err
		}
	}
	return nil
}

// ReadVariantList legge un QVariantList.
func (d *Decoder) ReadVariantList() (VariantList, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	if n > MaxReasonableSize {
		return nil, fmt.Errorf("qds: QVariantList troppo grande (%d > cap %d)", n, MaxReasonableSize)
	}
	out := make(VariantList, n)
	for i := range out {
		v, err := d.ReadVariant()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// WriteVariantMap scrive un QVariantMap: quint32 count + N × (QString
// key + QVariant value), con chiavi ordinate lessicograficamente.
//
// L'ordinamento e' obbligatorio: Qt's QMap mantiene le chiavi sorted
// internamente, quindi un map serializzato da un client Qt ha sempre
// chiavi ordinate. Per garantire byte-equality (utile per test) e
// matching con cache/hash lato server, replichiamo l'ordine.
func (e *Encoder) WriteVariantMap(m VariantMap) error {
	if err := e.WriteUint32(uint32(len(m))); err != nil {
		return err
	}
	for _, k := range sortedKeys(m) {
		if err := e.WriteString(k); err != nil {
			return err
		}
		if err := e.WriteVariant(m[k]); err != nil {
			return err
		}
	}
	return nil
}

// ReadVariantMap legge un QVariantMap.
func (d *Decoder) ReadVariantMap() (VariantMap, error) {
	n, err := d.ReadUint32()
	if err != nil {
		return nil, err
	}
	if n > MaxReasonableSize {
		return nil, fmt.Errorf("qds: QVariantMap troppo grande (%d > cap %d)", n, MaxReasonableSize)
	}
	out := make(VariantMap, n)
	for i := uint32(0); i < n; i++ {
		k, err := d.ReadString()
		if err != nil {
			return nil, err
		}
		v, err := d.ReadVariant()
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}
