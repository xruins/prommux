package handler

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestGeneateURLFromLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   model.LabelSet
		expected string
	}{
		{
			name: "Default labels",
			labels: model.LabelSet{
				labelNameSchemeLabel:   "http",
				model.AddressLabel:     "example.com:9090",
				model.MetricsPathLabel: "/metrics",
			},
			expected: "http://example.com:9090/metrics",
		},
		{
			name: "Override scheme",
			labels: model.LabelSet{
				labelNameSchemeLabel:         "http",
				model.AddressLabel:           "example.com:9090",
				model.MetricsPathLabel:       "/metrics",
				labelNameOverrideSchemeLabel: "https",
			},
			expected: "https://example.com:9090/metrics",
		},
		{
			name: "Override address",
			labels: model.LabelSet{
				labelNameSchemeLabel:          "http",
				model.AddressLabel:            "example.com:9090",
				model.MetricsPathLabel:        "/metrics",
				labelNameOverrideAddressLabel: "override.com:8080",
			},
			expected: "http://override.com:8080/metrics",
		},
		{
			name: "Override metrics path",
			labels: model.LabelSet{
				labelNameSchemeLabel:              "http",
				model.AddressLabel:                "example.com:9090",
				model.MetricsPathLabel:            "/metrics",
				labelNameOverrideMetricsPathLabel: "/new-metrics",
			},
			expected: "http://example.com:9090/new-metrics",
		},
		{
			name: "Missing labels with defaults",
			labels: model.LabelSet{
				model.AddressLabel: "example.com:9090",
			},
			expected: "http://example.com:9090" + defaultMetricPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := geneateURLFromLabels(tt.labels)
			if u.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, u.String())
			}
		})
	}
}

func TestEndpointHash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name:     "Simple string",
			input:    "test",
			expected: "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := endpointHash(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
