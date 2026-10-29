package metrics

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// The label set is the whole reason this package exists: a status code per
// series multiplies with every code a client can invent, and a concrete path
// would turn every id into a series of its own. Both are asserted here rather
// than trusted to the comment.
func TestStatusLabelBuckets(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{200, "2xx"}, {204, "2xx"}, {299, "2xx"},
		{301, "3xx"}, {304, "3xx"},
		{400, "4xx"}, {404, "4xx"}, {429, "4xx"}, {499, "4xx"},
		{500, "5xx"}, {502, "5xx"}, {503, "5xx"},
		// Outside every range is a broken response, and 5xx is the bucket that
		// says so. 499 stays 4xx on purpose: it is the cancel case.
		{0, "5xx"}, {600, "5xx"},
	}
	for _, tc := range cases {
		if got := statusLabel(tc.status); got != tc.want {
			t.Errorf("statusLabel(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// Two requests to one route land in one series per status class, whatever the
// status: the counter's cardinality is bounded by the route table plus four.
func TestObserveRequestKeepsOneSeriesPerStatusClass(t *testing.T) {
	const route = "/test/observe/:id"

	ObserveRequest("GET", route, 200, 5*time.Millisecond)
	ObserveRequest("GET", route, 404, 7*time.Millisecond)

	family := gather(t, "http_requests_total")
	if got := seriesFor(family, route); got != 2 {
		t.Fatalf("route %s holds %d series, want 2 (2xx and 4xx)", route, got)
	}
}

// Routes are told apart and methods are too, which is what keeps a GET and a
// DELETE to the same pattern from averaging into one line.
func TestObserveRequestSeparatesMethods(t *testing.T) {
	const route = "/test/methods"

	ObserveRequest("GET", route, 200, time.Millisecond)
	ObserveRequest("DELETE", route, 204, time.Millisecond)

	family := gather(t, "http_requests_total")
	if got := seriesFor(family, route); got != 2 {
		t.Fatalf("route %s holds %d series, want 2 (one per method)", route, got)
	}
}

// The histogram is per route rather than per request: the same route observed
// twice is one series holding two samples.
func TestObserveRequestRecordsDuration(t *testing.T) {
	const route = "/test/duration"

	ObserveRequest("POST", route, 200, 3*time.Millisecond)
	ObserveRequest("POST", route, 200, 4*time.Millisecond)

	family := gather(t, "http_request_duration_seconds")
	for _, metric := range family.GetMetric() {
		if labelValue(metric, "route") != route || labelValue(metric, "method") != "POST" {
			continue
		}
		if count := metric.GetHistogram().GetSampleCount(); count != 2 {
			t.Fatalf("sample count = %d, want 2", count)
		}
		return
	}
	t.Fatalf("no duration series was recorded for %s", route)
}

// An address-timeline write that failed is a log line and nothing else - the
// run goes on - so it needs a counter before anyone can alert on it.
func TestIncPublicIPChangeError(t *testing.T) {
	IncPublicIPChangeError()

	family := gather(t, "public_ip_change_errors_total")
	if family.GetMetric()[0].GetCounter().GetValue() <= 0 {
		t.Fatal("the failed write was not counted")
	}
}

// A refusal is counted by reason, so a rising rate can be read without the log.
func TestIncAuthRejection(t *testing.T) {
	IncAuthRejection("invalid")

	family := gather(t, "auth_rejections_total")
	for _, metric := range family.GetMetric() {
		if labelValue(metric, "reason") == "invalid" && metric.GetCounter().GetValue() > 0 {
			return
		}
	}
	t.Fatal("the invalid reason was not counted")
}

func gather(t *testing.T, name string) *dto.MetricFamily {
	t.Helper()

	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("%s is missing from the registry", name)
	return nil
}

func seriesFor(family *dto.MetricFamily, route string) int {
	count := 0
	for _, metric := range family.GetMetric() {
		if labelValue(metric, "route") == route {
			count++
		}
	}
	return count
}

func labelValue(metric *dto.Metric, name string) string {
	for _, pair := range metric.GetLabel() {
		if strings.EqualFold(pair.GetName(), name) {
			return pair.GetValue()
		}
	}
	return ""
}
