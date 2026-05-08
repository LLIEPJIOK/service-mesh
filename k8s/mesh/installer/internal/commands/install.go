package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/adapters/kube"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/app/installer"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/config"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/domain"
)

func newInstallCommand(version string) *cobra.Command {
	var (
		configFile string
		namespace  string
		waitReady  bool
		timeoutRaw string
		dryRun     bool
		kubeconfig string
	)

	cmd := &cobra.Command{
		Use:   "install -f CONFIG",
		Short: "Install service mesh into the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadFromFile(configFile)
			if err != nil {
				return err
			}

			timeout, err := time.ParseDuration(timeoutRaw)
			if err != nil {
				return fmt.Errorf("parse --timeout: %w", err)
			}

			logger := log.New(os.Stdout, "mesh install ", log.LstdFlags|log.LUTC)
			opts := domain.InstallOptions{
				ConfigFile: configFile,
				Namespace:  namespace,
				Wait:       waitReady,
				Timeout:    timeout,
				DryRun:     dryRun,
				Kubeconfig: kubeconfig,
				CLIVersion: version,
			}

			if dryRun {
				service := installer.NewService(nil, logger)
				return service.Install(context.Background(), cfg, opts)
			}

			kubeClient, err := kube.NewClient(kubeconfig, logger)
			if err != nil {
				return err
			}

			service := installer.NewService(kubeClient, logger)
			return service.Install(cmd.Context(), cfg, opts)
		},
	}

	cmd.Flags().StringVarP(&configFile, "file", "f", "", "Path to mesh config YAML (required)")
	_ = cmd.MarkFlagRequired("file")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", domain.DefaultNamespace, "Namespace for mesh system components")
	cmd.Flags().BoolVar(&waitReady, "wait", true, "Wait for critical components to become ready")
	cmd.Flags().StringVar(&timeoutRaw, "timeout", "5m", "Timeout for readiness wait")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Render install plan without applying resources")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")

	return cmd
}
