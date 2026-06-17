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
    environment:
      DOCKHAND_API_TOKEN: ${DOCKHAND_API_TOKEN:-}
    command:
      - -config
      - /etc/reveille/reveille.yml
      - -hosts
      - /etc/reveille/hosts
    volumes:
      - ./reveille.yml:/etc/reveille/reveille.yml:ro
      - ./targets:/etc/reveille/hosts:ro
    networks:
      - <traefik-shared-network>

networks:
  <traefik-shared-network>:
    external: true
```

Create local config from the example:

```sh
cp reveille.example.yml reveille.yml
cp .env.example .env
```

## Run Locally

```sh
go test ./...
go run ./cmd/reveille -config reveille.yml -hosts targets
```

## Documentation

- Traefik quick start: [docs/traefik/get-started.md](docs/traefik/get-started.md)
- Traefik reference: [docs/traefik/reference.md](docs/traefik/reference.md)
- Target quick start: [docs/targets/get-started.md](docs/targets/get-started.md)
- Target parser reference: [docs/targets/parser-reference.md](docs/targets/parser-reference.md)
- Runtime config reference: [docs/reveille-yml.md](docs/reveille-yml.md)
- Changelog: [changelog.md](changelog.md)
