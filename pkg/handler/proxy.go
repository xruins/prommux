package handler

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gorilla/mux"
)

func createProxy(target url.URL, timeout time.Duration) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL = &target
		},
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
		},
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

// endpointServiceDiscovery serves reverseproxy for the exporters detected by Docker API.
func (h *Handler) endpointProxy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	source := vars["source"]

	var rp *httputil.ReverseProxy
	func() {
		h.targetsMutex.RLock()
		defer h.targetsMutex.RUnlock()

		u, ok := h.proxies[source]
		if !ok {
			http.Error(w, "missing source", http.StatusNotFound)
			return
		}

		// find reverse proxy handler
		if h.reverseProxyMap == nil {
			h.reverseProxyMap = make(map[url.URL]*httputil.ReverseProxy, 1)
		}
		rp, ok = h.reverseProxyMap[*u]
		if !ok {
			rp = createProxy(*u, h.proxyTimeout)
			h.reverseProxyMap[*u] = rp
		}
	}()
	statusCode := proxy(rp, w, r)

	// record metrics
	if statusCode == http.StatusOK {
		proxySuccessCountMetrics.Inc()
	} else {
		proxyFailureCountMetrics.Inc()
	}
}

// proxy proxies request to reverse proxy and returns response status code.
func proxy(rp *httputil.ReverseProxy, w http.ResponseWriter, r *http.Request) int {
	rec := statusRecorder{w, 0}
	rp.ServeHTTP(w, r)
	return rec.status
}
