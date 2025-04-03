package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEndpointHealth(t *testing.T) {
	type pattern struct {
		description     string
		atomicBoolValue bool
		wantCode        int
	}

	patterns := []*pattern{
		{
			description:     "status ok",
			atomicBoolValue: true,
			wantCode:        http.StatusOK,
		},
		{
			description:     "status failed",
			atomicBoolValue: false,
			wantCode:        http.StatusServiceUnavailable,
		},
	}

	for _, p := range patterns {
		t.Run(p.description, func(t *testing.T) {
			handler, err := createHandlerByParams(&HandlerParams{DiscovererParams: &DiscovererParams{}})
			if err != nil {
				t.Fatalf("failed to create handler. err: %s", err)
			}
			b := &handler.isReady
			b.Store(p.atomicBoolValue)

			r := httptest.NewRequest(http.MethodGet, "/-/health", nil)
			w := httptest.NewRecorder()
			handler.endpointServiceDiscovery(w, r)
			res := w.Result()
			defer res.Body.Close()
			got := res.StatusCode
			want := p.wantCode
			if got != want {
				t.Errorf("unexpected status code of the response. got: %d, want: %d", got, want)
			}
		})
	}
}
