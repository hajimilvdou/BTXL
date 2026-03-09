# BTXL

English | [中文](README_CN.md)

BTXL is an open-source community quota platform built on top of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It extends a single-user CLI proxy into an operator-facing multi-user service with quota distribution, credential pool management, risk control, and a web panel.

## Overview

BTXL is intended for operators who need to provide shared model access to a team or community.

Core flow:

1. Operators configure upstream credentials, routing rules, and security policies.
2. Users register accounts and receive quota through invite or redemption workflows.
3. Requests enter BTXL through OpenAI-, Claude-, or Gemini-compatible endpoints.
4. BTXL selects an upstream credential, applies quota and security checks, and forwards the request.
5. Operators manage users, credentials, quota, and settings through the web panel.

## Key Capabilities

- Multi-user account system with JWT authentication
- Shared credential pool and model routing
- Invite, referral, and redemption workflows
- Quota allocation and policy controls
- Risk control, IP policy, anomaly detection, and audit logs
- Embedded web panel for user and administrator operations

## Architecture

| Layer | Technology |
| --- | --- |
| Backend | Go 1.26, Gin |
| Database | SQLite by default (`modernc.org/sqlite`, CGO-free) |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS |
| Authentication | JWT + provider-specific OAuth flows |
| Container | Docker multi-arch image |
| Delivery | GitHub Actions + GHCR |

## Ports and Access Model

### Runtime Port

| Port | Required | Purpose |
| --- | --- | --- |
| `8317` | Yes | Main HTTP service port. Serves the API gateway, web panel, and management API. |

### Access Paths

BTXL serves both the API and the panel on the same runtime port.

| Function | Path |
| --- | --- |
| Root endpoint | `/` |
| Web panel | `/panel/` |
| OpenAI-compatible API | `/v1/...` |
| Gemini-compatible API | `/v1beta/...` |
| Management API | `/v0/management/...` |

Operational behavior:

- Browser requests to `/` are redirected to `/panel/` when the community panel is enabled.
- Non-browser clients continue to receive the JSON root response.
- No separate panel port is required.

### OAuth Callback Ports

The following ports appear in provider login code but are not normal server runtime ports and should not be exposed in standard public deployment:

| Port | Purpose |
| --- | --- |
| `8085` | Gemini localhost callback |
| `1455` | Codex / OpenAI OAuth localhost callback |
| `54545` | Claude localhost callback |
| `51121` | Antigravity localhost callback |
| `11451` | iFlow localhost callback |

## Deployment

### Docker Compose

The main `docker-compose.yml` is deployment-oriented and intended for direct use.

```bash
git clone https://github.com/hajimilvdou/BTXL.git
cd BTXL
docker compose up -d
```

On first startup, the container initializes the following host-side paths automatically:

- `./data/config/config.yaml`
- `./data/auths/`
- `./data/logs/`

### Deployment Model

| Item | Value |
| --- | --- |
| Service name | `btxl` |
| Container name | `btxl` |
| Published port | `${BTXL_PORT:-8317}:8317` |
| Config mount | `${BTXL_CONFIG_DIR:-./data/config}` -> `/opt/btxl/config` |
| Auth mount | `${BTXL_AUTH_PATH:-./data/auths}` -> `/root/.btxl` |
| Log mount | `${BTXL_LOG_PATH:-./data/logs}` -> `/opt/btxl/logs` |
| Health check | `http://127.0.0.1:8317/` |

### Direct Image Deployment

Image:

```bash
ghcr.io/hajimilvdou/btxl:latest
```

The container image supports both file-style and directory-style configuration mounts. If no runtime configuration is present, it initializes `config/config.yaml` from `config.example.yaml`.

Example:

```bash
docker run -d \
  --name btxl \
  -p 8317:8317 \
  -v $(pwd)/data/config:/opt/btxl/config \
  -v $(pwd)/data/auths:/root/.btxl \
  -v $(pwd)/data/logs:/opt/btxl/logs \
  ghcr.io/hajimilvdou/btxl:latest
```

Preparation example:

```bash
mkdir -p data/config data/auths data/logs
cp config.example.yaml data/config/config.yaml
```

### Environment Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `BTXL_IMAGE` | `ghcr.io/hajimilvdou/btxl:latest` | Image tag used by `docker compose` |
| `BTXL_PORT` | `8317` | Host-side published port |
| `BTXL_CONFIG_DIR` | `./data/config` | Host-side config directory |
| `BTXL_AUTH_PATH` | `./data/auths` | Host-side auth directory |
| `BTXL_LOG_PATH` | `./data/logs` | Host-side log directory |

### Development Build

Local source builds use `docker-compose.dev.yml` as an override file.

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml build
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

## Configuration

### `config.example.yaml`

The example configuration is a runnable baseline and should be reviewed before production rollout.

Important fields:

- `port`: main runtime port, default `8317`
- `api-keys`: client-facing API keys
- `auth-dir`: credential storage directory
- `remote-management.secret-key`: management API key
- `community.*`: community platform and panel settings

### Community Panel

The embedded panel is controlled by the `community` section.

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

New deployments initialized from the current image enable the community panel by default.

For existing deployments, verify that your current `data/config/config.yaml` includes the `community` section above. If it does not, `/panel/` will return `404` and `/` will continue to show the API root response.

### Security Note

The default `jwt-secret` in `config.example.yaml` is a placeholder. Replace it before production use.

## Operational Notes

- API and panel share the same port.
- Public deployment normally requires only `8317`.
- Management API remains available under `/v0/management/...`.
- OAuth callback helper ports should not be published as public service ports.

## Troubleshooting

### Root path shows JSON instead of the panel

Check the active runtime configuration at `data/config/config.yaml`.

Common causes:

- `community.enabled` is missing or `false`
- `community.panel.enabled` is missing or `false`
- The container is still using an older configuration file created before panel defaults were introduced

### `/panel/` returns `404`

Confirm that the active configuration contains:

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

### Docker reports `not a directory` while mounting `config.yaml`

Some panels create a missing host-side `config.yaml` path as a directory. BTXL avoids this by using directory mounts:

- Host: `./data/config`
- Container: `/opt/btxl/config`
- Actual config file: `/opt/btxl/config/config.yaml`

### Provider OAuth cannot complete from the web panel

This is usually not a Docker port publishing problem. The provider flows rely on localhost callback semantics and are separate from the main runtime port.

## Project Structure

```text
cmd/server/              entrypoint
internal/community/      community platform core
internal/panel/web/      React frontend
internal/api/            HTTP server and management handlers
docs/                    project documentation
test/                    integration and regression tests
```

## Attribution

This project is a derivative work of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) by Router-For.ME.

## License

MIT. See `LICENSE`.
