package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

type Route struct {
	Prefix  string `json:"prefix"`
	Target  string `json:"target"`
	Auth    bool   `json:"auth"`
	Timeout string `json:"timeout"`
}

type Config struct {
	Listen       string
	JWTSecret    string
	RateRPS      float64
	RateBurst    int
	Routes       []Route
	AdminToken   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
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
		Routes: []Route{
			{Prefix: "/httpbin/", Target: "https://httpbin.org/", Auth: false, Timeout: "15s"},
			{Prefix: "/postman/", Target: "https://postman-echo.com/", Auth: true, Timeout: "15s"},
		},
	}
	if raw := os.Getenv("GATE_ROUTES_JSON"); raw != "" {
		var routes []Route
		if err := json.Unmarshal([]byte(raw), &routes); err == nil && len(routes) > 0 {
			cfg.Routes = routes
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
