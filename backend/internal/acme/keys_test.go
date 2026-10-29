package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"testing"
)

func TestValidKeyType(t *testing.T) {
	for _, keyType := range KeyTypes {
		if !ValidKeyType(keyType) {
			t.Errorf("ValidKeyType(%q) = false, want true", keyType)
		}
	}
	for _, keyType := range []string{"", "ec521", "rsa1024", "EC256", "ed25519"} {
		if ValidKeyType(keyType) {
			t.Errorf("ValidKeyType(%q) = true, want false", keyType)
		}
	}
}

// Every key type has to survive the round trip through the file the writer
// leaves behind: the PEM is what the TLS application reads, and a key that
// cannot be parsed back is a certificate nobody can serve.
func TestGenerateKeyRoundTrip(t *testing.T) {
	for _, keyType := range KeyTypes {
		t.Run(keyType, func(t *testing.T) {
			key, err := generateKey(keyType)
			if err != nil {
				t.Fatalf("generateKey: %v", err)
			}

			encoded, err := keyPEM(key)
			if err != nil {
				t.Fatalf("keyPEM: %v", err)
			}
			parsed, err := parseKey(encoded)
			if err != nil {
				t.Fatalf("parseKey: %v", err)
			}
			if !parsed.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(key.Public()) {
				t.Fatal("the parsed key is not the generated one")
			}

			switch keyType {
			case KeyEC256:
				if curve := key.(*ecdsa.PrivateKey).Curve.Params().BitSize; curve != 256 {
					t.Fatalf("curve is P-%d, want P-256", curve)
				}
			case KeyEC384:
				if curve := key.(*ecdsa.PrivateKey).Curve.Params().BitSize; curve != 384 {
					t.Fatalf("curve is P-%d, want P-384", curve)
				}
			case KeyRSA2048:
				if size := key.(*rsa.PrivateKey).N.BitLen(); size != 2048 {
					t.Fatalf("modulus is %d bits, want 2048", size)
				}
			case KeyRSA4096:
				if size := key.(*rsa.PrivateKey).N.BitLen(); size != 4096 {
					t.Fatalf("modulus is %d bits, want 4096", size)
				}
			}
		})
	}
}

func TestGenerateKeyRefusesAnUnknownType(t *testing.T) {
	if _, err := generateKey("ec521"); err == nil {
		t.Fatal("want an error for a key type that cannot be generated")
	}
}

// The request is what the CA reads the names from, so every name has to be in
// it - as a subject alternative name, which is the part that counts.
func TestCSRDERCarriesTheNames(t *testing.T) {
	names := []string{"example.com", "www.example.com", "*.example.com"}
	key, err := generateKey(KeyEC256)
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}

	der, err := csrDER(key, names)
	if err != nil {
		t.Fatalf("csrDER: %v", err)
	}
	request, err := x509.ParseCertificateRequest(der)
	if err != nil {
		t.Fatalf("parse the request: %v", err)
	}
	if err := request.CheckSignature(); err != nil {
		t.Fatalf("the request is not signed by its own key: %v", err)
	}
	if request.Subject.CommonName != names[0] {
		t.Errorf("common name = %q, want %q", request.Subject.CommonName, names[0])
	}
	if len(request.DNSNames) != len(names) {
		t.Fatalf("DNS names = %v, want %v", request.DNSNames, names)
	}
	for i, name := range names {
		if request.DNSNames[i] != name {
			t.Errorf("DNS name %d = %q, want %q", i, request.DNSNames[i], name)
		}
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	for _, input := range []string{"", "not pem at all", "-----BEGIN PRIVATE KEY-----\nnope\n-----END PRIVATE KEY-----"} {
		if _, err := parseKey([]byte(input)); err == nil {
			t.Errorf("parseKey(%q) = nil error, want a failure", input)
		}
	}
}
