package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	PodName        string
	Namespace      string
	ServiceAccount string

	InboundPlainPort   int
	OutboundPort       int
	InboundMTLSPort    int
	MetricsPort        int
	MonitoringEnabled  bool
	AppTargetAddr      string
	ShutdownTimeout    time.Duration
	LoadBalancerConfig LoadBalancerConfig

	ExcludeInboundPorts string
	ExcludeOutboundIPs  string

	RetryPolicy RetryPolicy
	Timeout     time.Duration
	DialTimeout time.Duration

	CircuitBreakerPolicy CircuitBreakerPolicy

	CertFile                string
	KeyFile                 string
	CAFile                  string
	CertManagerSignURL      string
	ServiceAccountTokenPath string
	BootstrapCertificates   bool

	KubeConfigPath string
}

type LoadBalancerConfig struct {
	Algorithm string
}

type RetryPolicy struct {
	Attempts     int
	BackoffType  string
	BaseInterval time.Duration
}

type CircuitBreakerPolicy struct {
	FailureThreshold uint32
	RecoveryTime     time.Duration
}

func LoadFromEnv() (Config, error) {
	timeout := envDurationWithAliases(5*time.Second, "TIMEOUT", "SIDECAR_TIMEOUT")
	dialTimeout := envDurationWithAliases(timeout, "DIAL_TIMEOUT", "SIDECAR_DIAL_TIMEOUT")

	cfg := Config{
		PodName:        envStringWithAliases("unknown-pod", "POD_NAME"),
		Namespace:      envStringWithAliases("default", "POD_NAMESPACE"),
		ServiceAccount: envStringWithAliases("default", "SERVICE_ACCOUNT"),

		InboundPlainPort:  envIntWithAliases(15006, "INBOUND_PLAIN_PORT", "SIDECAR_INBOUND_PLAIN_PORT"),
		OutboundPort:      envIntWithAliases(15002, "OUTBOUND_PORT", "SIDECAR_OUTBOUND_PORT"),
		InboundMTLSPort:   envIntWithAliases(15001, "INBOUND_MTLS_PORT", "SIDECAR_INBOUND_MTLS_PORT"),
		MetricsPort:       envIntWithAliases(9090, "METRICS_PORT", "SIDECAR_METRICS_PORT"),
		MonitoringEnabled: envBoolWithAliases(true, "MONITORING_ENABLED", "SIDECAR_MONITORING_ENABLED"),
		AppTargetAddr:     envStringWithAliases("127.0.0.1:8080", "APP_TARGET_ADDR", "SIDECAR_APP_TARGET_ADDR"),
		ShutdownTimeout:   envDurationWithAliases(30*time.Second, "SHUTDOWN_TIMEOUT", "SIDECAR_SHUTDOWN_TIMEOUT"),
		LoadBalancerConfig: LoadBalancerConfig{
			Algorithm: envStringWithAliases("roundRobin", "LOAD_BALANCER_ALGORITHM", "SIDECAR_LOAD_BALANCER_ALGORITHM"),
		},

		ExcludeInboundPorts: envStringWithAliases("9090", "EXCLUDE_INBOUND_PORTS", "SIDECAR_EXCLUDE_INBOUND_PORTS"),
		ExcludeOutboundIPs:  envStringWithAliases("169.254.169.254/32", "EXCLUDE_OUTBOUND_IPS", "SIDECAR_EXCLUDE_OUTBOUND_IPS"),

		RetryPolicy: RetryPolicy{
			Attempts:     envIntWithAliases(3, "RETRY_ATTEMPTS", "SIDECAR_RETRY_ATTEMPTS"),
			BackoffType:  envStringWithAliases("exponential", "RETRY_BACKOFF_TYPE", "SIDECAR_RETRY_BACKOFF_TYPE"),
			BaseInterval: envDurationWithAliases(100*time.Millisecond, "RETRY_BASE_INTERVAL", "SIDECAR_RETRY_BASE_INTERVAL"),
		},
		Timeout:     timeout,
		DialTimeout: dialTimeout,
		CircuitBreakerPolicy: CircuitBreakerPolicy{
			FailureThreshold: envUint32WithAliases(5, "CIRCUIT_BREAKER_FAILURE_THRESHOLD", "SIDECAR_CIRCUIT_BREAKER_FAILURE_THRESHOLD"),
			RecoveryTime:     envDurationWithAliases(30*time.Second, "CIRCUIT_BREAKER_RECOVERY_TIME", "SIDECAR_CIRCUIT_BREAKER_RECOVERY_TIME"),
		},

		CertFile:                envStringWithAliases("", "CERT_FILE", "SIDECAR_CERT_FILE"),
		KeyFile:                 envStringWithAliases("", "KEY_FILE", "SIDECAR_KEY_FILE"),
		CAFile:                  envStringWithAliases("", "CA_FILE", "SIDECAR_CA_FILE"),
		CertManagerSignURL:      envStringWithAliases("http://mesh-cert-manager.mesh-system.svc.cluster.local:8080/sign", "CERT_MANAGER_SIGN_URL", "SIDECAR_CERT_MANAGER_SIGN_URL"),
		ServiceAccountTokenPath: envStringWithAliases("/var/run/secrets/kubernetes.io/serviceaccount/token", "SERVICE_ACCOUNT_TOKEN_PATH", "SIDECAR_SERVICE_ACCOUNT_TOKEN_PATH"),
		BootstrapCertificates:   envBoolWithAliases(true, "BOOTSTRAP_CERTIFICATES", "SIDECAR_BOOTSTRAP_CERTIFICATES"),

		KubeConfigPath: envStringWithAliases("", "KUBECONFIG"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.InboundPlainPort <= 0 || c.OutboundPort <= 0 || c.InboundMTLSPort <= 0 || c.MetricsPort <= 0 {
		return fmt.Errorf("all ports must be positive")
	}

	ports := map[int]string{
		c.InboundPlainPort: "inbound plain",
		c.OutboundPort:     "outbound",
		c.InboundMTLSPort:  "inbound mtls",
		c.MetricsPort:      "metrics",
	}

	if len(ports) != 4 {
		return fmt.Errorf("sidecar ports must be unique")
	}

	if c.Timeout <= 0 || c.DialTimeout <= 0 {
		return fmt.Errorf("timeout values must be positive")
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}

	if c.RetryPolicy.Attempts < 1 {
		return fmt.Errorf("retry attempts must be at least 1")
	}

	if c.RetryPolicy.BaseInterval <= 0 {
		return fmt.Errorf("retry base interval must be positive")
	}

	switch c.RetryPolicy.BackoffType {
	case "linear", "exponential":
	default:
		return fmt.Errorf("unsupported retry backoff type %q", c.RetryPolicy.BackoffType)
	}

	switch c.LoadBalancerConfig.Algorithm {
	case "roundRobin", "random":
	default:
		return fmt.Errorf("unsupported load balancer algorithm %q", c.LoadBalancerConfig.Algorithm)
	}

	if c.CircuitBreakerPolicy.FailureThreshold == 0 {
		return fmt.Errorf("circuit breaker failure threshold must be at least 1")
	}

	if c.CircuitBreakerPolicy.RecoveryTime <= 0 {
		return fmt.Errorf("circuit breaker recovery time must be positive")
	}

	if _, _, err := net.SplitHostPort(c.AppTargetAddr); err != nil {
		return fmt.Errorf("invalid app target address %q: %w", c.AppTargetAddr, err)
	}

	if !containsPort(c.ExcludeInboundPorts, c.MetricsPort) {
		return fmt.Errorf("metrics port %d must be in EXCLUDE_INBOUND_PORTS", c.MetricsPort)
	}

	if c.BootstrapCertificates {
		if c.CertManagerSignURL == "" {
			return fmt.Errorf("cert manager sign URL is required when bootstrap is enabled")
		}

		if c.ServiceAccountTokenPath == "" {
			return fmt.Errorf("service account token path is required when bootstrap is enabled")
		}
	} else {
		if c.CertFile == "" || c.KeyFile == "" || c.CAFile == "" {
			return fmt.Errorf("cert, key and ca files are required when bootstrap is disabled")
		}
	}

	return nil
}

func containsPort(csv string, port int) bool {
	if csv == "" {
		return false
	}

	for _, value := range strings.Split(csv, ",") {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			continue
		}

		if parsed == port {
			return true
		}
	}

	return false
}

func envStringWithAliases(fallback string, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}

	return fallback
}

func envIntWithAliases(fallback int, keys ...string) int {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}

		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}

func envUint32WithAliases(fallback uint32, keys ...string) uint32 {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}

		parsed, err := strconv.ParseUint(value, 10, 32)
		if err == nil {
			return uint32(parsed)
		}
	}

	return fallback
}

func envBoolWithAliases(fallback bool, keys ...string) bool {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}

		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}

func envDurationWithAliases(fallback time.Duration, keys ...string) time.Duration {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}

		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}
