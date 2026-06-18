# Readiness

Reveille uses readiness checks to decide whether Traefik can send the browser to
the target app or should keep showing the wait UI.

The important distinction:

**Dockhand readiness and `healthUrl` readiness are alternatives for deciding
whether the target is ready, but Dockhand is still used for start and stop either
way.**

If a target does not define `healthUrl`, Reveille asks Dockhand whether the
target is ready. If a target defines `healthUrl`, Reveille checks that URL
instead. In both cases, Reveille still uses Dockhand to start and stop the
container or stack.

## Overview

Readiness is checked in two places:

- Traefik `forwardAuth`, before the request reaches the app
- wait UI status polling, while the browser is waiting

When readiness passes during `forwardAuth`, Reveille returns `204 No Content`
and Traefik forwards the original request to the app.

When readiness does not pass, Reveille starts the target through Dockhand and
redirects the browser to the wait UI.

## Readiness Sources

### Dockhand Readiness

Dockhand readiness is used when the target does not define `healthUrl`.

For container targets, Reveille asks Dockhand whether the container is ready.
Dockhand's answer reflects Docker state and health information. If Docker health
status exists, Dockhand can wait for `healthy`; otherwise it can treat a running
container as ready.

Dockhand is also the control plane for start and stop calls.

### Health URL Readiness

`healthUrl` readiness is used when the target defines `healthUrl`.

In this mode, Reveille performs an HTTP `GET` to the configured URL and compares
the response status with `healthyStatus`. A matching status is healthy. A
non-matching status is unhealthy. A request or connection error is unreachable.

`healthUrl` replaces Dockhand readiness for the ready/not-ready decision only.
Dockhand is still used to start and stop the target.

Configured health URLs must be absolute `http://` or `https://` URLs with a
host. Embedded credentials are rejected, and URL fragments are ignored. Private,
LAN, Docker, and Tailscale addresses are allowed because health checks commonly
use internal network paths.

Stack targets must define `healthUrl` because a stack can contain multiple
containers.

## Healthy Status

Use `healthyStatus` to list the HTTP response codes that count as healthy:

```yaml
target:
  jellyfin:
    id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com
    healthUrl: http://jellyfin:8096/health
    healthyStatus:
      - 200
      - 302
```

If `healthyStatus` is omitted, Reveille defaults it to `[200]`.

Health URL checks use a 5-second request timeout. Invalid URLs, DNS failures,
connection failures, and timeouts are reported as health errors.

## Readiness During ForwardAuth

The browser-facing app router uses Traefik `forwardAuth` before sending a
request to the target app:

1. Traefik calls `/api/traefik/forward-auth`.
2. Reveille looks up the target by `X-Forwarded-Host`.
3. Reveille checks readiness.
4. If ready, Reveille returns `204 No Content`.
5. If not ready, Reveille asks Dockhand to start the target.
6. Reveille redirects the browser to the wait UI.

If the forwarded host is unknown, Reveille returns `204 No Content` and lets the
request pass. This keeps Reveille from blocking unrelated Traefik routes.
Set `server.failClosedUnknownHosts: true` to return `404 Not Found` for unknown
forward-auth hosts instead.

If the start call fails, Reveille returns a service error instead of redirecting
to the wait UI.

## Readiness During Wait UI Polling

After the browser reaches the wait UI, it polls:

```text
GET /_reveille/wait?host=app.example.com&returnTo=/docs&format=status&token=<wait-token>
```

The status response tells the UI whether the target is healthy, whether a lease
is active, and what readiness message to show.

The wait UI redirects only when:

- readiness is healthy
- a lease is active
- this browser session has started a timer

This means a healthy target can still show timer selection if the current
browser session has not started a timer yet.

## Status Fields

Status JSON includes readiness and lease fields:

| Field | Meaning |
| --- | --- |
| `healthy` | Whether readiness currently passes |
| `readinessState` | Server classification for the UI |
| `statusMessage` | User-facing readiness message |
| `healthStatus` | HTTP status returned by `healthUrl` |
| `healthError` | Connection, request, or URL error from `healthUrl` |
| `lastCheck` | Timestamp of the last health URL check |
| `leaseActive` | Whether a timer is active |
| `expiresAt` | Expiration timestamp for a finite active lease |
| `never` | Whether the active lease disables automatic stop |

Example:

```json
{
  "host": "app.example.com",
  "healthy": false,
  "returnTo": "/docs",
  "leaseActive": true,
  "expiresAt": "2026-06-17T18:30:00Z",
  "readinessState": "health_unhealthy",
  "statusMessage": "App start was requested, but the health endpoint is responding with a non-healthy status.",
  "healthStatus": 503,
  "lastCheck": "2026-06-17T16:30:00Z"
}
```

## Readiness States

### `ready`

The target is ready to receive traffic.

If a lease is active, the wait UI redirects. If no lease is active, the wait UI
asks the user to start a timer first.

### `waiting_for_health`

The target is not ready yet, and Reveille does not have a specific health URL
failure to show.

This is common while a target is still starting or when readiness comes from
Dockhand instead of a configured `healthUrl`.

### `health_unreachable`

Reveille could not reach the configured `healthUrl`.

Common causes include bad URLs, DNS problems, missing Docker network access,
connection refused errors, and request timeouts.

### `health_unhealthy`

Reveille reached the configured `healthUrl`, but the response status was not in
`healthyStatus`.

For example, if `healthyStatus` is `[200]` and the endpoint returns `503`, the
target is reachable but unhealthy.

## Lease Interaction

Readiness and leases answer different questions:

- readiness asks whether the target can receive traffic
- leases ask how long Reveille should keep the target running

The wait UI needs both. It will not redirect only because the target is healthy;
the browser session must also have started a timer. Likewise, lease expiry can
stop a target even if readiness is healthy.

`never` changes automatic stop behavior, not readiness behavior. A `never` lease
can still wait on `healthUrl`, report unhealthy statuses, or redirect once the
target becomes ready.

## Troubleshooting

| Symptom | Likely Cause | Check |
| --- | --- | --- |
| Countdown never redirects | Health URL is unhealthy or unreachable | Status JSON `readinessState`, `healthStatus`, and `healthError` |
| App opens while stopped | App router is missing forwardAuth or host is unknown | Traefik router middleware and host file hostname |
| Wait page says health endpoint is not healthy | Response status is not in `healthyStatus` | App health response and host config |
| Wait page says Reveille cannot reach the health endpoint | Bad URL, DNS, network, or timeout | Reveille container network and `healthUrl` |
| Dockhand says healthy but UI keeps waiting | `healthUrl` overrides Dockhand readiness | Host target config |
| Stack target fails validation | Stack is missing `healthUrl` | Stack target entry |

## Related Docs

- Runtime flow: [runtime-flow.md](runtime-flow.md)
- Wait UI: [wait-ui.md](wait-ui.md)
- Leases: [leases.md](leases.md)
- Target parser reference: [targets/parser-reference.md](targets/parser-reference.md)
