package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type MeshConfig struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       MeshSpec `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type MeshSpec struct {
	Namespace    string          `yaml:"namespace"`
	Version      string          `yaml:"version"`
	Images       Images          `yaml:"images"`
	Certificates Certificates    `yaml:"certificates"`
	Sidecar      SidecarConfig   `yaml:"sidecar"`
	Injection    InjectionConfig `yaml:"injection"`
	CertManager  CertManager     `yaml:"certManager"`
}

type Images struct {
	Sidecar      string `yaml:"sidecar"`
	IptablesInit string `yaml:"iptablesInit"`
	CertManager  string `yaml:"certManager"`
}

type Certificates struct {
	RootCA   RootCA `yaml:"rootCA"`
	Validity string `yaml:"validity"`
}

type RootCA struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type SidecarConfig struct {
	InboundPlainPort      int            `yaml:"inboundPlainPort"`
	OutboundPort          int            `yaml:"outboundPort"`
	InboundMTLSPort       int            `yaml:"inboundMTLSPort"`
	MetricsPort           int            `yaml:"metricsPort"`
	MonitoringEnabled     bool           `yaml:"monitoringEnabled"`
	LoadBalancerAlgorithm string         `yaml:"loadBalancerAlgorithm"`
	RetryPolicy           RetryPolicy    `yaml:"retryPolicy"`
	Timeout               string         `yaml:"timeout"`
	CircuitBreakerPolicy  CircuitBreaker `yaml:"circuitBreakerPolicy"`
	ExcludeInboundPorts   string         `yaml:"excludeInboundPorts"`
	ExcludeOutboundIPs    string         `yaml:"excludeOutboundIPs"`
}

type RetryPolicy struct {
	Attempts int     `yaml:"attempts"`
	Backoff  Backoff `yaml:"backoff"`
}

type Backoff struct {
	Type         string `yaml:"type"`
	BaseInterval string `yaml:"baseInterval"`
}

type CircuitBreaker struct {
	FailureThreshold int    `yaml:"failureThreshold"`
	RecoveryTime     string `yaml:"recoveryTime"`
}

type InjectionConfig struct {
	NamespaceSelector NamespaceSelector `yaml:"namespaceSelector"`
}

type NamespaceSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels"`
}

type CertManager struct {
	Enabled   bool                   `yaml:"enabled"`
	Resources map[string]interface{} `yaml:"resources"`
}

func LoadFromFile(path string) (MeshConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MeshConfig{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg MeshConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return MeshConfig{}, fmt.Errorf("parse yaml config: %w", err)
	}

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return MeshConfig{}, err
	}

	return cfg, nil
}

func (c *MeshConfig) ApplyDefaults() {
	if strings.TrimSpace(c.Spec.Namespace) == "" {
		c.Spec.Namespace = "mesh-system"
	}
	if strings.TrimSpace(c.Spec.Version) == "" {
		c.Spec.Version = "v0.1.0"
	}

	if c.Spec.Sidecar.MetricsPort == 0 {
		c.Spec.Sidecar.MetricsPort = 9090
	}
	if strings.TrimSpace(c.Spec.Sidecar.LoadBalancerAlgorithm) == "" {
		c.Spec.Sidecar.LoadBalancerAlgorithm = "roundRobin"
	}
	if strings.TrimSpace(c.Spec.Sidecar.Timeout) == "" {
		c.Spec.Sidecar.Timeout = "5s"
	}
	if c.Spec.Sidecar.RetryPolicy.Attempts == 0 {
		c.Spec.Sidecar.RetryPolicy.Attempts = 3
	}
	if strings.TrimSpace(c.Spec.Sidecar.CircuitBreakerPolicy.RecoveryTime) == "" {
		c.Spec.Sidecar.CircuitBreakerPolicy.RecoveryTime = "30s"
	}
	if c.Spec.Sidecar.CircuitBreakerPolicy.FailureThreshold == 0 {
		c.Spec.Sidecar.CircuitBreakerPolicy.FailureThreshold = 5
	}
	if strings.TrimSpace(c.Spec.Sidecar.ExcludeInboundPorts) == "" {
		c.Spec.Sidecar.ExcludeInboundPorts = "9090"
	}
	if strings.TrimSpace(c.Spec.Sidecar.ExcludeOutboundIPs) == "" {
		c.Spec.Sidecar.ExcludeOutboundIPs = "169.254.169.254/32"
	}

	if strings.TrimSpace(c.Spec.Certificates.Validity) == "" {
		c.Spec.Certificates.Validity = "8760h"
	}

	if len(c.Spec.Injection.NamespaceSelector.MatchLabels) == 0 {
		c.Spec.Injection.NamespaceSelector.MatchLabels = map[string]string{"mesh-injection": "enabled"}
	}
}

func (c *MeshConfig) EffectiveNamespace(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return strings.TrimSpace(c.Spec.Namespace)
}

func (c MeshConfig) Validate() error {
	if strings.TrimSpace(c.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(c.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(c.Spec.Namespace) == "" {
		return fmt.Errorf("spec.namespace is required")
	}

	if c.Spec.Sidecar.InboundPlainPort <= 0 {
		return fmt.Errorf("spec.sidecar.inboundPlainPort must be positive")
	}
	if c.Spec.Sidecar.OutboundPort <= 0 {
		return fmt.Errorf("spec.sidecar.outboundPort must be positive")
	}
	if c.Spec.Sidecar.InboundMTLSPort <= 0 {
		return fmt.Errorf("spec.sidecar.inboundMTLSPort must be positive")
	}

	if strings.TrimSpace(c.Spec.Certificates.RootCA.Cert) == "" || strings.TrimSpace(c.Spec.Certificates.RootCA.Key) == "" {
		return fmt.Errorf("spec.certificates.rootCA.cert and spec.certificates.rootCA.key are required")
	}

	return nil
}
