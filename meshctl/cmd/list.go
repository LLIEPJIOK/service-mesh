package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all containers in the mesh",
	Long:  `List all containers managed by the service mesh`,
	RunE:  listContainers,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func listContainers(cmd *cobra.Command, args []string) error {
	client := DefaultClient()

	if err := client.HealthCheck(); err != nil {
		slog.Error(
			"cdocker service is not available. Run 'meshctl init' first.",
			slog.Any("error", err),
		)
		return err
	}

	services, err := client.ListServices()
	if err != nil {
		slog.Error("Failed to list containers", slog.Any("error", err))
		return err
	}

	if len(services) == 0 {
		fmt.Println("No containers found in the mesh")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tInstances\tSTATUS\t")
	fmt.Fprintln(w, "----\t---------\t------\t")

	for _, c := range services {
		fmt.Fprintf(w, "%s\t%d\t%s\n",
			c.Name, len(c.Instances), c.Status)
	}

	w.Flush()

	return nil
}
