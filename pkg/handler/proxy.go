package handler

import (
	"log/slog"
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
	slog.InfoContext(r.Context(), "endpointProxy", "url", u.String())
	rp, ok := h.reverseProxyMap[*u]
	if !ok {
		rp = createProxy(*u)
		h.reverseProxyMap[*u] = rp
	}

	rp.ServeHTTP(w, r)
}
