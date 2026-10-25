package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/kianPk/gateproxy/internal/auth"
	"github.com/kianPk/gateproxy/internal/breaker"
	"github.com/kianPk/gateproxy/internal/config"
	"github.com/kianPk/gateproxy/internal/metrics"
	"github.com/kianPk/gateproxy/internal/ratelimit"
)

type Gateway struct {
	cfg       config.Config
	limiter   *ratelimit.Limiter
	metrics   *metrics.Registry
	breaker   *breaker.Breaker
	log       *log.Logger
	startedAt time.Time
}

func New(cfg config.Config, m *metrics.Registry, logger *log.Logger) *Gateway {
	return &Gateway{
		cfg:       cfg,
		limiter:   ratelimit.New(cfg.RateRPS, cfg.RateBurst),
		metrics:   m,
		breaker:   breaker.New(cfg.BreakerFail, cfg.BreakerCool),
		log:       logger,
		startedAt: time.Now(),
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
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		uptime := time.Since(g.startedAt).Seconds()
		writeJSON(w, 200, map[string]any{
			"service":         "gateproxy",
			"routes":          len(g.cfg.Routes),
			"uptime_seconds":  uptime,
			"breaker":         g.breaker.Snapshot(),
			"rate":            map[string]any{"rps": g.cfg.RateRPS, "burst": g.cfg.RateBurst},
		})
	})
	mux.Handle("/metrics", http.HandlerFunc(g.metrics.Handler))
	mux.HandleFunc("/v1/token", g.issueToken)
	mux.HandleFunc("/v1/routes", g.listRoutes)
	mux.HandleFunc("/", g.proxy)
	return g.withCORS(mux)
}

func (g *Gateway) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allow := "*"
		if len(g.cfg.CORSOrigins) > 0 && g.cfg.CORSOrigins[0] != "*" {
			allow = ""
			for _, o := range g.cfg.CORSOrigins {
				if o == origin {
					allow = origin
					break
				}
			}
		}
		if allow != "" {
			w.Header().Set("Access-Control-Allow-Origin", allow)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, X-Gateproxy-Route")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) issueToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, g.cfg.MaxBodyBytes)
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
	writeJSON(w, 200, map[string]any{"token": tok, "expires_in": 86400, "role": body.Role})
}

func (g *Gateway) listRoutes(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(g.cfg.Routes))
	for _, rt := range g.cfg.Routes {
		out = append(out, map[string]any{
			"prefix":  rt.Prefix,
			"target":  rt.Target,
			"auth":    rt.Auth,
			"roles":   rt.Roles,
			"timeout": rt.Timeout,
			"retries": rt.Retries,
		})
	}
	writeJSON(w, 200, map[string]any{"routes": out})
}

func (g *Gateway) proxy(w http.ResponseWriter, r *http.Request) {
	g.metrics.Requests.Add(1)
	start := time.Now()
	defer func() { g.metrics.ObserveLatency(time.Since(start)) }()

	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = shortID()
	}
	w.Header().Set("X-Request-Id", reqID)

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

	if !g.breaker.Allow(route.Prefix) {
		g.metrics.Errors.Add(1)
		http.Error(w, "circuit open", http.StatusServiceUnavailable)
		return
	}

	claimsRole := ""
	if route.Auth {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			g.metrics.AuthFail.Add(1)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		claims, err := auth.Verify(g.cfg.JWTSecret, strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			g.metrics.AuthFail.Add(1)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		claimsRole = claims.Role
		if len(route.Roles) > 0 && !roleAllowed(claimsRole, route.Roles) {
			g.metrics.AuthFail.Add(1)
			http.Error(w, "forbidden role", http.StatusForbidden)
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

	origPath := r.URL.Path
	if route.StripPrefix || true {
		// default strip for gateway prefixes
		r.URL.Path = strings.TrimPrefix(origPath, strings.TrimSuffix(route.Prefix, "/"))
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
	}
	r.Host = target.Host
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Header.Set("X-Forwarded-For", ip)
	r.Header.Set("X-Request-Id", reqID)
	if claimsRole != "" {
		r.Header.Set("X-Gateproxy-Role", claimsRole)
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
		g.breaker.Failure(route.Prefix)
		g.metrics.Errors.Add(1)
		g.log.Printf("proxy error %s → %s: %v id=%s", req.URL.Path, route.Target, err, reqID)
		http.Error(rw, "upstream unavailable", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Gateproxy", "1")
		resp.Header.Set("X-Gateproxy-Route", route.Prefix)
		resp.Header.Set("X-Request-Id", reqID)
		if resp.StatusCode >= 500 {
			g.breaker.Failure(route.Prefix)
		} else {
			g.breaker.Success(route.Prefix)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)

	g.metrics.Proxied.Add(1)
	g.log.Printf("%s %s → %s%s id=%s role=%s", r.Method, origPath, target.Host, r.URL.Path, reqID, claimsRole)
	proxy.ServeHTTP(w, r)
}

func roleAllowed(role string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, role) {
			return true
		}
	}
	return false
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
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strings.ReplaceAll(time.Now().Format("150405.000"), ".", "")
	}
	return hex.EncodeToString(b[:])
}
