package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewAPIKey(t *testing.T) {
	plaintext, hash, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatalf("NewAPIKey: %v", err)
	}
	if !strings.HasPrefix(plaintext, keyPrefix) {
		t.Fatalf("key %q must start with %q", plaintext, keyPrefix)
	}
	if len(plaintext) != len(keyPrefix)+keyRandomLen*2 {
		t.Fatalf("key length = %d, want %d (prefix + %d hex chars)",
			len(plaintext), len(keyPrefix)+keyRandomLen*2, keyRandomLen*2)
	}

	// The hash is the canonical stored form of the plaintext
	sum := sha256.Sum256([]byte(plaintext))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %q is not sha256 of %q", hash, plaintext)
	}
	if prefix != plaintext[:keyDisplayPrefixLen] {
		t.Fatalf("prefix %q is not the head of %q", prefix, plaintext)
	}
}

func TestNewAPIKeyIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		plaintext, _, _, err := NewAPIKey()
		if err != nil {
			t.Fatalf("NewAPIKey: %v", err)
		}
		if _, dup := seen[plaintext]; dup {
			t.Fatalf("duplicate key %q after %d generations", plaintext, i)
		}
		seen[plaintext] = struct{}{}
	}
}

func TestNewAPIKeyHidesTheSecret(t *testing.T) {
	plaintext, hash, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatalf("NewAPIKey: %v", err)
	}
	// Neither the stored hash nor the display prefix may contain the secret tail
	tail := plaintext[keyDisplayPrefixLen:]
	if strings.Contains(hash, tail) || strings.Contains(prefix, tail) {
		t.Fatalf("the secret part %q leaked into the stored form", tail)
	}
}

func TestDisplayPrefixIsSafeForShortInput(t *testing.T) {
	if got := DisplayPrefix("short"); got != "short" {
		t.Fatalf("DisplayPrefix(short) = %q, want the input unchanged", got)
	}
	if got := DisplayPrefix(""); got != "" {
		t.Fatalf("DisplayPrefix() = %q, want empty", got)
	}
}

func TestHashAPIKeyIsStable(t *testing.T) {
	first := HashAPIKey("ddp_abc")
	second := HashAPIKey("ddp_abc")
	if first != second {
		t.Fatal("the same key must always hash the same way")
	}
	if len(first) != sha256.Size*2 {
		t.Fatalf("hash length = %d, want %d", len(first), sha256.Size*2)
	}
}
