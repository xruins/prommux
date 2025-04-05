package handler

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"text/template"

	"github.com/prometheus/common/model"
)

type OverrideLabelsTemplateParams struct {
	OriginalHost, OriginalPort, OriginalMetricsPath string
}

var (
	overrideLabelsTemplate = template.New("override_labels_template")
)

func applyOverrideLabelsTemplate(s string, param *OverrideLabelsTemplateParams) (string, error) {
	out := new(bytes.Buffer)
	tmpl, err := overrideLabelsTemplate.Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse template for override Labels: %w", err)
	}
	err = tmpl.Execute(out, *param)
	if err != nil {
		return "", fmt.Errorf("failed to execute template for override labels: %w", err)
	}

	return out.String(), nil
}

// geneateURLFromLabels generates the url to scrape metrics by system labels of Prometheus.
// It generates URL to scrape by the labels named `__scheme__`, `__address__` and `__metrics_path`.
// Additionally, these configuration can be overridden by `prommux.scheme`, `prommux.address`, and `prommux.metrics_path`.
func geneateURLFromLabels(ls model.LabelSet) (*url.URL, error) {
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

	originalHost, originalPort, err := net.SplitHostPort(host)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to split `%s` label into host and port: %w",
			model.AddressLabel,
			err,
		)
	}

	params := &OverrideLabelsTemplateParams{
		OriginalHost:        originalHost,
		OriginalPort:        originalPort,
		OriginalMetricsPath: string(path),
	}

	// override URL from Prommux override labels
	if v, ok := ls[labelNameOverrideSchemeLabel]; ok {
		scheme, err = applyOverrideLabelsTemplate(string(v), params)
		if err != nil {
			return nil, fmt.Errorf("failed to apply template for `scheme`: %w", err)
		}
	}
	if v, ok := ls[labelNameOverrideAddressLabel]; ok {
		host, err = applyOverrideLabelsTemplate(string(v), params)
		if err != nil {
			return nil, fmt.Errorf("failed to apply template for `host`: %w", err)
		}
	}
	if v, ok := ls[labelNameOverrideMetricsPathLabel]; ok {
		path, err = applyOverrideLabelsTemplate(string(v), params)
		if err != nil {
			return nil, fmt.Errorf("failed to apply template for `path`: %w", err)
		}
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}
	return u, nil
}

// endpointHash generates SHA1 hash by string.
// The hash is used to subpath of reverseproxy endpoint.
func endpointHash(s string) string {
	hash := sha1.Sum([]byte(s))
	ret := hex.EncodeToString(hash[:])
	return ret
}
