// Package store wraps SQLite persistence and credential encryption.
package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

// Store combines a database handle with an encryptor
type Store struct {
	db  *gorm.DB
	enc *Encryptor

	// Serialises the read-compare-write in RecordPublicIPChange. Tasks of the
	// same address family can run at the same time (a cron tick racing a manual
	// trigger on another task), and without this both would read the same
	// "last" row and insert the same change twice.
	ipMu sync.Mutex

	// Closed once, on Close, to stop the history keeper. The Once is because
	// the startup-failure paths close the store by hand while the deferred one
	// would too, and closing a channel twice is a panic.
	stopCleanup chan struct{}
	cleanupOnce sync.Once
}

// Ping answers whether the database still responds, which is what readiness
// means for this process: the HTTP server can be up while the file has gone.
func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// DB exposes the underlying GORM handle
func (s *Store) DB() *gorm.DB { return s.db }

// Encryptor exposes the encryptor
func (s *Store) Encryptor() *Encryptor { return s.enc }

// Close closes the database connection and stops the history keeper
func (s *Store) Close() error {
	s.cleanupOnce.Do(func() {
		if s.stopCleanup != nil {
			close(s.stopCleanup)
		}
	})
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// dsnFor builds the connection string the whole package shares.
//
// busy_timeout avoids concurrent write lock conflicts; WAL improves read/write
// concurrency. A path holding a "?" or "#" would be split by the DSN parser -
// everything after it becomes a query parameter - so a database name could
// otherwise smuggle pragmas past the ones set here.
func dsnFor(dbPath string) string {
	if strings.ContainsAny(dbPath, "?#") {
		dbPath = strings.NewReplacer("?", "%3f", "#", "%23").Replace(dbPath)
	}
	return fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
}

// New opens the database, loads the key and runs auto migration
func New(cfg *config.Config) (*Store, error) {
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "data/ddns.db"
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}

	key, err := loadOrCreateKey(cfg.Crypto.KeyFile)
	if err != nil {
		return nil, err
	}
	enc, err := NewEncryptor(key)
	if err != nil {
		return nil, err
	}

	dsn := dsnFor(dbPath)

	gormLogLevel := logger.Warn
	if cfg.Server.Mode == "debug" {
		gormLogLevel = logger.Info
	}

	// "record not found" is a normal outcome on the delete and update paths, and the
	// handlers already turn it into a 404, so it must not spam the log with noise.
	// Color codes are for interactive terminals only; journald or docker logs
	// would otherwise capture raw ANSI escapes into the log files.
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  gormLogLevel,
				IgnoreRecordNotFoundError: true,
				Colorful:                  cfg.Server.Mode == "debug",
			},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// One connection, so a read never has to upgrade to a write lock on a
	// second one: SQLite answers that upgrade with SQLITE_BUSY without ever
	// consulting busy_timeout, and this service is nowhere near needing the
	// concurrency a pool would add.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get database handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&model.Account{},
		&model.DDNSTask{},
		&model.DDNSHistory{},
		&model.PublicIPChange{},
		&model.NotifyChannel{},
		&model.APIKey{},
		&model.Certificate{},
		&model.ACMEAccount{},
	); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	s := &Store{db: db, enc: enc, stopCleanup: make(chan struct{})}
	s.keepHistory(cfg.History.RetentionDays)
	return s, nil
}

// historyKeepInterval is how often the keeper looks for rows to drop.
const historyKeepInterval = 24 * time.Hour

// keepHistory drops the change history older than the retention, once at start
// and then once a day.
//
// A table with no keeper only ever grows, and a record that flaps every five
// minutes produces a row each time - thousands a day of a value nobody will
// ever page through. A retention of 0 keeps everything, which is what the table
// did before it had a keeper.
func (s *Store) keepHistory(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	drop := func() {
		n, err := s.DeleteHistoryBefore(time.Now().AddDate(0, 0, -retentionDays))
		if err != nil {
			slog.Error("failed to prune the history", "error", err)
			return
		}
		if n > 0 {
			slog.Info("pruned history rows", "rows", n, "older_than_days", retentionDays)
		}
	}
	drop()
	go func() {
		ticker := time.NewTicker(historyKeepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				drop()
			case <-s.stopCleanup:
				return
			}
		}
	}()
}

// DeleteHistoryBefore removes the history rows older than the cutoff and
// reports how many it removed.
func (s *Store) DeleteHistoryBefore(cutoff time.Time) (int64, error) {
	res := s.db.Where("created_at < ?", cutoff).Delete(&model.DDNSHistory{})
	return res.RowsAffected, res.Error
}

// loadOrCreateKey reads the encryption key: env var first, then the key file, else generate and persist one
func loadOrCreateKey(keyFile string) ([]byte, error) {
	if env := os.Getenv("DDNS_CRYPTO_KEY"); env != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env))
		if err != nil {
			return nil, fmt.Errorf("DDNS_CRYPTO_KEY must be base64 encoded: %w", err)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("DDNS_CRYPTO_KEY must decode to %d bytes, got %d", KeySize, len(key))
		}
		return key, nil
	}

	if keyFile == "" {
		keyFile = "data/secret.key"
	}

	// Read it directly when the file already exists
	if raw, err := os.ReadFile(keyFile); err == nil {
		// A file copied or mounted into place can carry permissions no new key
		// would be created with, which undoes the point of encrypting anything
		// with it. It is tightened rather than refused, so a restored backup
		// still starts.
		if info, statErr := os.Stat(keyFile); statErr == nil && info.Mode().Perm()&0o077 != 0 {
			if chmodErr := os.Chmod(keyFile, 0o600); chmodErr != nil {
				slog.Warn("could not restrict the permissions of the key file", "path", keyFile, "error", chmodErr)
			}
		}

		key, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if decErr != nil {
			return nil, fmt.Errorf("key file %s is not valid base64: %w", keyFile, decErr)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("key file %s should hold %d bytes, got %d", keyFile, KeySize, len(key))
		}
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// First run: generate a new key
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o755); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(keyFile, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}
	return key, nil
}

// RecordPublicIPChange appends a row when this host's public address for the
// family differs from the one recorded last, and returns without writing when
// nothing moved.
//
// Every task of the same family sees the same egress address, so the first run
// after a change writes the row and the following ones observe the new address
// as already current. That is what collapses the N per-domain history rows of
// one real address change into a single row here.
func (s *Store) RecordPublicIPChange(family, ip string, at time.Time) error {
	s.ipMu.Lock()
	defer s.ipMu.Unlock()

	var last model.PublicIPChange
	err := s.db.Where("family = ?", family).Order("id DESC").Limit(1).First(&last).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("read the last public %s address: %w", family, err)
	}
	hasLast := err == nil
	if hasLast && last.NewIP == ip {
		return nil
	}

	entry := model.PublicIPChange{Family: family, NewIP: ip, CreatedAt: at}
	if hasLast {
		entry.OldIP = last.NewIP
		// How long the previous address stayed in place: from the moment it was
		// first seen to the moment it was replaced.
		sec := int64(at.Sub(last.CreatedAt).Seconds())
		entry.DurationSec = &sec
	}
	if err := s.db.Create(&entry).Error; err != nil {
		return fmt.Errorf("record the public %s address: %w", family, err)
	}
	return nil
}
