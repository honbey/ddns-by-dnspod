package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// OpenForBackup opens the database without migrating it and without the
// encryption key.
//
// A backup is a read of the live data: running schema steps from a cron job
// would take the write lock from the server and change what it is about to
// copy. The key is not needed either - a snapshot carries the ciphertext
// exactly as it sits in the file.
func OpenForBackup(path string) (*sql.DB, error) {
	// sqlite creates a database for a missing path and treats an empty file as
	// a fresh one; VACUUM INTO would then report a valid empty snapshot
	// forever. This process always migrates on first open, so neither is one
	// of ours.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open the database: %w", err)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("open the database: %s is empty, not a database this process created", path)
	}

	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		return nil, fmt.Errorf("open the database: %w", err)
	}
	return db, nil
}

// Backup writes a consistent snapshot of the database to path.
//
// `VACUUM INTO` is what makes it consistent: the WAL means a plain file copy of
// a running database loses whatever has not been checkpointed. The snapshot is
// written beside the target and renamed into place, so a run that dies halfway
// leaves the previous backup intact instead of a truncated one under the same
// name.
func Backup(ctx context.Context, db *sql.DB, path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create the backup directory: %w", err)
		}
	}

	tmp := fmt.Sprintf("%s.tmp-%d-%d", path, os.Getpid(), time.Now().UnixNano())
	defer func() {
		// A leftover from a failed snapshot is not a backup; the good file
		// behind `path` is what matters.
		_ = os.Remove(tmp)
	}()

	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("write the backup: %w", err)
	}
	// VACUUM writes the snapshot with the process umask, so the chmod below
	// narrows it before the file is moved into place; until then the temporary
	// file is as protected as the umask makes it. Flush before the rename, so a
	// crash cannot leave a directory entry pointing at data the disk never
	// received.
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("lock down the backup: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		return fmt.Errorf("flush the backup: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("put the backup in place: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		// The rename already happened; the backup is there. A directory that
		// refuses fsync does not fail the backup, it just weakens the crash
		// guarantee.
		slog.Warn("could not flush the backup directory", "error", err)
	}
	return nil
}

// syncFile flushes a file's contents to disk.
func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// syncDir flushes a directory entry change (a rename) to disk.
func syncDir(dir string) error {
	if dir == "" {
		dir = "."
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
