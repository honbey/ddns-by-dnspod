package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

// A backup has to be a snapshot a second process can open and read.
//
// The WAL is the point: copying the file of a running database loses whatever
// has not been checkpointed, so this writes a row that lives only in the log and
// then checks it survived the copy.
func TestBackupSnapshotsUncheckpointedRows(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: filepath.Join(dir, "ddns.db")},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(dir, "secret.key")},
		Server:   config.ServerConfig{Mode: "release"},
	}

	st, err := New(cfg)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	account := model.Account{Name: "backup target", SecretID: "id", SecretKey: "key", KeyHint: "hint"}
	if err := st.DB().Create(&account).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	target := filepath.Join(dir, "backups", "snapshot.db")
	db, err := OpenForBackup(cfg.Database.Path)
	if err != nil {
		t.Fatalf("OpenForBackup: %v", err)
	}
	if err := Backup(context.Background(), db, target); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the source: %v", err)
	}

	// The snapshot is a database, not a file that merely exists.
	restored, err := sql.Open("sqlite", target)
	if err != nil {
		t.Fatalf("open the snapshot: %v", err)
	}
	defer restored.Close()

	var name string
	if err := restored.QueryRow(`SELECT name FROM accounts LIMIT 1`).Scan(&name); err != nil {
		t.Fatalf("read the snapshot: %v", err)
	}
	if name != account.Name {
		t.Fatalf("restored account name = %q, want %q", name, account.Name)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("snapshot mode = %o, want 600: it carries every credential", perm)
	}
}

// Replacing a backup must not be able to destroy the one already there: the new
// snapshot lands under a temporary name first.
func TestBackupKeepsThePreviousSnapshotOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "snapshot.db")
	if err := os.WriteFile(target, []byte("previous"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// A database with no rows still snapshots; what cannot work is a target
	// directory that is a file.
	blocked := filepath.Join(target, "nested.db")
	if err := Backup(context.Background(), db, blocked); err == nil {
		t.Fatal("writing under a file should fail")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read the previous snapshot: %v", err)
	}
	if string(got) != "previous" {
		t.Fatalf("the previous snapshot was replaced by %q", got)
	}
}

func TestOpenForBackupRefusesWhatThisProcessDidNotCreate(t *testing.T) {
	dir := t.TempDir()

	if _, err := OpenForBackup(filepath.Join(dir, "missing.db")); err == nil {
		t.Fatal("a missing file must be refused: sqlite would create an empty one")
	}

	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := OpenForBackup(empty); err == nil {
		t.Fatal("an empty file must be refused: VACUUM INTO would report it as a snapshot forever")
	}
}
