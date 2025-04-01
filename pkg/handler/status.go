package handler

import (
	"encoding/json"
	"net/http"
)

type responseStatusTarget struct {
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

type responseStatus struct {
	Targets []*responseStatusTarget `json:"targets"`
	Config  HandlerParams           `json:"config"`
}

func (h *Handler) endpointStatus(w http.ResponseWriter, r *http.Request) {
	h.targetsMutex.RLock()
	defer h.targetsMutex.RUnlock()
	status := &responseStatus{
		Config: *h.config,
	}
	for hash, url := range h.proxies {
		status.Targets = append(status.Targets, &responseStatusTarget{
			URL:  url.String(),
			Hash: hash,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
