package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"net/url"

	"github.com/prometheus/common/model"
)

// geneateURLFromLabels generates the url to scrape metrics by system labels of Prometheus.
// It generates URL to scrape by the labels named `__scheme__`, `__address__` and `__metrics_path`.
// Additionally, these configuration can be overridden by `prommux.scheme`, `prommux.address`, and `prommux.metrics_path`.
func geneateURLFromLabels(ls model.LabelSet) *url.URL {
	// get URL from Prometheus reserved labels
	scheme := string(ls[labelNameSchemeLabel])
	if scheme == "" {
		scheme = defaultScheme
	}
	host := string(ls[model.AddressLabel])
	path := string(ls[model.MetricsPathLabel])
	if path == "" {
		path = defaultMetricPath
	}

	// override URL from Prommux override labels
	if v, ok := ls[labelNameOverrideSchemeLabel]; ok {
		scheme = string(v)
	}
	if v, ok := ls[labelNameOverrideAddressLabel]; ok {
		host = string(v)
	}
	if v, ok := ls[labelNameOverrideMetricsPathLabel]; ok {
		path = string(v)
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}
	return u
}

// endpointHash generates SHA1 hash by string.
// The hash is used to subpath of reverseproxy endpoint.
func endpointHash(s string) string {
	hash := sha1.Sum([]byte(s))
	ret := hex.EncodeToString(hash[:])
	return ret
}
