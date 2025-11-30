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
	Image    string         `json:"image"              yaml:"image"`
	Replicas int            `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Sidecar  map[string]any `json:"sidecar,omitempty"  yaml:"sidecar,omitempty"`
}

func DefaultManifest() Manifest {
	var manifest Manifest

	if err := yaml.Unmarshal([]byte(defaultManifest), &manifest); err != nil {
		slog.Error("failed to unmarshal default manifest", slog.Any("error", err))
	}

	return manifest
}

type ServiceInfo struct {
	Status    string         `json:"status"`
	Name      string         `json:"name"`
	Instances []InstanceInfo `json:"instances"`
}

type InstanceInfo struct {
	ContainerID string `json:"container_id"`
	SidecarID   string `json:"sidecar_id"`
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
	Name     string         `json:"name"`
	Image    string         `json:"image"`
	Replicas int            `json:"replicas,omitempty"`
	Sidecar  map[string]any `json:"sidecar,omitempty"`
}

// DeployServiceResponse represents the response after deploying a service.
type DeployServiceResponse struct {
	Service ServiceInfo `json:"service"`
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

// ContainerInfo represents basic container information.
type ContainerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Status  string            `json:"status"`
	State   string            `json:"state"`
	Ports   []PortBinding     `json:"ports,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	Network string            `json:"network,omitempty"`
}

// PortBinding represents a port binding configuration.
type PortBinding struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
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
