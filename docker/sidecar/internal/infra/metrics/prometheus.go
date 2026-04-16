package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Prometheus struct {
	httpRequests *prometheus.CounterVec
	httpDuration prometheus.Histogram
}

func NewPrometheus(service string) *Prometheus {
	return &Prometheus{
		httpRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: service + "_http_requests_total",
				Help: "Total number of HTTP requests proxied",
			},
			[]string{"code"},
		),
		httpDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    service + "_http_response_duration_seconds",
				Help:    "Histogram of response durations for proxied requests",
				Buckets: prometheus.DefBuckets,
			},
		),
	}
}

func (p *Prometheus) ObserveDuration(seconds float64) {
	p.httpDuration.Observe(seconds)
}

func (p *Prometheus) IncTotalRequests(code int) {
	p.httpRequests.WithLabelValues(fmt.Sprintf("%d", code)).Inc()
}
