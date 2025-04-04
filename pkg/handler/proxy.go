package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gorilla/mux"
)

func createProxy(target url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL = &target
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
	h.targetsMutex.RLock()
	defer h.targetsMutex.RUnlock()

	u, ok := h.proxies[source]
	if !ok {
		http.Error(w, "missing source", http.StatusNotFound)
		return
	}

	if h.reverseProxyMap == nil {
		h.reverseProxyMap = make(map[url.URL]*httputil.ReverseProxy, 1)
	}
	rp, ok := h.reverseProxyMap[*u]
	if !ok {
		rp = createProxy(*u)
		h.reverseProxyMap[*u] = rp
	}

	rec := statusRecorder{w, 200}
	rp.ServeHTTP(w, r)

	// record metrics
	if rec.status == http.StatusOK {
		proxySuccessCountMetrics.Inc()
	} else {
		proxyFailureCountMetrics.Inc()
	}
}
