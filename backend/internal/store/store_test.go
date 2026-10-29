package store

import (
	"path/filepath"
	"testing"
	"time"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Database: config.DatabaseConfig{Path: filepath.Join(dir, "test.db")},
		Crypto:   config.CryptoConfig{KeyFile: filepath.Join(dir, "secret.key")},
	}
	st, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// A boolean column declared with gorm:"default:true" makes it impossible to store
// false: GORM skips zero valued fields, so the database default wins and the value
// silently flips to true. These tests pin the behaviour of every boolean flag.
func TestBooleanDefaultsCanBeStoredAsFalse(t *testing.T) {
	db := newTestStore(t)

	account := model.Account{Name: "acc", SecretID: "id", SecretKey: "key"}
	if err := db.DB().Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	task := model.DDNSTask{
		AccountID:      account.ID,
		Domain:         "example.com",
		SubDomain:      "a",
		RecordType:     "A",
		Enabled:        false,
		NotifyOnChange: false,
	}
	if err := db.DB().Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	var stored model.DDNSTask
	if err := db.DB().First(&stored, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if stored.Enabled {
		t.Error("Enabled=false must survive the round trip")
	}
	if stored.NotifyOnChange {
		t.Error("NotifyOnChange=false must survive the round trip")
	}

	// Sanity check: the non default value still works
	if err := db.DB().Model(&stored).Update("enabled", true).Error; err != nil {
		t.Fatalf("enable task: %v", err)
	}
	var reloaded model.DDNSTask
	if err := db.DB().First(&reloaded, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if !reloaded.Enabled {
		t.Error("Enabled=true must survive the round trip")
	}
}

func TestChannelCanBeCreatedDisabled(t *testing.T) {
	db := newTestStore(t)

	ch := model.NotifyChannel{Name: "n", Type: model.ChannelNtfy, Enabled: false, Config: "{}"}
	if err := db.DB().Create(&ch).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	var stored model.NotifyChannel
	if err := db.DB().First(&stored, ch.ID).Error; err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if stored.Enabled {
		t.Error("a channel created disabled must stay disabled")
	}
}

// The public IP history must collapse repeated samples of the same address
// into one row, keep the address families apart, and measure a lifetime from
// the moment the previous address was first seen.
func TestRecordPublicIPChange(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	if err := db.RecordPublicIPChange(model.IPFamilyV4, "1.1.1.1", base); err != nil {
		t.Fatalf("record the v4 baseline: %v", err)
	}
	// Every task of the same family sees the same address, so this is the common
	// case and must not add a row
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "1.1.1.1", base.Add(time.Hour)); err != nil {
		t.Fatalf("record an unchanged v4 address: %v", err)
	}
	// A separate timeline: an IPv6 change must not touch the v4 rows
	if err := db.RecordPublicIPChange(model.IPFamilyV6, "2001:db8::1", base); err != nil {
		t.Fatalf("record the v6 baseline: %v", err)
	}
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "2.2.2.2", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("record the v4 change: %v", err)
	}

	var rows []model.PublicIPChange
	if err := db.DB().Order("id").Find(&rows).Error; err != nil {
		t.Fatalf("load rows: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (v4 baseline, v6 baseline, v4 change)", len(rows))
	}

	if rows[0].OldIP != "" || rows[0].NewIP != "1.1.1.1" || rows[0].DurationSec != nil {
		t.Errorf("baseline = %+v, want old empty, new 1.1.1.1 and no duration", rows[0])
	}
	if rows[1].Family != model.IPFamilyV6 || rows[1].NewIP != "2001:db8::1" {
		t.Errorf("v6 baseline = %+v, want family v6 and new 2001:db8::1", rows[1])
	}
	if rows[2].OldIP != "1.1.1.1" || rows[2].NewIP != "2.2.2.2" {
		t.Errorf("change = %+v, want 1.1.1.1 -> 2.2.2.2", rows[2])
	}
	if rows[2].DurationSec == nil || *rows[2].DurationSec != int64(2*time.Hour/time.Second) {
		t.Errorf("duration = %v, want 7200", rows[2].DurationSec)
	}
}

func TestStoreMigratesEveryModel(t *testing.T) {
	db := newTestStore(t)

	for _, table := range []string{"accounts", "ddns_tasks", "ddns_history", "public_ip_changes", "notify_channels", "api_keys"} {
		if !db.DB().Migrator().HasTable(table) {
			t.Errorf("table %s was not migrated", table)
		}
	}
}

// The keeper is only as good as the query it runs: rows past the retention go,
// rows younger stay. Pinned directly, because the goroutine that calls it once
// a day is not the part that can go wrong.
func TestDeleteHistoryBeforeKeepsTheYoungRows(t *testing.T) {
	st := newTestStore(t)

	old := time.Now().AddDate(0, 0, -100)
	recent := time.Now().Add(-time.Hour)
	rows := []model.DDNSHistory{
		{Domain: "example.com", SubDomain: "a", RecordType: "A", Status: "OK", CreatedAt: old},
		{Domain: "example.com", SubDomain: "b", RecordType: "A", Status: "OK", CreatedAt: old.Add(-time.Hour)},
		{Domain: "example.com", SubDomain: "c", RecordType: "A", Status: "OK", CreatedAt: recent},
	}
	for i := range rows {
		if err := st.DB().Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	n, err := st.DeleteHistoryBefore(time.Now().AddDate(0, 0, -90))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 2 {
		t.Fatalf("removed %d rows, want the 2 older than the cutoff", n)
	}

	var left []model.DDNSHistory
	if err := st.DB().Find(&left).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(left) != 1 || left[0].SubDomain != "c" {
		t.Fatalf("left = %+v, want only the young row", left)
	}
}
