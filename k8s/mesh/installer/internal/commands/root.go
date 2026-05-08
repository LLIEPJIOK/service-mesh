package commands

import (
	"github.com/spf13/cobra"
)

func NewRootCommand(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "mesh",
		Short:         "Mesh CLI manages the service mesh lifecycle",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInstallCommand(version))
	rootCmd.AddCommand(newUninstallCommand())
	rootCmd.AddCommand(newVersionCommand(version))

	return rootCmd
}

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print mesh CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(version)
		},
	}
}
