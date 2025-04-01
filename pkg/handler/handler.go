package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/moby"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/xruins/prommux/pkg/discovery"
)

type Handler struct {
	discoverer                      *discovery.Discoverer
	targets                         []*targetgroup.Group
	targetsMutex                    sync.RWMutex
	proxies                         map[string]*url.URL
	ch                              <-chan []*targetgroup.Group
	discovererTimeout, proxyTimeout time.Duration
	includeDockerLabels             bool
	additionalLabels                model.LabelSet
	regexpDockerLabels              *regexp.Regexp
	regexpMatchCache                map[string]bool
	logger                          slog.Logger
	reverseProxyMap                 map[url.URL]*httputil.ReverseProxy
	config                          *HandlerParams
}

// HandlerParam is the parameters to configure Handler.
type HandlerParams struct {
	Logger           slog.Logger       `json:"-"`
	ProxyTimeout     time.Duration     `json:"proxy_timeout"`
	DiscovererParams *DiscovererParams `json:"discoverer_params"`
	AdditionalLabels string            `json:"additional_labels,string"`
}

// DiscovererParams is the parameters to configure Discoverer.
type DiscovererParams struct {
	Host                string        `json:"host"`
	Port                int           `json:"port"`
	DiscovererTimeout   time.Duration `json:"discoverer_timeout"`
	RefreshInterval     time.Duration `json:"refresh_interval"`
	IncludeDockerLabels bool          `json:"include_docker_labels"`
	RegexpDockerLabels  string        `json:"regexp_docker_labels"`
	Filter              []moby.Filter `json:"filter"`
}

// NewHandler creates a new instance on Handler and returns it.
func NewHandler(params *HandlerParams) (*Handler, error) {
	ch := make(chan []*targetgroup.Group)
	discoverer := discovery.NewDiscoverer(
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

	var err error
	if al := params.AdditionalLabels; al != "" {
		var labelSet model.LabelSet
		err = labelSet.UnmarshalJSON([]byte(al))
		if err != nil {
			return nil, fmt.Errorf("failed to compile additionalLabels: %w", err)
		}
		h.additionalLabels = labelSet
	}

	if regexpDockerLabels := params.DiscovererParams.RegexpDockerLabels; regexpDockerLabels != "" {
		h.regexpDockerLabels, err = regexp.Compile(regexpDockerLabels)
		if err != nil {
			return nil, fmt.Errorf("failed to compile regexpDockerLabels: %w", err)
		}
	}

	h.logger.Debug("initialized Handler", slog.Any("params", params))
	return h, nil
}

// Run receives TargetGroups from discoverer and update reverse proxy periodically.
func (h *Handler) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := h.discoverer.Run(ctx)
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
						h.logger.DebugContext(
							ctx,
							"registered endpoint",
							slog.String("url", u.String()),
							slog.String("hash", hash),
							slog.Any("target", target),
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

// NewRouTer creates *mux.Router and returns it.
func (h *Handler) NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/discover", h.endpointServiceDiscovery)
	r.HandleFunc("/proxy/{source}", h.endpointProxy)
	r.HandleFunc("/status", h.endpointStatus)
	r.Handle("/metrics", promhttp.Handler())
	return r
}
