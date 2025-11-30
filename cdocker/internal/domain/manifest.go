package domain

import (
	"log/slog"

	"gopkg.in/yaml.v3"
)

const (
	defaultManifest = `
apiVersion: v1
kind: Service

metadata:
  name: counter
  labels:
    app: counter
    version: v1

spec:
  image: lliepjiok/counter:latest
  replicas: 3
  sidecar:
    ratelimiter:
      max_hits: 100
      window: 1m

    client:
      retry:
        retry_max: 3
        retry_wait_min: 100ms
        retry_wait_max: 1s
      circuit_breaker:
        max_half_open_requests: 5
        timeout: 30s

	`
)

type Manifest struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind"       yaml:"kind"`
	Metadata   ManifestMetadata `json:"metadata"   yaml:"metadata"`
	Spec       Spec             `json:"spec"       yaml:"spec"`
}

type ManifestMetadata struct {
	Name   string            `json:"name"             yaml:"name"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type Spec struct {
	Image          string         `json:"image"                    yaml:"image"`
	Replicas       int            `json:"replicas,omitempty"       yaml:"replicas,omitempty"`
	Sidecar        map[string]any `json:"sidecar,omitempty"        yaml:"sidecar,omitempty"`
	LivenessProbe  *Probe         `json:"livenessProbe,omitempty"  yaml:"livenessProbe,omitempty"`
	ReadinessProbe *Probe         `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
}

// Probe defines a health check probe configuration.
type Probe struct {
	HTTPGet             *HTTPGetAction `json:"httpGet,omitempty"             yaml:"httpGet,omitempty"`
	InitialDelaySeconds int            `json:"initialDelaySeconds,omitempty" yaml:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int            `json:"periodSeconds,omitempty"       yaml:"periodSeconds,omitempty"`
	TimeoutSeconds      int            `json:"timeoutSeconds,omitempty"      yaml:"timeoutSeconds,omitempty"`
	FailureThreshold    int            `json:"failureThreshold,omitempty"    yaml:"failureThreshold,omitempty"`
	SuccessThreshold    int            `json:"successThreshold,omitempty"    yaml:"successThreshold,omitempty"`
}

// HTTPGetAction describes an HTTP GET action for probes.
type HTTPGetAction struct {
	Path   string `json:"path"             yaml:"path"`
	Port   int    `json:"port,omitempty"   yaml:"port,omitempty"`
	Scheme string `json:"scheme,omitempty" yaml:"scheme,omitempty"`
}

// ProbeDefaults returns default values for probe configuration.
func (p *Probe) WithDefaults() *Probe {
	if p == nil {
		return nil
	}
	probe := *p
	if probe.PeriodSeconds == 0 {
		probe.PeriodSeconds = 60
	}
	if probe.TimeoutSeconds == 0 {
		probe.TimeoutSeconds = 5
	}
	if probe.FailureThreshold == 0 {
		probe.FailureThreshold = 3
	}
	if probe.SuccessThreshold == 0 {
		probe.SuccessThreshold = 1
	}
	if probe.HTTPGet != nil && probe.HTTPGet.Port == 0 {
		probe.HTTPGet.Port = 8080
	}
	if probe.HTTPGet != nil && probe.HTTPGet.Scheme == "" {
		probe.HTTPGet.Scheme = "http"
	}
	return &probe
}

func DefaultManifest() Manifest {
	var manifest Manifest

	if err := yaml.Unmarshal([]byte(defaultManifest), &manifest); err != nil {
		slog.Error("failed to unmarshal default manifest", slog.Any("error", err))
	}

	return manifest
}

type ContainerInfo struct {
	Name        string `json:"name"`
	ServiceName string `json:"service_name"`
	Status      string `json:"status"`
	Restarts    int    `json:"restarts"`
	ContainerID string `json:"container_id"`
	SidecarID   string `json:"sidecar_id"`
}

// ProbeStatus represents the health status of a container.
type ProbeStatus string

const (
	ProbeStatusHealthy   ProbeStatus = "healthy"
	ProbeStatusUnhealthy ProbeStatus = "unhealthy"
	ProbeStatusUnknown   ProbeStatus = "unknown"
)

// ProbeReport represents a health check report from sidecar.
type ProbeReport struct {
	ContainerName string      `json:"container_name"`
	ProbeName     string      `json:"probe_name"`
	Status        ProbeStatus `json:"status"`
}

// HealthState tracks the health state of a service.
type HealthState struct {
	ContainerName     string `json:"container_name"`
	LivenessFails     int    `json:"liveness_fails"`
	ReadinessFails    int    `json:"readiness_fails"`
	LastLivenessTime  int64  `json:"last_liveness_time"`
	LastReadinessTime int64  `json:"last_readiness_time"`
}

// MonitoringManifest represents a monitoring stack manifest.
type MonitoringManifest struct {
	APIVersion string           `json:"apiVersion" yaml:"apiVersion"`
	Kind       string           `json:"kind"       yaml:"kind"`
	Metadata   ManifestMetadata `json:"metadata"   yaml:"metadata"`
	Spec       MonitoringSpec   `json:"spec"       yaml:"spec"`
}

// MonitoringSpec defines the monitoring specification.
type MonitoringSpec struct {
	Prometheus PrometheusSpec `json:"prometheus" yaml:"prometheus"`
	Grafana    GrafanaSpec    `json:"grafana"    yaml:"grafana"`
}

// PrometheusSpec defines Prometheus configuration.
type PrometheusSpec struct {
	Config string `json:"config,omitempty" yaml:"config,omitempty"`
	Port   int    `json:"port,omitempty"   yaml:"port,omitempty"`
}

// GrafanaSpec defines Grafana configuration.
type GrafanaSpec struct {
	Port  int         `json:"port,omitempty"  yaml:"port,omitempty"`
	Admin AdminConfig `json:"admin,omitempty" yaml:"admin,omitempty"`
}

// AdminConfig defines admin credentials.
type AdminConfig struct {
	User     string `json:"user,omitempty"     yaml:"user,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

// DeployPlaneRequest represents a request to deploy the control plane.
type DeployPlaneRequest struct {
	Config map[string]string `json:"config,omitempty"`
}

// DeployPlaneResponse represents the response after deploying the control plane.
type DeployPlaneResponse struct {
	ContainerID string `json:"container_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
}

// DeployServiceRequest represents a request to deploy a service with sidecar.
type DeployServiceRequest struct {
	Name           string         `json:"name"`
	Image          string         `json:"image"`
	Replicas       int            `json:"replicas,omitempty"`
	Sidecar        map[string]any `json:"sidecar,omitempty"`
	LivenessProbe  *Probe         `json:"livenessProbe,omitempty"`
	ReadinessProbe *Probe         `json:"readinessProbe,omitempty"`
}

// DeployServiceResponse represents the response after deploying a service.
type DeployServiceResponse struct {
	Services []ContainerInfo `json:"services"`
}

// DeployMonitoringRequest represents a request to deploy monitoring stack.
type DeployMonitoringRequest struct {
	PrometheusConfig string `json:"prometheus_config,omitempty"`
	PrometheusPort   int    `json:"prometheus_port,omitempty"`
	GrafanaPort      int    `json:"grafana_port,omitempty"`
	GrafanaUser      string `json:"grafana_user,omitempty"`
	GrafanaPassword  string `json:"grafana_password,omitempty"`
}

// DeployMonitoringResponse represents the response after deploying monitoring.
type DeployMonitoringResponse struct {
	PrometheusID   string `json:"prometheus_id"`
	GrafanaID      string `json:"grafana_id"`
	PrometheusPort int    `json:"prometheus_port"`
	GrafanaPort    int    `json:"grafana_port"`
	Status         string `json:"status"`
}

// ListContainersResponse represents the response for listing containers.
type ListContainersResponse struct {
	Containers []ContainerInfo `json:"containers"`
}

// StopContainerRequest represents a request to stop a container.
type StopContainerRequest struct {
	Name string `json:"name"`
}

// StopContainerResponse represents the response after stopping a container.
type StopContainerResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// RemoveContainerRequest represents a request to remove a container.
type RemoveContainerRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

// RemoveContainerResponse represents the response after removing a container.
type RemoveContainerResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// CreateNetworkRequest represents a request to create a Docker network.
type CreateNetworkRequest struct {
	Name   string `json:"name"`
	Driver string `json:"driver,omitempty"`
}

// CreateNetworkResponse represents the response after creating a network.
type CreateNetworkResponse struct {
	NetworkID string `json:"network_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}
