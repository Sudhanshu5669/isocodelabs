package pii

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// =============================================================================
// THE DRIVER INTERCEPTOR
//
// Personal-data columns are `bytea` in Postgres. Store code passes
//
//	pii.Seal(pii.Phone, phone)       as a query argument, and
//	pii.Open(pii.Phone, &lead.Phone) as a scan destination.
//
// pgx (the Postgres driver) then calls the hooks below while it encodes the
// query and decodes the result, so encryption and decryption happen at the
// database boundary, on every query, without the rest of the app knowing:
//
//	Sealed.BytesValue  binary protocol (normal pgx, migrations, tests)
//	Sealed.TextValue   text protocol (the Supabase transaction pooler)
//	Opened.ScanBytes   both protocols; pgx has already decoded the bytea
//
// pgx checks these hooks BEFORE its generic fallbacks, so a sealed value is
// never accidentally sent as plain text. The tests in tests/pii prove this for
// both protocols.
//
// A missing key fails closed: the query errors instead of writing plaintext.
// =============================================================================

// Sealed is a query argument holding personal data. It encrypts itself when
// pgx encodes it.
type Sealed struct {
	kind  Kind
	plain string
	valid bool // false means SQL NULL
}

// Seal marks plain as personal data of the given kind.
func Seal(kind Kind, plain string) Sealed { return Sealed{kind: kind, plain: plain, valid: true} }

// SealOptional is Seal, except that "" is stored as SQL NULL.
func SealOptional(kind Kind, plain string) Sealed {
	return Sealed{kind: kind, plain: plain, valid: plain != ""}
}

// SealPtr is Seal for a nullable value: nil is stored as SQL NULL.
func SealPtr(kind Kind, plain *string) Sealed {
	if plain == nil {
		return Sealed{kind: kind}
	}
	return Seal(kind, *plain)
}

// BytesValue is pgx's hook for bytea in the binary protocol.
func (s Sealed) BytesValue() ([]byte, error) {
	if !s.valid {
		return nil, nil
	}
	return Encrypt(s.kind, s.plain)
}

// TextValue is pgx's hook for the text (simple) protocol: the ciphertext as a
// bytea hex literal.
func (s Sealed) TextValue() (pgtype.Text, error) {
	b, err := s.BytesValue()
	if err != nil || b == nil {
		return pgtype.Text{}, err
	}
	return pgtype.Text{String: `\x` + hex.EncodeToString(b), Valid: true}, nil
}

// Value is the database/sql hook, so a Sealed is safe there too.
func (s Sealed) Value() (driver.Value, error) {
	b, err := s.BytesValue()
	if err != nil || b == nil {
		return nil, err
	}
	return b, nil
}

// String keeps personal data out of logs and error messages: printing a Sealed
// with fmt shows "[pii]", never the value.
func (s Sealed) String() string { return "[pii]" }

// GoString does the same for %#v.
func (s Sealed) GoString() string { return "pii.Sealed{[redacted]}" }

// Opened is a scan destination for a personal-data column. It decrypts what
// pgx reads into a plain string.
type Opened struct {
	kind   Kind
	dst    *string
	dstPtr **string
}

// Open decrypts a column into dst. SQL NULL (e.g. from a LEFT JOIN) gives "".
func Open(kind Kind, dst *string) *Opened { return &Opened{kind: kind, dst: dst} }

// OpenPtr decrypts a nullable column into dst. SQL NULL gives nil.
func OpenPtr(kind Kind, dst **string) *Opened { return &Opened{kind: kind, dstPtr: dst} }

// ScanBytes is pgx's hook for bytea, in both protocols. b is nil for SQL NULL.
func (o *Opened) ScanBytes(b []byte) error {
	if b == nil {
		if o.dstPtr != nil {
			*o.dstPtr = nil
		} else {
			*o.dst = ""
		}
		return nil
	}
	plain, err := Decrypt(o.kind, b)
	if err != nil {
		return fmt.Errorf("%s column: %w", o.kind, err)
	}
	if o.dstPtr != nil {
		*o.dstPtr = &plain
	} else {
		*o.dst = plain
	}
	return nil
}

// Scan is the database/sql hook.
func (o *Opened) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		return o.ScanBytes(nil)
	case []byte:
		return o.ScanBytes(v)
	default:
		return fmt.Errorf("pii: cannot read %T as an encrypted column", src)
	}
}
