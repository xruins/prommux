package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var healthCheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "A server for HTTP service-discovery and reverse-proxy for Prometheus exporters",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		level, err := setLogLevel(logLevel)
		if err != nil {
			return fmt.Errorf("failed to set log level: %w", err)
		}

		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

		client := http.Client{
			Timeout: healthcheckTimeout,
		}
		var u *url.URL
		if paramURL != "" {
			u, err = url.Parse(paramURL)
			if err != nil {
				logger.Error("failed to parse url option", "error", err)
				return err
			}
		} else {
			u = &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", bindAddress, port),
				Path:   "/-/health",
			}
		}
		logger.DebugContext(ctx, "generated request URL", slog.String("url", u.String()))
		resp, err := client.Get(u.String())
		if err != nil {
			logger.Error("failed to request healthcheck API", "error", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Error("the healthcheck API returned non-OK status code", slog.Int("code", resp.StatusCode))
			return err
		}
		logger.Info("healthcheck passed")
		return nil
	},
}

var (
	paramURL           string
	healthcheckTimeout time.Duration
)

func init() {
	healthCheckCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "the severity for logging (error, info, warn, debug)")
	healthCheckCmd.Flags().StringVarP(&paramURL, "url", "u", "", "the url to check health on. if specified, `-h` and `-p` options will be ignored.")
	healthCheckCmd.Flags().StringVarP(&bindAddress, "address", "a", "127.0.0.1", "the address to check health on")
	healthCheckCmd.Flags().IntVarP(&port, "port", "p", 11298, "the port to check health on")
	healthCheckCmd.Flags().DurationVarP(&healthcheckTimeout, "timeout", "t", 30*time.Second, "the timeout to poll health")
	rootCmd.AddCommand(healthCheckCmd)
}
