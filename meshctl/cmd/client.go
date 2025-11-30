package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func DefaultClient() *Client {
	return NewClient(fmt.Sprintf("http://localhost:%d", cdockerPort))
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

type ListServicesResponse map[string]ServiceInfo

type StopContainerRequest struct {
	Name string `json:"name"`
}

type RemoveContainerRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

type APIError struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type DeployMonitoringRequest struct {
	PrometheusConfig string `json:"prometheus_config,omitempty"`
	GrafanaUser      string `json:"grafana_user,omitempty"`
	GrafanaPassword  string `json:"grafana_password,omitempty"`
}

type DeployMonitoringResponse struct {
	PrometheusID   string `json:"prometheus_id"`
	GrafanaID      string `json:"grafana_id"`
	PrometheusPort int    `json:"prometheus_port"`
	GrafanaPort    int    `json:"grafana_port"`
	Status         string `json:"status"`
}

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

type PortBinding struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func (c *Client) DeployMonitoring(req DeployMonitoringRequest) (*DeployMonitoringResponse, error) {
	var resp DeployMonitoringResponse
	if err := c.post("/monitoring", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListServices() (ListServicesResponse, error) {
	var resp ListServicesResponse

	if err := c.get("/containers", &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) StopContainer(name string) error {
	req := StopContainerRequest{Name: name}
	return c.post("/containers/stop", req, nil)
}

func (c *Client) RemoveContainer(name string, force bool) error {
	req := RemoveContainerRequest{Name: name, Force: force}
	return c.post("/containers/remove", req, nil)
}

func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("cdocker service is not available: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cdocker service returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) ApplyManifest(yamlContent io.Reader) error {
	resp, err := c.httpClient.Post(c.baseURL+"/apply", "application/x-yaml", yamlContent)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError

		if err := json.Unmarshal(respBody, &apiErr); err == nil {
			return fmt.Errorf("%s: %s", apiErr.Error, apiErr.Details)
		}

		return fmt.Errorf(
			"request failed with status %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}

	return nil
}

// post performs a POST request to the API.
func (c *Client) post(path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil {
			return fmt.Errorf("%s: %s", apiErr.Error, apiErr.Details)
		}
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// get performs a GET request to the API.
func (c *Client) get(path string, result any) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil {
			return fmt.Errorf("%s: %s", apiErr.Error, apiErr.Details)
		}
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}
