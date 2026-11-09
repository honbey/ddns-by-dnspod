package notifier

import (
	"errors"
	"strings"
	"testing"
)

func TestUnmarshalConfigEmptyStringIsNoop(t *testing.T) {
	var cfg NtfyConfig
	if err := unmarshalConfig("   ", &cfg); err != nil {
		t.Fatalf("empty config should not error: %v", err)
	}
	if cfg.Server != "" {
		t.Fatalf("want zero value, got %+v", cfg)
	}
}

func TestUnmarshalConfigParsesJSON(t *testing.T) {
	var cfg NtfyConfig
	raw := `{"server":"https://ntfy.example.com","topic":"ddns","priority":4}`
	if err := unmarshalConfig(raw, &cfg); err != nil {
		t.Fatalf("unmarshalConfig: %v", err)
	}
	if cfg.Server != "https://ntfy.example.com" || cfg.Topic != "ddns" || cfg.Priority != 4 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestUnmarshalConfigRejectsBrokenJSON(t *testing.T) {
	var cfg NtfyConfig
	if err := unmarshalConfig("{not json", &cfg); err == nil {
		t.Fatal("broken JSON must be rejected")
	}
}

func TestRenderBodyFallsBackToJSON(t *testing.T) {
	msg := Message{
		Title:      "DDNS update succeeded",
		Content:    "a.example.com 1.2.3.4 -> 5.6.7.8",
		Status:     "OK",
		Domain:     "example.com",
		SubDomain:  "a",
		RecordType: "A",
		OldIP:      "1.2.3.4",
		NewIP:      "5.6.7.8",
	}

	body, err := renderBody("", msg)
	if err != nil {
		t.Fatalf("renderBody: %v", err)
	}
	for _, want := range []string{`"title"`, `"old_ip":"1.2.3.4"`, `"new_ip":"5.6.7.8"`, `"domain":"example.com"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("fallback body %q missing %s", body, want)
		}
	}
}

func TestRenderBodyUsesTemplate(t *testing.T) {
	msg := Message{Domain: "example.com", SubDomain: "a", OldIP: "1.1.1.1", NewIP: "2.2.2.2", Status: "OK"}

	body, err := renderBody("{{.SubDomain}}.{{.Domain}}: {{.OldIP}} -> {{.NewIP}} ({{.Status}})", msg)
	if err != nil {
		t.Fatalf("renderBody: %v", err)
	}
	if body != "a.example.com: 1.1.1.1 -> 2.2.2.2 (OK)" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestRenderBodyRejectsBrokenTemplate(t *testing.T) {
	if _, err := renderBody("{{ .Missing", Message{}); err == nil {
		t.Fatal("broken template must be rejected")
	}
}

func TestSendNtfyRequiresServerAndTopic(t *testing.T) {
	s := New(nil, Config{})

	if err := s.SendNtfy(NtfyConfig{}, Message{}); err == nil {
		t.Error("missing server must be rejected")
	}
	if err := s.SendNtfy(NtfyConfig{Server: "https://ntfy.example.com"}, Message{}); err == nil {
		t.Error("missing topic must be rejected")
	}
}

func TestSendWebhookRequiresURL(t *testing.T) {
	s := New(nil, Config{})
	if err := s.SendWebhook(WebhookConfig{}, Message{}); err == nil {
		t.Error("missing url must be rejected")
	}
}

// Without a decryptor the value is used as is, which keeps plain text configs
// working. With one, a value that will not decrypt is an error: it is
// ciphertext, and sending it as the password would hand the stored secret to
// the remote server.
func TestResolveSecretKeepsPlainTextWithoutADecryptor(t *testing.T) {
	s := New(nil, Config{})

	got, err := s.resolveSecret("plain-password")
	if err != nil {
		t.Fatalf("resolveSecret: %v", err)
	}
	if got != "plain-password" {
		t.Fatalf("want plain value, got %q", got)
	}
}

func TestResolveSecretRefusesAnUndecryptableValue(t *testing.T) {
	s := New(failingDecryptor{}, Config{})

	if _, err := s.resolveSecret("stored-ciphertext"); err == nil {
		t.Fatal("a value that cannot be decrypted must be refused, not sent as is")
	}
}

type failingDecryptor struct{}

func (failingDecryptor) Decrypt(string) (string, error) {
	return "", errors.New("decrypt failed")
}
