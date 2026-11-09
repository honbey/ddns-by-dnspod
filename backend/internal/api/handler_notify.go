package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
)

// channelInput is the payload for creating or updating a channel. Config arrives as a JSON object.
type channelInput struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Enabled *bool           `json:"enabled"`
	Config  json.RawMessage `json:"config"`
}

// listChannels lists channels, with the stored password removed from each config
func (s *Server) listChannels(c *gin.Context) {
	var channels []model.NotifyChannel
	params, total, fine := pageRows(c, s.store.DB(), &channels)
	if !fine {
		return
	}
	// Masked after the read rather than in the query: the stored password is
	// never one of the columns the page carries back.
	for i := range channels {
		channels[i] = maskedChannel(channels[i])
	}
	paged(c, channels, total, params)
}

// createChannel creates a channel
func (s *Server) createChannel(c *gin.Context) {
	var input channelInput
	if !bindJSON(c, &input) {
		return
	}
	if input.Name == "" {
		badRequest(c, "name is required")
		return
	}
	if input.Type != model.ChannelNtfy && input.Type != model.ChannelWebhook {
		badRequest(c, "type must be ntfy or webhook")
		return
	}

	// No previous config to carry a password over from on creation
	storedConfig, err := s.mergeChannelConfig(input.Type, "", input.Config)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	ch := model.NotifyChannel{
		Name:    input.Name,
		Type:    input.Type,
		Enabled: enabled,
		Config:  storedConfig,
	}
	if err := s.store.DB().Create(&ch).Error; err != nil {
		serverError(c, err)
		return
	}

	slog.Info("notification channel created", "channel", ch.Name,
		"caller", middleware.CurrentUser(c), "rid", middleware.RequestIDFrom(c))
	created(c, maskedChannel(ch))
}

// updateChannel updates a channel
func (s *Server) updateChannel(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var ch model.NotifyChannel
	if err := s.store.DB().First(&ch, id).Error; err != nil {
		notFound(c, "channel not found")
		return
	}

	var input channelInput
	if !bindJSON(c, &input) {
		return
	}

	if input.Name != "" {
		ch.Name = input.Name
	}
	if input.Type != "" {
		if input.Type != model.ChannelNtfy && input.Type != model.ChannelWebhook {
			badRequest(c, "type must be ntfy or webhook")
			return
		}
		// The stored config describes the old type and cannot be read as the new
		// one, so switching type is only accepted together with a matching config
		if input.Type != ch.Type {
			if len(input.Config) == 0 {
				badRequest(c, "changing the channel type requires a config for the new type")
				return
			}
			ch.Config = ""
		}
		ch.Type = input.Type
	}
	if input.Enabled != nil {
		ch.Enabled = *input.Enabled
	}
	if len(input.Config) > 0 {
		storedConfig, err := s.mergeChannelConfig(ch.Type, ch.Config, input.Config)
		if err != nil {
			badRequest(c, err.Error())
			return
		}
		ch.Config = storedConfig
	}

	if err := s.store.DB().Save(&ch).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, maskedChannel(ch))
}

// deleteChannel deletes a channel
func (s *Server) deleteChannel(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}
	tx := s.store.DB().Delete(&model.NotifyChannel{}, id)
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	// GORM reports no error when the row is already gone, so the affected row
	// count is what tells us whether the channel ever existed
	if tx.RowsAffected == 0 {
		notFound(c, "channel not found")
		return
	}
	ok(c, gin.H{"deleted": id})
}

// testChannel sends a test message to verify the channel configuration
func (s *Server) testChannel(c *gin.Context) {
	id := parseID(c)
	if id == 0 {
		return
	}

	var ch model.NotifyChannel
	if err := s.store.DB().First(&ch, id).Error; err != nil {
		notFound(c, "channel not found")
		return
	}

	msg := notifier.Message{
		Title:   "DDNS test notification",
		Content: "This is a test message from ddns-by-dnspod; receiving it means the channel is configured correctly.",
		Status:  "TEST",
		Time:    time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := s.sender.Dispatch(ch.Type, ch.Config, msg); err != nil {
		// The reason says where the message was sent and what came back, which
		// is worth keeping in the log but not handing to the browser
		slog.Error("test notification failed", "channel", id, "rid", middleware.RequestIDFrom(c), "error", err)
		badRequest(c, "send failed, check the channel configuration")
		return
	}
	ok(c, gin.H{"sent": true})
}

// mergeChannelConfig encrypts a newly supplied ntfy password.
//
// An empty password keeps the previously stored one, mirroring the account
// update semantics where an empty secret means "unchanged". Without it, an edit
// that only touches, say, the priority would silently wipe the password, because
// the stored config is replaced wholesale.
//
// The old value is copied verbatim rather than encrypted again: it is already
// the stored ciphertext, and re-encrypting it would be encrypting twice.
func (s *Server) mergeChannelConfig(chType, oldRaw string, newRaw json.RawMessage) (string, error) {
	if chType != model.ChannelNtfy {
		return string(newRaw), nil
	}
	// Reported as a missing config instead of the "unexpected end of JSON input"
	// an empty payload would produce
	if len(newRaw) == 0 {
		return "", fmt.Errorf("config is required for an ntfy channel")
	}

	var newCfg notifier.NtfyConfig
	if err := json.Unmarshal(newRaw, &newCfg); err != nil {
		return "", fmt.Errorf("invalid channel config: %w", err)
	}

	if newCfg.Password == "" {
		var oldCfg notifier.NtfyConfig
		if err := json.Unmarshal([]byte(oldRaw), &oldCfg); err == nil {
			newCfg.Password = oldCfg.Password
		}
	} else {
		encrypted, err := s.store.Encryptor().Encrypt(newCfg.Password)
		if err != nil {
			return "", err
		}
		newCfg.Password = encrypted
	}

	// Merge rather than round-trip the struct. Marshalling NtfyConfig would
	// drop any key the struct does not declare, so a row written by a newer
	// version of the app would silently lose its newer settings the first time
	// an older client edits the channel.
	//
	// The stored config is the base and the incoming one is layered on top, so
	// keys unknown to this build survive. The password is written separately
	// because it is the one field handled above rather than copied verbatim.
	merged := map[string]any{}
	if oldRaw != "" {
		if err := json.Unmarshal([]byte(oldRaw), &merged); err != nil {
			// A malformed stored config cannot be merged into; starting fresh
			// keeps the update usable instead of failing the edit outright.
			merged = map[string]any{}
		}
	}

	incoming := map[string]any{}
	if len(newRaw) > 0 {
		if err := json.Unmarshal(newRaw, &incoming); err != nil {
			return "", fmt.Errorf("invalid channel config: %w", err)
		}
		for k, v := range incoming {
			merged[k] = v
		}
	}
	merged["password"] = newCfg.Password

	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// maskedChannel returns a copy of the channel whose config no longer carries the
// password. Handing the ciphertext out serves no purpose, and it stops a client
// from sending it back and having it encrypted a second time.
//
// The password is removed from a generic map instead of round-tripping the
// struct, so keys unknown to this build survive the response untouched - the
// stored row keeps them either way, but echoing the row verbatim would make an
// edit from an older client look like it lost them.
func maskedChannel(ch model.NotifyChannel) model.NotifyChannel {
	if ch.Type != model.ChannelNtfy || ch.Config == "" {
		return ch
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return ch
	}
	if _, ok := cfg["password"]; !ok {
		return ch
	}
	delete(cfg, "password")
	if encoded, err := json.Marshal(cfg); err == nil {
		ch.Config = string(encoded)
	}
	return ch
}
