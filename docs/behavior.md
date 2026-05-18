# Reveille Behavior

This document describes how Reveille behaves at runtime and how it wires into
Traefik and Dockhand.

## Runtime Flow

1. A user opens `https://app.example.com/`.
2. The Traefik router for `app.example.com` runs the `reveille@file`
   `forwardAuth` middleware.
3. Traefik calls Reveille at `/api/traefik/forward-auth` and includes
   `X-Forwarded-Method`, `X-Forwarded-Host`, `X-Forwarded-Uri`,
   `X-Forwarded-Proto`, and `X-Forwarded-For`.
4. Reveille finds the matching managed host config.
5. If the target is already healthy, Reveille returns `204 No Content`.
   Traefik then forwards the original request to the target service.
6. If the target is stopped, Reveille calls Dockhand to start the container or
   stack and redirects the browser to `/_reveille/wait?...`.
7. The wait page lets the user choose a lease duration, including `Never`.
8. The wait page polls `/_reveille/api/status` every 5 seconds until the target
   health check passes. As a no-JavaScript fallback, the page may refresh every
   5 seconds.
9. Once the target is healthy, Reveille redirects the browser back to the
   original URL.
10. When a finite lease expires, Reveille calls Dockhand to stop the target and
    return it to the stopped state.

## Lease Behavior

The wait page is the lease selection point for stopped targets. Users should be
able to choose one of the configured durations or `Never`.

Finite leases are tracked by host and target. If a user selects `2h`, Reveille
keeps the target available for two hours after the lease is created or extended.
When the lease ends, Reveille calls the appropriate Dockhand stop endpoint and
the container or stack returns to the stopped state.

`Never` disables automatic shutdown for that target lease. Reveille still shows
the service as managed, but it must not stop the target because of a timer while
the active lease is set to `Never`.

## Traefik Wiring

Traefik keeps ownership of routing. Reveille only participates through a
`forwardAuth` middleware plus its own wait UI route.

Reveille needs two Traefik pieces:

- `forwardAuth` middleware endpoint used by protected routers
- `/_reveille/*` UI/API route served by Reveille itself

Static Traefik config must enable the providers you use. A typical Docker setup
uses Docker labels for app routers and the file provider for the reusable
Reveille middleware:

```yaml
# traefik.yml
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

providers:
  docker:
    exposedByDefault: false
  file:
    directory: /etc/traefik/dynamic
    watch: true
```

CLI equivalent:

```yaml
command:
  - --providers.docker=true
  - --providers.docker.exposedbydefault=false
  - --providers.file.directory=/etc/traefik/dynamic
  - --providers.file.watch=true
  - --entrypoints.web.address=:80
  - --entrypoints.websecure.address=:443
```

If Traefik is behind another proxy, configure trusted forwarded-header IPs at
the entrypoint level. Then set `trustForwardHeader: true` on the middleware so
Reveille receives the sanitized `X-Forwarded-*` headers it needs.

Dynamic file provider config:

```yaml
# traefik/dynamic/reveille.yml
http:
  middlewares:
    reveille:
      forwardAuth:
        address: http://reveille:8080/api/traefik/forward-auth
        trustForwardHeader: true
        authRequestHeaders:
          - X-Forwarded-Method
          - X-Forwarded-Proto
          - X-Forwarded-Host
          - X-Forwarded-Uri
          - X-Forwarded-For

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

The `reveille-ui` router must not use the `reveille@file` middleware, otherwise
the wait page can authenticate itself in a loop.

Attach the middleware to any managed app router. File-provider example:

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

Docker label equivalent for a managed app container:

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

With this shape, Traefik still sends successful requests to the normal app
service. Reveille returns a non-2xx response only when it needs Traefik to show
the wait flow or an error.

## Dockhand Wiring

Reveille talks to Dockhand over HTTP and does not need direct access to
`/var/run/docker.sock`.

Dockhand's source route tree is under `src/routes/api`. The lifecycle paths
Reveille needs live under `api/containers` and `api/stacks/[name]`, and the
published Dockhand API reference documents the matching REST endpoints.

Config shape:

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

Expected Dockhand container calls:

```http
GET  /api/containers?all=true&env=1
GET  /api/containers/{id}/inspect?env=1
POST /api/containers/{id}/start?env=1
POST /api/containers/{id}/stop?env=1
```

`all=true` matters for discovery because Reveille must be able to find stopped
containers.

For Dockhand-managed compose stacks, Reveille can use stack endpoints instead:

```http
POST /api/stacks/{name}/start?env=1
POST /api/stacks/{name}/stop?env=1
```

Every Dockhand request should include:

```http
Authorization: Bearer dh_xxx
Accept: application/json
```

Requests with JSON bodies must also include:

```http
Content-Type: application/json
```

Prefer container IDs internally once discovered, because container names can be
renamed. Host config may still accept a friendly name and resolve it with
`GET /api/containers?all=true&env=1`.

## Host Config

Reveille follows Traefik's static plus dynamic config pattern:

- `config.yml` contains Reveille's own settings
- `hosts/` contains one file per managed host and is watched for live changes

Container target example:

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

## Reveille HTTP Contract

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

- Dockhand route tree:
  <https://github.com/Finsys/dockhand/tree/main/src/routes/api>
- Dockhand API reference:
  <https://dockhand.pro/manual/#api-reference>
- Traefik ForwardAuth:
  <https://doc.traefik.io/traefik/reference/routing-configuration/http/middlewares/forwardauth/>
- Traefik file provider:
  <https://doc.traefik.io/traefik/reference/install-configuration/providers/others/file/>
- Traefik Docker provider:
  <https://doc.traefik.io/traefik/reference/install-configuration/providers/docker/>
