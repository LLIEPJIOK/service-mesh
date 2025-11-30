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
	RunE:  deployMonitoringCmd,
}

func init() {
	promCmd.Flags().StringP("config", "c", "", "Config for prometheus")
	promCmd.Flags().String("grafana-user", "admin", "Grafana admin username")
	promCmd.Flags().String("grafana-password", "admin", "Grafana admin password")
	promCmd.Flags().Bool("direct", false, "Use direct docker commands instead of cdocker API")
	rootCmd.AddCommand(promCmd)
}

func deployMonitoringCmd(cmd *cobra.Command, args []string) error {
	direct, _ := cmd.Flags().GetBool("direct")

	if direct {
		return deployMonitoringDirect(cmd, args)
	}

	return deployMonitoringViaAPI(cmd, args)
}

func deployMonitoringViaAPI(cmd *cobra.Command, args []string) error {
	client := DefaultClient()

	// Check if cdocker is available
	if err := client.HealthCheck(); err != nil {
		slog.Error("cdocker service is not available. Run 'meshctl init' first.", slog.Any("error", err))
		return err
	}

	config := cmd.Flag("config").Value.String()
	grafanaUser, _ := cmd.Flags().GetString("grafana-user")
	grafanaPassword, _ := cmd.Flags().GetString("grafana-password")

	var absConfig string
	if config != "" {
		var err error
		absConfig, err = filepath.Abs(config)
		if err != nil {
			return fmt.Errorf("failed to get absolute path of config: %w", err)
		}

		if _, err := os.Stat(absConfig); err != nil {
			return fmt.Errorf("prometheus config file does not exist: %w", err)
		}
	}

	slog.Info("Deploying monitoring stack via cdocker API...")

	resp, err := client.DeployMonitoring(DeployMonitoringRequest{
		PrometheusConfig: absConfig,
		GrafanaUser:      grafanaUser,
		GrafanaPassword:  grafanaPassword,
	})
	if err != nil {
		slog.Error("Failed to deploy monitoring", slog.Any("error", err))
		return err
	}

	slog.Info("Monitoring stack deployed successfully",
		slog.String("prometheus_id", resp.PrometheusID),
		slog.String("grafana_id", resp.GrafanaID),
		slog.Int("prometheus_port", resp.PrometheusPort),
		slog.Int("grafana_port", resp.GrafanaPort),
		slog.String("status", resp.Status))

	slog.Info(fmt.Sprintf("Prometheus available at http://localhost:%d", resp.PrometheusPort))
	slog.Info(fmt.Sprintf("Grafana available at http://localhost:%d (user: %s)", resp.GrafanaPort, grafanaUser))

	return nil
}

func deployMonitoringDirect(cmd *cobra.Command, args []string) error {
	config := cmd.Flag("config").Value.String()

	absConfig, err := filepath.Abs(config)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of config: %w", err)
	}

	if _, err := os.Stat(absConfig); err != nil {
		return fmt.Errorf("prometheus config file does not exist: %w", err)
	}

	grafanaUser, _ := cmd.Flags().GetString("grafana-user")
	grafanaPassword, _ := cmd.Flags().GetString("grafana-password")

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
		"-e", "GF_SECURITY_ADMIN_USER="+grafanaUser,
		"-e", "GF_SECURITY_ADMIN_PASSWORD="+grafanaPassword,
		"-v", "grafana-storage:/var/lib/grafana",
		"--label", "com.docker.compose.project=control-plane",
		"--label", "com.docker.compose.service=grafana",
		"grafana/grafana:latest"); err != nil {
		return fmt.Errorf("error starting Grafana: %w", err)
	}

	slog.Info("Monitoring stack is up")

	return nil
}
