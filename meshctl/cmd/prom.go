package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var promCmd = &cobra.Command{
	Use:   "prom",
	Short: "Deploy Prometheus and Grafana in Docker with default settings",
	Long:  `Запускает два контейнера: Prometheus и Grafana, с базовой настройкой портов и томом для хранения данных Grafana.`,
	RunE:  deployMonitoring,
}

func init() {
	promCmd.Flags().StringP("config", "c", "", "Config for prometheus")
	rootCmd.AddCommand(promCmd)
}

func deployMonitoring(cmd *cobra.Command, args []string) error {
	config := cmd.Flag("config").Value.String()

	absConfig, err := filepath.Abs(config)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of config: %w", err)
	}

	if _, err := os.Stat(absConfig); err != nil {
		return fmt.Errorf("prometheus config file does not exist: %w", err)
	}

	// 1. Запускаем Prometheus
	slog.Info("Starting Prometheus container...")
	if err := run("docker", "run", "-d",
		"--name", "prometheus",
		"--network", "mesh_network",
		"-p", "9090:9090",
		"-v", absConfig+":/etc/prometheus/prometheus.yml",
		"--label", "com.docker.compose.project=control-plane",
		"--label", "com.docker.compose.service=prometheus",
		"prom/prometheus:latest"); err != nil {
		return fmt.Errorf("error starting Prometheus: %w", err)
	}

	// 2. Запускаем Grafana
	slog.Info("Starting Grafana container...")
	if err := run("docker", "run", "-d",
		"--name", "grafana",
		"--network", "mesh_network",
		"-p", "3000:3000",
		"-e", "GF_SECURITY_ADMIN_USER=admin",
		"-e", "GF_SECURITY_ADMIN_PASSWORD=admin",
		"-v", "grafana-storage:/var/lib/grafana",
		"--label", "com.docker.compose.project=control-plane",
		"--label", "com.docker.compose.service=grafana",
		"grafana/grafana:latest"); err != nil {
		return fmt.Errorf("error starting Grafana: %w", err)
	}

	slog.Info("Monitoring stack is up")

	return nil
}
