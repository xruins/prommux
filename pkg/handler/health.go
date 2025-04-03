package handler

import (
	"fmt"
	"net/http"
)

// endpointHealth serves the endpoint for health check.
func (h *Handler) endpointHealth(w http.ResponseWriter, r *http.Request) {
	ok := h.isReady.Load()
	if !ok {
		fmt.Println("respond ng")
		http.Error(w, "healthcheck failed: handler is not ready", http.StatusServiceUnavailable)
		return
	}
	fmt.Println("respond ok")
	w.WriteHeader(http.StatusOK)
}
