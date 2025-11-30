package cdocker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/config"
	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/domain"
	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/infra/docker"
	"gopkg.in/yaml.v3"
)

const (
	meshNetwork         = "mesh_network"
	controlPlane        = "control-plane"
	controlPlaneProject = "control-plane"
	sidecarImage        = "lliepjiok/sidecar:latest"
	controlPlaneImage   = "lliepjiok/control-plane:latest"
	prometheusImage     = "prom/prometheus:latest"
	grafanaImage        = "grafana/grafana:latest"
)

type CDocker struct {
	services map[string]*domain.ServiceInfo
	cfg      *config.CDocker
	docker   *docker.Client
}

func New(ctx context.Context, cfg *config.CDocker, dockerClient *docker.Client) (*CDocker, error) {
	cd := &CDocker{
		services: make(map[string]*domain.ServiceInfo),
		cfg:      cfg,
		docker:   dockerClient,
	}

	if err := cd.start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start cdocker: %w", err)
	}

	return cd, nil
}

func (c *CDocker) start(ctx context.Context) error {
	exists, err := c.docker.ContainerExists(ctx, controlPlane)
	if err != nil {
		return fmt.Errorf("failed to check container existence: %w", err)
	}
	if exists {
		return fmt.Errorf("control-plane already exists")
	}

	if err := c.docker.PullImage(ctx, controlPlaneImage); err != nil {
		slog.Warn("Failed to pull image, trying local", slog.Any("error", err))
	}

	_, err = c.docker.CreateAndStartContainer(ctx, docker.ContainerConfig{
		Name:    controlPlane,
		Image:   controlPlaneImage,
		Env:     planeEnvs,
		Network: meshNetwork,
		Labels: map[string]string{
			"com.docker.compose.project": controlPlaneProject,
			"com.docker.compose.service": controlPlane,
		},
		PortMapping: map[int]int{
			8080: 8080,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create control-plane: %w", err)
	}

	return nil
}

func (c *CDocker) Stop(ctx context.Context) error {
	if err := c.docker.RemoveContainer(ctx, controlPlane, true); err != nil {
		return fmt.Errorf("failed to remove control-plane: %w", err)
	}

	return nil
}

func (c *CDocker) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /apply", c.applyManifest)

	// Monitoring
	mux.HandleFunc("POST /monitoring", c.deployMonitoring)

	// Container management
	mux.HandleFunc("GET /containers", c.listContainers)
	mux.HandleFunc("POST /containers/stop", c.stopContainer)
	mux.HandleFunc("POST /containers/remove", c.removeContainer)

	// Network management
	mux.HandleFunc("POST /network", c.createNetwork)

	// Health check
	mux.HandleFunc("GET /health", c.healthCheck)
}

// applyManifest parses and applies a YAML manifest.
func (c *CDocker) applyManifest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.writeError(w, http.StatusBadRequest, "failed to read request body", err)
		return
	}

	manifest := domain.DefaultManifest()

	if err := yaml.Unmarshal(body, &manifest); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid YAML manifest", err)
		return
	}

	req := domain.DeployServiceRequest{
		Name:     manifest.Metadata.Name,
		Image:    manifest.Spec.Image,
		Replicas: manifest.Spec.Replicas,
		Sidecar:  manifest.Spec.Sidecar,
	}

	c.deployServiceInternal(w, r, req)
}

func (c *CDocker) deployServiceInternal(
	w http.ResponseWriter,
	r *http.Request,
	req domain.DeployServiceRequest,
) {
	ctx := r.Context()

	if req.Name == "" || req.Image == "" {
		c.writeError(w, http.StatusBadRequest, "name and image are required", nil)
		return
	}

	replicas := req.Replicas
	if replicas < 1 {
		c.writeError(w, http.StatusBadRequest, "replicas must be at least 1", nil)
		return
	}

	if !c.docker.ImageExists(ctx, sidecarImage) {
		if err := c.docker.PullImage(ctx, sidecarImage); err != nil {
			c.writeError(w, http.StatusInternalServerError, "failed to pull sidecar image", err)
			return
		}
	}

	if !c.docker.ImageExists(ctx, req.Image) {
		if err := c.docker.PullImage(ctx, req.Image); err != nil {
			c.writeError(w, http.StatusBadRequest, "no image found", err)
			return
		}
	}

	instances := make([]domain.InstanceInfo, 0, replicas)

	clean := func(created int) {
		for i := range created {
			containerName := fmt.Sprintf("%s-%d", req.Name, i+1)
			c.docker.RemoveContainer(ctx, containerName, true)
			c.docker.RemoveContainer(ctx, containerName+"-sidecar", true)
		}
	}

	for i := range replicas {
		instanceInfo, err := c.deploySingleService(ctx, req, i+1)
		if err != nil {
			clean(i)
			c.writeError(
				w,
				http.StatusInternalServerError,
				fmt.Sprintf("failed to deploy service replica %d", i+1),
				err,
			)

			return
		}

		instances = append(instances, instanceInfo)
	}

	service := domain.ServiceInfo{
		Name:      req.Name,
		Instances: instances,
		Status:    "running",
	}

	c.services[req.Name] = &service
	resp := domain.DeployServiceResponse{
		Service: service,
	}

	c.writeJSON(w, http.StatusCreated, resp)
}

func (c *CDocker) deploySingleService(
	ctx context.Context,
	req domain.DeployServiceRequest,
	idx int,
) (domain.InstanceInfo, error) {
	containerName := fmt.Sprintf("%s-%d", req.Name, idx)
	sidecarName := containerName + "-sidecar"

	sidecarEnv := buildEnvVarsFromMap(req.Sidecar)
	sidecarEnv = append(sidecarEnv,
		fmt.Sprintf("SIDECAR_TARGET=%s:8080", containerName),
		fmt.Sprintf("SIDECAR_SERVICE_NAME=%s", containerName),
	)

	sidecarID, err := c.docker.CreateAndStartContainer(ctx, docker.ContainerConfig{
		Name:    sidecarName,
		Image:   sidecarImage,
		Env:     sidecarEnv,
		Network: meshNetwork,
		Labels: map[string]string{
			"com.docker.compose.project": controlPlaneProject,
			"com.docker.compose.service": sidecarName,
		},
	})
	if err != nil {
		return domain.InstanceInfo{}, fmt.Errorf("failed to create sidecar: %w", err)
	}

	appEnv := []string{
		fmt.Sprintf("HTTP_PROXY=http://%s:8080", sidecarName),
		fmt.Sprintf("HTTPS_PROXY=http://%s:8080", sidecarName),
		fmt.Sprintf("SERVICE_NAME=%s", containerName),
	}

	appID, err := c.docker.CreateAndStartContainer(ctx, docker.ContainerConfig{
		Name:    containerName,
		Image:   req.Image,
		Env:     appEnv,
		Network: meshNetwork,
		Labels: map[string]string{
			"com.docker.compose.project": controlPlaneProject,
			"com.docker.compose.service": containerName,
		},
	})
	if err != nil {
		return domain.InstanceInfo{}, fmt.Errorf("failed to create app: %w", err)
	}

	if err := c.registerService(ctx, req.Name, sidecarName); err != nil {
		slog.Warn("Failed to register service with control plane", slog.Any("error", err))
	}

	return domain.InstanceInfo{
		ContainerID: appID,
		SidecarID:   sidecarID,
	}, nil
}

func (c *CDocker) registerService(ctx context.Context, name, sidecarName string) error {
	resp, err := http.DefaultClient.Post(
		"http://control-plane:8080/register",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"name":%q,"address":"%s:8080"}`, name, sidecarName)),
	)
	if err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	defer func() {
		if clErr := resp.Body.Close(); clErr != nil {
			slog.Error("failed to close response body", slog.Any("error", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to register service: %w", NewErrInvalidCode(resp.StatusCode))
	}

	return nil
}

func (c *CDocker) listContainers(w http.ResponseWriter, r *http.Request) {
	c.writeJSON(w, http.StatusOK, c.services)
}

func (c *CDocker) stopContainer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req domain.StopContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.Name == "" {
		c.writeError(w, http.StatusBadRequest, "name is required", nil)
		return
	}

	service, ok := c.services[req.Name]
	if !ok {
		c.writeError(w, http.StatusNotFound, "service not found", nil)
		return
	}

	service.Status = "stopping"

	for _, instance := range service.Instances {
		if err := c.docker.StopContainer(ctx, instance.ContainerID); err != nil {
			c.writeError(w, http.StatusInternalServerError, "failed to stop container", err)
			return
		}

		if err := c.docker.StopContainer(ctx, instance.SidecarID); err != nil {
			c.writeError(w, http.StatusInternalServerError, "failed to stop sidecar", err)
			return
		}
	}

	service.Status = "stopped"

	resp := domain.StopContainerResponse{
		Name:   req.Name,
		Status: "stopped",
	}

	c.writeJSON(w, http.StatusOK, resp)
}

func (c *CDocker) removeContainer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req domain.RemoveContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.Name == "" {
		c.writeError(w, http.StatusBadRequest, "name is required", nil)
		return
	}

	service, ok := c.services[req.Name]
	if !ok {
		c.writeError(w, http.StatusNotFound, "service not found", nil)
		return
	}

	service.Status = "removing"

	for _, instance := range service.Instances {
		if err := c.docker.RemoveContainer(ctx, instance.ContainerID, req.Force); err != nil {
			c.writeError(w, http.StatusInternalServerError, "failed to remove container", err)
			return
		}

		if err := c.docker.RemoveContainer(ctx, instance.SidecarID, req.Force); err != nil {
			c.writeError(w, http.StatusInternalServerError, "failed to remove sidecar", err)
			return
		}
	}

	delete(c.services, req.Name)
	resp := domain.RemoveContainerResponse{
		Name:   req.Name,
		Status: "removed",
	}

	c.writeJSON(w, http.StatusOK, resp)
}

// deployMonitoring deploys Prometheus and Grafana (JSON API).
func (c *CDocker) deployMonitoring(w http.ResponseWriter, r *http.Request) {
	var req domain.DeployMonitoringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		c.writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	c.deployMonitoringInternal(w, r, req)
}

// deployMonitoringInternal contains the actual monitoring deployment logic.
func (c *CDocker) deployMonitoringInternal(
	w http.ResponseWriter,
	r *http.Request,
	req domain.DeployMonitoringRequest,
) {
	ctx := r.Context()

	// Ensure network exists
	if _, err := c.docker.CreateNetwork(ctx, meshNetwork); err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed to create network", err)
		return
	}

	// Pull images
	if err := c.docker.PullImage(ctx, prometheusImage); err != nil {
		slog.Warn("Failed to pull prometheus image", slog.Any("error", err))
	}
	if err := c.docker.PullImage(ctx, grafanaImage); err != nil {
		slog.Warn("Failed to pull grafana image", slog.Any("error", err))
	}

	// Deploy Prometheus
	promVolumes := make(map[string]string)
	if req.PrometheusConfig != "" {
		promVolumes[req.PrometheusConfig] = "/etc/prometheus/prometheus.yml"
	}

	prometheusID, err := c.docker.CreateAndStartContainer(ctx, docker.ContainerConfig{
		Name:    "prometheus",
		Image:   prometheusImage,
		Network: meshNetwork,
		Labels: map[string]string{
			"com.docker.compose.project": controlPlaneProject,
			"com.docker.compose.service": "prometheus",
		},
		PortMapping: map[int]int{
			9090: 9090,
		},
		Volumes: promVolumes,
	})
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed to create prometheus", err)
		return
	}

	// Deploy Grafana
	grafanaUser := req.GrafanaUser
	if grafanaUser == "" {
		grafanaUser = "admin"
	}
	grafanaPassword := req.GrafanaPassword
	if grafanaPassword == "" {
		grafanaPassword = "admin"
	}

	grafanaID, err := c.docker.CreateAndStartContainer(ctx, docker.ContainerConfig{
		Name:  "grafana",
		Image: grafanaImage,
		Env: []string{
			"GF_SECURITY_ADMIN_USER=" + grafanaUser,
			"GF_SECURITY_ADMIN_PASSWORD=" + grafanaPassword,
		},
		Network: meshNetwork,
		Labels: map[string]string{
			"com.docker.compose.project": controlPlaneProject,
			"com.docker.compose.service": "grafana",
		},
		PortMapping: map[int]int{
			3000: 3000,
		},
		Volumes: map[string]string{
			"grafana-storage": "/var/lib/grafana",
		},
	})
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed to create grafana", err)
		return
	}

	resp := domain.DeployMonitoringResponse{
		PrometheusID:   prometheusID,
		GrafanaID:      grafanaID,
		PrometheusPort: 9090,
		GrafanaPort:    3000,
		Status:         "running",
	}

	c.writeJSON(w, http.StatusCreated, resp)
}

// createNetwork creates a Docker network.
func (c *CDocker) createNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req domain.CreateNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	name := req.Name
	if name == "" {
		name = meshNetwork
	}

	networkID, err := c.docker.CreateNetwork(ctx, name)
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed to create network", err)
		return
	}

	resp := domain.CreateNetworkResponse{
		NetworkID: networkID,
		Name:      name,
		Status:    "created",
	}

	c.writeJSON(w, http.StatusCreated, resp)
}

// healthCheck returns the health status of the service.
func (c *CDocker) healthCheck(w http.ResponseWriter, r *http.Request) {
	c.writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// writeJSON writes a JSON response.
func (c *CDocker) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	fmt.Println(data)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", slog.Any("error", err))
	}
}

// writeError writes an error response.
func (c *CDocker) writeError(w http.ResponseWriter, status int, message string, err error) {
	slog.Error(message, slog.Any("error", err))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	errResp := map[string]string{"error": message}
	if err != nil {
		errResp["details"] = err.Error()
	}

	if encErr := json.NewEncoder(w).Encode(errResp); encErr != nil {
		slog.Error("Failed to encode error response", slog.Any("error", encErr))
	}
}

// buildEnvVars builds environment variables from a config map.
func buildEnvVars(config map[string]string, defaults string) []string {
	if len(config) == 0 {
		return []string{}
	}

	envVars := make([]string, 0, len(config))
	for k, v := range config {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	return envVars
}

// buildEnvVarsFromMap builds environment variables from a nested map (for sidecar config).
func buildEnvVarsFromMap(m map[string]any) []string {
	if len(m) == 0 {
		return []string{}
	}

	envMap := make(map[string]string)
	flattenMap(m, "", envMap)

	envVars := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	fmt.Println(envVars)

	return envVars
}

// flattenMap flattens a nested map into environment variable format.
func flattenMap(m map[string]any, prefix string, result map[string]string) {
	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "_" + key
		}

		switch v := value.(type) {
		case map[string]any:
			flattenMap(v, fullKey, result)
		case []any:
			s := ""
			for i, elem := range v {
				if i > 0 {
					s += ","
				}
				s += fmt.Sprint(elem)
			}
			result[toEnvKey(fullKey)] = s
		default:
			result[toEnvKey(fullKey)] = fmt.Sprint(v)
		}
	}
}

// toEnvKey converts a key to environment variable format (uppercase with underscores).
func toEnvKey(key string) string {
	result := ""
	for i, r := range key {
		if r >= 'A' && r <= 'Z' && i > 0 {
			result += "_"
		}
		if r == '-' || r == '.' {
			result += "_"
		} else {
			result += string(r)
		}
	}
	return strings.ToUpper(result)
}
