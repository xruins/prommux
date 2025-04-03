package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "prommux",
	Short: "A server for HTTP service-discovery and reverse-proxy for Prometheus exporters",
}
