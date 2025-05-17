package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy application in docker",
	Long:  `Deploy application in docker with service mesh before him. All requests will be validated with config`,
	RunE:  deploy,
}

func init() {
	deployCmd.Flags().StringP("name", "n", "", "Name of application")
	deployCmd.Flags().StringP("dockerfile", "d", "", "Path to dockerfile file")
	rootCmd.AddCommand(deployCmd)
}

func deploy(cmd *cobra.Command, args []string) error {
	dockerfile := cmd.Flag("dockerfile").Value.String()
	name := getAppName(cmd)

	// Run the 'sidecar' container
	slog.Info("Starting container: sidecar...")
	err := run(
		"docker", "run", "-d",
		"--name", name+"-sidecar",
		"--network", "mesh_network",
		"--env-file", ".env",
		"-e", fmt.Sprintf("PROXY_TARGET=%s:8080", name),
		"-e", "PROXY_SERVICE_NAME="+name,
		"--label", "com.docker.compose.project=myproject",
		"--label", "com.docker.compose.service=mesh",
		"lliepjiok/sidecar:latest",
	)
	if err != nil {
		return fmt.Errorf("error running mesh: %w", err)
	}

	// Build the image
	slog.Info("Building image...")
	if err := run("docker", "build", "-t", name, dockerfile); err != nil {
		return fmt.Errorf("error building image: %w", err)
	}

	// Run the container
	slog.Info("Starting container...")
	err = run(
		"docker", "run", "-d",
		"--name", name,
		"--network", "mesh_network",
		"--env-file", ".env",
		"-e", fmt.Sprintf("HTTP_PROXY=http://%s-sidecar:8080", name),
		"-e", fmt.Sprintf("HTTPS_PROXY=http://%s-sidecar:8080", name),
		"--label", "com.docker.compose.project=myproject",
		"--label", "com.docker.compose.service="+name,
		name,
	)
	if err != nil {
		return fmt.Errorf("error running app: %w", err)
	}

	// register service
	resp, err := http.DefaultClient.Post(
		"http://localhost:8080/register",
		"application/json",
		strings.NewReader(fmt.Sprintf(`{"name":%q,"address":"%s-sidecar:8080"}`, name, name)),
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
		return NewErrInvalidCode(resp.StatusCode)
	}

	slog.Info("Application was successfully up")

	return nil
}

func getAppName(cmd *cobra.Command) string {
	name := cmd.Flag("name").Value.String()
	if name == "" {
		name = getUUID()
	}

	return name
}

func getUUID() string {
	u, err := uuid.NewV7()
	if err != nil {
		u = uuid.New()
	}

	return u.String()
}
