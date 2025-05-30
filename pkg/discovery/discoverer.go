package discovery

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/moby"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"context"
)

// Discoverer is to get information of Docker containers from Docker API.
type Discoverer struct {
	logger             *slog.Logger
	host               string
	port               int
	filter             []moby.Filter
	interval           time.Duration
	hostNetworkingHost string
	ch                 chan<- []*targetgroup.Group
}

// NewDiscover instantinates discoverer and returns it.
func NewDiscoverer(
	logger *slog.Logger,
	host string,
	port int,
	filter []moby.Filter,
	interval time.Duration,
	hostNetworkingHost string,
	ch chan<- []*targetgroup.Group,
) *Discoverer {
	return &Discoverer{
		logger:             logger,
		host:               host,
		port:               port,
		filter:             filter,
		interval:           interval,
		hostNetworkingHost: hostNetworkingHost,
		ch:                 ch,
	}
}

// Run runs initialize docker discoverer and let it run.
func (d *Discoverer) Run(ctx context.Context) error {
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	cfg := moby.DefaultDockerSDConfig
	cfg.Host = d.host
	cfg.Port = d.port
	cfg.RefreshInterval = model.Duration(d.interval)
	cfg.Filters = d.filter
	if d.hostNetworkingHost != "" {
		cfg.HostNetworkingHost = d.hostNetworkingHost
	}
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		return fmt.Errorf("could not register service discovery metrics: %w", err)
	}

	discoverer, err := cfg.NewDiscoverer(discovery.DiscovererOptions{Logger: d.logger, Metrics: metrics})
	if err != nil {
		return fmt.Errorf("could not create Discoverer: %w", err)
	}

	defer func() {
		metrics.Unregister()
		refreshMetrics.Unregister()
	}()
	discoverer.Run(ctx, d.ch)
	return nil
}
