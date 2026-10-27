package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalidSeal = errors.New("auth: invalid or expired cookie")

// Sealer encrypts and authenticates small values for cookies, with AES-256-GCM.
// The web session lives in a sealed cookie rather than in the database:
// rotating the key signs every session out.
type Sealer struct {
	aead cipher.AEAD
	now  func() time.Time
}

// NewSealer takes a 32-byte key.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("auth: session key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, now: time.Now}, nil
}

type sealed struct {
	Exp int64           `json:"exp"`
	V   json.RawMessage `json:"v"`
}

// Seal encrypts v until ttl from now. The purpose is bound into the
// ciphertext, so a value sealed for one cookie never opens as another.
func (s *Sealer) Seal(purpose string, v any, ttl time.Duration) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(sealed{Exp: s.now().Add(ttl).Unix(), V: raw})
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := s.aead.Seal(nonce, nonce, plain, []byte(purpose))
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// Open decrypts what Seal made for the same purpose, if it has not expired.
func (s *Sealer) Open(purpose, token string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(b) < s.aead.NonceSize() {
		return ErrInvalidSeal
	}
	n := s.aead.NonceSize()
	plain, err := s.aead.Open(nil, b[:n], b[n:], []byte(purpose))
	if err != nil {
		return ErrInvalidSeal
	}
	var env sealed
	if err := json.Unmarshal(plain, &env); err != nil {
		return ErrInvalidSeal
	}
	if !s.now().Before(time.Unix(env.Exp, 0)) {
		return ErrInvalidSeal
	}
	if err := json.Unmarshal(env.V, v); err != nil {
		return ErrInvalidSeal
	}
	return nil
}
