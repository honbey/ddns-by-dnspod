package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// testBundle builds a bundle around a self-signed leaf, so everything below the
// ACME protocol can be exercised without a server: the DER chain is what the CA
// would have returned, and the intermediates are copies of the leaf because
// only the file layout cares about how many there are.
func testBundle(t *testing.T, parts int) (*Bundle, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(4242),
		Subject:      pkix.Name{CommonName: "example.com"},
		DNSNames:     []string{"example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	chain := make([][]byte, 0, parts)
	for range parts {
		chain = append(chain, der)
	}
	bundle, err := bundleFromDER(chain, key)
	if err != nil {
		t.Fatalf("bundleFromDER: %v", err)
	}
	return bundle, key
}

func TestWriteFilesLayoutAndModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site")
	bundle, key := testBundle(t, 3)

	// A file of somebody else's, to prove the writer leaves the directory
	// otherwise alone.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o644); err != nil {
		// The directory does not exist yet, which is the point of the writer
		// creating it: make it by hand only to place this file.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatalf("write notes: %v", err)
		}
	}

	if err := WriteFiles(dir, bundle, 0o600); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	// Exactly the four files certbot writes, and nothing left over: a temporary
	// file that survived a rename would be a key sitting in the directory under
	// an unexpected name.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	want := []string{FileCert, FileChain, FileFullChain, FilePrivKey, "notes.txt"}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("directory holds %v, want %v", names, want)
	}

	modes := map[string]os.FileMode{
		FileCert:      0o644,
		FileChain:     0o644,
		FileFullChain: 0o644,
		FilePrivKey:   0o600,
	}
	for name, want := range modes {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %o, want %o", name, info.Mode().Perm(), want)
		}
	}

	if kept, err := os.ReadFile(filepath.Join(dir, "notes.txt")); err != nil || string(kept) != "keep" {
		t.Fatalf("notes.txt = %q (%v), want it untouched", kept, err)
	}

	// The chain is written into fullchain.pem after the leaf, which is the order
	// a TLS application reads it in.
	fullchain, err := os.ReadFile(filepath.Join(dir, FileFullChain))
	if err != nil {
		t.Fatalf("read fullchain: %v", err)
	}
	cert, err := os.ReadFile(filepath.Join(dir, FileCert))
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	chain, err := os.ReadFile(filepath.Join(dir, FileChain))
	if err != nil {
		t.Fatalf("read chain: %v", err)
	}
	if string(fullchain) != string(cert)+string(chain) {
		t.Fatal("fullchain.pem is not the leaf followed by the chain")
	}
	if blocks := countPEMBlocks(t, chain, "CERTIFICATE"); blocks != 2 {
		t.Fatalf("chain.pem holds %d certificates, want the 2 intermediates", blocks)
	}

	// The key is readable by whoever serves TLS, and is the key that was
	// generated: PKCS#8, which every stack reads.
	keyBytes, err := os.ReadFile(filepath.Join(dir, FilePrivKey))
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("privkey.pem is not a PKCS#8 block: %v", block)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	got, ok := parsed.(crypto.Signer)
	if !ok {
		t.Fatalf("parsed key is %T, want a crypto.Signer", parsed)
	}
	if !got.Public().(interface{ Equal(crypto.PublicKey) bool }).Equal(key.Public()) {
		t.Fatal("privkey.pem is not the key the bundle was built with")
	}
}

// A leaf with no intermediate is a real answer from some issuers, and then
// there is nothing to put in chain.pem.
func TestWriteFilesWithoutAChain(t *testing.T) {
	dir := t.TempDir()
	bundle, _ := testBundle(t, 1)

	if err := WriteFiles(dir, bundle, 0o600); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileChain)); !os.IsNotExist(err) {
		t.Fatalf("chain.pem should not exist when there is no intermediate: %v", err)
	}
	fullchain, err := os.ReadFile(filepath.Join(dir, FileFullChain))
	if err != nil {
		t.Fatalf("read fullchain: %v", err)
	}
	cert, err := os.ReadFile(filepath.Join(dir, FileCert))
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	if string(fullchain) != string(cert) {
		t.Fatal("fullchain.pem should be the leaf alone")
	}
}

// Replacing a file is what a renewal does every time, and the mode has to
// follow the new contents rather than the file that was there before.
func TestWriteFilesTightensTheKeyMode(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, FilePrivKey)
	// What a restore or a `cp -r` out of a backup leaves behind.
	if err := os.WriteFile(keyPath, []byte("old key"), 0o644); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	bundle, _ := testBundle(t, 1)
	if err := WriteFiles(dir, bundle, 0o600); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %o, want 0600 after the replacement", info.Mode().Perm())
	}
	content, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if string(content) == "old key" {
		t.Fatal("the key was not replaced")
	}
}

// A chain that is empty this time has to take the chain from the issue before
// with it: cert.pem and chain.pem are read as a pair, and the old intermediate
// beside a new leaf is a pair that does not belong together.
func TestWriteFilesRemovesAStaleChain(t *testing.T) {
	dir := t.TempDir()
	withChain, _ := testBundle(t, 2)
	if err := WriteFiles(dir, withChain, 0o600); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileChain)); err != nil {
		t.Fatalf("chain.pem should have been written: %v", err)
	}

	leafOnly, _ := testBundle(t, 1)
	if err := WriteFiles(dir, leafOnly, 0o600); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileChain)); !os.IsNotExist(err) {
		t.Fatalf("chain.pem outlived the chain it held: %v", err)
	}
}

func countPEMBlocks(t *testing.T, data []byte, blockType string) int {
	t.Helper()
	count := 0
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return count
		}
		if block.Type == blockType {
			count++
		}
	}
}
