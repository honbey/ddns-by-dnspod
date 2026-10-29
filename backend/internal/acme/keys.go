package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
)

// The key types a certificate may be issued with, spelled the way the API and
// the form spell them.
const (
	KeyEC256   = "ec256"
	KeyEC384   = "ec384"
	KeyRSA2048 = "rsa2048"
	KeyRSA4096 = "rsa4096"
)

// KeyTypes is every accepted key type, in the order the form offers them: P-256
// first, because it is what a deployment that has a choice picks and what an
// ACME account key is made of here as well.
var KeyTypes = []string{KeyEC256, KeyEC384, KeyRSA2048, KeyRSA4096}

// ValidKeyType reports whether this package can generate a key type.
func ValidKeyType(keyType string) bool {
	for _, known := range KeyTypes {
		if keyType == known {
			return true
		}
	}
	return false
}

// generateKey builds the private key of one certificate.
func generateKey(keyType string) (crypto.Signer, error) {
	switch keyType {
	case KeyEC256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case KeyEC384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case KeyRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case KeyRSA4096:
		return rsa.GenerateKey(rand.Reader, 4096)
	default:
		return nil, fmt.Errorf("unknown key type %q", keyType)
	}
}

// keyPEM encodes a private key as PKCS#8.
//
// One encoding for both algorithms: every TLS stack in use reads it, and it
// keeps the file format independent of the algorithm - which is the one thing a
// script on the other side would otherwise have to know.
func keyPEM(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode the private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// parseKey reads back a key this package wrote.
func parseKey(pemBytes []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("the stored key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse the stored key: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("the stored key is not a signing key")
	}
	return signer, nil
}

// csrDER builds the signing request for names.
//
// The subject carries the first name and every name is a subject alternative
// name, which is what an ACME server looks at: a request whose SANs are missing
// one of the ordered names is refused.
func csrDER(key crypto.Signer, names []string) ([]byte, error) {
	request := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: names[0]},
		DNSNames: names,
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, request, key)
	if err != nil {
		return nil, fmt.Errorf("create the certificate request: %w", err)
	}
	return der, nil
}
