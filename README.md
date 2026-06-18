# Reveille

Reveille is an on-demand lifecycle manager for homelab services behind Traefik.
It wakes stopped Dockhand-managed containers or stacks when someone visits their
public domain, shows a timer selection page while the app starts, and lets
Traefik resume normal routing once the app is healthy.

```text
Browser -> Traefik app router -> Reveille forwardAuth -> Dockhand -> Target
                              \-> /_reveille/wait
```

## What It Does

- Integrates with Traefik through `forwardAuth`.
- Starts and stops targets through the Dockhand API.
- Shows a browser wait page with lease/timer selection.
- Polls readiness before redirecting back to the requested app.
- Stops finite leases automatically when their timer expires.
- Loads target definitions from YAML files.

## Minimal Compose

```yaml
services:
  reveille:
    image: your-registry/reveille:latest
    container_name: reveille
    restart: unless-stopped
    env_file:
      - path: .env
        required: false
    expose:
      - "8080"
    volumes:
      - ./reveille.yml:/etc/reveille/reveille.yml:ro
      - ./targets:/etc/reveille/hosts:ro
    networks:
      - <traefik-shared-network>

networks:
  <traefik-shared-network>:
    external: true
```

Reveille is designed to sit behind Traefik. The Compose examples use `expose`
so Traefik can reach Reveille on the shared Docker network without publishing a
host-level port.

Create local config from the example:

```sh
cp reveille.example.yml reveille.yml
cp .env.example .env
```

Put `DOCKHAND_API_TOKEN` in `.env` when Dockhand authentication is enabled.
Compose loads that file into the container through `env_file`.

## Run Locally

```sh
go test ./...
go run ./cmd/reveille -config reveille.yml -hosts targets
```

## Documentation

See [docs/](docs/) for setup guides, runtime behavior, configuration
reference, and troubleshooting notes.
