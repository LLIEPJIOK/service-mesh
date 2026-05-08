package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Recorder struct {
	registry *prometheus.Registry

	requestsTotal       *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	requestErrors       *prometheus.CounterVec
	retryAttempts       *prometheus.CounterVec
	circuitBreakerState *prometheus.GaugeVec
	endpointsReady      *prometheus.GaugeVec
}

func NewRecorder() *Recorder {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)

	recorder := &Recorder{
		registry: registry,
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mesh_requests_total",
				Help: "Total proxied requests grouped by service, status code and direction.",
			},
			[]string{"service", "status_code", "direction"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "mesh_request_duration_seconds",
				Help:    "Request duration in seconds grouped by service and direction.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "direction"},
		),
		requestErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mesh_request_errors_total",
				Help: "Total proxy errors grouped by service and normalized error type.",
			},
			[]string{"service", "error_type"},
		),
		retryAttempts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mesh_retry_attempts_total",
				Help: "Total retry attempts grouped by service.",
			},
			[]string{"service"},
		),
		circuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mesh_circuit_breaker_state",
				Help: "Current circuit breaker state by service (0 closed, 1 open, 2 half-open).",
			},
			[]string{"service"},
		),
		endpointsReady: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "mesh_endpoints_ready",
				Help: "Number of ready endpoints in discovery cache grouped by service.",
			},
			[]string{"service"},
		),
	}

	registry.MustRegister(
		recorder.requestsTotal,
		recorder.requestDuration,
		recorder.requestErrors,
		recorder.retryAttempts,
		recorder.circuitBreakerState,
		recorder.endpointsReady,
	)

	return recorder
}

func (r *Recorder) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (r *Recorder) ObserveRequest(service string, statusCode string, direction string, duration time.Duration) {
	r.requestsTotal.WithLabelValues(normalizeService(service), statusCode, normalizeDirection(direction)).Inc()
	r.requestDuration.WithLabelValues(normalizeService(service), normalizeDirection(direction)).Observe(duration.Seconds())
}

func (r *Recorder) ObserveError(service string, errorType string) {
	r.requestErrors.WithLabelValues(normalizeService(service), normalizeErrorType(errorType)).Inc()
}

func (r *Recorder) IncRetry(service string) {
	r.retryAttempts.WithLabelValues(normalizeService(service)).Inc()
}

func (r *Recorder) SetCircuitBreakerState(service string, state int) {
	r.circuitBreakerState.WithLabelValues(normalizeService(service)).Set(float64(state))
}

func (r *Recorder) SetEndpointsReady(service string, ready int) {
	r.endpointsReady.WithLabelValues(normalizeService(service)).Set(float64(ready))
}

func normalizeService(service string) string {
	if service == "" {
		return "external"
	}

	return service
}

func normalizeDirection(direction string) string {
	switch direction {
	case "inbound", "outbound":
		return direction
	default:
		return "outbound"
	}
}

func normalizeErrorType(errorType string) string {
	if errorType == "" {
		return "unknown"
	}

	return errorType
}
