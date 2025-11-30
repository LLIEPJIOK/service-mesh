package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/domain"
)

// Client wraps the Docker client for container management.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// Close closes the Docker client connection.
func (c *Client) Close() error {
	return c.cli.Close()
}

// CreateNetwork creates a Docker network if it doesn't exist.
func (c *Client) CreateNetwork(ctx context.Context, name string) (string, error) {
	// Check if network already exists
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == name {
			slog.Info("Network already exists", slog.String("name", name))
			return n.ID, nil
		}
	}

	// Create the network
	resp, err := c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create network: %w", err)
	}

	slog.Info("Network created", slog.String("name", name), slog.String("id", resp.ID))

	return resp.ID, nil
}

// ImageExists checks if an image exists locally.
func (c *Client) ImageExists(ctx context.Context, imageName string) bool {
	_, _, err := c.cli.ImageInspectWithRaw(ctx, imageName)
	return err == nil
}

func (c *Client) PullImage(ctx context.Context, imageName string) error {
	slog.Info("Pulling image", slog.String("image", imageName))

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull output: %w", err)
	}

	slog.Info("Image pulled successfully", slog.String("image", imageName))

	return nil
}

// BuildImage builds a Docker image from a Dockerfile path.
func (c *Client) BuildImage(ctx context.Context, dockerfilePath, tag string) (string, error) {
	slog.Info("Building image", slog.String("dockerfile", dockerfilePath), slog.String("tag", tag))

	// Create a tar archive with the build context
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	// For now, we'll just use docker build command approach
	// In a real implementation, you'd need to read the Dockerfile and context
	tw.Close()

	resp, err := c.cli.ImageBuild(ctx, buf, types.ImageBuildOptions{
		Dockerfile: dockerfilePath,
		Tags:       []string{tag},
		Remove:     true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	// Read the build output
	var lastMessage struct {
		Stream string `json:"stream"`
		Aux    struct {
			ID string `json:"ID"`
		} `json:"aux"`
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		if err := decoder.Decode(&lastMessage); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("failed to decode build output: %w", err)
		}
	}

	slog.Info("Image built successfully", slog.String("tag", tag))

	return lastMessage.Aux.ID, nil
}

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name        string
	Image       string
	Env         []string
	Network     string
	Labels      map[string]string
	PortMapping map[int]int // container port -> host port
	Volumes     map[string]string
}

// CreateAndStartContainer creates and starts a Docker container.
func (c *Client) CreateAndStartContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	slog.Info("Creating container", slog.String("name", cfg.Name), slog.String("image", cfg.Image))

	// Build port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	for containerPort, hostPort := range cfg.PortMapping {
		port := nat.Port(fmt.Sprintf("%d/tcp", containerPort))
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", hostPort),
			},
		}
	}

	// Build volume binds
	var binds []string
	for hostPath, containerPath := range cfg.Volumes {
		binds = append(binds, fmt.Sprintf("%s:%s", hostPath, containerPath))
	}

	// Create container
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        cfg.Image,
			Env:          cfg.Env,
			Labels:       cfg.Labels,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			PortBindings: portBindings,
			Binds:        binds,
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				cfg.Network: {},
			},
		},
		nil,
		cfg.Name,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	slog.Info("Container started", slog.String("name", cfg.Name), slog.String("id", resp.ID))

	return resp.ID, nil
}

// StopContainer stops a container by name or ID.
func (c *Client) StopContainer(ctx context.Context, nameOrID string) error {
	slog.Info("Stopping container", slog.String("container", nameOrID))

	if err := c.cli.ContainerStop(ctx, nameOrID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	slog.Info("Container stopped", slog.String("container", nameOrID))

	return nil
}

// RemoveContainer removes a container by name or ID.
func (c *Client) RemoveContainer(ctx context.Context, nameOrID string, force bool) error {
	slog.Info("Removing container", slog.String("container", nameOrID))

	if err := c.cli.ContainerRemove(ctx, nameOrID, container.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	slog.Info("Container removed", slog.String("container", nameOrID))

	return nil
}

// ListContainers returns a list of containers filtered by project label.
func (c *Client) ListContainers(
	ctx context.Context,
	project string,
) ([]domain.ContainerInfo, error) {
	filterArgs := filters.NewArgs()
	if project != "" {
		filterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", project))
	}

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]domain.ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		ports := make([]domain.PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, domain.PortBinding{
				HostPort:      int(p.PublicPort),
				ContainerPort: int(p.PrivatePort),
				Protocol:      p.Type,
			})
		}

		networkName := ""
		for netName := range c.NetworkSettings.Networks {
			networkName = netName
			break
		}

		result = append(result, domain.ContainerInfo{
			ID:      c.ID[:12],
			Name:    name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Ports:   ports,
			Labels:  c.Labels,
			Network: networkName,
		})
	}

	return result, nil
}

// ContainerExists checks if a container with the given name exists.
func (c *Client) ContainerExists(ctx context.Context, name string) (bool, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w", err)
	}

	for _, cont := range containers {
		for _, n := range cont.Names {
			if strings.TrimPrefix(n, "/") == name {
				return true, nil
			}
		}
	}

	return false, nil
}

// GetContainerLogs returns the logs for a container.
func (c *Client) GetContainerLogs(
	ctx context.Context,
	nameOrID string,
	tail string,
) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	}

	reader, err := c.cli.ContainerLogs(ctx, nameOrID, opts)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return buf.String(), nil
}
