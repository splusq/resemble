# resemble

HTTP caching proxy for deterministic integration tests.

Record upstream API responses, replay them instantly. Cache is stored in a separate git repo, shared across developers and CI with zero provisioning.

## Why

Integration tests break because upstream services are slow, rate-limited, intermittently down, or return non-deterministic responses. Existing solutions are language-locked (VCR), heavyweight (WireMock/JVM), or abandoned.

resemble is a single binary that sits between your test suite and the upstream API. First run records responses. Every subsequent run replays from cache in <1ms.

## Quick Start

```bash
# Install
go install github.com/splusq/resemble/cmd/resemble@latest

# Initialize with a cache repo
resemble init --cache-repo git@github.com:myorg/api-cache.git

# Start proxy
resemble start --upstream https://api.example.com --port 9999

# Point your tests at the proxy
API_BASE_URL=http://localhost:9999 go test ./...

# Stop and push cache
resemble stop
```

## How It Works

```
┌──────────────┐    ┌───────────┐    ┌──────────────────┐
│  Test Suite   │───▶│ resemble  │───▶│  Upstream API     │
│  (any lang)  │◀───│  (proxy)  │◀───│  (real service)   │
└──────────────┘    └─────┬─────┘    └──────────────────┘
                          │
                     read/write
                          │
                    ┌─────▼─────┐
                    │ Cache Repo │
                    └───────────┘
```

## Modes

| Mode | Behavior |
|------|----------|
| `auto` (default) | Replay if cached, record if not |
| `record` | Always forward to upstream, overwrite cache |
| `replay` | Only serve from cache, fail on miss |

## Commands

```
resemble init --cache-repo <git-url>     # Clone cache repo, write .testproxy.yml
resemble start [--mode auto|record|replay] [--upstream <url>] [--port <port>]
resemble stop                              # Stop proxy, push cache if dirty
resemble status                            # Show cache stats
resemble clear [pattern] [--all]           # Delete cache entries
```

## Configuration

Create `.testproxy.yml` in your project root (or run `resemble init`):

```yaml
cache_repo: git@github.com:myorg/api-cache.git
listen: ":9999"

defaults:
  ttl: 24h
  mode: auto
  ignore_headers:
    - Authorization
    - X-Request-Id
    - Date
    - User-Agent
  ignore_query: []
  strict_drift: false
```

**Config resolution**: CLI flags > individual `RESEMBLE_*` env vars > `RESEMBLE_CONFIG` env var > `.testproxy.yml` > built-in defaults.

**Environment variables**: `RESEMBLE_CACHE_REPO`, `RESEMBLE_LISTEN`, `RESEMBLE_MODE`.

**Inline config via `RESEMBLE_CONFIG`**: Pass the full YAML configuration as an environment variable. Useful in Docker Compose where you want all config in one place:

```yaml
services:
  resemble:
    image: ghcr.io/splusq/resemble
    environment:
      RESEMBLE_CONFIG: |
        defaults:
          ttl: 24h
          mode: auto
          ignore_headers: [Authorization, X-Request-Id]
          ignore_query: ["*"]
          ignore_body: true
    command: ["start", "--upstream", "https://api.example.com"]
```

## Cache Format

Cache entries are stored as human-readable files, designed for git:

```
<cache-repo>/.cache/<upstream-host>/<key>.meta.json
<cache-repo>/.cache/<upstream-host>/<key>.body
```

Cache keys are SHA-256 hashes of `method + path + sorted_query + body_hash`, truncated to 16 hex chars.

## Docker

```bash
docker build -t resemble .
docker run -p 9999:9999 resemble start --upstream https://api.example.com
```

## License

Apache 2.0
