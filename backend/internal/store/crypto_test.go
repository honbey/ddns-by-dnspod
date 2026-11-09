package store

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plain := "AKIDExampleSecretKey-123456"
	encoded, err := enc.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encoded == plain {
		t.Fatal("ciphertext must differ from plaintext")
	}

	got, err := enc.Decrypt(encoded)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip mismatch: %q != %q", got, plain)
	}
}

func TestDecryptFailsWithAnotherKey(t *testing.T) {
	keyA, _ := GenerateKey()
	keyB, _ := GenerateKey()
	encA, _ := NewEncryptor(keyA)
	encB, _ := NewEncryptor(keyB)

	encoded, err := encA.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := encB.Decrypt(encoded); err == nil {
		t.Fatal("decryption with the wrong key must fail")
	}
}

func TestNewEncryptorRejectsWrongKeySize(t *testing.T) {
	if _, err := NewEncryptor(make([]byte, KeySize-1)); err == nil {
		t.Fatal("short key must be rejected")
	}
	if _, err := NewEncryptor(make([]byte, KeySize+1)); err == nil {
		t.Fatal("long key must be rejected")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	key, _ := GenerateKey()
	enc, _ := NewEncryptor(key)

	if _, err := enc.Decrypt(strings.Repeat("A", 64)); err == nil {
		t.Fatal("garbage input must fail to decrypt")
	}
	if _, err := enc.Decrypt("not-base64!!!"); err == nil {
		t.Fatal("non base64 input must fail to decrypt")
	}
}
