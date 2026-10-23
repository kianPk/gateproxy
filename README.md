# Gateproxy

**Production-shaped API gateway in Go** — reverse proxy, JWT roles, circuit breaker, token-bucket rate limiting, CORS, and an operator console.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)

## Features

- Longest-prefix route matching → upstream targets
- Per-route JWT + role allow-lists (`user` / `admin`)
- Circuit breaker per route (`GATE_BREAKER_FAILS`, `GATE_BREAKER_COOLDOWN_MS`)
- IP token-bucket rate limit
- Request IDs (`X-Request-Id`) and CORS
- `/v1/status`, `/metrics`, `/healthz`, `/readyz`
- `POST /v1/token` mint HS256 JWTs (admin gated)
- Routes from env JSON or `GATE_ROUTES_FILE`
- Embedded web console + GitHub Pages demo

## Quick start

```bash
go test ./...
go run .
# open http://localhost:8080
```

## Env

| Variable | Default | Purpose |
|---|---|---|
| `GATE_LISTEN` | `:8080` | bind address |
| `GATE_JWT_SECRET` | dev secret | HS256 key |
| `GATE_ADMIN_TOKEN` | `admin-dev-token` | mint gate |
| `GATE_RATE_RPS` / `GATE_RATE_BURST` | `20` / `40` | limiter |
| `GATE_ROUTES_JSON` / `GATE_ROUTES_FILE` | built-in | route table |
| `GATE_CORS_ORIGINS` | `*` | CORS |

## Demo

[kianpk.github.io/gateproxy](https://kianpk.github.io/gateproxy/)
