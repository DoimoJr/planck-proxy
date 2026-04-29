package qds

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

// hx pulisce una stringa hex (toglie spazi/newline) e la decoda.
func hx(t *testing.T, s string) []byte {
	t.Helper()
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\t' {
			continue
		}
		clean = append(clean, c)
	}
	out, err := hex.DecodeString(string(clean))
	if err != nil {
		t.Fatalf("hex invalida: %v", err)
	}
	return out
}

// roundtrip codifica con `enc`, poi decodifica con `dec`, e confronta
// risultato e bytes attesi (se forniti).
func checkBytes(t *testing.T, got, want []byte, label string) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: bytes diversi\n got: %s\nwant: %s", label, hex.EncodeToString(got), hex.EncodeToString(want))
	}
}

// ============================================================
// Primitives
// ============================================================

func TestPrimitivesRoundtrip(t *testing.T) {
	tests := []struct {
		name    string
		writeFn func(*Encoder) error
		readFn  func(*Decoder) (any, error)
		want    any
		wireHex string
	}{
		{"bool true", func(e *Encoder) error { return e.WriteBool(true) }, func(d *Decoder) (any, error) { return d.ReadBool() }, true, "01"},
		{"bool false", func(e *Encoder) error { return e.WriteBool(false) }, func(d *Decoder) (any, error) { return d.ReadBool() }, false, "00"},
		{"int32 +1", func(e *Encoder) error { return e.WriteInt32(1) }, func(d *Decoder) (any, error) { return d.ReadInt32() }, int32(1), "00000001"},
		{"int32 -1", func(e *Encoder) error { return e.WriteInt32(-1) }, func(d *Decoder) (any, error) { return d.ReadInt32() }, int32(-1), "ffffffff"},
		{"int32 max", func(e *Encoder) error { return e.WriteInt32(2147483647) }, func(d *Decoder) (any, error) { return d.ReadInt32() }, int32(2147483647), "7fffffff"},
		{"uint32 0xdeadbeef", func(e *Encoder) error { return e.WriteUint32(0xdeadbeef) }, func(d *Decoder) (any, error) { return d.ReadUint32() }, uint32(0xdeadbeef), "deadbeef"},
		{"int64 +1", func(e *Encoder) error { return e.WriteInt64(1) }, func(d *Decoder) (any, error) { return d.ReadInt64() }, int64(1), "0000000000000001"},
		{"int64 -1", func(e *Encoder) error { return e.WriteInt64(-1) }, func(d *Decoder) (any, error) { return d.ReadInt64() }, int64(-1), "ffffffffffffffff"},
		{"double 1.0", func(e *Encoder) error { return e.WriteDouble(1.0) }, func(d *Decoder) (any, error) { return d.ReadDouble() }, 1.0, "3ff0000000000000"},
		{"double -2.0", func(e *Encoder) error { return e.WriteDouble(-2.0) }, func(d *Decoder) (any, error) { return d.ReadDouble() }, -2.0, "c000000000000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tt.writeFn(NewEncoder(&buf)); err != nil {
				t.Fatalf("write: %v", err)
			}
			checkBytes(t, buf.Bytes(), hx(t, tt.wireHex), "encode")
			got, err := tt.readFn(NewDecoder(bytes.NewReader(buf.Bytes())))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

// ============================================================
// QString
// ============================================================

func TestStringEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteString(""); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), hx(t, "00000000"), "empty string")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadString()
	if err != nil || got != "" {
		t.Fatalf("read empty: got=%q err=%v", got, err)
	}
}

func TestStringHello(t *testing.T) {
	// "Hello" UTF-16 BE = 00 48 00 65 00 6C 00 6C 00 6F (10 bytes).
	// Length prefix = quint32(10) = 00 00 00 0A.
	want := hx(t, "0000000A 0048 0065 006C 006C 006F")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteString("Hello"); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "Hello string")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadString()
	if err != nil || got != "Hello" {
		t.Fatalf("read: got=%q err=%v", got, err)
	}
}

func TestStringUnicode(t *testing.T) {
	// "ciao è" — 'è' = U+00E8, surrogato non necessario.
	// Codifica UTF-16 BE: 00 63 00 69 00 61 00 6F 00 20 00 E8 (12 bytes)
	want := hx(t, "0000000C 0063 0069 0061 006F 0020 00E8")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteString("ciao è"); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "ciao è")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadString()
	if err != nil || got != "ciao è" {
		t.Fatalf("read: got=%q err=%v", got, err)
	}
}

func TestStringEmoji(t *testing.T) {
	// 🚀 = U+1F680, fuori dal BMP, richiede surrogate pair UTF-16:
	// D83D DE80
	want := hx(t, "00000004 D83D DE80")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteString("🚀"); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "rocket emoji")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadString()
	if err != nil || got != "🚀" {
		t.Fatalf("read: got=%q err=%v", got, err)
	}
}

// ============================================================
// QByteArray
// ============================================================

func TestByteArrayEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteByteArray([]byte{}); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), hx(t, "00000000"), "empty bytearray")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadByteArray()
	if err != nil || len(got) != 0 {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

func TestByteArrayHello(t *testing.T) {
	// "Hello" raw = 48 65 6C 6C 6F (5 bytes), len prefix = 00 00 00 05
	want := hx(t, "00000005 48656C6C6F")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteByteArray([]byte("Hello")); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "Hello bytearray")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadByteArray()
	if err != nil || string(got) != "Hello" {
		t.Fatalf("read: got=%q err=%v", got, err)
	}
}

func TestByteArrayNull(t *testing.T) {
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteNullByteArray(); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), hx(t, "FFFFFFFF"), "null bytearray")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadByteArray()
	if err != nil || got != nil {
		t.Fatalf("read null: got=%v err=%v", got, err)
	}
}

// ============================================================
// QUuid
// ============================================================

func TestUuidParseFormat(t *testing.T) {
	// UUID di Veyon RunProgram feature: da9ca56a-b2ad-4fff-8f8a-929b2927b442
	const s = "da9ca56a-b2ad-4fff-8f8a-929b2927b442"
	u, err := UuidFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != s {
		t.Fatalf("roundtrip string: got %q want %q", u.String(), s)
	}
	want := hx(t, "DA9CA56AB2AD4FFF8F8A929B2927B442")
	if !bytes.Equal(u[:], want) {
		t.Fatalf("bytes: got %x want %x", u[:], want)
	}
}

func TestUuidEncode(t *testing.T) {
	u, _ := UuidFromString("da9ca56a-b2ad-4fff-8f8a-929b2927b442")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteUuid(u); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), hx(t, "DA9CA56AB2AD4FFF8F8A929B2927B442"), "uuid")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadUuid()
	if err != nil || got != u {
		t.Fatalf("read: got=%x err=%v", got, err)
	}
}

func TestUuidWithBraces(t *testing.T) {
	u, err := UuidFromString("{da9ca56a-b2ad-4fff-8f8a-929b2927b442}")
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != "da9ca56a-b2ad-4fff-8f8a-929b2927b442" {
		t.Fatal("string mismatch with braces input")
	}
}

// ============================================================
// QStringList
// ============================================================

func TestStringList(t *testing.T) {
	// ["a", "b"] -> count=2 + QString("a") + QString("b")
	// QString("a") = len=2 + "00 61"
	// QString("b") = len=2 + "00 62"
	want := hx(t, "00000002 00000002 0061 00000002 0062")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteStringList([]string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "stringlist")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadStringList()
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

// ============================================================
// QRect
// ============================================================

func TestRect(t *testing.T) {
	r := QRect{X: 10, Y: 20, Width: 100, Height: 200}
	want := hx(t, "0000000A 00000014 00000064 000000C8")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteRect(r); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "rect")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadRect()
	if err != nil || got != r {
		t.Fatalf("read: got=%+v err=%v", got, err)
	}
}

// ============================================================
// QVariant
// ============================================================

func TestVariantString(t *testing.T) {
	// QVariant(QString("Hi"))
	// typeId = 10 (QString), isNull = 0
	// payload = QString("Hi") = len=4 + "00 48 00 69"
	want := hx(t, "0000000A 00 00000004 00480069")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariant("Hi"); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "variant string")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariant()
	if err != nil || got != "Hi" {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

func TestVariantInt(t *testing.T) {
	want := hx(t, "00000002 00 00000042")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariant(int32(0x42)); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "variant int")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariant()
	if err != nil || got != int32(0x42) {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

func TestVariantBool(t *testing.T) {
	want := hx(t, "00000001 00 01")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariant(true); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "variant bool")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariant()
	if err != nil || got != true {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

func TestVariantNil(t *testing.T) {
	// Nil mappato a (QString, isNull=1) — convenzione Veyon
	want := hx(t, "0000000A 01")
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariant(nil); err != nil {
		t.Fatal(err)
	}
	checkBytes(t, buf.Bytes(), want, "variant nil")

	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariant()
	if err != nil || got != nil {
		t.Fatalf("read: got=%v err=%v", got, err)
	}
}

func TestVariantInt64(t *testing.T) {
	const want int64 = 0x0123456789ABCDEF
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariant(want); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariant()
	if err != nil || got != want {
		t.Fatalf("read: got=%v(%T) err=%v", got, got, err)
	}
}

// ============================================================
// QVariantList
// ============================================================

func TestVariantList(t *testing.T) {
	// [int32(1), "x"] -> count=2 + QVariant(1) + QVariant("x")
	in := VariantList{int32(1), "x"}
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariantList(in); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariantList()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

// ============================================================
// QVariantMap
// ============================================================

func TestVariantMap(t *testing.T) {
	// {"k1": "v1", "k2": int32(7)} — chiavi ordinate alfabeticamente
	in := VariantMap{"k2": int32(7), "k1": "v1"}
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariantMap(in); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariantMap()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %+v want %+v", got, in)
	}
}

func TestVariantMapKeyOrderDeterministic(t *testing.T) {
	// Due Encode dello stesso map devono produrre bytes identici,
	// indipendentemente dall'ordine di iterazione del map Go.
	in := VariantMap{"zebra": "z", "alpha": "a", "monkey": "m"}
	var b1, b2 bytes.Buffer
	for i := 0; i < 100; i++ { // bash ripetuto per coprire diverse iter order
		b1.Reset()
		b2.Reset()
		if err := NewEncoder(&b1).WriteVariantMap(in); err != nil {
			t.Fatal(err)
		}
		if err := NewEncoder(&b2).WriteVariantMap(in); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
			t.Fatalf("non determinismo iter %d:\nb1=%x\nb2=%x", i, b1.Bytes(), b2.Bytes())
		}
	}
}

// ============================================================
// Realistic Veyon message: RunProgram args
// ============================================================

func TestRunProgramArgs(t *testing.T) {
	// Veyon StartApp/RunProgram passa {"applications": ["notepad.exe"]}
	// (in realta' usa "program" + "arguments" come QStringList — ma
	// costruisco un caso plausibile per validare la composizione).
	args := VariantMap{
		"applications": []string{"notepad.exe", "calc.exe"},
	}
	var buf bytes.Buffer
	if err := NewEncoder(&buf).WriteVariantMap(args); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(bytes.NewReader(buf.Bytes())).ReadVariantMap()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("got %+v want %+v", got, args)
	}
}

// ============================================================
// Error paths
// ============================================================

func TestStringTooLong(t *testing.T) {
	// Header dichiara 32MB — sopra il cap.
	bad := hx(t, "02000000")
	_, err := NewDecoder(bytes.NewReader(bad)).ReadString()
	if err == nil {
		t.Fatal("expected error for oversized string")
	}
}

func TestStringOddLength(t *testing.T) {
	// Header dispari: invalido per UTF-16
	bad := hx(t, "00000003 414243")
	_, err := NewDecoder(bytes.NewReader(bad)).ReadString()
	if err == nil {
		t.Fatal("expected error for odd-length string")
	}
}

func TestVariantUnknownType(t *testing.T) {
	bad := hx(t, "00000063 00") // typeId=99, unknown
	_, err := NewDecoder(bytes.NewReader(bad)).ReadVariant()
	if err == nil {
		t.Fatal("expected error for unknown variant type")
	}
}

func TestVariantUnsupportedGoType(t *testing.T) {
	var buf bytes.Buffer
	type custom struct{ X int }
	err := NewEncoder(&buf).WriteVariant(custom{1})
	if err == nil {
		t.Fatal("expected ErrUnsupportedType for custom struct")
	}
}

// ============================================================
// Uuid hex parser
// ============================================================

func TestUuidInvalid(t *testing.T) {
	tests := []string{
		"",
		"da9ca56a",                                  // troppo corto
		"da9ca56a-b2ad-4fff-8f8a-929b2927b44",       // 31 hex
		"da9ca56a-b2ad-4fff-8f8a-929b2927b442-aaaa", // troppo lungo
		"za9ca56a-b2ad-4fff-8f8a-929b2927b442",      // hex invalido
	}
	for _, s := range tests {
		if _, err := UuidFromString(s); err == nil {
			t.Errorf("UuidFromString(%q) deve fallire", s)
		}
	}
}
