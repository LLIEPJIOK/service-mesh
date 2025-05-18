package cmd

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/spf13/cobra"
)

var planeCmd = &cobra.Command{
	Use:   "plane",
	Short: "Deploy control plane in docker",
	Long:  `Deploy lliepjiok/control-plane in docker`,
	RunE:  mesh,
}

func init() {
	rootCmd.AddCommand(planeCmd)
}

func mesh(cmd *cobra.Command, args []string) error {
	// Create the Docker network
	slog.Info("Creating network: mesh_network...")
	if err := run("docker", "network", "create", "mesh_network"); err != nil {
		slog.Warn("Failed to create network", slog.Any("error", err))
	}

	// Run the 'control-plane' container
	slog.Info("Starting container: control plane...")
	err := run(
		"docker", "run", "-d",
		"--name", "control-plane",
		"--network", "mesh_network",
		"-p", "8080:8080",
		"--env-file", ".env",
		"--label", "com.docker.compose.project=control-plane",
		"--label", "com.docker.compose.service=control-plane",
		"lliepjiok/control-plane:latest",
	)
	if err != nil {
		return fmt.Errorf("error running mesh: %w", err)
	}

	slog.Info("Control plane was successfully up")

	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running %s %v: %w", name, args, err)
	}

	return nil
}
