# Reveille

On-demand container lifecycle manager for homelab services behind Traefik.

Reveille is designed to run as a Traefik `forwardAuth` middleware. When a
request arrives for a managed host, Traefik asks Reveille whether the upstream
container is ready. Reveille starts stopped containers through the Dockhand API,
shows a waiting/timer UI, then lets Traefik send the original request to the
real service once it is healthy.

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

## Request Flow

1. A user opens `https://app.example.com/`.
2. The Traefik router for `app.example.com` runs the `reveille@file`
   `forwardAuth` middleware.
3. Traefik calls Reveille at `/api/traefik/forward-auth` and includes
   `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-Proto`, and related
   headers.
4. Reveille finds the matching host config.
5. If the target is already healthy, Reveille returns `204 No Content` and
   Traefik forwards the original request to the target service.
6. If the target is stopped, Reveille calls Dockhand to start it and returns a
   redirect to `/_reveille/wait?...`.
7. The browser loads Reveille's wait UI through Traefik.
8. The wait UI lets the user choose a lease duration, including `Never`.
9. The wait UI polls `/_reveille/api/status` every 5 seconds until the target
   health check passes. As a no-JavaScript fallback, the page may refresh itself
   every 5 seconds.
10. Reveille redirects the browser back to the original URL once the target is
    healthy.
11. When a finite lease expires, Reveille calls Dockhand to stop the target and
    returns it to the stopped state. `Never` means Reveille leaves the target
    running until a user stops it manually or chooses a finite lease later.

## Lease Behavior

The wait page is the lease selection point for stopped targets. Users should be
able to choose one of the configured durations or `Never`.

Finite leases are tracked by host and target. If a user selects `2h`, Reveille
keeps the target available for two hours after the lease is created or extended.
When the lease ends, Reveille calls the appropriate Dockhand stop endpoint and
the container or stack returns to the stopped state.

`Never` disables automatic shutdown for that target lease. Reveille should still
show the service as managed, but it must not stop the target because of a timer
while the active lease is set to `Never`.

## Traefik Integration

Reveille needs two Traefik paths:

- `forwardAuth` middleware endpoint used by protected routers
- `/_reveille/*` UI/API route served by Reveille itself

Example dynamic file provider config:

```yaml
# traefik/dynamic/reveille.yml
http:
  middlewares:
    reveille:
      forwardAuth:
        address: http://reveille:8080/api/traefik/forward-auth
        trustForwardHeader: true

  routers:
    reveille-ui:
      rule: PathPrefix(`/_reveille`)
      entryPoints:
        - websecure
      service: reveille
      tls: true
      priority: 10000

  services:
    reveille:
      loadBalancer:
        servers:
          - url: http://reveille:8080
```

Then attach the middleware to any managed app router:

```yaml
http:
  routers:
    jellyfin:
      rule: Host(`jellyfin.example.com`)
      entryPoints:
        - websecure
      service: jellyfin
      middlewares:
        - reveille@file
      tls: true

  services:
    jellyfin:
      loadBalancer:
        servers:
          - url: http://jellyfin:8096
```

Docker label equivalent for an app container:

```yaml
services:
  jellyfin:
    image: jellyfin/jellyfin
    labels:
      - traefik.enable=true
      - traefik.http.routers.jellyfin.rule=Host(`jellyfin.example.com`)
      - traefik.http.routers.jellyfin.entrypoints=websecure
      - traefik.http.routers.jellyfin.tls=true
      - traefik.http.routers.jellyfin.middlewares=reveille@file
      - traefik.http.services.jellyfin.loadbalancer.server.port=8096
```

## Dockhand Integration

Reveille talks to Dockhand over HTTP and does not need direct access to
`/var/run/docker.sock`.

Dockhand config in `config.yml`:

```yaml
server:
  listen: ":8080"
  publicPath: "/_reveille"

dockhand:
  baseUrl: "http://dockhand:3000"
  apiToken: "${DOCKHAND_API_TOKEN}"
  environmentId: 1
  timeout: "30s"

defaults:
  lease: "2h"
  leaseOptions:
    - "30m"
    - "1h"
    - "2h"
    - "4h"
    - "never"
  startTimeout: "3m"
  stopGrace: "30s"
  pollInterval: "5s"
```

Expected Dockhand API calls:

```http
GET  /api/containers?env=1
GET  /api/containers/{id}/inspect?env=1
POST /api/containers/{id}/start?env=1
POST /api/containers/{id}/stop?env=1
```

For Dockhand-managed compose stacks, Reveille can use stack endpoints instead:

```http
POST /api/stacks/{name}/start?env=1
POST /api/stacks/{name}/stop?env=1
```

Every request should include:

```http
Authorization: Bearer dh_xxx
Accept: application/json
```

Requests with JSON bodies must also include:

```http
Content-Type: application/json
```

## Host Config

Reveille follows Traefik's static plus dynamic config pattern:

- `config.yml` contains Reveille's own settings
- `hosts/` contains one file per managed host and is watched for live changes

Example host file:

```yaml
# hosts/jellyfin.yml
host: jellyfin.example.com

target:
  type: container
  id: jellyfin
  healthUrl: http://jellyfin:8096/health
  healthyStatus:
    - 200
    - 302

lease:
  default: 2h
  options:
    - 30m
    - 1h
    - 2h
    - 4h
    - never

routing:
  returnToHeader: X-Forwarded-Uri
```

Stack target example:

```yaml
# hosts/paperless.yml
host: paperless.example.com

target:
  type: stack
  name: paperless
  healthUrl: http://paperless-webserver:8000/
  healthyStatus:
    - 200
    - 302
```

## Compose Example

```yaml
services:
  traefik:
    image: traefik:v3
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --providers.file.directory=/etc/traefik/dynamic
      - --providers.file.watch=true
      - --entrypoints.web.address=:80
      - --entrypoints.websecure.address=:443
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./traefik/dynamic:/etc/traefik/dynamic:ro

  dockhand:
    image: fnsys/dockhand:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - dockhand_data:/app/data

  reveille:
    image: ghcr.io/example/reveille:latest
    environment:
      - DOCKHAND_API_TOKEN=${DOCKHAND_API_TOKEN}
    volumes:
      - ./config.yml:/config/config.yml:ro
      - ./hosts:/config/hosts:ro
    labels:
      - traefik.enable=true
      - traefik.http.routers.reveille.rule=PathPrefix(`/_reveille`)
      - traefik.http.routers.reveille.entrypoints=websecure
      - traefik.http.routers.reveille.tls=true
      - traefik.http.routers.reveille.priority=10000
      - traefik.http.services.reveille.loadbalancer.server.port=8080

volumes:
  dockhand_data:
```

## HTTP Contract

Reveille should expose:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/traefik/forward-auth` | Called by Traefik `forwardAuth` |
| `GET` | `/_reveille/wait` | Lease selection and startup wait page |
| `GET` | `/_reveille/api/status` | Poll target status every 5 seconds |
| `POST` | `/_reveille/api/lease` | Create or extend a lease |
| `POST` | `/_reveille/api/stop` | Stop a target early |
| `GET` | `/healthz` | Reveille health check |

Forward-auth responses:

| State | Response |
| --- | --- |
| Host is unmanaged | `204 No Content` |
| Target is healthy | `204 No Content` |
| Target is starting | `302 Location: /_reveille/wait?...` |
| Target failed to start | `503 Service Unavailable` |
| Config or Dockhand error | `500 Internal Server Error` |

## Security Notes

- Keep Dockhand and Reveille on an internal Docker network.
- Do not expose Dockhand's API publicly.
- Store `DOCKHAND_API_TOKEN` as a secret or environment variable, not in Git.
- Give the Dockhand token only the access Reveille needs when RBAC is available.
- Preserve Traefik's forwarded headers only from trusted proxy networks.
- Validate redirect targets so `returnTo` cannot become an open redirect.

## References

- Traefik `forwardAuth` middleware returns the original request only when the
  auth service responds with a 2xx status; otherwise Traefik returns the auth
  service response to the client.
- Dockhand exposes container lifecycle endpoints such as
  `POST /api/containers/{id}/start` and `POST /api/containers/{id}/stop`, stack
  lifecycle endpoints, and Bearer API tokens for automation.
