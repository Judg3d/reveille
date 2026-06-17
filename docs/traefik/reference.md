# Traefik Reference

This page explains how Reveille's Traefik integration works after the YAML is
loaded. Use [get-started.md](get-started.md) when you just want the shortest
working setup.

## Request Flow

Reveille uses two separate Traefik paths:

- `forwardAuth` middleware for Traefik-to-Reveille decisions
- `/_reveille/*` route for browser-to-Reveille wait UI traffic

Runtime flow:

1. A browser opens `https://app.example.com/`.
2. The app router runs `reveille@file`.
3. Traefik calls `http://reveille:8080/api/traefik/forward-auth`.
4. Reveille checks whether the hostname is managed and whether the target is
   healthy.
5. If the target is healthy, Reveille returns `204 No Content`.
6. If the target is stopped, Reveille starts it through Dockhand and redirects
   the browser to `/_reveille/wait?...`.
7. The browser uses the `/_reveille/*` route to render the wait page, submit the
   timer, and poll readiness.

## Middleware Endpoint

The middleware address should use the internal container DNS name:

```yaml
address: http://reveille:8080/api/traefik/forward-auth
```

Do not point this at a public domain. Traefik is the caller, so it should use
the Docker network path.

Recommended middleware:

```yaml
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
```

## Browser Route

The browser route must match the public wait path:

```text
/_reveille/*
```

It must not use the `reveille@file` middleware. If it does, the wait page can
authenticate itself in a loop.

The route should have a high priority so it wins over app routers that match the
same hostname:

```yaml
priority: 100000
```

### Docker Label Route

```yaml
services:
  reveille:
    image: your-registry/reveille:latest
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=<traefik-shared-network>"
      - "traefik.http.routers.reveille-ui.rule=PathPrefix(`/_reveille`)"
      - "traefik.http.routers.reveille-ui.entrypoints=<https-entrypoint>"
      - "traefik.http.routers.reveille-ui.tls=true"
      - "traefik.http.routers.reveille-ui.tls.certresolver=<cert-resolver>"
      - "traefik.http.routers.reveille-ui.priority=100000"
      - "traefik.http.routers.reveille-ui.middlewares=<https-header-middleware>@file"
      - "traefik.http.routers.reveille-ui.service=reveille-ui"
      - "traefik.http.services.reveille-ui.loadbalancer.server.port=8080"
    networks:
      - <traefik-shared-network>
```

### File Provider Route

```yaml
http:
  routers:
    reveille-ui:
      rule: PathPrefix(`/_reveille`)
      entryPoints:
        - websecure
      service: reveille
      tls: true
      priority: 100000
      middlewares:
        - sslheader@file

  services:
    reveille:
      loadBalancer:
        servers:
          - url: http://reveille:8080
```

Use either Docker labels or a file-provider route for `reveille-ui`, not both,
unless you intentionally want duplicate provider-specific routers for
debugging.

## Wait UI Control Channel

The wait page uses `/_reveille/wait` as its browser control channel:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/_reveille/wait?...` | Render the timer-selection page |
| `GET` | `/_reveille/wait?...&format=status` | Return status JSON |
| `POST` | `/_reveille/wait?...` | Create a lease or stop the app by form action |

Compatibility routes may still exist:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/_reveille/api/status` | Status API |
| `POST` | `/_reveille/api/lease` | Lease API |
| `POST` | `/_reveille/api/stop` | Stop API |

The browser UI should not depend on Traefik routing those compatibility API
paths separately.

## Forwarded Headers

Reveille uses forwarded headers to build the public wait URL and preserve the
original destination:

- `X-Forwarded-Host`
- `X-Forwarded-Proto`
- `X-Forwarded-Uri`
- optional host-level `routing.returnToHeader`

If redirects point at `http://reveille:8080/...`, Traefik is not passing the
public forwarded host/proto that Reveille needs.

## Troubleshooting

If the request never reaches Reveille:

- verify Traefik and Reveille share a Docker network
- verify the file-provider middleware is loaded
- verify the app router includes `reveille@file`
- verify the app hostname matches the Reveille target hostname

If the wait page loads but timer updates fail with `404 NOT_FOUND`:

- verify `PathPrefix('/_reveille')` is routed to Reveille
- verify the `reveille-ui` route priority is higher than the app router
- verify the live page loads the current `wait.js` cache-buster URL
- verify `GET /_reveille/wait?...&format=status` returns JSON
- verify `POST /_reveille/wait?host=...` reaches Reveille and logs a lease
  accepted/rejected message
- hard-refresh or use a private window if Safari reused old JavaScript

If Reveille starts the target but never redirects:

- verify the host file `hostname` matches the public Traefik host
- verify the target uses the correct Dockhand `environment`
- verify `healthUrl` returns a status listed in `healthyStatus`
- inspect `/_reveille/wait?...&format=status` for `readinessState`,
  `healthError`, and `healthStatus`
