package ddns

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ddns-by-dnspod/internal/config"
	"ddns-by-dnspod/internal/ipdetect"
	"ddns-by-dnspod/internal/model"
	"ddns-by-dnspod/internal/notifier"
)

// A host whose address moved takes every record with it. Announcing each one
// separately is how a person learns to stop reading the alerts, so a run is one
// message - and the single-record case still reads exactly as it did.

func changedRow(sub, oldIP, newIP string) *Result {
	return &Result{
		Domain:     "example.com",
		SubDomain:  sub,
		RecordType: "A",
		OldIP:      oldIP,
		NewIP:      newIP,
		Status:     model.StatusOK,
		Message:    oldIP + " -> " + newIP,
		Changed:    true,
		notify:     true,
	}
}

func failedRow(sub, ip, reason string) *Result {
	return &Result{
		Domain:     "example.com",
		SubDomain:  sub,
		RecordType: "A",
		OldIP:      ip,
		Status:     model.StatusFAIL,
		Message:    reason,
		notify:     true,
	}
}

func TestBuildRunMessageKeepsTheSingleRecordWording(t *testing.T) {
	msg, ok := buildRunMessage([]*Result{changedRow("home", "1.2.3.4", "5.6.7.8")})
	if !ok {
		t.Fatal("a changed record must be announced")
	}
	if msg.Title != "DDNS update succeeded" {
		t.Fatalf("title = %q, want the wording a single change always had", msg.Title)
	}
	if msg.Content != "home.example.com (A) 1.2.3.4 -> 5.6.7.8" {
		t.Fatalf("content = %q", msg.Content)
	}
	if msg.Status != model.StatusOK {
		t.Fatalf("status = %q, want OK", msg.Status)
	}
	if len(msg.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(msg.Records))
	}
}

// The other half of the single-record wording: one failure keeps the wording it
// always had, without a count.
func TestBuildRunMessageKeepsTheFailureWordingForOneRow(t *testing.T) {
	msg, ok := buildRunMessage([]*Result{failedRow("home", "1.2.3.4", "the record could not be updated")})
	if !ok {
		t.Fatal("a failed record must be announced")
	}
	if msg.Title != "DDNS update failed" {
		t.Fatalf("title = %q, want the wording a single failure always had", msg.Title)
	}
	if msg.Status != model.StatusFAIL {
		t.Fatalf("status = %q, want FAIL", msg.Status)
	}
	if len(msg.Records) != 1 || msg.Records[0].Status != model.StatusFAIL {
		t.Fatalf("records = %+v, want the one failed row", msg.Records)
	}
}

func TestBuildRunMessageCollapsesARunIntoOneMessage(t *testing.T) {
	rows := []*Result{
		changedRow("a", "1.1.1.1", "2.2.2.2"),
		changedRow("b", "1.1.1.1", "2.2.2.2"),
		changedRow("c", "1.1.1.1", "2.2.2.2"),
	}

	msg, ok := buildRunMessage(rows)
	if !ok {
		t.Fatal("three changes must be announced")
	}
	if msg.Title != "DDNS update: 3 records updated" {
		t.Fatalf("title = %q", msg.Title)
	}
	if lines := strings.Count(msg.Content, "\n") + 1; lines != 3 {
		t.Fatalf("content holds %d lines, want one per record:\n%s", lines, msg.Content)
	}
	for _, sub := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		if !strings.Contains(msg.Content, sub) {
			t.Fatalf("content is missing %s: %s", sub, msg.Content)
		}
	}
	if len(msg.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(msg.Records))
	}
	// The scalars mirror the first row, so a template written for one record
	// still renders something true instead of empty strings.
	if msg.SubDomain != "a" || msg.OldIP != "1.1.1.1" || msg.NewIP != "2.2.2.2" {
		t.Fatalf("scalars = %q %q %q, want the first record", msg.SubDomain, msg.OldIP, msg.NewIP)
	}
}

func TestBuildRunMessageReportsAFailureAmongChanges(t *testing.T) {
	rows := []*Result{
		changedRow("a", "1.1.1.1", "2.2.2.2"),
		changedRow("b", "1.1.1.1", "2.2.2.2"),
		failedRow("c", "1.1.1.1", "the record could not be updated"),
	}

	msg, ok := buildRunMessage(rows)
	if !ok {
		t.Fatal("a failure must be announced")
	}
	if msg.Title != "DDNS update: 2 updated, 1 failed" {
		t.Fatalf("title = %q", msg.Title)
	}
	// One failure in the run makes the run a failure: that is the half someone
	// has to act on, and the per-record statuses say which row it was.
	if msg.Status != model.StatusFAIL {
		t.Fatalf("status = %q, want FAIL", msg.Status)
	}
}

func TestBuildRunMessageSkipsARunWithNothingToSay(t *testing.T) {
	quiet := changedRow("a", "1.1.1.1", "1.1.1.1")
	quiet.notify = false

	if _, ok := buildRunMessage([]*Result{quiet}); ok {
		t.Fatal("a record that asked for no announcement must not produce a message")
	}
	if _, ok := buildRunMessage(nil); ok {
		t.Fatal("an empty run must not produce a message")
	}
}

// The end-to-end property behind all of the above: three changes, one request.
func TestNotifyRunSendsOneRequestForManyChanges(t *testing.T) {
	st := newTestStore(t)

	var bodies []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	cfg, err := json.Marshal(map[string]string{"url": hook.URL})
	if err != nil {
		t.Fatalf("marshal channel config: %v", err)
	}
	channel := model.NotifyChannel{Name: "hook", Type: model.ChannelWebhook, Enabled: true, Config: string(cfg)}
	if err := st.DB().Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	e := NewExecutor(st,
		ipdetect.NewWithJSON(nil, nil, nil, nil, 1),
		notifier.New(nil, notifier.Config{AllowPrivate: []string{"127.0.0.0/8"}}),
		config.DNSPodConfig{}, config.SchedulerConfig{}, config.CacheConfig{})

	e.notifyRun([]*Result{
		changedRow("a", "1.1.1.1", "2.2.2.2"),
		changedRow("b", "1.1.1.1", "2.2.2.2"),
		changedRow("c", "1.1.1.1", "2.2.2.2"),
	})

	if len(bodies) != 1 {
		t.Fatalf("%d requests were sent, want exactly one per run", len(bodies))
	}
	for _, sub := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		if !strings.Contains(bodies[0], sub) {
			t.Fatalf("the one message is missing %s: %s", sub, bodies[0])
		}
	}
	if !strings.Contains(bodies[0], `"records"`) {
		t.Fatalf("the payload does not carry the per-record list: %s", bodies[0])
	}
}
