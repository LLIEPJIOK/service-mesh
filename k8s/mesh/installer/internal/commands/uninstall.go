package commands

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/adapters/kube"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/app/uninstaller"
	"github.com/LLIEPJIOK/service-mesh/installer/internal/domain"
)

func newUninstallCommand() *cobra.Command {
	var (
		namespace       string
		deleteNamespace bool
		kubeconfig      string
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall service mesh resources from the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := log.New(os.Stdout, "mesh uninstall ", log.LstdFlags|log.LUTC)

			kubeClient, err := kube.NewClient(kubeconfig, logger)
			if err != nil {
				return err
			}

			service := uninstaller.NewService(kubeClient, logger)
			opts := domain.UninstallOptions{
				Namespace:       namespace,
				DeleteNamespace: deleteNamespace,
				Kubeconfig:      kubeconfig,
			}

			return service.Uninstall(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", domain.DefaultNamespace, "Namespace for mesh system components")
	cmd.Flags().BoolVar(&deleteNamespace, "delete-namespace", false, "Delete namespace after removing mesh resources")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")

	return cmd
}
