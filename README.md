# Reveille

On-demand container lifecycle manager for homelab services behind Traefik.

Reveille is designed to run as a Traefik `forwardAuth` middleware. When a user
requests a managed host whose container is stopped, Reveille starts it through
the Dockhand API, presents a wait page with lease selection, and lets Traefik
resume normal routing once the target is healthy.

```
User -> Traefik router -> Reveille forwardAuth -> Dockhand API -> Target container
                         \-> Reveille wait UI
```

## Goals

- Wake stopped containers when users request configured Traefik hosts
- Use Traefik middleware instead of replacing Traefik as the reverse proxy
- Use Dockhand's API for container and stack lifecycle operations
- Serve a minimal wait page with lease selection while the target service starts
- Forward users to the target service once health checks pass
- Stop the target container or stack when the selected lease expires
- Support hot-reloadable host config without restarting Reveille

## Configuration Direction

Reveille follows Traefik's static plus dynamic config pattern:

- `config.yml` contains Reveille's own settings, including Dockhand connection,
  default leases, poll interval, and listen address.
- `hosts/` contains one file per managed host and is watched for live changes.

## Stack

- Traefik: reverse proxy, host rules, and middleware execution
- Dockhand: Docker container and compose stack lifecycle API
- Reveille: lease tracking, wait UI, health polling, and lifecycle coordination

## Behavior

Runtime behavior, Traefik wiring, Dockhand endpoints, lease handling, and the
HTTP contract are documented in [docs/behavior.md](docs/behavior.md).
