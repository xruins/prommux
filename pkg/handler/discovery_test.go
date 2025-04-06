package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestEndpointServiceDiscovery(t *testing.T) {
	ctx := t.Context()
	type pattern struct {
		description  string
		params       *HandlerParams
		tg           []*targetgroup.Group
		wantResponse []*staticConfig
		wantCode     int
	}

	patterns := []*pattern{
		{
			description: "includeDockerLabels",
			params: &HandlerParams{
				DiscovererParams: &DiscovererParams{
					IncludeDockerLabels: true,
				},
			},
			tg: []*targetgroup.Group{testTargetGroup},
			wantResponse: []*staticConfig{
				{
					Targets: []string{"example.com"},
					Labels: model.LabelSet{
						labelNameSchemeLabel: "http",
						labelNameMetricsPathLabel: model.LabelValue(
							"/proxy/" + endpointHash("http://example.com/metrics"),
						),
						"foo":                            "bar",
						"hoge":                           "fuga",
						labelNameLabelPrommuxDetectedURL: "http://example.com/metrics",
					},
				},
			},
		},
		{
			description: "default",
			params: &HandlerParams{
				DiscovererParams: &DiscovererParams{
					IncludeDockerLabels: false,
				},
			},
			tg: []*targetgroup.Group{testTargetGroup},
			wantResponse: []*staticConfig{
				{
					Targets: []string{"example.com"},
					Labels: model.LabelSet{
						labelNameSchemeLabel: "http",
						labelNameMetricsPathLabel: model.LabelValue(
							"/proxy/" + endpointHash("http://example.com/metrics"),
						),
						labelNameLabelPrommuxDetectedURL: "http://example.com/metrics",
					},
				},
			},
		},
		{
			description: "includeDockerLabels + regexp filtering",
			params: &HandlerParams{
				DiscovererParams: &DiscovererParams{
					IncludeDockerLabels: true,
					RegexpDockerLabels:  "^foo$",
				},
			},
			tg: []*targetgroup.Group{testTargetGroup},
			wantResponse: []*staticConfig{
				{
					Targets: []string{"example.com"},
					Labels: model.LabelSet{
						labelNameSchemeLabel: "http",
						labelNameMetricsPathLabel: model.LabelValue(
							"/proxy/" + endpointHash("http://example.com/metrics"),
						),
						"foo":                            "bar",
						labelNameLabelPrommuxDetectedURL: "http://example.com/metrics",
					},
				},
			},
		},
		{
			description: "additionalLabel",
			params: &HandlerParams{
				DiscovererParams: &DiscovererParams{
					IncludeDockerLabels: false,
				},
				AdditionalLabels: `{"foofoo":"barbar"}`,
			},
			tg: []*targetgroup.Group{testTargetGroup},
			wantResponse: []*staticConfig{
				{
					Targets: []string{"example.com"},
					Labels: model.LabelSet{
						labelNameSchemeLabel: "http",
						labelNameMetricsPathLabel: model.LabelValue(
							"/proxy/" + endpointHash("http://example.com/metrics"),
						),
						"foofoo":                         "barbar",
						labelNameLabelPrommuxDetectedURL: "http://example.com/metrics",
					},
				},
			},
		},
	}

	for _, p := range patterns {
		t.Run(p.description, func(t *testing.T) {
			handler, err := createTestHandler(t, p.tg, p.params)
			if err != nil {
				t.Fatal(err)
			}
			readyCh := make(chan bool, 1)
			handler.isReady.subscribe(readyCh)
			go func() {
				handler.Run(ctx)
			}()
			for {
				v := <-readyCh
				if v {
					break
				}
			}

			r := httptest.NewRequest(http.MethodGet, "/discovery", nil)
			w := httptest.NewRecorder()
			handler.endpointServiceDiscovery(w, r)
			res := w.Result()
			defer res.Body.Close()
			data, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			var got []*staticConfig
			err = json.Unmarshal(data, &got)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(got, p.wantResponse); diff != "" {
				t.Errorf("unexpected response. diff(-got, +want): %s", diff)
			}
		})
	}
}
