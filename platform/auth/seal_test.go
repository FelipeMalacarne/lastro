package auth

import (
	"errors"
	"testing"
	"time"
)

func TestSealRoundTrip(t *testing.T) {
	s := testSealer(t)
	tok, err := s.Seal("a", session{Sub: "x", Email: "me@example.com"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got session
	if err := s.Open("a", tok, &got); err != nil || got.Sub != "x" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSealRejects(t *testing.T) {
	s := testSealer(t)
	tok, _ := s.Seal("a", session{Sub: "x"}, time.Hour)
	other, _ := NewSealer([]byte("ffffffffffffffffffffffffffffffff"))
	expired, _ := s.Seal("a", session{Sub: "x"}, -time.Second)
	flipped := []byte(tok)
	flipped[len(flipped)/2] ^= 1

	for name, open := range map[string]func(*session) error{
		"another purpose": func(v *session) error { return s.Open("b", tok, v) },
		"another key":     func(v *session) error { return other.Open("a", tok, v) },
		"expired":         func(v *session) error { return s.Open("a", expired, v) },
		"tampered":        func(v *session) error { return s.Open("a", string(flipped), v) },
		"garbage":         func(v *session) error { return s.Open("a", "not-a-cookie", v) },
	} {
		var v session
		if err := open(&v); !errors.Is(err, ErrInvalidSeal) {
			t.Errorf("%s: want ErrInvalidSeal, got %v", name, err)
		}
	}
}

func TestSealerKeyLength(t *testing.T) {
	if _, err := NewSealer([]byte("short")); err == nil {
		t.Fatal("want an error for a short key")
	}
}
