# Reveille

On-demand container lifecycle manager that sits between Traefik and Dockhand.

## Concept

When a user hits a Traefik-defined host for a service that is currently stopped, Reveille intercepts the request, brings the container up via Dockhand, and presents the user with a simple UI to select how long they need the service. When the timer expires, Reveille shuts the container back down.

```
User → Traefik → Reveille → Dockhand → Target Container
```

## Goals

- Intercept requests to configured Traefik hosts and wake the associated container
- Serve a minimal timer-selection page while the container is starting
- Forward the user through once the container is healthy
- Automatically stop the container when the selected timer expires
- Support hot-reloadable config without restarting the service

## Configuration

Follows Traefik's static + dynamic config pattern:

- **`config.yml`** — static config: Reveille's own settings (Dockhand connection, default timers, listen address)
- **`hosts/`** — dynamic config directory: one file per managed host, watched for live changes

## Stack

- Traefik — reverse proxy and host definitions
- Dockhand — Docker container start/stop control
- Reveille — this service, the glue between the two
