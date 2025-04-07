package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/common/model"
)

type staticConfig struct {
	Targets []string       `json:"targets"`
	Labels  model.LabelSet `json:"labels,omitempty"`
}

var (
	filteredLabels = []model.LabelName{
		model.AddressLabel, model.SchemeLabel, model.MetricsPathLabel,
		labelNameOverrideAddressLabel, labelNameOverrideSchemeLabel, labelNameOverrideMetricsPathLabel,
	}
)

// endpointServiceDiscovery serves the endpoint for Docker HTTP service discovery.
func (h *Handler) endpointServiceDiscovery(w http.ResponseWriter, r *http.Request) {
	h.targetsMutex.RLock()
	ret := make([]*staticConfig, 0, len(h.targets))
	dedupMap := make(map[string]struct{})
	for _, tg := range h.targets {
		for _, ls := range tg.Targets {

			newLabels := ls.Clone()

			url, err := geneateURLFromLabels(newLabels)
			if err != nil {
				h.logger.ErrorContext(r.Context(), "failed to generate URL", "error", err)
				http.Error(
					w,
					fmt.Sprintf("failed to generate URL. err: %s", err),
					http.StatusInternalServerError,
				)
				return
			}
			hash := endpointHash(url.String())

			// dedup targets by TargetURL
			if _, ok := dedupMap[hash]; ok {
				continue
			}
			dedupMap[hash] = struct{}{}

			for _, key := range filteredLabels {
				delete(newLabels, model.LabelName(key))
			}

			// generate URL to scrape metrics
			scheme := defaultScheme
			if r.URL.Scheme != "" {
				scheme = r.URL.Scheme
			}

			address := r.Host
			if r.Headers.Get("X-Forwarded-Proto") != "" {
				scheme = r.Headers.Get("X-Forwarded-Proto")
				address = r.Headers.Get("X-Forwarded-For")
			}

			config := &staticConfig{
				Targets: []string{address},
				Labels: model.LabelSet{
					labelNameMetricsPathLabel: model.LabelValue("/proxy/" + hash),
					labelNameSchemeLabel:      model.LabelValue(scheme),
				},
			}
			if h.includeDockerLabels {
				config.Labels = config.Labels.Merge(h.filterLabels(newLabels))
			}
			if h.additionalLabels != nil {
				config.Labels = config.Labels.Merge(h.additionalLabels)
			}
			config.Labels = config.Labels.Merge(
				model.LabelSet{
					labelNameLabelPrommuxDetectedURL: model.LabelValue(url.String()),
				},
			)

			ret = append(ret, config)
		}
	}
	countTargets := len(ret)
	h.targetsMutex.RUnlock()

	// update Prometheus metrics
	proxiedEndpointsCountMetrics.Set(float64(countTargets))
	discoveryLastReloadSuccessfulMetrics.Set(float64(time.Now().Unix()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ret)
	return
}

// filterLabels filters LabelSet with regexp defined in regexpDockerLabels.
// If `includeDockerLabels` is false or regexpDockerLabels is nil,
// it returns original LabelSet as is.
func (h *Handler) filterLabels(labels model.LabelSet) model.LabelSet {
	if !h.includeDockerLabels || h.regexpDockerLabels == nil {
		return labels
	}
	if h.regexpMatchCache == nil {
		h.regexpMatchCache = make(map[string]bool, len(labels))
	}

	var newLabelSet model.LabelSet
	for name, value := range labels {
		s := string(name)
		// check cache and use its result if found
		cacheMatched, ok := h.regexpMatchCache[string(s)]
		if ok {
			if cacheMatched {
				if newLabelSet == nil {
					newLabelSet = model.LabelSet{}
				}
				newLabelSet[name] = value
			}
			continue
		}
		// check a label with regexp and cache result
		matched := h.regexpDockerLabels.MatchString(s)
		h.regexpMatchCache[s] = matched
		if matched {
			if newLabelSet == nil {
				newLabelSet = model.LabelSet{}
			}
			newLabelSet[name] = value
		}
	}

	return newLabelSet
}
