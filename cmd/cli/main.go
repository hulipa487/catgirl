package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath string
)

var rootCmd = &cobra.Command{
	Use:   "catgirl-cli",
	Short: "Catgirl CLI tool",
}

func main() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/catgirl.conf", "Path to configuration file")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
