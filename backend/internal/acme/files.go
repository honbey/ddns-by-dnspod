package acme

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The file names certbot writes, so an application whose configuration already
// points at live/<domain>/fullchain.pem keeps working unchanged.
const (
	FileCert      = "cert.pem"
	FileChain     = "chain.pem"
	FileFullChain = "fullchain.pem"
	FilePrivKey   = "privkey.pem"
)

// Bundle is one issued certificate in the form it is written to disk.
type Bundle struct {
	Cert      []byte
	Chain     []byte
	FullChain []byte
	Key       []byte
	NotAfter  time.Time
	Serial    string
}

// WriteFiles writes the bundle into dir, creating the directory if it is not
// there yet, and leaves every other file in it alone.
//
// Each file goes through a temporary name in the same directory and a rename
// over the target. A rename is atomic within a filesystem, so whoever reads the
// key sees either the previous certificate or the new one and never half of
// either, which is what makes a renewal safe to run while a TLS application is
// serving from the same directory.
func WriteFiles(dir string, bundle *Bundle, keyMode os.FileMode) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	files := []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{FileCert, bundle.Cert, 0o644},
		{FileChain, bundle.Chain, 0o644},
		{FileFullChain, bundle.FullChain, 0o644},
		{FilePrivKey, bundle.Key, keyMode},
	}
	for _, file := range files {
		// An issuer may return a leaf with no intermediate, and then there is
		// nothing to write to chain.pem: an empty file would only read as a
		// truncated chain to whatever loads it. A chain from an earlier issue
		// has to go as well - cert.pem and chain.pem are read as a pair, and
		// leaving the old one would pair the new leaf with an intermediate that
		// does not belong to it.
		if len(file.data) == 0 {
			if file.name == FileChain {
				if err := os.Remove(filepath.Join(dir, FileChain)); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove the stale %s: %w", FileChain, err)
				}
			}
			continue
		}
		if err := writeFileAtomic(filepath.Join(dir, file.name), file.data, file.mode); err != nil {
			return err
		}
	}
	return nil
}

// writeFileAtomic replaces path in one step, with the mode it ends up having.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create a temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// A no-op once the rename has moved the file out of the way; otherwise
		// it keeps a failed write from leaving the temporary file behind.
		_ = os.Remove(tmpName)
	}()

	// The mode is set before the rename rather than after it, so the key never
	// exists under its final name with the mode CreateTemp gives it.
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	// Flushed before the rename: a rename that lands while the contents are
	// still in the page cache is a file a power cut turns into an empty one.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("flush %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// bundleFromDER turns what the CA returned into the files on disk.
//
// The slices are leaf first, which is the order a chain is written in:
// fullchain.pem is the leaf followed by the intermediates, and that is the file
// a TLS application wants in one piece.
func bundleFromDER(der [][]byte, key crypto.Signer) (*Bundle, error) {
	if len(der) == 0 {
		return nil, fmt.Errorf("the certificate authority returned no certificate")
	}
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return nil, fmt.Errorf("parse the issued certificate: %w", err)
	}
	keyBytes, err := keyPEM(key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der[0]})
	var chainPEM []byte
	for _, part := range der[1:] {
		chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: part})...)
	}
	fullChain := append(append([]byte{}, certPEM...), chainPEM...)

	return &Bundle{
		Cert:      certPEM,
		Chain:     chainPEM,
		FullChain: fullChain,
		Key:       keyBytes,
		NotAfter:  leaf.NotAfter,
		Serial:    leaf.SerialNumber.String(),
	}, nil
}
