package main

import (
	"fmt"
	"os"

	"github.com/LLIEPJIOK/service-mesh/installer/internal/commands"
)

var version = "dev"

func main() {
	rootCmd := commands.NewRootCommand(version)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
