package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrDecryptFailed means decryption failed, usually a replaced key file or corrupted data
var ErrDecryptFailed = errors.New("decrypt failed: key mismatch or corrupted data")

// KeySize is the key length required by AES-256
const KeySize = 32

// Encryptor protects sensitive fields (Tencent Cloud SecretKey, ntfy passwords, ...) with AES-256-GCM
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor builds an encryptor from a 32 byte key
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Encryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns base64 encoded nonce+ciphertext
func (e *Encryptor) Encrypt(plain string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	// The nonce is prefixed to the ciphertext and stored along with it
	sealed := e.gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a string produced by Encrypt
func (e *Encryptor) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: base64 decode failed", ErrDecryptFailed)
	}
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize+1 {
		return "", fmt.Errorf("%w: data too short", ErrDecryptFailed)
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}
	return string(plain), nil
}

// GenerateKey generates a new random key
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate random key: %w", err)
	}
	return key, nil
}
