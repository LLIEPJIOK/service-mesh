package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	RootCACertFile    string
	RootCAKeyFile     string
	LeafTTL           time.Duration
	MaxRequestBytes   int64
	RateLimitRPS      float64
	RateLimitBurst    int
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	KubeConfigPath    string
}

func LoadFromEnv() (Config, error) {
	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		port := envInt(8080, "PORT")
		httpAddr = ":" + strconv.Itoa(port)
	}

	cfg := Config{
		HTTPAddr:          httpAddr,
		RootCACertFile:    envString("/etc/mesh/ca/tls.crt", "ROOT_CA_CERT_FILE"),
		RootCAKeyFile:     envString("/etc/mesh/ca/tls.key", "ROOT_CA_KEY_FILE"),
		LeafTTL:           envDuration(8760*time.Hour, "LEAF_TTL"),
		MaxRequestBytes:   envInt64(1<<20, "MAX_REQUEST_BYTES"),
		RateLimitRPS:      envFloat64(0, "RATE_LIMIT_RPS"),
		RateLimitBurst:    envInt(0, "RATE_LIMIT_BURST"),
		ReadHeaderTimeout: envDuration(10*time.Second, "READ_HEADER_TIMEOUT"),
		IdleTimeout:       envDuration(60*time.Second, "IDLE_TIMEOUT"),
		ShutdownTimeout:   envDuration(10*time.Second, "SHUTDOWN_TIMEOUT"),
		KubeConfigPath:    envString("", "KUBECONFIG"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return fmt.Errorf("HTTP_ADDR must not be empty")
	}

	if c.RootCACertFile == "" || c.RootCAKeyFile == "" {
		return fmt.Errorf("ROOT_CA_CERT_FILE and ROOT_CA_KEY_FILE must not be empty")
	}

	if c.LeafTTL <= 0 {
		return fmt.Errorf("LEAF_TTL must be positive")
	}

	if c.MaxRequestBytes <= 0 {
		return fmt.Errorf("MAX_REQUEST_BYTES must be positive")
	}

	if c.RateLimitRPS < 0 {
		return fmt.Errorf("RATE_LIMIT_RPS must be non-negative")
	}

	if c.RateLimitBurst < 0 {
		return fmt.Errorf("RATE_LIMIT_BURST must be non-negative")
	}

	if c.ReadHeaderTimeout <= 0 || c.IdleTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}

	return nil
}

func envString(fallback string, key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func envInt(fallback int, key string) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envInt64(fallback int64, key string) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envFloat64(fallback float64, key string) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}

func envDuration(fallback time.Duration, key string) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
