package config

import "time"

type Config struct {
	HTTPClient     HTTPClient     `envPrefix:"HTTP_"`
	Retry          Retry          `envPrefix:"RETRY_"`
	CircuitBreaker CircuitBreaker `envPrefix:"CIRCUIT_BREAKER_"`
}

type HTTPClient struct {
	DialTimeout           time.Duration `env:"DIAL_TIMEOUT"            envDefault:"5s"`
	DialKeepAlive         time.Duration `env:"DIAL_KEEP_ALIVE"         envDefault:"30s"`
	MaxIdleConns          int           `env:"MAX_IDLE_CONNS"          envDefault:"100"`
	IdleConnTimeout       time.Duration `env:"IDLE_CONN_TIMEOUT"       envDefault:"90s"`
	TLSHandshakeTimeout   time.Duration `env:"TLS_HANDSHAKE_TIMEOUT"   envDefault:"10s"`
	ExpectContinueTimeout time.Duration `env:"EXPECT_CONTINUE_TIMEOUT" envDefault:"1s"`
	Timeout               time.Duration `env:"TIMEOUT"                 envDefault:"30s"`
}

type Retry struct {
	RetryMax     int           `env:"RETRY_MAX"      envDefault:"4"`
	RetryWaitMin time.Duration `env:"RETRY_WAIT_MIN" envDefault:"200ms"`
	RetryWaitMax time.Duration `env:"RETRY_WAIT_MAX" envDefault:"2s"`
	BackoffType  string        `env:"BACKOFF_TYPE"   envDefault:"exponential"`
}

type CircuitBreaker struct {
	MaxHalfOpenRequests uint32        `env:"MAX_HALF_OPEN_REQUESTS" envDefault:"5"`
	Interval            time.Duration `env:"INTERVAL"               envDefault:"60s"`
	Timeout             time.Duration `env:"TIMEOUT"                envDefault:"30s"`
	MinRequests         uint32        `env:"MIN_REQUESTS"           envDefault:"10"`
	ConsecutiveFailures uint32        `env:"CONSECUTIVE_FAILURES"   envDefault:"5"`
	FailureRate         float64       `env:"FAILURE_RATE"           envDefault:"0.6"`
}
