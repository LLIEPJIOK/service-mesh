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
	TLSCertFile       string
	TLSKeyFile        string
	MaxRequestBytes   int64
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	IgnoreNamespaces  map[string]struct{}

	IptablesImage string
	SidecarImage  string
	MeshVersion   string

	MonitoringEnabled bool
	MetricsPort       int

	InboundPlainPort int
	OutboundPort     int
	InboundMTLSPort  int
	ExcludeInbound   string
	ExcludeOutbound  string
	SidecarUID       int64

	LoadBalancerAlgorithm          string
	RetryAttempts                  int
	ConnectTimeout                 time.Duration
	CircuitBreakerFailureThreshold int
	CircuitBreakerRecoveryTime     time.Duration
}

func LoadFromEnv() (Config, error) {
	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		port := envInt(8080, "PORT")
		httpAddr = ":" + strconv.Itoa(port)
	}

	cfg := Config{
		HTTPAddr:          httpAddr,
		TLSCertFile:       strings.TrimSpace(os.Getenv("TLS_CERT_FILE")),
		TLSKeyFile:        strings.TrimSpace(os.Getenv("TLS_KEY_FILE")),
		MaxRequestBytes:   envInt64(1<<20, "MAX_REQUEST_BYTES"),
		ReadHeaderTimeout: envDuration(10*time.Second, "READ_HEADER_TIMEOUT"),
		IdleTimeout:       envDuration(60*time.Second, "IDLE_TIMEOUT"),
		ShutdownTimeout:   envDuration(10*time.Second, "SHUTDOWN_TIMEOUT"),
		IgnoreNamespaces:  toSet(envCSV([]string{"kube-system", "mesh-system"}, "IGNORE_NAMESPACES")),

		IptablesImage: envString("mesh/iptables-init:latest", "IPTABLES_IMAGE"),
		SidecarImage:  envString("mesh/sidecar:latest", "SIDECAR_IMAGE"),
		MeshVersion:   envString("v0.1.0", "MESH_VERSION"),

		MonitoringEnabled: envBool(true, "MONITORING_ENABLED"),
		MetricsPort:       envInt(9090, "METRICS_PORT"),

		InboundPlainPort: envInt(15006, "INBOUND_PLAIN_PORT"),
		OutboundPort:     envInt(15002, "OUTBOUND_PORT"),
		InboundMTLSPort:  envInt(15001, "INBOUND_MTLS_PORT"),
		ExcludeInbound:   envString("9090", "EXCLUDE_INBOUND_PORTS"),
		ExcludeOutbound:  envString("169.254.169.254/32", "EXCLUDE_OUTBOUND_IPS"),
		SidecarUID:       envInt64(1337, "SIDECAR_UID"),

		LoadBalancerAlgorithm:          envString("roundRobin", "LOAD_BALANCER_ALGORITHM"),
		RetryAttempts:                  envInt(3, "RETRY_ATTEMPTS"),
		ConnectTimeout:                 envDuration(5*time.Second, "TIMEOUT"),
		CircuitBreakerFailureThreshold: envInt(5, "CIRCUIT_BREAKER_FAILURE_THRESHOLD"),
		CircuitBreakerRecoveryTime:     envDuration(30*time.Second, "CIRCUIT_BREAKER_RECOVERY_TIME"),
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

	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}

	if c.MaxRequestBytes <= 0 {
		return fmt.Errorf("MAX_REQUEST_BYTES must be positive")
	}

	if c.ReadHeaderTimeout <= 0 || c.IdleTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}

	if c.IptablesImage == "" || c.SidecarImage == "" {
		return fmt.Errorf("IPTABLES_IMAGE and SIDECAR_IMAGE must not be empty")
	}

	if c.MeshVersion == "" {
		return fmt.Errorf("MESH_VERSION must not be empty")
	}

	if c.MetricsPort <= 0 || c.InboundPlainPort <= 0 || c.OutboundPort <= 0 || c.InboundMTLSPort <= 0 {
		return fmt.Errorf("ports must be positive")
	}

	if c.SidecarUID <= 0 {
		return fmt.Errorf("SIDECAR_UID must be positive")
	}

	if c.RetryAttempts < 0 {
		return fmt.Errorf("RETRY_ATTEMPTS must be non-negative")
	}

	if c.ConnectTimeout <= 0 {
		return fmt.Errorf("TIMEOUT must be positive")
	}

	if c.CircuitBreakerFailureThreshold < 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_FAILURE_THRESHOLD must be non-negative")
	}

	if c.CircuitBreakerRecoveryTime <= 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_RECOVERY_TIME must be positive")
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

func envBool(fallback bool, key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envCSV(fallback []string, key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return fallback
	}

	return result
}

func toSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}

	return result
}
