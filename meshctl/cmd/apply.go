package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply -f <manifest.yaml>",
	Short: "Apply a manifest file to deploy resources",
	Long: `Apply a YAML manifest file to deploy resources via cdocker.

Supported manifest kinds:
  - Service: Deploy an application with sidecar

Examples:
  meshctl apply -f service.yaml`,
	RunE: apply,
}

func init() {
	applyCmd.Flags().StringP("file", "f", "", "Path to manifest file (required)")
	applyCmd.MarkFlagRequired("file")
	rootCmd.AddCommand(applyCmd)
}

func apply(cmd *cobra.Command, args []string) error {
	filePath := cmd.Flag("file").Value.String()
	client := DefaultClient()

	if err := client.HealthCheck(); err != nil {
		slog.Error(
			"cdocker service is not available. Run 'meshctl init' first.",
			slog.Any("error", err),
		)

		return err
	}

	data, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer data.Close()

	if err := client.ApplyManifest(data); err != nil {
		slog.Error("Failed to apply manifest", slog.Any("error", err))
		return err
	}

	return nil
}
