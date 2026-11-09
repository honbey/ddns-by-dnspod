// Package metrics is the single registry this process reports to.
//
// One registry, not the global default: what a scrape sees is exactly what
// this package registered, and a test can read it in isolation.
//
// Labels are chosen for cardinality: a route pattern (`/api/todos/:id`), never
// a concrete path. A metric whose labels grow with the data is a metric that
// quietly grows the Prometheus bill until someone notices.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds every metric of this process, Go and process collectors
// included - the stock runtime dashboards work without extra scrape config.
var Registry = prometheus.NewRegistry()

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Requests served, by method, route pattern and status class.",
		},
		[]string{"method", "route", "status"},
	)

	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "How long a request takes, by method and route pattern.",
			// The tail matters: without buckets past the server write timeout,
			// every slow request lands in +Inf and no SLO can be alerted on.
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		},
		[]string{"method", "route"},
	)

	requestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Requests being handled right now.",
		},
	)

	// The host address timeline is a side record: a write that fails there does
	// not stop the DNS update, so the only way to notice is a counter.
	publicIPChangeErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "public_ip_change_errors_total",
			Help: "Times the host address timeline rejected a write; the run goes on.",
		},
	)

	authRejections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_rejections_total",
			Help: "Requests refused by the bearer check, by reason.",
		},
		[]string{"reason"},
	)
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		requestsTotal,
		requestDuration,
		requestsInFlight,
		authRejections,
		publicIPChangeErrors,
	)
}

// Handler serves the scrape endpoint. The timeout bounds a slow consumer: the
// scrape endpoint is unauthenticated and should not hold a connection open.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Timeout: 10 * time.Second})
}

// ObserveRequest records one finished request. route is the matched pattern -
// c.FullPath() in gin - or "unmatched"; a concrete path would turn every id
// into a new time series.
func ObserveRequest(method, route string, status int, elapsed time.Duration) {
	requestsTotal.WithLabelValues(method, route, statusLabel(status)).Inc()
	requestDuration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}

// IncInFlight and DecInFlight bracket a request being handled.
func IncInFlight() { requestsInFlight.Inc() }
func DecInFlight() { requestsInFlight.Dec() }

// IncAuthRejection counts a refused token by reason: "invalid" for one that
// did not verify, "rate_limited" for a source that spent its budget.
func IncAuthRejection(reason string) {
	authRejections.WithLabelValues(reason).Inc()
}

// IncPublicIPChangeError counts an address-timeline write that failed. The run
// continues - the address did change - so this is the only trace of it.
func IncPublicIPChangeError() { publicIPChangeErrors.Inc() }

// statusLabel keeps the label set bounded: every 4xx is one series, not one
// per code a client can invent.
func statusLabel(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
