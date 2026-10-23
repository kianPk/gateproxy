package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

type Route struct {
	Prefix      string   `json:"prefix"`
	Target      string   `json:"target"`
	Auth        bool     `json:"auth"`
	Roles       []string `json:"roles,omitempty"`
	Timeout     string   `json:"timeout"`
	Retries     int      `json:"retries,omitempty"`
	StripPrefix bool     `json:"strip_prefix"`
}

type Config struct {
	Listen         string
	JWTSecret      string
	RateRPS        float64
	RateBurst      int
	Routes         []Route
	AdminToken     string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	CORSOrigins    []string
	BreakerFail    int
	BreakerCool    time.Duration
	MaxBodyBytes   int64
}

func Load() Config {
	cfg := Config{
		Listen:       env("GATE_LISTEN", ":8080"),
		JWTSecret:    env("GATE_JWT_SECRET", "gateproxy-dev-secret-change-me"),
		RateRPS:      envFloat("GATE_RATE_RPS", 20),
		RateBurst:    envInt("GATE_RATE_BURST", 40),
		AdminToken:   env("GATE_ADMIN_TOKEN", "admin-dev-token"),
		ReadTimeout:  time.Duration(envInt("GATE_READ_TIMEOUT_MS", 15000)) * time.Millisecond,
		WriteTimeout: time.Duration(envInt("GATE_WRITE_TIMEOUT_MS", 30000)) * time.Millisecond,
		CORSOrigins:  splitCSV(env("GATE_CORS_ORIGINS", "*")),
		BreakerFail:  envInt("GATE_BREAKER_FAILS", 5),
		BreakerCool:  time.Duration(envInt("GATE_BREAKER_COOLDOWN_MS", 20000)) * time.Millisecond,
		MaxBodyBytes: int64(envInt("GATE_MAX_BODY_BYTES", 1<<20)),
		Routes: []Route{
			{Prefix: "/httpbin/", Target: "https://httpbin.org/", Auth: false, Timeout: "15s", Retries: 1, StripPrefix: true},
			{Prefix: "/postman/", Target: "https://postman-echo.com/", Auth: true, Roles: []string{"user", "admin"}, Timeout: "15s", Retries: 1, StripPrefix: true},
			{Prefix: "/admin-echo/", Target: "https://postman-echo.com/", Auth: true, Roles: []string{"admin"}, Timeout: "10s", Retries: 0, StripPrefix: true},
		},
	}
	if raw := os.Getenv("GATE_ROUTES_JSON"); raw != "" {
		var routes []Route
		if err := json.Unmarshal([]byte(raw), &routes); err == nil && len(routes) > 0 {
			cfg.Routes = routes
		}
	}
	if path := os.Getenv("GATE_ROUTES_FILE"); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			var routes []Route
			if err := json.Unmarshal(b, &routes); err == nil && len(routes) > 0 {
				cfg.Routes = routes
			}
		}
	}
	return cfg
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
