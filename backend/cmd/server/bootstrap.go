package main

import (
	"encoding/json"
	"log/slog"
	"os"

	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
	"ddns-by-dnspod/internal/store"
)

// bootstrap imports credentials and notification settings from env vars on first run.
// It skips once data exists, so later edits made in the UI are never overwritten.
func bootstrap(st *store.Store) {
	bootstrapAccount(st)
	bootstrapNotifyChannel(st)
}

// bootstrapAccount creates the default account from Tencent Cloud credential
// env vars, named the way the SDK names them.
func bootstrapAccount(st *store.Store) {
	var count int64
	if err := st.DB().Model(&model.Account{}).Count(&count).Error; err != nil {
		slog.Error("failed to count accounts", "error", err)
		return
	}
	if count > 0 {
		return
	}

	secretID := os.Getenv("TENCENTCLOUD_SECRET_ID")
	secretKey := os.Getenv("TENCENTCLOUD_SECRET_KEY")
	if secretID == "" || secretKey == "" {
		slog.Info("no Tencent Cloud credential env vars found, add an account in the UI")
		return
	}

	encrypted, err := st.Encryptor().Encrypt(secretKey)
	if err != nil {
		slog.Error("failed to encrypt the secret key", "error", err)
		return
	}

	account := model.Account{
		Name:      "default account",
		SecretID:  secretID,
		SecretKey: encrypted,
		KeyHint:   model.MaskSecret(secretKey),
		IsDefault: true,
	}
	if err := st.DB().Create(&account).Error; err != nil {
		slog.Error("failed to create the default account", "error", err)
		return
	}
	// The hint rather than the SecretId: it is what identifies the credential,
	// and the log line is not the place to keep the value itself
	slog.Info("created the default account from env vars", "key_hint", account.KeyHint)
}

// bootstrapNotifyChannel creates the default notification channel from NTFY_* env vars
func bootstrapNotifyChannel(st *store.Store) {
	var count int64
	if err := st.DB().Model(&model.NotifyChannel{}).Count(&count).Error; err != nil {
		slog.Error("failed to count notification channels", "error", err)
		return
	}
	if count > 0 {
		return
	}

	user := os.Getenv("NTFY_USER")
	pass := os.Getenv("NTFY_PASS")
	if user == "" && pass == "" {
		slog.Info("NTFY_USER/NTFY_PASS not set, skipping notification channel setup")
		return
	}

	server := firstNonEmpty(os.Getenv("NTFY_SERVER"), "https://ntfy.sh")
	topic := firstNonEmpty(os.Getenv("NTFY_TOPIC"), "ddns_by_dnspod")

	cfg := notifier.NtfyConfig{
		Server:   server,
		Topic:    topic,
		Username: user,
		Password: pass,
		Priority: 3,
		Tags:     "ddns,dnspod",
	}

	// The password is stored encrypted and decrypted by notifier at runtime
	if pass != "" {
		encrypted, err := st.Encryptor().Encrypt(pass)
		if err != nil {
			slog.Error("failed to encrypt the ntfy password", "error", err)
			return
		}
		cfg.Password = encrypted
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		slog.Error("failed to encode the ntfy config", "error", err)
		return
	}

	ch := model.NotifyChannel{
		Name:    "ntfy",
		Type:    model.ChannelNtfy,
		Enabled: true,
		Config:  string(encoded),
	}
	if err := st.DB().Create(&ch).Error; err != nil {
		slog.Error("failed to create the ntfy channel", "error", err)
		return
	}
	// The topic is left out on purpose: on a public ntfy server it is itself
	// everything needed to post to the channel
	slog.Info("created the ntfy channel from env vars", "server", server)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
