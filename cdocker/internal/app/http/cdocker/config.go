package cdocker

var (
	planeEnvs = []string{
		"APP_TERMINATE_TIMEOUT=5s",
		"APP_SHUTDOWN_TIMEOUT=2s",
		"PLANE_URL=:8080",
		"PLANE_READ_TIMEOUT=1s",
		"PLANE_READ_HEADER_TIMEOUT=1s",
	}
)

const (
	// defaultPlaneConfig is the default configuration for control plane.
	defaultPlaneConfig = `
app:
  terminate_timeout: 5s
  shutdown_timeout: 2s
plane:
  url: :8080
  read_timeout: 1s
  read_header_timeout: 1s
`

	// defaultSidecarConfig is the default configuration for sidecar.
	defaultSidecarConfig = `
app:
  terminate_timeout: 5s
  shutdown_timeout: 2s
sidecar:
  port: 8080
  read_timeout: 1s
  read_header_timeout: 1s
client:
  http:
    dial_timeout: 5s
    dial_keep_alive: 30s
    max_idle_conns: 100
    idle_conn_timeout: 90s
    tls_handshake_timeout: 10s
    expect_continue_timeout: 1s
    timeout: 30s
  retry:
    retry_max: 4
    retry_wait_min: 200ms
    retry_wait_max: 2s
    backoff_type: exponential
  circuit_breaker:
    max_half_open_requests: 5
    interval: 60s
    timeout: 30s
    min_requests: 10
    consecutive_failures: 5
    failure_rate: 0.6
ratelimiter:
  name: sidecar
  max_hits: 10
  window: 1m
`
)
