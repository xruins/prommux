package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/prometheus/discovery/moby"
	"github.com/spf13/cobra"
	"github.com/xruins/prommux/pkg/handler"
	alogger "github.com/xruins/prommux/pkg/logger"
)

// filterStringToMobyFilter converts string into the struct used by Docker API.
func filterStringToMobyFilter(s string) ([]moby.Filter, error) {
	if s == "" {
		return nil, nil
	}

	intermediateMap := make(map[string][]string)
	err := json.Unmarshal([]byte(s), &intermediateMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert parameter from JSON: %w", err)
	}

	ret := make([]moby.Filter, 0, len(intermediateMap))
	for k, v := range intermediateMap {
		ret = append(ret, moby.Filter{
			Name:   k,
			Values: v,
		})
	}
	return ret, nil
}

func setLogLevel(level string) (slog.Level, error) {
	s := strings.ToLower(level)
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("invalid log level. (candidates: error, warn, info, debug)")
	}
}

// serverCmd represents the base command when called without any subcommands
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Launch server",
	Long:  "Launch the server for HTTP service-discovery and reverse-proxy for Prometheus exporters",
	RunE: func(cmd *cobra.Command, args []string) error {
		level, err := setLogLevel(logLevel)
		if err != nil {
			return fmt.Errorf("failed to set log level: %w", err)
		}

		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

		mobyFilter, err := filterStringToMobyFilter(filter)
		if err != nil {
			return fmt.Errorf("failed to convert the value of `filter`: %w", err)
		}

		params := &handler.HandlerParams{
			Logger:           *logger,
			ProxyTimeout:     proxyTimeout,
			AdditionalLabels: additionalLabels,
			DiscovererParams: &handler.DiscovererParams{
				Host:                dockerAddress,
				Port:                dockerPort,
				DiscovererTimeout:   discoverTimeout,
				IncludeDockerLabels: includeDockerLabels,
				RegexpDockerLabels:  regexpDockerLabels,
				RefreshInterval:     dockerRefreshInterval,
				HostNetworkingHost:  hostNetworkingHost,
				Filter:              mobyFilter,
			},
		}
		r, err := handler.NewHandler(params)
		if err != nil {
			return fmt.Errorf("failed to initialize handler: %w", err)
		}
		ctx := context.Background()
		runErrCh := make(chan error, 1)
		go func() {
			err := r.Run(ctx)
			if err != nil {
				runErrCh <- fmt.Errorf("background task exited with an error: %w", err)
			}
		}()
		serverErrCh := make(chan error, 1)
		mux := http.NewServeMux()
		mux.Handle("/", r.NewRouter())

		handler := alogger.AccessLogger(mux, *logger)
		server := &http.Server{Addr: fmt.Sprintf("%s:%d", bindAddress, port), Handler: handler}
		go func() {
			err := server.ListenAndServe()
			if err != nil {
				serverErrCh <- fmt.Errorf("background task exited with an error: %w", err)
			}
		}()

		logger.Info("prommux is ready to serve", "address", fmt.Sprintf("%s:%d", bindAddress, port))
		select {
		case err := <-runErrCh:
			return err
		case err := <-serverErrCh:
			return err
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		slog.Default().Error("an error occured", "error", err)
		os.Exit(1)
	}
}

var (
	port, dockerPort int
	bindAddress, dockerAddress,
	regexpDockerLabels, filter,
	logLevel, additionalLabels,
	hostNetworkingHost string
	includeDockerLabels                                  bool
	dockerRefreshInterval, discoverTimeout, proxyTimeout time.Duration
)

func init() {
	serverCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "the severity for logging (error, info, warn, debug)")
	serverCmd.Flags().StringVarP(&dockerAddress, "docker-address", "d", "unix:///var/run/docker.sock", "the address for Docker API")
	serverCmd.Flags().IntVarP(&dockerPort, "docker-port", "", 8080, "the port for Docker API")
	serverCmd.Flags().StringVarP(&bindAddress, "bind-address", "b", "0.0.0.0", "the address listening on")
	serverCmd.Flags().IntVarP(&port, "port", "p", 11298, "the port listening on")
	serverCmd.Flags().DurationVarP(&dockerRefreshInterval, "docker-refresh-interval", "", 30*time.Second, "the interval to poll Docker API")
	serverCmd.Flags().DurationVarP(&discoverTimeout, "discover-timeout", "o", 30*time.Second, "timeout of discovery endpoint")
	serverCmd.Flags().DurationVarP(&proxyTimeout, "proxy-timeout", "t", 30*time.Second, "timeout of reverse-proxy endpoint")
	serverCmd.Flags().BoolVarP(&includeDockerLabels, "include-labels", "i", false, "whether the labels retrieved by docker API on discover endpoint response")
	serverCmd.Flags().StringVarP(&regexpDockerLabels, "regexp-labels", "r", "", "regexp to filter Docker labels. must be used with --include-labels(-i) switch.")
	serverCmd.Flags().StringVarP(&filter, "filter", "f", "", "filter output based on conditions provided. see https://docs.docker.com/reference/api/engine/version/v1.40/#tag/Container for the format.")
	serverCmd.Flags().StringVarP(&additionalLabels, "additional-labels", "a", "", "labels to append on `labels` field of discover API response. must be key-value pair in JSON.")
	serverCmd.Flags().StringVar(&hostNetworkingHost, "host-networking-host", "", "`HostNetworkingHost` value of Docker Service Discovery config")
	rootCmd.AddCommand(serverCmd)
}
