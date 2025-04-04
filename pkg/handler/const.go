package handler

import "github.com/prometheus/common/model"

var (
	labelNameSchemeLabel              = model.LabelName(model.SchemeLabel)
	labelNameAddressLabel             = model.LabelName(model.AddressLabel)
	labelNameMetricsPathLabel         = model.LabelName(model.MetricsPathLabel)
	labelNameOverrideSchemeLabel      = model.LabelName(overrideLabelPrefix + overrideLabelScheme)
	labelNameOverrideAddressLabel     = model.LabelName(overrideLabelPrefix + overrideLabelAddress)
	labelNameOverrideMetricsPathLabel = model.LabelName(overrideLabelPrefix + overrideLabelMetricPath)
)

const (
	// defaultMetricPath is the default path for scraping by Prometheus.
	defaultMetricPath = "/metrics"
	// defaultScheme is the default scheme for scraping by Prometheus.
	defaultScheme = "http"
	// overrideLabelPrefix is a prefix to override configuration to scrape metrics.
	overrideLabelPrefix = "__meta_docker_container_label_prommux_"
	// overrideLabelAddress is the name of label to override address to scrape.
	overrideLabelAddress = "address"
	// overrideLabelScheme is the name of label to override scheme to scrape.
	overrideLabelScheme = "scheme"
	// overrideLabelMetricPath is the name of label to override metric path to scrape.
	overrideLabelMetricPath = "metrics_path"
)
