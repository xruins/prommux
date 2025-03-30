package pkg

import "github.com/prometheus/client_golang/prometheus"

const (
	metricsPrefix = "prommux_"
)

var (
	proxiedEndpointsCountMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: metricsPrefix + "proxied_endpoints_count",
			Help: "Number of proxied exporter endpoints",
		},
	)
	proxiedEndpointsHTTPStatusMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricsPrefix + "http_requests_total",
			Help: "Counter of requests made to the HTTP endpoints.",
		},
		[]string{"code", "handler", "method", "path"},
	)
	discoveryLastReloadSuccessfulMetrics = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: metricsPrefix + "prommux_discovery_updated_count_successful",
			Help: "Whether the last reload of Docker discovery succeeded or not",
		},
	)
)

func init() {
	prometheus.MustRegister(
		proxiedEndpointsCountMetrics,
		proxiedEndpointsHTTPStatusMetrics,
		discoveryLastReloadSuccessfulMetrics,
	)
}
