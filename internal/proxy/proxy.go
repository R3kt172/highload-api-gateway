package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/R3kt172/highload-api-gateway/internal/config"
	"github.com/R3kt172/highload-api-gateway/internal/middleware"
	"github.com/R3kt172/highload-api-gateway/internal/observability"
)

type route struct {
	prefix string
	proxy  *httputil.ReverseProxy
}

type Router struct {
	routes  []route
	logger  *slog.Logger
	metrics *observability.Metrics
}

func NewRouter(configs []config.Route, logger *slog.Logger, metrics *observability.Metrics) (*Router, error) {
	router := &Router{logger: logger, metrics: metrics}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	for _, item := range configs {
		target, err := url.Parse(item.Upstream)
		if err != nil || target.Scheme == "" || target.Host == "" {
			return nil, fmt.Errorf("invalid upstream %q", item.Upstream)
		}
		prefix := "/" + strings.Trim(item.Prefix, "/")
		reverse := httputil.NewSingleHostReverseProxy(target)
		originalDirector := reverse.Director
		reverse.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = target.Host
			req.Header.Set("X-Forwarded-By", "highload-api-gateway")
		}
		reverse.Transport = transport
		reverse.ErrorHandler = func(w http.ResponseWriter, req *http.Request, proxyErr error) {
			metrics.UpstreamError.WithLabelValues(prefix).Inc()
			logger.Error("upstream request failed", "trace_id", middleware.TraceID(req.Context()), "route", prefix, "error", proxyErr)
			http.Error(w, `{"error":"upstream unavailable"}`, http.StatusBadGateway)
		}
		router.routes = append(router.routes, route{prefix: prefix, proxy: reverse})
	}
	sort.Slice(router.routes, func(i, j int) bool { return len(router.routes[i].prefix) > len(router.routes[j].prefix) })
	return router, nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, item := range r.routes {
		if req.URL.Path == item.prefix || strings.HasPrefix(req.URL.Path, item.prefix+"/") {
			item.proxy.ServeHTTP(w, req)
			return
		}
	}
	http.NotFound(w, req)
}
