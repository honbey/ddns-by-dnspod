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
	"sort"
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
	// By instant, not by id: after an import the ids are assigned in timeline
	// order, but a database that was written by an older build may not be, and
	// "the last row" has to be the newest address either way.
	err := s.db.Where("family = ?", family).
		Order("created_at DESC, id DESC").
		Limit(1).
		First(&last).Error
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

// IPHistorySample is one address observed at a point in time, as imported.
type IPHistorySample struct {
	Family string
	IP     string
	At     time.Time
}

// ipHistoryPoint is one entry of the merged timeline. fromImport marks a point
// this import added, and id remembers its index in the input so a refused
// sample can be named.
type ipHistoryPoint struct {
	at         time.Time
	ip         string
	fromImport bool
	id         int
}

// ImportPublicIPHistory merges imported samples into the per-family timeline
// and returns the indexes of the samples it did not store.
//
// A row here is a change, not a sample: old_ip and duration_sec describe the
// gap since the row before it. A sample that lands in the middle of the
// timeline therefore changes what every later row means, which is why the
// family is rebuilt from the merged points rather than patched in place. The
// stored rows keep their addresses and instants; only their ids are new.
//
// Only a sample can be skipped, never a stored row: the rows are what this
// process saw, and dropping one would delete a real change the file says
// nothing about. The decision is made on the fully merged timeline - judging a
// sample against the nearest stored row before the merge would compare it with
// a row that other samples may not sit next to.
func (s *Store) ImportPublicIPHistory(samples []IPHistorySample) (imported int, skipped []int, err error) {
	if len(samples) == 0 {
		return 0, nil, nil
	}

	// The same lock as RecordPublicIPChange: a task that starts while an import
	// rewrites a family must not read a half-replaced timeline and append to it.
	s.ipMu.Lock()
	defer s.ipMu.Unlock()

	type indexedSample struct {
		IPHistorySample
		index int
	}
	byFamily := make(map[string][]indexedSample, 2)
	for i, sample := range samples {
		byFamily[sample.Family] = append(byFamily[sample.Family], indexedSample{sample, i})
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		for family, list := range byFamily {
			var existing []model.PublicIPChange
			if err := tx.Where("family = ?", family).
				Order("created_at ASC, id ASC").
				Find(&existing).Error; err != nil {
				return fmt.Errorf("read the public %s history: %w", family, err)
			}

			// Merge first, cut later: a sample's neighbours are only known once
			// every sample has been placed.
			points := make([]ipHistoryPoint, 0, len(existing)+len(list))
			occupied := make(map[int64]struct{}, len(existing))
			for _, row := range existing {
				points = append(points, ipHistoryPoint{at: row.CreatedAt, ip: row.NewIP})
				occupied[row.CreatedAt.Unix()] = struct{}{}
			}

			sort.SliceStable(list, func(i, j int) bool { return list[i].At.Before(list[j].At) })
			for _, sample := range list {
				if _, taken := occupied[sample.At.Unix()]; taken {
					skipped = append(skipped, sample.index)
					continue
				}
				points = append(points, ipHistoryPoint{
					at:         sample.At,
					ip:         sample.IP,
					fromImport: true,
					id:         sample.index,
				})
				occupied[sample.At.Unix()] = struct{}{}
			}
			sort.SliceStable(points, func(i, j int) bool { return points[i].at.Before(points[j].at) })

			// Pass 1, left to right: a sample repeating the address to its
			// left says the address did not change, so the earlier row wins.
			kept := make([]ipHistoryPoint, 0, len(points))
			for _, p := range points {
				if p.fromImport && len(kept) > 0 && kept[len(kept)-1].ip == p.ip {
					skipped = append(skipped, p.id)
					continue
				}
				kept = append(kept, p)
			}

			// Pass 2, right to left: dropping a point exposes a new right
			// neighbour to the point on its left, so this decision has to be
			// made on the final timeline. Only a stored row directly to the
			// right makes an imported point redundant; keeping one resets the
			// neighbour to "nothing stored in view".
			timeline := make([]ipHistoryPoint, 0, len(kept))
			nextStored := ""
			for k := len(kept) - 1; k >= 0; k-- {
				p := kept[k]
				if p.fromImport && nextStored != "" && p.ip == nextStored {
					skipped = append(skipped, p.id)
					continue
				}
				timeline = append(timeline, p)
				if p.fromImport {
					nextStored = ""
				} else {
					nextStored = p.ip
				}
			}
			for i, j := 0, len(timeline)-1; i < j; i, j = i+1, j-1 {
				timeline[i], timeline[j] = timeline[j], timeline[i]
			}

			added := len(timeline) - len(existing)

			if added == 0 {
				continue
			}
			imported += added

			entries := make([]model.PublicIPChange, 0, len(timeline))
			for i, p := range timeline {
				entry := model.PublicIPChange{Family: family, NewIP: p.ip, CreatedAt: p.at}
				if i > 0 {
					entry.OldIP = timeline[i-1].ip
					sec := int64(p.at.Sub(timeline[i-1].at).Seconds())
					entry.DurationSec = &sec
				}
				entries = append(entries, entry)
			}

			if err := tx.Where("family = ?", family).Delete(&model.PublicIPChange{}).Error; err != nil {
				return fmt.Errorf("replace the public %s history: %w", family, err)
			}
			if len(entries) > 0 {
				if err := tx.CreateInBatches(entries, 200).Error; err != nil {
					return fmt.Errorf("write the public %s history: %w", family, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return imported, skipped, nil
}
