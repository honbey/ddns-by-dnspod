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

// loadIPHistory reads one family's rows in the order the chart and the table
// read them.
func loadIPHistory(t *testing.T, db *Store, family string) []model.PublicIPChange {
	t.Helper()
	var rows []model.PublicIPChange
	if err := db.DB().Where("family = ?", family).Order("created_at, id").Find(&rows).Error; err != nil {
		t.Fatalf("load the %s history: %v", family, err)
	}
	return rows
}

// A sample that falls between two stored changes rewrites what the later row
// means: its old address and the lifetime it measures both move.
func TestImportPublicIPHistoryMergesIntoTheTimeline(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "1.1.1.1", base); err != nil {
		t.Fatalf("record the baseline: %v", err)
	}
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "2.2.2.2", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("record the change: %v", err)
	}

	imported, skipped, err := db.ImportPublicIPHistory([]IPHistorySample{
		{Family: model.IPFamilyV4, IP: "3.3.3.3", At: base.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 1 || len(skipped) != 0 {
		t.Fatalf("imported = %d, skipped = %d, want 1 and 0", imported, len(skipped))
	}

	rows := loadIPHistory(t, db, model.IPFamilyV4)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].NewIP != "1.1.1.1" || rows[0].OldIP != "" || rows[0].DurationSec != nil {
		t.Errorf("baseline = %+v, want 1.1.1.1 with no previous address", rows[0])
	}
	if rows[1].OldIP != "1.1.1.1" || rows[1].NewIP != "3.3.3.3" {
		t.Errorf("middle = %+v, want 1.1.1.1 -> 3.3.3.3", rows[1])
	}
	if rows[1].DurationSec == nil || *rows[1].DurationSec != 3600 {
		t.Errorf("middle duration = %v, want 3600", rows[1].DurationSec)
	}
	if rows[2].OldIP != "3.3.3.3" || rows[2].NewIP != "2.2.2.2" {
		t.Errorf("tail = %+v, want 3.3.3.3 -> 2.2.2.2", rows[2])
	}
	if rows[2].DurationSec == nil || *rows[2].DurationSec != 3600 {
		t.Errorf("tail duration = %v, want 3600", rows[2].DurationSec)
	}
}

// Re-importing the same file must add nothing: an instant that already has a
// row is the same fact, and a repeated address is not a change.
func TestImportPublicIPHistoryIsIdempotent(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	samples := []IPHistorySample{
		{Family: model.IPFamilyV4, IP: "1.1.1.1", At: base},
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base.Add(time.Hour)},
		// Same address as the row before it: not a change, so not a row.
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base.Add(90 * time.Minute)},
	}

	imported, skipped, err := db.ImportPublicIPHistory(samples)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if imported != 2 || len(skipped) != 1 {
		t.Fatalf("first import = %d/%d, want 2 imported and 1 skipped", imported, len(skipped))
	}
	if rows := loadIPHistory(t, db, model.IPFamilyV4); len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}

	imported, skipped, err = db.ImportPublicIPHistory(samples)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if imported != 0 || len(skipped) != 3 {
		t.Fatalf("second import = %d/%d, want 0 imported and 3 skipped", imported, len(skipped))
	}
	if rows := loadIPHistory(t, db, model.IPFamilyV4); len(rows) != 2 {
		t.Fatalf("rows after the second import = %d, want 2", len(rows))
	}
}

// A sample that repeats an address the stored timeline already holds must not
// replace the stored row: the file adds facts, it does not delete them.
func TestImportPublicIPHistoryKeepsStoredRows(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "1.1.1.1", base); err != nil {
		t.Fatalf("record the baseline: %v", err)
	}
	if err := db.RecordPublicIPChange(model.IPFamilyV4, "2.2.2.2", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("record the change: %v", err)
	}

	// The sample claims 2.2.2.2 appeared an hour earlier than the stored row.
	// The stored row is what this process saw and has to survive untouched.
	imported, skipped, err := db.ImportPublicIPHistory([]IPHistorySample{
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 0 || len(skipped) != 1 {
		t.Fatalf("imported = %d, skipped = %d, want 0 and 1", imported, len(skipped))
	}

	rows := loadIPHistory(t, db, model.IPFamilyV4)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the stored 2", len(rows))
	}
	if !rows[1].CreatedAt.Equal(base.Add(2 * time.Hour)) {
		t.Errorf("stored change moved to %v, want %v", rows[1].CreatedAt, base.Add(2*time.Hour))
	}
	if rows[1].OldIP != "1.1.1.1" || rows[1].NewIP != "2.2.2.2" {
		t.Errorf("stored change = %+v, want 1.1.1.1 -> 2.2.2.2", rows[1])
	}
	if rows[1].DurationSec == nil || *rows[1].DurationSec != 7200 {
		t.Errorf("stored duration = %v, want 7200", rows[1].DurationSec)
	}
}

// A sample between two stored rows is judged against its real neighbours on
// the merged timeline, not against a stored row that another sample will sit
// in front of. The middle sample here is a genuine change and must survive.
func TestImportPublicIPHistoryKeepsSamplesBetweenStoredRows(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	stored := model.PublicIPChange{
		Family:    model.IPFamilyV4,
		NewIP:     "1.1.1.1",
		CreatedAt: base.Add(100 * time.Second),
	}
	if err := db.DB().Create(&stored).Error; err != nil {
		t.Fatalf("seed the stored row: %v", err)
	}

	imported, skipped, err := db.ImportPublicIPHistory([]IPHistorySample{
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base.Add(10 * time.Second)},
		{Family: model.IPFamilyV4, IP: "1.1.1.1", At: base.Add(20 * time.Second)},
		{Family: model.IPFamilyV4, IP: "3.3.3.3", At: base.Add(30 * time.Second)},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 3 || len(skipped) != 0 {
		t.Fatalf("imported = %d, skipped = %d, want all 3 accepted", imported, len(skipped))
	}

	rows := loadIPHistory(t, db, model.IPFamilyV4)
	want := []string{"2.2.2.2", "1.1.1.1", "3.3.3.3", "1.1.1.1"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, ip := range want {
		if rows[i].NewIP != ip {
			t.Errorf("row %d = %s, want %s", i, rows[i].NewIP, ip)
		}
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].OldIP == rows[i].NewIP {
			t.Errorf("row %d is a self-change: %s -> %s", i, rows[i].OldIP, rows[i].NewIP)
		}
	}
}

// Dropping a sample exposes a new right neighbour to the point on its left.
// The 30s sample goes in pass 1 for repeating the 20s address, which leaves
// the 20s point facing the stored 1.1.1.1 row directly; keeping it would put
// two 1.1.1.1 rows next to each other and call the second one a change.
func TestImportPublicIPHistoryDropsRedundantSamples(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	stored := model.PublicIPChange{
		Family:    model.IPFamilyV4,
		NewIP:     "1.1.1.1",
		CreatedAt: base.Add(100 * time.Second),
	}
	if err := db.DB().Create(&stored).Error; err != nil {
		t.Fatalf("seed the stored row: %v", err)
	}

	imported, skipped, err := db.ImportPublicIPHistory([]IPHistorySample{
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base.Add(10 * time.Second)},
		{Family: model.IPFamilyV4, IP: "1.1.1.1", At: base.Add(20 * time.Second)},
		// Same address as the point before it, so this one goes away and
		// leaves the 20s point facing the stored row directly.
		{Family: model.IPFamilyV4, IP: "1.1.1.1", At: base.Add(30 * time.Second)},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 1 || len(skipped) != 2 {
		t.Fatalf("imported = %d, skipped = %d, want 1 and 2", imported, len(skipped))
	}

	rows := loadIPHistory(t, db, model.IPFamilyV4)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].NewIP != "2.2.2.2" || rows[1].NewIP != "1.1.1.1" {
		t.Fatalf("rows = %+v, want 2.2.2.2 then 1.1.1.1", rows)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].OldIP == rows[i].NewIP {
			t.Errorf("row %d is a self-change: %s -> %s", i, rows[i].OldIP, rows[i].NewIP)
		}
	}
}

// Two samples in the same second are one instant, and one instant holds at
// most one row: the second would measure a zero lifetime.
func TestImportPublicIPHistoryUsesOneRowPerSecond(t *testing.T) {
	db := newTestStore(t)

	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	imported, skipped, err := db.ImportPublicIPHistory([]IPHistorySample{
		{Family: model.IPFamilyV4, IP: "1.1.1.1", At: base},
		{Family: model.IPFamilyV4, IP: "2.2.2.2", At: base},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 1 || len(skipped) != 1 {
		t.Fatalf("imported = %d, skipped = %d, want 1 and 1", imported, len(skipped))
	}
	if rows := loadIPHistory(t, db, model.IPFamilyV4); len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
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
