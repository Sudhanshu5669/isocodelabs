package pii_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"leadflow/internal/pii"
)

// Unit tests for personal-data encryption. No database: the driver tests use
// pgx's own type map, which is exactly what turns query arguments into the
// bytes sent to Postgres.

func newKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func mustKeys(t *testing.T, enc, index string) *pii.Keys {
	t.Helper()
	k, err := pii.ParseKeys(enc, index)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// install puts fresh keys in place for tests that use the package-level API.
func install(t *testing.T) *pii.Keys {
	t.Helper()
	k := mustKeys(t, "1:"+newKey(), newKey())
	pii.Use(k)
	return k
}

func TestEncryptRoundTripAndRandomness(t *testing.T) {
	k := mustKeys(t, "1:"+newKey(), newKey())
	for _, plain := range []string{"Aarav Shah", "+919000110205", "", "आरव शाह", "આરવ શાહ", strings.Repeat("x", 5000)} {
		a, err := k.Encrypt(pii.Name, plain)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := k.Encrypt(pii.Name, plain)
		if bytes.Equal(a, b) {
			t.Errorf("%q encrypted twice gave the same bytes; the nonce must be random", plain)
		}
		if plain != "" && bytes.Contains(a, []byte(plain)) {
			t.Errorf("ciphertext contains the plaintext %q", plain)
		}
		got, err := k.Decrypt(pii.Name, a)
		if err != nil || got != plain {
			t.Errorf("round trip of %q gave %q, %v", plain, got, err)
		}
		// format byte + key id + 12-byte nonce + data + 16-byte tag
		if len(a) != 2+12+len(plain)+16 {
			t.Errorf("ciphertext length %d for %d bytes of plaintext", len(a), len(plain))
		}
	}
}

func TestCiphertextIsBoundToItsKind(t *testing.T) {
	k := mustKeys(t, "1:"+newKey(), newKey())
	phone, _ := k.Encrypt(pii.Phone, "+919000110205")
	if _, err := k.Decrypt(pii.Name, phone); !errors.Is(err, pii.ErrBadCipher) {
		t.Errorf("a phone ciphertext read as a name: err = %v, want ErrBadCipher", err)
	}
}

func TestTamperingIsDetected(t *testing.T) {
	k := mustKeys(t, "1:"+newKey(), newKey())
	sealed, _ := k.Encrypt(pii.Text, "Please call me after 6 pm")
	for i := 2; i < len(sealed); i++ { // every byte after the header
		bad := bytes.Clone(sealed)
		bad[i] ^= 0x01
		if _, err := k.Decrypt(pii.Text, bad); err == nil {
			t.Fatalf("flipping byte %d went unnoticed", i)
		}
	}
	if _, err := k.Decrypt(pii.Text, []byte("plain text")); !errors.Is(err, pii.ErrNotCipher) {
		t.Errorf("plaintext accepted as ciphertext: %v", err)
	}
	if _, err := k.Decrypt(pii.Text, nil); !errors.Is(err, pii.ErrNotCipher) {
		t.Errorf("empty value accepted: %v", err)
	}
}

func TestWrongKeyCannotDecrypt(t *testing.T) {
	a := mustKeys(t, "1:"+newKey(), newKey())
	b := mustKeys(t, "1:"+newKey(), newKey())
	sealed, _ := a.Encrypt(pii.Name, "Meera Desai")
	if _, err := b.Decrypt(pii.Name, sealed); err == nil {
		t.Error("a different key with the same id decrypted the value")
	}
}

func TestKeyRotation(t *testing.T) {
	oldKey, newKeyText, index := newKey(), newKey(), newKey()
	before := mustKeys(t, "1:"+oldKey, index)
	sealedOld, _ := before.Encrypt(pii.Email, "rohan@companya.test")

	after := mustKeys(t, "1:"+oldKey+", 2:"+newKeyText, index)
	if got, err := after.Decrypt(pii.Email, sealedOld); err != nil || got != "rohan@companya.test" {
		t.Fatalf("old value after rotation: %q, %v", got, err)
	}
	sealedNew, _ := after.Encrypt(pii.Email, "rohan@companya.test")
	if sealedNew[1] != 2 {
		t.Errorf("new values must use the newest key (id 2), used %d", sealedNew[1])
	}
	if _, err := before.Decrypt(pii.Email, sealedNew); err == nil {
		t.Error("keys without id 2 decrypted a value made with key 2")
	}
	if fmt.Sprint(after.KeyIDs()) != "[1 2]" {
		t.Errorf("KeyIDs = %v", after.KeyIDs())
	}
	// Rotating the encryption key leaves blind indexes unchanged.
	if !bytes.Equal(before.Index(pii.Email, "x"), after.Index(pii.Email, "x")) {
		t.Error("blind index changed with the encryption key")
	}
}

func TestParseKeysRejectsBadSettings(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	k1, k2 := newKey(), newKey()
	cases := map[string][2]string{
		"no encryption key":          {"", k2},
		"no id":                      {k1, k2},
		"id zero":                    {"0:" + k1, k2},
		"id too big":                 {"256:" + k1, k2},
		"not base64":                 {"1:not-base64!", k2},
		"16-byte (AES-128) key":      {"1:" + short, k2},
		"duplicate id":               {"1:" + k1 + ",1:" + newKey(), k2},
		"no index key":               {"1:" + k1, ""},
		"index key = encryption key": {"1:" + k1, k1},
	}
	for name, c := range cases {
		if _, err := pii.ParseKeys(c[0], c[1]); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestBlindIndexes(t *testing.T) {
	k := mustKeys(t, "1:"+newKey(), newKey())
	a, b := k.Index(pii.Phone, "+919000110205"), k.Index(pii.Phone, "+919000110205")
	if !bytes.Equal(a, b) || len(a) != 32 {
		t.Fatalf("blind index must be a stable 32-byte HMAC")
	}
	if bytes.Equal(a, k.Index(pii.Phone, "+919000110206")) {
		t.Error("different phones share an index")
	}
	if bytes.Equal(k.Index(pii.Name, "shah"), k.Index(pii.NameWord, "shah")) {
		t.Error("the kind must be part of the index")
	}
	other := mustKeys(t, "1:"+newKey(), newKey())
	if bytes.Equal(a, other.Index(pii.Phone, "+919000110205")) {
		t.Error("the index must depend on the secret key")
	}

	install(t)
	if !bytes.Equal(pii.EmailIndex(" Rohan@CompanyA.test "), pii.EmailIndex("rohan@companya.test")) {
		t.Error("email index must ignore case and outer spaces")
	}
}

func TestNameWords(t *testing.T) {
	cases := map[string]string{
		"Aarav Shah":          "aarav shah",
		"  SHAH,  aarav ":     "aarav shah",
		"Rakesh Rakesh Bhatt": "bhatt rakesh",
		"D'Souza-Pinto":       "d pinto souza",
		"आरव शाह":             "आरव शाह", // Devanagari vowel signs (combining marks) stay inside the word
		"":                    "",
	}
	for in, want := range cases {
		if got := strings.Join(pii.NameWords(in), " "); got != want {
			t.Errorf("NameWords(%q) = %q, want %q", in, got, want)
		}
	}
	install(t)
	if got := pii.NameWordIndexes(""); got == nil || len(got) != 0 {
		t.Errorf("an empty name must give an empty, non-nil list, got %v", got)
	}
}

// ---- the driver interceptor -------------------------------------------------

// The binary protocol (normal pgx): the argument for a bytea column must be
// ciphertext that decrypts back to the value.
func TestSealedArgumentBinaryProtocol(t *testing.T) {
	k := install(t)
	m := pgtype.NewMap()
	buf, err := m.Encode(pgtype.ByteaOID, pgtype.BinaryFormatCode, pii.Seal(pii.Phone, "+919000110205"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf, []byte("9000110205")) {
		t.Fatal("plaintext phone reached the wire")
	}
	if got, err := k.Decrypt(pii.Phone, buf); err != nil || got != "+919000110205" {
		t.Fatalf("wire bytes decrypt to %q, %v", got, err)
	}
}

// The text protocol (Supabase transaction pooler, OID unknown): the argument
// must be a bytea hex literal of the ciphertext, never the plain string, even
// though pgx would happily send a plain Go string as text.
func TestSealedArgumentTextProtocol(t *testing.T) {
	k := install(t)
	m := pgtype.NewMap()
	buf, err := m.Encode(0, pgtype.TextFormatCode, pii.Seal(pii.Name, "Aarav Shah"), nil)
	if err != nil {
		t.Fatal(err)
	}
	text := string(buf)
	if strings.Contains(text, "Aarav") || !strings.HasPrefix(text, `\x`) {
		t.Fatalf("text protocol sent %q", text)
	}
	raw, _ := hex.DecodeString(text[2:])
	if got, err := k.Decrypt(pii.Name, raw); err != nil || got != "Aarav Shah" {
		t.Fatalf("decrypts to %q, %v", got, err)
	}
	// A known bytea OID in text format takes the same path.
	buf, _ = m.Encode(pgtype.ByteaOID, pgtype.TextFormatCode, pii.Seal(pii.Name, "Aarav Shah"), nil)
	if strings.Contains(string(buf), "Aarav") {
		t.Fatal("plaintext in bytea text format")
	}
}

func TestSealNullHandling(t *testing.T) {
	install(t)
	m := pgtype.NewMap()
	for name, v := range map[string]pii.Sealed{
		"SealOptional(\"\")": pii.SealOptional(pii.Text, ""),
		"SealPtr(nil)":       pii.SealPtr(pii.Text, nil),
	} {
		for _, format := range []int16{pgtype.BinaryFormatCode, pgtype.TextFormatCode} {
			buf, err := m.Encode(pgtype.ByteaOID, format, v, nil)
			if err != nil || buf != nil {
				t.Errorf("%s format %d: got %v, %v; want SQL NULL", name, format, buf, err)
			}
		}
	}
	// Seal("") is an encrypted empty string, not NULL.
	if buf, _ := m.Encode(pgtype.ByteaOID, pgtype.BinaryFormatCode, pii.Seal(pii.Text, ""), nil); buf == nil {
		t.Error("Seal(\"\") must not be NULL")
	}
}

func TestOpenedScanBothProtocols(t *testing.T) {
	k := install(t)
	m := pgtype.NewMap()
	sealed, _ := k.Encrypt(pii.Name, "Sneha Patel")

	var bin string
	if err := m.Scan(pgtype.ByteaOID, pgtype.BinaryFormatCode, sealed, pii.Open(pii.Name, &bin)); err != nil || bin != "Sneha Patel" {
		t.Errorf("binary scan: %q, %v", bin, err)
	}
	var txt string
	hexText := []byte(`\x` + hex.EncodeToString(sealed))
	if err := m.Scan(pgtype.ByteaOID, pgtype.TextFormatCode, hexText, pii.Open(pii.Name, &txt)); err != nil || txt != "Sneha Patel" {
		t.Errorf("text scan: %q, %v", txt, err)
	}

	// NULL: "" for Open, nil for OpenPtr.
	s, p := "stale", new(string)
	if err := m.Scan(pgtype.ByteaOID, pgtype.BinaryFormatCode, nil, pii.Open(pii.Name, &s)); err != nil || s != "" {
		t.Errorf("NULL into Open: %q, %v", s, err)
	}
	if err := m.Scan(pgtype.ByteaOID, pgtype.BinaryFormatCode, nil, pii.OpenPtr(pii.Name, &p)); err != nil || p != nil {
		t.Errorf("NULL into OpenPtr: %v, %v", p, err)
	}
	var ptr *string
	if err := m.Scan(pgtype.ByteaOID, pgtype.BinaryFormatCode, sealed, pii.OpenPtr(pii.Name, &ptr)); err != nil || ptr == nil || *ptr != "Sneha Patel" {
		t.Errorf("OpenPtr: %v, %v", ptr, err)
	}

	// Wrong kind or tampered data is an error, never garbage.
	var wrong string
	if err := m.Scan(pgtype.ByteaOID, pgtype.BinaryFormatCode, sealed, pii.Open(pii.Phone, &wrong)); err == nil {
		t.Error("a name read as a phone without error")
	}
}

func TestSealedNeverPrintsItsValue(t *testing.T) {
	v := pii.Seal(pii.Phone, "+919000110205")
	for _, out := range []string{fmt.Sprint(v), fmt.Sprintf("%v %+v %#v %s", v, v, v, v)} {
		if strings.Contains(out, "9000110205") {
			t.Errorf("printed: %s", out)
		}
	}
}

func TestMissingKeysFailClosed(t *testing.T) {
	pii.Use(nil)
	defer install(t)
	if _, err := pii.Seal(pii.Name, "x").BytesValue(); !errors.Is(err, pii.ErrNoKeys) {
		t.Errorf("BytesValue without keys: %v", err)
	}
	if _, err := pii.Seal(pii.Name, "x").TextValue(); !errors.Is(err, pii.ErrNoKeys) {
		t.Errorf("TextValue without keys: %v", err)
	}
	m := pgtype.NewMap()
	if _, err := m.Encode(0, pgtype.TextFormatCode, pii.Seal(pii.Name, "x"), nil); err == nil {
		t.Error("encoding without keys must fail, not send plaintext")
	}
}
