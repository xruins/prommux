package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
						"foo":  "bar",
						"hoge": "fuga",
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
						"foo": "bar",
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
						"foofoo": "barbar",
					},
				},
			},
		},
	}

	for _, p := range patterns {
		handler, readyCh, err := createTestHandler(t, p.tg, p.params)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			handler.Run(ctx)
		}()
		<-readyCh
		time.Sleep(time.Millisecond * 50) // [TODO] do something better

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
	}
}
