package handler

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	testTargetGroup = &targetgroup.Group{
		Targets: []model.LabelSet{
			model.LabelSet{
				labelNameSchemeLabel:      "http",
				labelNameAddressLabel:     "example.com",
				labelNameMetricsPathLabel: "/metrics",
				"foo":                     "bar",
				"hoge":                    "fuga",
			},
		},
		Labels: model.LabelSet{
			"foo":  "bar",
			"hoge": "fuga",
		},
	}
	testTargetGroupWithOverride = &targetgroup.Group{
		Targets: []model.LabelSet{
			model.LabelSet{
				labelNameSchemeLabel:      "http",
				labelNameAddressLabel:     "example.com",
				labelNameMetricsPathLabel: "/metrics",
			},
		},
		Labels: model.LabelSet{
			labelNameOverrideSchemeLabel:      "https",
			labelNameOverrideAddressLabel:     "override.com",
			labelNameOverrideMetricsPathLabel: "/metrics_overridden",
			"foo":                             "bar",
			"hoge":                            "fuga",
		},
	}
)

type mockDiscoverer struct {
	ch      chan []*targetgroup.Group
	tg      []*targetgroup.Group
	readyCh chan struct{}
}

func (m *mockDiscoverer) Run(ctx context.Context) error {
	m.ch <- m.tg
	m.readyCh <- struct{}{}
	<-ctx.Done()
	return nil
}

func createTestHandler(t *testing.T, mockTargetGroups []*targetgroup.Group, params *HandlerParams) (*Handler, error) {
	tgCh := make(chan []*targetgroup.Group, 1)

	h, err := createHandlerByParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Handler by params: %w", err)
	}

	h.discoverer = &mockDiscoverer{
		ch: tgCh,
		tg: mockTargetGroups,
	}

	logger := slog.New(slog.DiscardHandler)
	h.discovererTimeout = 30 * time.Second
	h.proxyTimeout = 30 * time.Second
	h.logger = *logger
	h.ch = tgCh

	return h, nil
}
