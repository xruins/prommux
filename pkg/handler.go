package pkg

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/moby"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Handler struct {
	discoverer                      *discoverer
	targets                         []*targetgroup.Group
	targetsMutex                    sync.RWMutex
	proxies                         map[string]*url.URL
	ch                              <-chan []*targetgroup.Group
	discovererTimeout, proxyTimeout time.Duration
	includeDockerLabels             bool
	regexpDockerLabels              *regexp.Regexp
	regexpMatchCache                map[string]bool
	logger                          slog.Logger
	reverseProxyMap                 map[url.URL]*httputil.ReverseProxy
	config                          *HandlerParams
}

var (
	labelNameSchemeLabel      = model.LabelName(model.SchemeLabel)
	labelNameAddressLabel     = model.LabelName(model.AddressLabel)
	labelNameMetricsPathLabel = model.LabelName(model.MetricsPathLabel)
)

type HandlerParams struct {
	Logger           slog.Logger       `json:"-"`
	ProxyTimeout     time.Duration     `json:"proxy_timeout"`
	DiscovererParams *DiscovererParams `json:"discoverer_params"`
}

type DiscovererParams struct {
	Host                string        `json:"host"`
	Port                int           `json:"port"`
	DiscovererTimeout   time.Duration `json:"discoverer_timeout"`
	RefreshInterval     time.Duration `json:"refresh_interval"`
	IncludeDockerLabels bool          `json:"include_docker_labels"`
	RegexpDockerLabels  string        `json:"regexp_docker_labels"`
	Filter              []moby.Filter `json:"filter"`
}

func NewHandler(params *HandlerParams) (*Handler, error) {
	ch := make(chan []*targetgroup.Group)
	discoverer := newDiscoverer(
		&params.Logger,
		params.DiscovererParams.Host,
		params.DiscovererParams.Port,
		params.DiscovererParams.Filter,
		params.DiscovererParams.RefreshInterval,
		ch,
	)

	h := &Handler{
		discoverer:          discoverer,
		ch:                  ch,
		proxies:             make(map[string]*url.URL),
		discovererTimeout:   params.DiscovererParams.DiscovererTimeout,
		proxyTimeout:        params.ProxyTimeout,
		includeDockerLabels: params.DiscovererParams.IncludeDockerLabels,
		regexpMatchCache:    make(map[string]bool),
		logger:              params.Logger,
		reverseProxyMap:     make(map[url.URL]*httputil.ReverseProxy),
		config:              params,
	}

	if regexpDockerLabels := params.DiscovererParams.RegexpDockerLabels; regexpDockerLabels != "" {
		var err error
		h.regexpDockerLabels, err = regexp.Compile(regexpDockerLabels)
		if err != nil {
			return nil, fmt.Errorf("failed to compile regexpDockerLabels: %w", err)
		}
	}
	return h, nil
}

// geneateURLFromLabels generates the url to scrape metrics by system labels of Prometheus.
func geneateURLFromLabels(ls model.LabelSet) *url.URL {
	scheme := string(ls[labelNameSchemeLabel])
	if scheme == "" {
		scheme = defaultScheme
	}
	host := string(ls[model.AddressLabel])
	path := string(ls[model.MetricsPathLabel])
	if path == "" {
		path = defaultMetricPath
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path,
	}
	return u
}

// endpointHash generates SHA1 hash by string.
// The hash is used to subpath of reverseproxy endpoint.
func endpointHash(s string) string {
	hash := sha1.Sum([]byte(s))
	ret := hex.EncodeToString(hash[:])
	return ret
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

// run receives TargetGroups from discoverer and update reverse proxy periodically.
func (h *Handler) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := h.discoverer.run(ctx)
		errCh <- err
	}()
	for {
		select {
		case v := <-h.ch:
			func() {
				h.targetsMutex.Lock()
				defer h.targetsMutex.Unlock()
				h.targets = v
				for _, tg := range v {
					for _, target := range tg.Targets {
						u := geneateURLFromLabels(target)
						hash := endpointHash(u.String())
						h.proxies[hash] = u
						h.logger.InfoContext(
							ctx,
							"registered endpoint",
							slog.String("url", u.String()),
							slog.String("hash", hash),
						)
					}
				}
			}()
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return fmt.Errorf("an error occured when executing docker discoverer: %w", err)
		}
	}
}

func (h *Handler) NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/discover", h.endpointServiceDiscovery)
	r.HandleFunc("/proxy/{source}", h.endpointProxy)
	r.HandleFunc("/status", h.endpointStatus)
	r.Handle("/metrics", promhttp.Handler())
	return r
}

type staticConfig struct {
	Targets []string       `json:"targets"`
	Labels  model.LabelSet `json:"labels,omitempty"`
	Source  string         `json:"source"`
}

const (
	// defaultMetricPath is the default path for scraping by Prometheus.
	defaultMetricPath = "/metrics"
	// defaultScheme is the default scheme for scraping by Prometheus.
	defaultScheme = "http"
	// souceStaticConfig is the name for `Source` label.
	souceStaticConfig = "prommux"
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
	slog.Info("endpointStatus", "proxies", h.proxies)
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

// endpointServiceDiscovery serves the endpoint for Docker HTTP service discovery.
func (h *Handler) endpointServiceDiscovery(w http.ResponseWriter, r *http.Request) {
	h.targetsMutex.RLock()
	ret := make([]*staticConfig, 0, len(h.targets))
	dedupMap := make(map[string]struct{})
	for _, tg := range h.targets {
		for _, ls := range tg.Targets {
			newLabels := ls.Clone()

			targetUrl := endpointHash(geneateURLFromLabels(newLabels).String())

			// dedup targets by TargetURL
			if _, ok := dedupMap[targetUrl]; ok {
				continue
			}
			dedupMap[targetUrl] = struct{}{}

			for _, key := range []string{model.AddressLabel, model.SchemeLabel, model.MetricsPathLabel} {
				delete(newLabels, model.LabelName(key))
			}
			config := &staticConfig{
				Targets: []string{strings.Join([]string{r.Host, "proxy", targetUrl}, "/")},
				Source:  souceStaticConfig,
			}
			if h.includeDockerLabels {
				config.Labels = h.filterLabels(newLabels)
			}
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
