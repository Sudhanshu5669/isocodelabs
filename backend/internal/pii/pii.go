// Package pii protects personal data at rest, as the DPDP Act 2023 expects:
// names, phone numbers, email addresses and free text such as chat messages
// are encrypted before they reach the database.
//
//   - Encryption: AES-256-GCM. Every value gets a fresh random 12-byte nonce,
//     and GCM's tag means a changed or swapped ciphertext fails to decrypt
//     instead of quietly reading as something else.
//   - Blind indexes: HMAC-SHA-256 with a SEPARATE key. Encryption is
//     randomised, so the same phone number encrypts differently every time and
//     cannot be searched. Its blind index is always the same for the same
//     number, so the database can still find a lead by phone, enforce "one lead
//     per phone" and find a user by email, without ever seeing the value.
//
// The interception happens in the database driver (driver.go): store code
// marks a query argument with Seal and a result column with Open, and pgx calls
// our hooks as it encodes and decodes. No plaintext personal data is sent to
// Postgres and nothing is stored unencrypted.
//
// What this does NOT hide: blind indexes reveal when two rows hold the same
// value (two leads with the same first name have the same name-word index).
// That is the usual, accepted trade-off for searchable encryption.
package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// Kind says what sort of personal data a value is. It is bound into every
// ciphertext (as GCM "additional data") and into every blind index, so a value
// encrypted as a phone number can never be passed off as a name, and a name
// index can never match a phone index.
type Kind string

// The kinds of personal data the app stores.
const (
	Name     Kind = "name"      // a person's name (buyers and staff)
	Phone    Kind = "phone"     // a phone number, already normalised to E.164
	Email    Kind = "email"     // a staff email address
	Text     Kind = "text"      // free text: chat messages, summaries, notes, reminder bodies
	NameWord Kind = "name-word" // one word of a name; used only for blind indexes (search)
)

const (
	formatV1  byte = 1  // first byte of every ciphertext, so the layout can change later
	keySize        = 32 // AES-256 and the HMAC key both use 32 random bytes
	headerLen      = 2  // format byte + key id byte
)

// Errors.
var (
	ErrNoKeys     = errors.New("pii: encryption keys are not configured")
	ErrBadCipher  = errors.New("pii: value cannot be decrypted (wrong key, wrong kind or tampered data)")
	ErrNotCipher  = errors.New("pii: value is not an encrypted pii value")
	errUnknownKey = errors.New("pii: value was encrypted with a key that is not configured")
)

// Keys holds the encryption keys and the blind-index key.
//
// Several encryption keys can be loaded at once so keys can be ROTATED: new
// values are always written with the newest key (the highest id), and old
// values written with an older key still decrypt. Each ciphertext records the
// id of the key that made it.
//
// The blind-index key cannot be rotated the same way (every index would have
// to be recomputed), so it is a single key.
type Keys struct {
	aeads   map[byte]cipher.AEAD
	current byte
	index   []byte
}

// ParseKeys reads the two settings:
//
//	encryptionKeys  "1:<base64 32 bytes>" or, after a rotation, "1:<old>,2:<new>"
//	indexKey        "<base64 32 bytes>", different from every encryption key
//
// Generate a key with: openssl rand -base64 32
func ParseKeys(encryptionKeys, indexKey string) (*Keys, error) {
	k := &Keys{aeads: map[byte]cipher.AEAD{}}
	seen := map[string]bool{}

	for _, part := range strings.Split(encryptionKeys, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idText, b64, ok := strings.Cut(part, ":")
		if !ok {
			return nil, errors.New(`pii: each encryption key must look like "1:<base64>"`)
		}
		id, err := strconv.Atoi(strings.TrimSpace(idText))
		if err != nil || id < 1 || id > 255 {
			return nil, fmt.Errorf("pii: key id %q must be a number from 1 to 255", idText)
		}
		raw, err := decodeKey(b64)
		if err != nil {
			return nil, fmt.Errorf("pii: encryption key %d: %w", id, err)
		}
		if _, dup := k.aeads[byte(id)]; dup {
			return nil, fmt.Errorf("pii: encryption key id %d is listed twice", id)
		}
		block, err := aes.NewCipher(raw) // 32-byte key = AES-256
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		k.aeads[byte(id)] = aead
		seen[string(raw)] = true
		if byte(id) > k.current {
			k.current = byte(id)
		}
	}
	if len(k.aeads) == 0 {
		return nil, errors.New("pii: no encryption key given")
	}

	raw, err := decodeKey(indexKey)
	if err != nil {
		return nil, fmt.Errorf("pii: index key: %w", err)
	}
	if seen[string(raw)] {
		return nil, errors.New("pii: the index key must be different from every encryption key")
	}
	k.index = raw
	return k, nil
}

func decodeKey(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, errors.New("not valid base64")
	}
	if len(raw) != keySize {
		return nil, fmt.Errorf("must be %d bytes, got %d", keySize, len(raw))
	}
	return raw, nil
}

// KeyIDs lists the configured encryption key ids, oldest first.
func (k *Keys) KeyIDs() []int {
	ids := make([]int, 0, len(k.aeads))
	for id := range k.aeads {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	return ids
}

// additionalData binds a ciphertext to its header and its kind.
func additionalData(header []byte, kind Kind) []byte {
	return append(append([]byte("leadflow/pii/v1/"), header...), kind...)
}

// Encrypt seals plain with the newest key. The result is
//
//	[format 1 byte][key id 1 byte][nonce 12 bytes][ciphertext + 16-byte GCM tag]
func (k *Keys) Encrypt(kind Kind, plain string) ([]byte, error) {
	aead := k.aeads[k.current]
	out := make([]byte, headerLen+aead.NonceSize(), headerLen+aead.NonceSize()+len(plain)+aead.Overhead())
	out[0], out[1] = formatV1, k.current
	nonce := out[headerLen:]
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(out, nonce, []byte(plain), additionalData(out[:headerLen], kind)), nil
}

// Decrypt opens a value made by Encrypt with the same kind.
func (k *Keys) Decrypt(kind Kind, sealed []byte) (string, error) {
	if len(sealed) < headerLen || sealed[0] != formatV1 {
		return "", ErrNotCipher
	}
	aead, ok := k.aeads[sealed[1]]
	if !ok {
		return "", errUnknownKey
	}
	if len(sealed) < headerLen+aead.NonceSize()+aead.Overhead() {
		return "", ErrNotCipher
	}
	nonce := sealed[headerLen : headerLen+aead.NonceSize()]
	body := sealed[headerLen+aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, body, additionalData(sealed[:headerLen], kind))
	if err != nil {
		return "", ErrBadCipher
	}
	return string(plain), nil
}

// Index returns the blind index of an already-normalised value:
// HMAC-SHA-256(indexKey, kind + 0x00 + value). Same input, same 32 bytes.
func (k *Keys) Index(kind Kind, normalized string) []byte {
	mac := hmac.New(sha256.New, k.index)
	mac.Write([]byte("leadflow/bidx/v1/"))
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(normalized))
	return mac.Sum(nil)
}

// -----------------------------------------------------------------------------
// The keys the running program uses
// -----------------------------------------------------------------------------

// active holds the process-wide keys. The driver hooks cannot be handed keys
// per call (pgx calls them), so the program installs them once at start-up.
//
// Go lesson: atomic.Pointer lets many goroutines read the value safely while
// one writes it, without a mutex.
var active atomic.Pointer[Keys]

// Use installs the keys for this process. Call it once at start-up, before
// any database work.
func Use(k *Keys) { active.Store(k) }

func current() (*Keys, error) {
	if k := active.Load(); k != nil {
		return k, nil
	}
	return nil, ErrNoKeys
}

// Active returns the installed keys (ErrNoKeys if Use was never called).
func Active() (*Keys, error) { return current() }

// Encrypt seals plain with the installed keys.
func Encrypt(kind Kind, plain string) ([]byte, error) {
	k, err := current()
	if err != nil {
		return nil, err
	}
	return k.Encrypt(kind, plain)
}

// Decrypt opens a value with the installed keys.
func Decrypt(kind Kind, sealed []byte) (string, error) {
	k, err := current()
	if err != nil {
		return "", err
	}
	return k.Decrypt(kind, sealed)
}

// Index returns a blind index with the installed keys. It panics if no keys
// are installed: a missing index would silently break uniqueness, so this must
// never run before Use (every command calls Use right after loading config).
func Index(kind Kind, normalized string) []byte {
	k, err := current()
	if err != nil {
		panic(err)
	}
	return k.Index(kind, normalized)
}
