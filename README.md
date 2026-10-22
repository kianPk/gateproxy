# Gateproxy

**Production-shaped API gateway in Go** — reverse proxy, JWT auth, token-bucket rate limiting, Prometheus-style metrics, and an operator dashboard.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)

## Features

- Longest-prefix route matching → upstream targets
- Per-route JWT requirement
- IP token-bucket rate limit (`GATE_RATE_RPS` / `GATE_RATE_BURST`)
- `/metrics` (Prometheus text), `/healthz`, `/readyz`
- `POST /v1/token` to mint HS256 JWTs (admin token gated)
- Embedded web console for minting tokens & probing routes
- Graceful shutdown

## Quick start

```bash
go run .
# dashboard → http://localhost:8080
# public proxy → http://localhost:8080/httpbin/get
```

Mint a token:

```bash
curl -s localhost:8080/v1/token -H 'content-type: application/json' \
  -d '{"sub":"kian","admin_token":"admin-dev-token"}'
```

Call a protected route:

```bash
curl -H "Authorization: Bearer $TOKEN" localhost:8080/postman/get
```

## Config (env)

| Var | Default | Meaning |
|-----|---------|---------|
| `GATE_LISTEN` | `:8080` | bind address |
| `GATE_JWT_SECRET` | dev secret | HS256 key |
| `GATE_ADMIN_TOKEN` | `admin-dev-token` | mint gate |
| `GATE_RATE_RPS` | `20` | refill rate |
| `GATE_ROUTES_JSON` | built-in | override routes JSON |

## License

MIT © [kianPk](https://github.com/kianPk)
