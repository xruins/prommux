package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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

type discoverer interface {
	Run(ctx context.Context) error
}

type targets struct {
	mu       sync.RWMutex
	targets  []*targetgroup.Group
	proxyMap map[string]*url.URL
}

func (t *targets) getAllTargets() []*targetgroup.Group {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.targets
}

func (t *targets) setTargets(targets []*targetgroup.Group) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets = targets
}

type Handler struct {
	discoverer   discoverer
	targets      []*targetgroup.Group
	targetsMutex sync.RWMutex
	proxies      map[string]*url.URL
	ch           <-chan []*targetgroup.Group
	discovererTimeout, proxyTimeout,
	healthcheckTimeout time.Duration
	includeDockerLabels bool
	additionalLabels    model.LabelSet
	regexpDockerLabels  *regexp.Regexp
	regexpMatchCache    map[string]bool
	logger              slog.Logger
	reverseProxyMap     map[url.URL]*httputil.ReverseProxy
	config              *HandlerParams
	isReady             notifiableAtomicBool
}

// HandlerParam is the parameters to configure Handler.
type HandlerParams struct {
	Logger             slog.Logger       `json:"-"`
	ProxyTimeout       time.Duration     `json:"proxy_timeout"`
	DiscovererParams   *DiscovererParams `json:"discoverer_params"`
	HealthcheckTimeout time.Duration     `json:"healthcheck_timeout"`
	AdditionalLabels   string            `json:"additional_labels,string"`
}

// DiscovererParams is the parameters to configure Discoverer.
type DiscovererParams struct {
	Host                string        `json:"host"`
	Port                int           `json:"port"`
	DiscovererTimeout   time.Duration `json:"discoverer_timeout"`
	RefreshInterval     time.Duration `json:"refresh_interval"`
	IncludeDockerLabels bool          `json:"include_docker_labels"`
	RegexpDockerLabels  string        `json:"regexp_docker_labels"`
	HostNetworkingHost  string        `json:"host_networking_host"`
	Filter              []moby.Filter `json:"filter"`
}

func createHandlerByParams(params *HandlerParams) (*Handler, error) {
	h := &Handler{
		proxies:             make(map[string]*url.URL),
		discovererTimeout:   params.DiscovererParams.DiscovererTimeout,
		proxyTimeout:        params.ProxyTimeout,
		includeDockerLabels: params.DiscovererParams.IncludeDockerLabels,
		healthcheckTimeout:  params.HealthcheckTimeout,
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

	return h, nil
}

// NewHandler creates a new instance on Handler and returns it.
func NewHandler(params *HandlerParams) (*Handler, error) {
	h, err := createHandlerByParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to process handlerParams: %w", err)
	}
	ch := make(chan []*targetgroup.Group, 1)
	h.ch = ch
	h.discoverer = discovery.NewDiscoverer(
		&params.Logger,
		params.DiscovererParams.Host,
		params.DiscovererParams.Port,
		params.DiscovererParams.Filter,
		params.DiscovererParams.RefreshInterval,
		params.DiscovererParams.HostNetworkingHost,
		ch,
	)

	h.logger.Debug("initialized Handler", slog.Any("params", params))
	return h, nil
}

// nopWriter is a no-operation http.ResponseWriter.
// It is used to be embedded to statusRecorder to read only response status code.
type nopWriter struct{}

func (n *nopWriter) Header() http.Header {
	return http.Header{}
}
func (n *nopWriter) Write([]byte) (int, error) {
	return 0, nil
}
func (n *nopWriter) WriteHeader(_ int) {
	// NOP
}

func (h *Handler) checkEndpointHealth(u *url.URL) (int, error) {
	rp := createProxy(*u, h.healthcheckTimeout)
	request, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	request.Header.Set("User-Agent", userAgentName)
	request.Header.Set("Accept", acceptHeader)
	request.Header.Set("Accept-Encoding", acceptEncodingHeader)

	w := &nopWriter{}
	status := proxy(rp, w, request)
	return status, nil
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
			h.logger.DebugContext(
				ctx,
				"received target groups",
				slog.Any("target_group", v),
			)
			err := func() error {
				h.targetsMutex.Lock()
				defer h.targetsMutex.Unlock()
				h.targets = v
				innerRoutineMutex := sync.Mutex{}
				newProxies := make(map[string]*url.URL, len(v))
				for _, tg := range v {
					for _, target := range tg.Targets {
						u, err := geneateURLFromLabels(target)
						if err != nil {
							return fmt.Errorf("failed to generate URL for `%s`: %w", target, err)
						}
						hash := endpointHash(u.String())

						// check health of the endpoint if healthcheckTimeout is not zero
						if h.healthcheckTimeout > 0 {
							statusCode, err := h.checkEndpointHealth(u)
							if err == nil {
								return fmt.Errorf("failed to check health of `%s`: %w", u.String(), err)
							}
							if statusCode != http.StatusOK {
								h.logger.WarnContext(
									ctx,
									"endpoint healthcheck failed",
									slog.String("url", u.String()),
									slog.Int("status_code", statusCode),
									slog.String("hash", hash),
								)
								continue
							}
						}

						innerRoutineMutex.Lock()
						newProxies[hash] = u
						innerRoutineMutex.Unlock()
						h.logger.DebugContext(
							ctx,
							"registered endpoint",
							slog.String("url", u.String()),
							slog.String("hash", hash),
							slog.Any("target", target),
						)
					}
				}
				return nil
			}()
			if err != nil {
				return err
			}
			h.isReady.Store(true)
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			h.isReady.Store(false)
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
	r.HandleFunc("/-/health", h.endpointHealth)
	r.Handle("/metrics", promhttp.Handler())
	return r
}
