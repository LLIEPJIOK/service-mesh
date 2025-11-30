package cmd

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/spf13/cobra"
)

const (
	cdockerImage     = "lliepjiok/cdocker:latest"
	cdockerContainer = "cdocker"
	cdockerPort      = 8081
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the mesh control service (cdocker)",
	Long: `Initialize and start the cdocker service which manages Docker containers via API.
This command must be run first before using other meshctl commands.
It will:
1. Create the mesh_network if it doesn't exist
2. Pull and start the cdocker container
3. Expose the API on port 8081`,
	RunE: initMesh,
}

func init() {
	initCmd.Flags().BoolP("force", "f", false, "Force restart if already running")
	rootCmd.AddCommand(initCmd)
}

func initMesh(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")

	// Check if cdocker is already running
	slog.Info("Checking if cdocker is already running...")

	checkCmd := fmt.Sprintf("docker ps -q -f name=%s", cdockerContainer)
	output, err := runWithOutput("bash", "-c", checkCmd)
	if err == nil && len(output) > 0 {
		if !force {
			slog.Info("cdocker is already running. Use --force to restart.")
			return nil
		}

		// Stop and remove existing container
		slog.Info("Stopping existing cdocker container...")
		_ = run("docker", "stop", cdockerContainer)
		_ = run("docker", "rm", cdockerContainer)
	}

	// Create the Docker network
	slog.Info("Creating network: mesh_network...")
	if err := run("docker", "network", "create", "mesh_network"); err != nil {
		slog.Warn("Network might already exist", slog.Any("error", err))
	}

	// Pull the cdocker image
	slog.Info("Pulling cdocker image...", slog.String("image", cdockerImage))
	if err := run("docker", "pull", cdockerImage); err != nil {
		slog.Warn("Failed to pull image, trying local", slog.Any("error", err))
	}

	// Run the cdocker container
	slog.Info("Starting cdocker container...")
	err = run(
		"docker", "run", "-d",
		"--name", cdockerContainer,
		"--network", "mesh_network",
		"-p", fmt.Sprintf("%d:8080", cdockerPort),
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"--label", "com.docker.compose.project=control-plane",
		"--label", "com.docker.compose.service=cdocker",
		cdockerImage,
	)
	if err != nil {
		return fmt.Errorf("failed to start cdocker: %w", err)
	}

	slog.Info("cdocker service started successfully",
		slog.String("container", cdockerContainer),
		slog.Int("port", cdockerPort))
	slog.Info("You can now use meshctl commands to manage your mesh")
	slog.Info(fmt.Sprintf("API available at http://localhost:%d", cdockerPort))

	return nil
}

// runWithOutput runs a command and returns its output
func runWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	return string(output), err
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running %s %v: %w", name, args, err)
	}

	return nil
}
