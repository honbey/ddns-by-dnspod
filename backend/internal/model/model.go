// Package model defines the GORM data models.
package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Task run statuses
const (
	StatusOK   = "OK"   // updated successfully
	StatusFAIL = "FAIL" // update failed
	StatusSKIP = "SKIP" // IP unchanged, skipped
)

// Notification channel types
const (
	ChannelNtfy    = "ntfy"
	ChannelWebhook = "webhook"
)

// Account holds Tencent Cloud API credentials. SecretKey is stored encrypted and never serialized.
type Account struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	Name      string    `json:"name" gorm:"size:64;not null;uniqueIndex"`
	SecretID  string    `json:"secret_id" gorm:"size:128;not null"`
	SecretKey string    `json:"-" gorm:"size:512;not null"`
	KeyHint   string    `json:"key_hint" gorm:"size:16"`
	IsDefault bool      `json:"is_default" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (Account) TableName() string { return "accounts" }

// MaskSecret renders a secret as first three + six asterisks + last three,
// e.g. "AbC******xYz". Six masked characters give a consistent width that does
// not leak the true length, while showing both ends lets a human confirm which
// credential is stored without exposing anything brute-forceable.
//
// Secrets shorter than nine characters are fully masked: head and tail would
// otherwise overlap and leak characters from the middle.
func MaskSecret(secret string) string {
	// Counted in runes: a multibyte secret sliced by bytes would produce halves
	// of a character, which survives into JSON as a replacement rune.
	runes := []rune(secret)
	if len(runes) < 9 {
		return "*********"
	}
	return string(runes[:3]) + "******" + string(runes[len(runes)-3:])
}

// DDNSTask is a DDNS update task. Each task keeps its own IP state,
// avoiding the flaw of the old Python version, where a single global last_ip prevented retries.
type DDNSTask struct {
	ID             uint       `json:"id" gorm:"primarykey"`
	AccountID      uint       `json:"account_id" gorm:"index;not null"`
	Account        *Account   `json:"account,omitempty" gorm:"foreignKey:AccountID;references:ID"`
	Domain         string     `json:"domain" gorm:"size:128;not null"`
	SubDomain      string     `json:"sub_domain" gorm:"size:128;not null;default:'@'"`
	RecordType     string     `json:"record_type" gorm:"size:16;not null;default:'A'"`
	RecordLine     string     `json:"record_line" gorm:"size:64;not null;default:'默认'"`
	TTL            int        `json:"ttl" gorm:"default:600"`
	Enabled        bool       `json:"enabled"`
	CronExpr       string     `json:"cron_expr" gorm:"size:64;not null;default:'*/5 * * * *'"`
	CurrentIP      string     `json:"current_ip" gorm:"size:64"`
	IPSince        *time.Time `json:"ip_since"`
	LastRunAt      *time.Time `json:"last_run_at"`
	LastStatus     string     `json:"last_status" gorm:"size:16"`
	LastMessage    string     `json:"last_message" gorm:"size:512"`
	FailCount      int        `json:"fail_count" gorm:"default:0"`
	NotifyOnChange bool       `json:"notify_on_change"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName specifies the table name
func (DDNSTask) TableName() string { return "ddns_tasks" }

// DDNSHistory is one change a DDNS task applied to a domain's DNS record.
//
// It is keyed by domain, so with several tasks on the same address family one
// real address change produces one row per task. The host wide, domain
// independent timeline lives in PublicIPChange instead.
type DDNSHistory struct {
	ID          uint      `json:"id" gorm:"primarykey"`
	TaskID      *uint     `json:"task_id" gorm:"index"`
	Domain      string    `json:"domain" gorm:"size:128;not null"`
	SubDomain   string    `json:"sub_domain" gorm:"size:128;not null"`
	RecordType  string    `json:"record_type" gorm:"size:16;not null"`
	RecordID    *uint64   `json:"record_id"`
	OldIP       string    `json:"old_ip" gorm:"size:64"`
	NewIP       string    `json:"new_ip" gorm:"size:64"`
	Status      string    `json:"status" gorm:"size:16;not null"`
	Message     string    `json:"message" gorm:"size:1024"`
	DurationSec *int64    `json:"duration_sec"`
	CreatedAt   time.Time `json:"created_at" gorm:"index"`
}

// TableName specifies the table name
func (DDNSHistory) TableName() string { return "ddns_history" }

// Public address families used by the IP change history.
//
// They are deliberately not the DNS record types: the history tracks this
// host's egress address, which exists whether or not any task points at it.
// Recording "v4"/"v6" also keeps the two timelines independent, so an IPv6
// change never truncates the lifetime measured for IPv4.
const (
	IPFamilyV4 = "v4"
	IPFamilyV6 = "v6"
)

// PublicIPChange is one change of this host's public address, independent of
// any domain or DDNS task.
//
// It exists because DDNSHistory is per task: with N tasks on the same family a
// single address change used to be written N times, one row per domain, which
// made "how long did that IP live" impossible to read off the chart.
//
// DurationSec is how long the previous address (OldIP) was in place, measured
// from the moment it was first seen to the moment it changed. It is NULL for
// the very first row of a family, which only establishes the baseline.
type PublicIPChange struct {
	ID          uint      `json:"id" gorm:"primarykey"`
	Family      string    `json:"family" gorm:"size:8;not null;index"`
	OldIP       string    `json:"old_ip" gorm:"size:64"`
	NewIP       string    `json:"new_ip" gorm:"size:64;not null"`
	DurationSec *int64    `json:"duration_sec"`
	CreatedAt   time.Time `json:"created_at" gorm:"index"`
}

// TableName specifies the table name
func (PublicIPChange) TableName() string { return "public_ip_changes" }

// NotifyChannel is a notification channel. Config holds type specific JSON.
type NotifyChannel struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	Name      string    `json:"name" gorm:"size:64;not null"`
	Type      string    `json:"type" gorm:"size:32;not null"`
	Enabled   bool      `json:"enabled"`
	Config    string    `json:"config" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (NotifyChannel) TableName() string { return "notify_channels" }

// APIKey is a credential for the public REST API under /api/v1.
//
// Only the SHA-256 hash is stored: the plaintext is returned once, when the key
// is created, and can never be read back. Domain is the scope of the key: one
// zone name, or APIKeyWildcard for every domain of the account.
type APIKey struct {
	ID         uint       `json:"id" gorm:"primarykey"`
	Name       string     `json:"name" gorm:"size:64;not null"`
	KeyHash    string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	KeyPrefix  string     `json:"key_prefix" gorm:"size:16;not null"`
	Domain     string     `json:"domain" gorm:"size:128;not null"`
	AccountID  uint       `json:"account_id" gorm:"not null"`
	Enabled    bool       `json:"enabled"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TableName specifies the table name
func (APIKey) TableName() string { return "api_keys" }

// APIKeyWildcard is the scope of a key that may touch every domain of its
// account rather than one name. No zone name can be this value: a domain has at
// least two labels, and a star is not one of them.
const APIKeyWildcard = "*"

// Certificate run statuses. OK and FAIL mean what they mean for a DDNS task;
// RUNNING is a run in progress, which is also what the API answers with while
// the issuance happens in the background.
const (
	CertStatusOK      = "OK"
	CertStatusFAIL    = "FAIL"
	CertStatusRUNNING = "RUNNING"
)

// Certificate is one ACME certificate this deployment manages.
//
// The private key is never in here and never leaves the filesystem it is
// written to: these fields describe what was issued and where it landed, and
// that is all the API ever returns.
type Certificate struct {
	ID   uint   `json:"id" gorm:"primarykey"`
	Name string `json:"name" gorm:"size:64;not null;uniqueIndex"`
	// AccountID is the DNSPod account whose zones answer the DNS-01 challenge.
	// Unlike a DDNS task it declares no association, so there is no foreign
	// key: deleting an account that is still in use is refused by the handler,
	// in words that say what to do instead, rather than by the driver.
	AccountID uint `json:"account_id" gorm:"index;not null"`
	// Domains is a JSON array of names: the first is the certificate's subject,
	// the rest are subject alternative names. A name may start with "*.".
	Domains string `json:"domains" gorm:"type:text;not null"`
	// KeyType is one of the key types acme.KeyTypes lists.
	KeyType string `json:"key_type" gorm:"size:16;not null;default:'ec256'"`
	// OutputDir is the subdirectory of acme.output_dir the files are written
	// to. Empty means one named after the certificate.
	OutputDir string `json:"output_dir" gorm:"size:128"`
	// AutoRenew has the daily check renew this certificate before it expires.
	AutoRenew bool `json:"auto_renew"`
	// NotifyOnIssue announces a renewal the way a DDNS change is announced; a
	// failure is always announced.
	NotifyOnIssue bool `json:"notify_on_issue"`
	// LastIssuedAt is when the files on disk were written, which is what the
	// countdown in the UI is measured from.
	LastIssuedAt *time.Time `json:"last_issued_at"`
	NotAfter     *time.Time `json:"not_after" gorm:"index"`
	LastStatus   string     `json:"last_status" gorm:"size:16"`
	LastMessage  string     `json:"last_message" gorm:"size:512"`
	FailCount    int        `json:"fail_count" gorm:"default:0"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName specifies the table name
func (Certificate) TableName() string { return "certificates" }

// CertificateNames decodes the stored domain list of a certificate.
func CertificateNames(encoded string) ([]string, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(encoded), &names); err != nil {
		return nil, fmt.Errorf("decode certificate domains: %w", err)
	}
	return names, nil
}

// EncodeCertificateNames is the storage form of a certificate's domain list.
func EncodeCertificateNames(names []string) (string, error) {
	encoded, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("encode certificate domains: %w", err)
	}
	return string(encoded), nil
}

// ACMEAccount is this deployment's registration at one ACME directory.
//
// The key is the account: the CA identifies it by that key alone, so it is
// stored encrypted exactly like the Tencent Cloud credentials, and losing it
// means registering again, as a new account.
type ACMEAccount struct {
	ID         uint      `json:"id" gorm:"primarykey"`
	Directory  string    `json:"directory" gorm:"size:255;not null;uniqueIndex"`
	Email      string    `json:"email" gorm:"size:255"`
	Key        string    `json:"-" gorm:"size:2048;not null"`
	KeyHint    string    `json:"key_hint" gorm:"size:32"`
	AccountURL string    `json:"account_url" gorm:"size:512"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (ACMEAccount) TableName() string { return "acme_accounts" }

// APIKeyCovers reports whether a key scoped to scope may touch domain.
//
// The scope is either a single name - compared case-insensitively, because the
// same zone can be written either way - or the wildcard, which covers the whole
// account. Every public API call goes through here, which is what keeps a key
// bound to one domain from reaching another domain of the same account.
func APIKeyCovers(scope, domain string) bool {
	return scope == APIKeyWildcard || strings.EqualFold(scope, domain)
}
