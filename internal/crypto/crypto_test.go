package crypto

import (
	"strings"
	"testing"
)

func TestNewRejectsEmptyKey(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty master key, got nil")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	s, err := New("a-high-entropy-master-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plaintext := "super-secret-community-string"
	sealed, err := s.SealString(plaintext)
	if err != nil {
		t.Fatalf("SealString: %v", err)
	}
	if strings.Contains(sealed, plaintext) {
		t.Fatal("sealed value leaks plaintext")
	}
	got, err := s.OpenString(sealed)
	if err != nil {
		t.Fatalf("OpenString: %v", err)
	}
	if got != plaintext {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestSealIsNonDeterministic(t *testing.T) {
	s, _ := New("key")
	a, _ := s.SealString("value")
	b, _ := s.SealString("value")
	if a == b {
		t.Fatal("two seals of the same plaintext must differ (random nonce)")
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	s1, _ := New("key-one")
	s2, _ := New("key-two")
	sealed, _ := s1.SealString("value")
	if _, err := s2.OpenString(sealed); err == nil {
		t.Fatal("expected auth failure opening with wrong key")
	}
}

func TestOpenRejectsGarbage(t *testing.T) {
	s, _ := New("key")
	if _, err := s.OpenString("not-base64!!"); err == nil {
		t.Fatal("expected error on malformed input")
	}
	if _, err := s.OpenString("dG9vc2hvcnQ="); err == nil {
		t.Fatal("expected error on too-short ciphertext")
	}
}
