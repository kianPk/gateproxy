package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Registry struct {
	Requests   atomic.Uint64
	Errors     atomic.Uint64
	Proxied    atomic.Uint64
	RateLimited atomic.Uint64
	AuthFail   atomic.Uint64
	Started    time.Time
	latCount   atomic.Uint64
	latSumMs   atomic.Uint64
}

func New() *Registry {
	return &Registry{Started: time.Now()}
}

func (r *Registry) ObserveLatency(d time.Duration) {
	r.latCount.Add(1)
	r.latSumMs.Add(uint64(d.Milliseconds()))
}

func (r *Registry) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	avg := 0.0
	if c := r.latCount.Load(); c > 0 {
		avg = float64(r.latSumMs.Load()) / float64(c)
	}
	fmt.Fprintf(w, "# HELP gateproxy_up 1 if process is up\n")
	fmt.Fprintf(w, "gateproxy_up 1\n")
	fmt.Fprintf(w, "gateproxy_requests_total %d\n", r.Requests.Load())
	fmt.Fprintf(w, "gateproxy_proxied_total %d\n", r.Proxied.Load())
	fmt.Fprintf(w, "gateproxy_errors_total %d\n", r.Errors.Load())
	fmt.Fprintf(w, "gateproxy_rate_limited_total %d\n", r.RateLimited.Load())
	fmt.Fprintf(w, "gateproxy_auth_fail_total %d\n", r.AuthFail.Load())
	fmt.Fprintf(w, "gateproxy_latency_ms_avg %.2f\n", avg)
	fmt.Fprintf(w, "gateproxy_uptime_seconds %.0f\n", time.Since(r.Started).Seconds())
}
