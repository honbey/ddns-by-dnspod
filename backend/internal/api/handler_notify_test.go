package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
)

// createNtfyChannel posts an ntfy channel and returns its stored id
func createNtfyChannel(t *testing.T, srv *Server, cfg *config.Config, password string) uint {
	t.Helper()

	rec := do(t, srv, cfg, http.MethodPost, "/api/notify/channels", gin.H{
		"name":    "ntfy",
		"type":    model.ChannelNtfy,
		"enabled": true,
		"config": gin.H{
			"server":   "https://ntfy.example.com",
			"topic":    "ddns",
			"username": "user",
			"password": password,
			"priority": 3,
			"tags":     "ddns",
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create channel: want 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	var created struct {
		Data model.NotifyChannel `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return created.Data.ID
}

// storedNtfyPassword reads the password field straight out of the stored config
func storedNtfyPassword(t *testing.T, srv *Server, id uint) string {

	t.Helper()

	var ch model.NotifyChannel
	if err := srv.store.DB().First(&ch, id).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	var cfg notifier.NtfyConfig
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		t.Fatalf("decode stored config: %v", err)
	}
	return cfg.Password
}

// channelConfigs reads the configs as the API returns them
func channelConfigs(t *testing.T, srv *Server, cfg *config.Config) []notifier.NtfyConfig {
	t.Helper()

	rec := do(t, srv, cfg, http.MethodGet, "/api/notify/channels", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list channels: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	listed := decodeList[model.NotifyChannel](t, rec.Body.Bytes())

	configs := make([]notifier.NtfyConfig, 0, len(listed))
	for _, ch := range listed {
		var parsed notifier.NtfyConfig
		if err := json.Unmarshal([]byte(ch.Config), &parsed); err != nil {
			t.Fatalf("decode channel config: %v", err)
		}
		configs = append(configs, parsed)
	}
	return configs
}

// Editing a channel without restating the password used to wipe it, because the
// stored config is replaced wholesale. The frontend relies on "leave it blank to
// keep it", so this is the behaviour to pin.
func TestUpdateChannelKeepsTheStoredPassword(t *testing.T) {
	srv, cfg := newTestServer(t)
	id := createNtfyChannel(t, srv, cfg, "first-password")

	rec := do(t, srv, cfg, http.MethodPut, "/api/notify/channels/"+itoa(id), gin.H{
		"name": "ntfy",
		"type": model.ChannelNtfy,
		// No password: the stored one must survive an edit that only moves the priority
		"config": gin.H{
			"server":   "https://ntfy.example.com",
			"topic":    "ddns",
			"username": "user",
			"priority": 5,
			"tags":     "ddns",
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update channel: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	stored := storedNtfyPassword(t, srv, id)
	if stored == "" {
		t.Fatal("the stored password must survive an edit that does not restate it")
	}
	plain, err := srv.store.Encryptor().Decrypt(stored)
	if err != nil {
		t.Fatalf("the surviving password must still decrypt: %v", err)
	}
	if plain != "first-password" {
		t.Fatalf("password = %q, want first-password", plain)
	}
}

// A password that is supplied replaces the old one, and is stored encrypted
func TestUpdateChannelReplacesThePasswordWhenGiven(t *testing.T) {
	srv, cfg := newTestServer(t)
	id := createNtfyChannel(t, srv, cfg, "first-password")

	rec := do(t, srv, cfg, http.MethodPut, "/api/notify/channels/"+itoa(id), gin.H{
		"name": "ntfy",
		"type": model.ChannelNtfy,
		"config": gin.H{
			"server":   "https://ntfy.example.com",
			"topic":    "ddns",
			"password": "second-password",
			"priority": 3,
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update channel: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	plain, err := srv.store.Encryptor().Decrypt(storedNtfyPassword(t, srv, id))
	if err != nil {
		t.Fatalf("the new password must be stored encrypted: %v", err)
	}
	if plain != "second-password" {
		t.Fatalf("password = %q, want second-password", plain)
	}
}

// The stored ciphertext has no business being handed out, and sending it back
// would have it encrypted a second time
func TestChannelResponsesNeverCarryThePassword(t *testing.T) {
	srv, cfg := newTestServer(t)
	createNtfyChannel(t, srv, cfg, "a-password")

	for i, parsed := range channelConfigs(t, srv, cfg) {
		if parsed.Password != "" {
			t.Fatalf("channel %d exposes the password: %q", i, parsed.Password)
		}
	}
}

// storedRawConfig reads the config as the literal JSON stored in the row
func storedRawConfig(t *testing.T, srv *Server, id uint) map[string]any {
	t.Helper()

	var ch model.NotifyChannel
	if err := srv.store.DB().First(&ch, id).Error; err != nil {
		t.Fatalf("load channel: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		t.Fatalf("decode stored config: %v", err)
	}
	return cfg
}

// A config row written by a build that knows more fields than this one must
// survive an edit untouched. Round-tripping the NtfyConfig struct used to drop
// such keys, silently erasing newer settings the first time an older client
// edited the channel.
func TestUpdateChannelPreservesUnknownConfigFields(t *testing.T) {
	srv, cfg := newTestServer(t)

	// Seed a row directly, the way a newer build would have written it
	ch := model.NotifyChannel{
		Name:    "ntfy",
		Type:    model.ChannelNtfy,
		Enabled: true,
		Config:  `{"server":"https://ntfy.example.com","topic":"ddns","priority":3,"future_setting":true}`,
	}
	if err := srv.store.DB().Create(&ch).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	rec := do(t, srv, cfg, http.MethodPut, "/api/notify/channels/"+itoa(ch.ID), gin.H{
		"name": "ntfy",
		"type": model.ChannelNtfy,
		"config": gin.H{
			"server":   "https://ntfy.example.com",
			"topic":    "ddns",
			"priority": 5,
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update channel: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	stored := storedRawConfig(t, srv, ch.ID)
	if v, ok := stored["future_setting"].(bool); !ok || !v {
		t.Fatalf("future_setting = %v, want true: the unknown key must survive an edit", stored["future_setting"])
	}
	if stored["priority"] != float64(5) {
		t.Fatalf("priority = %v, want 5", stored["priority"])
	}
	if stored["password"] != "" {
		t.Fatalf("password = %v, want an empty string when the edit does not restate it", stored["password"])
	}
}

// The masked response keeps the same guarantee: masking must not strip keys it
// does not know about
func TestChannelResponsesPreserveUnknownConfigFields(t *testing.T) {
	srv, cfg := newTestServer(t)

	ch := model.NotifyChannel{
		Name:    "ntfy",
		Type:    model.ChannelNtfy,
		Enabled: true,
		Config:  `{"server":"https://ntfy.example.com","topic":"ddns","password":"secret","future_setting":true}`,
	}
	if err := srv.store.DB().Create(&ch).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	rec := do(t, srv, cfg, http.MethodGet, "/api/notify/channels", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list channels: want 200, got %d", rec.Code)
	}
	listed := decodeList[model.NotifyChannel](t, rec.Body.Bytes())

	found := false
	for _, got := range listed {
		if got.ID != ch.ID {
			continue
		}
		found = true
		if strings.Contains(got.Config, "secret") {
			t.Fatal("the response still carries the password")
		}
		if !strings.Contains(got.Config, "future_setting") {
			t.Fatalf("the response dropped the unknown key: %s", got.Config)
		}
	}
	if !found {
		t.Fatal("the seeded channel is missing from the list")
	}
}
