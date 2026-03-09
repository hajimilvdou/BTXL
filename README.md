# BTXL

English | [中文](README_CN.md)

BTXL (Bingtang Xueli) is an open-source community quota platform built on top of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It turns a single-user CLI proxy into a multi-user service with registration, quota distribution, credential pool scheduling, risk control, and a web panel.

## Overview

BTXL is designed for operators who want to run a shared AI access platform instead of a personal proxy only.

Typical flow:

1. Operators configure upstream credentials and routing rules.
2. Users register accounts and receive quota through invite/redeem workflows.
3. Requests enter BTXL through an OpenAI / Claude / Gemini compatible gateway.
4. BTXL selects an upstream credential, applies limits and security rules, then proxies the request.
5. Operators manage users, credentials, quota, and system settings through the web panel.

## What BTXL Adds on Top of CLIProxyAPI

- Multi-user community platform
- Quota templates, redemption codes, invite / referral workflows
- Shared credential pool management with scheduling and health control
- User/admin web panel
- Risk control, IP policy, rate limiting, anomaly detection, audit logs
- Community-oriented operations capabilities instead of single-user local usage only

## Tech Stack

| Layer | Technology |
| --- | --- |
| Backend | Go 1.26, Gin |
| Database | SQLite by default (`modernc.org/sqlite`, CGO-free), optional PostgreSQL in some storage flows |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS, Zustand, Recharts |
| Auth | JWT + provider-specific OAuth flows |
| Container | Docker multi-arch image |
| CI/CD | GitHub Actions + GHCR |

## Port Reference

This is the most important deployment clarification for this repository.

### Runtime Port to Expose on the Server

| Port | Required | Purpose |
| --- | --- | --- |
| `8317` | Yes | Main HTTP service port. Serves the proxy API and, when enabled in config, the community web panel and management routes. |

### OAuth Callback Ports

The following ports exist in code for provider login callback helpers. They are **not normal server runtime ports** and should **not** be exposed in a standard Docker server deployment.

| Port | Provider / Flow | Usage |
| --- | --- | --- |
| `8085` | Gemini | Localhost callback used during Gemini OAuth helper flow |
| `1455` | Codex / OpenAI OAuth | Localhost callback used during Codex/OpenAI OAuth helper flow |
| `54545` | Claude | Localhost callback used during Claude OAuth helper flow |
| `51121` | Antigravity | Localhost callback used during Antigravity OAuth helper flow |
| `11451` | iFlow | Localhost callback used during iFlow OAuth helper flow |

Important notes:

- For **containerized server deployment**, only publish `8317`.
- Publishing the callback ports does **not** make remote web-panel OAuth magically work, because those flows are based on `localhost` callback semantics.
- If your hosting platform auto-detects ports from the image, it should only need `8317`.

## Why the Previous Deployment Looked Wrong

The old compose file published multiple OAuth callback ports. That is misleading in a server deployment because:

- the application itself only listens as a normal HTTP service on the configured main port (`8317` by default);
- the extra ports are callback helper ports for specific OAuth flows, not public runtime service ports;
- many PaaS / panel products auto-open only the image's real service port, so exposing callback ports in docs or compose creates confusion rather than solving deployment;
- if you pull and run the image directly without mounting a config file, the process may fail or idle depending on deployment mode.

This repository now treats `8317` as the only normal Docker-exposed runtime port.

## Docker Deployment

### Docker Compose

The compose service and container are now both named `btxl`.

```bash
git clone https://github.com/hajimilvdou/BTXL.git
cd BTXL

cp config.example.yaml config.yaml
# Edit config.yaml before production use

docker compose up -d
```

Current compose behavior:

- Service name: `btxl`
- Container name: `btxl`
- Published port: `8317:8317`
- Mounted config path inside container: `/opt/btxl/config.yaml`
- Mounted auth directory inside container: `/root/.btxl`
- Mounted log directory inside container: `/opt/btxl/logs`

### Direct Image Deployment

Image:

```bash
ghcr.io/hajimilvdou/btxl:latest
```

The image now contains a fallback `/opt/btxl/config.yaml` generated from `config.example.yaml`, so the container can boot more predictably when a platform pulls the image directly.

For real deployment, you should still mount your own config and persistent data:

```bash
docker run -d \
  --name btxl \
  -p 8317:8317 \
  -v $(pwd)/config.yaml:/opt/btxl/config.yaml \
  -v $(pwd)/auths:/root/.btxl \
  -v $(pwd)/logs:/opt/btxl/logs \
  ghcr.io/hajimilvdou/btxl:latest
```

### Docker Environment Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `BTXL_IMAGE` | `ghcr.io/hajimilvdou/btxl:latest` | Image tag used by `docker compose` |
| `BTXL_CONFIG_PATH` | `./config.yaml` | Host-side config file mounted into container |
| `BTXL_AUTH_PATH` | `./auths` | Host-side auth directory mounted to `/root/.btxl` |
| `BTXL_LOG_PATH` | `./logs` | Host-side log directory mounted into container |

## Configuration Notes

### `config.example.yaml`

The shipped example config is a bootable template, but you still need to customize it for production.

Pay special attention to:

- `port`: main runtime port, default `8317`
- `api-keys`: client-facing API keys
- `auth-dir`: where provider credentials are stored
- `remote-management.secret-key`: management API key if you use management routes
- `community.*`: enables the BTXL community platform features

### Web Panel Availability

The BTXL web panel is not implicitly enabled just because the frontend code exists in the repository.

The legacy upstream management page has been removed from BTXL.

To expose `/panel`, you must enable the community module in configuration, including panel settings such as:

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

If `community.enabled` is disabled or commented out, the proxy may still start, but the BTXL community panel routes will not be available.

BTXL now only keeps the new community panel under `/panel`.

## GitHub Actions / Image Publishing

The repository already publishes Docker images through GitHub Actions to GHCR.

Current delivery chain:

1. `docker-image.yml` builds `amd64` and `arm64` images.
2. GitHub Actions pushes architecture-specific tags.
3. A multi-arch manifest is created and pushed.
4. Server-side deployment platforms can pull `ghcr.io/hajimilvdou/btxl:latest`.

Deployment recommendation for server panels or PaaS:

- publish only port `8317`;
- mount a real `config.yaml` for production;
- persist `auths` and `logs` directories;
- do not treat provider callback ports as public service ports.

## Local Build

### Build Binary

```bash
go build -o btxl ./cmd/server
./btxl
```

### Docker Helper Scripts

- `docker-build.sh`
- `docker-build.ps1`

These scripts now use the local image tag `btxl:local`.

## Project Structure

```text
cmd/server/              entrypoint
internal/community/      BTXL community platform core
internal/panel/web/      React frontend
internal/api/            HTTP server and management handlers
auths/                   local provider credentials directory
docs/                    project documentation
test/                    integration / regression tests
```

## Troubleshooting

### Container starts but the service is unavailable

Check the following in order:

1. Confirm the platform publishes `8317`.
2. Confirm the process is using a valid `config.yaml`.
3. Confirm the service is not waiting for cloud-deploy configuration.
4. Confirm your mounted config has a valid `port` and no YAML syntax errors.

### Server deployment cannot complete provider OAuth from the web panel

This is usually **not** a Docker port publishing problem.

The relevant provider flows rely on localhost callback behavior. In other words, opening `8085`, `1455`, `54545`, `51121`, or `11451` on a public server is normally the wrong fix.

### `/panel` returns 404

Enable `community.enabled` and `community.panel.enabled` in `config.yaml`.

## Attribution

This project is a derivative work of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) by Router-For.ME.

## License

MIT. See `LICENSE`.
