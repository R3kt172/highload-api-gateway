package observability

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Requests      *prometheus.CounterVec
	Duration      *prometheus.HistogramVec
	RateRejected  *prometheus.CounterVec
	UpstreamError *prometheus.CounterVec
	RedisDuration prometheus.Histogram
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "route", "status"}),
		Duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "HTTP request duration.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		RateRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_rate_limit_rejected_total",
			Help: "Requests rejected by role.",
		}, []string{"role"}),
		UpstreamError: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_upstream_errors_total",
			Help: "Upstream proxy errors.",
		}, []string{"route"}),
		RedisDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "gateway_redis_duration_seconds",
			Help:    "Redis limiter operation duration.",
			Buckets: prometheus.DefBuckets,
		}),
	}
	reg.MustRegister(m.Requests, m.Duration, m.RateRejected, m.UpstreamError, m.RedisDuration)
	return m
}

func (m *Metrics) ObserveRequest(method, route string, status int, elapsed time.Duration) {
	m.Requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.Duration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}
