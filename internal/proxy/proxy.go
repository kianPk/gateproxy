package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/kianPk/gateproxy/internal/auth"
	"github.com/kianPk/gateproxy/internal/config"
	"github.com/kianPk/gateproxy/internal/metrics"
	"github.com/kianPk/gateproxy/internal/ratelimit"
)

type Gateway struct {
	cfg     config.Config
	limiter *ratelimit.Limiter
	metrics *metrics.Registry
	log     *log.Logger
}

func New(cfg config.Config, m *metrics.Registry, logger *log.Logger) *Gateway {
	return &Gateway{
		cfg:     cfg,
		limiter: ratelimit.New(cfg.RateRPS, cfg.RateBurst),
		metrics: m,
		log:     logger,
	}
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "service": "gateproxy"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"ready": true, "routes": len(g.cfg.Routes)})
	})
	mux.Handle("/metrics", http.HandlerFunc(g.metrics.Handler))
	mux.HandleFunc("/v1/token", g.issueToken)
	mux.HandleFunc("/v1/routes", g.listRoutes)
	mux.HandleFunc("/", g.proxy)
	return mux
}

func (g *Gateway) issueToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Sub   string `json:"sub"`
		Role  string `json:"role"`
		Admin string `json:"admin_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if body.Admin != g.cfg.AdminToken {
		g.metrics.AuthFail.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if body.Sub == "" {
		body.Sub = "demo"
	}
	if body.Role == "" {
		body.Role = "user"
	}
	tok, err := auth.Sign(g.cfg.JWTSecret, body.Sub, body.Role, 24*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "expires_in": 86400})
}

func (g *Gateway) listRoutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"routes": g.cfg.Routes})
}

func (g *Gateway) proxy(w http.ResponseWriter, r *http.Request) {
	g.metrics.Requests.Add(1)
	start := time.Now()
	defer func() { g.metrics.ObserveLatency(time.Since(start)) }()

	ip := clientIP(r)
	if !g.limiter.Allow(ip) {
		g.metrics.RateLimited.Add(1)
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	route, ok := g.match(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if route.Auth {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			g.metrics.AuthFail.Add(1)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if _, err := auth.Verify(g.cfg.JWTSecret, strings.TrimPrefix(h, "Bearer ")); err != nil {
			g.metrics.AuthFail.Add(1)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	}

	target, err := url.Parse(route.Target)
	if err != nil {
		g.metrics.Errors.Add(1)
		http.Error(w, "bad route target", 500)
		return
	}

	timeout := 15 * time.Second
	if d, err := time.ParseDuration(route.Timeout); err == nil {
		timeout = d
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		g.metrics.Errors.Add(1)
		g.log.Printf("proxy error %s → %s: %v", req.URL.Path, route.Target, err)
		http.Error(rw, "upstream unavailable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Gateproxy", "1")
		resp.Header.Set("X-Gateproxy-Route", route.Prefix)
		return nil
	}

	// Strip prefix when forwarding
	origPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(route.Prefix, "/"))
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	r.Host = target.Host
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Header.Set("X-Forwarded-For", ip)
	r.Header.Set("X-Request-Id", shortID())

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)

	g.metrics.Proxied.Add(1)
	g.log.Printf("%s %s → %s%s", r.Method, origPath, target.Host, r.URL.Path)
	proxy.ServeHTTP(w, r)
}

func (g *Gateway) match(path string) (config.Route, bool) {
	var best config.Route
	bestLen := -1
	for _, rt := range g.cfg.Routes {
		if strings.HasPrefix(path, rt.Prefix) && len(rt.Prefix) > bestLen {
			best = rt
			bestLen = len(rt.Prefix)
		}
	}
	return best, bestLen >= 0
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func shortID() string {
	return strings.ReplaceAll(time.Now().Format("150405.000"), ".", "")
}

// Drain for completeness
func Drain(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
}
