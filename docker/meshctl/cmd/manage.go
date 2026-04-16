package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [container-name]",
	Short: "Stop a container in the mesh",
	Long:  `Stop a container managed by the service mesh`,
	Args:  cobra.ExactArgs(1),
	RunE:  stopContainer,
}

var removeCmd = &cobra.Command{
	Use:   "rm [container-name]",
	Short: "Remove a container from the mesh",
	Long:  `Remove a container managed by the service mesh`,
	Args:  cobra.ExactArgs(1),
	RunE:  removeContainer,
}

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy the entire mesh including cdocker",
	Long:  `Stop and remove all containers in the mesh, including the cdocker service`,
	RunE:  destroyMesh,
}

func init() {
	removeCmd.Flags().BoolP("force", "f", false, "Force remove the container")
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(destroyCmd)
}

func stopContainer(cmd *cobra.Command, args []string) error {
	containerName := args[0]

	client := DefaultClient()

	// Check if cdocker is available
	if err := client.HealthCheck(); err != nil {
		slog.Error(
			"cdocker service is not available. Run 'meshctl init' first.",
			slog.Any("error", err),
		)
		return err
	}

	slog.Info("Stopping container...", slog.String("name", containerName))

	if err := client.StopContainer(containerName); err != nil {
		slog.Error("Failed to stop container", slog.Any("error", err))
		return err
	}

	slog.Info("Container stopped successfully", slog.String("name", containerName))
	return nil
}

func removeContainer(cmd *cobra.Command, args []string) error {
	containerName := args[0]
	force, _ := cmd.Flags().GetBool("force")

	client := DefaultClient()

	// Check if cdocker is available
	if err := client.HealthCheck(); err != nil {
		slog.Error(
			"cdocker service is not available. Run 'meshctl init' first.",
			slog.Any("error", err),
		)
		return err
	}

	slog.Info("Removing container...", slog.String("name", containerName))

	if err := client.RemoveContainer(containerName, force); err != nil {
		slog.Error("Failed to remove container", slog.Any("error", err))
		return err
	}

	slog.Info("Container removed successfully", slog.String("name", containerName))
	return nil
}

func destroyMesh(cmd *cobra.Command, args []string) error {
	slog.Info("Destroying mesh...")

	// Try to list and remove all mesh containers via API first
	client := DefaultClient()
	if err := client.HealthCheck(); err == nil {
		services, err := client.ListContainers()
		if err == nil {
			uniqueServices := make(map[string]struct{})
			for _, s := range services {
				uniqueServices[s.ServiceName] = struct{}{}
			}

			for s := range uniqueServices {
				slog.Info("Removing service...", slog.String("name", s))
				_ = client.StopContainer(s)
				_ = client.RemoveContainer(s, true)
			}
		}
	}

	// Stop and remove cdocker
	slog.Info("Stopping cdocker service...")
	_ = run("docker", "stop", cdockerContainer)
	_ = run("docker", "rm", cdockerContainer)

	// Remove network
	slog.Info("Removing mesh network...")
	_ = run("docker", "network", "rm", "mesh_network")

	slog.Info("Mesh destroyed successfully")
	return nil
}
