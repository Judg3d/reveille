# API

This page lists Reveille's HTTP endpoints.

Reveille also calls Dockhand, but this page does not duplicate Dockhand's API.
For Dockhand API details, use the Dockhand manual:

https://dockhand.pro/manual/#

## Paths

Reveille has two fixed internal endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness check |
| `GET` | `/api/traefik/forward-auth` | Traefik `forwardAuth` decision endpoint |

Browser-facing endpoints live under `server.publicPath`, which defaults to:

```text
/_reveille
```

This page uses the default prefix in examples. If `server.publicPath` changes,
replace `/_reveille` with your configured prefix.

## Host Lookup

Most Reveille endpoints need a managed host.

For wait/control and compatibility API routes, Reveille looks for the host in
this order:

1. `host` query parameter
2. `X-Forwarded-Host` header
3. request `Host`

For Traefik `forwardAuth`, Reveille uses `X-Forwarded-Host`.

Unknown wait/control hosts return `404 Not Found`. Unknown `forwardAuth` hosts
return `204 No Content` by default so unrelated Traefik routes can pass
through. Set `server.failClosedUnknownHosts: true` to make unknown
`forwardAuth` hosts return `404 Not Found`.

## `GET /healthz`

Returns a basic liveness response.

Example:

```sh
wget -qO- http://reveille:8080/healthz
```

Response:

```text
ok
```

Status:

| Status | Meaning |
| --- | --- |
| `200 OK` | Reveille HTTP server is responding |

## `GET /api/traefik/forward-auth`

Traefik calls this endpoint through the `reveille@file` middleware.

Required forwarded headers:

| Header | Purpose |
| --- | --- |
| `X-Forwarded-Host` | managed host lookup |
| `X-Forwarded-Proto` | expected origin for mutating wait-control requests |
| `X-Forwarded-Uri` | default return path |

Example:

```sh
docker exec traefik wget -S -O- \
  --header 'X-Forwarded-Host: app.example.com' \
  --header 'X-Forwarded-Proto: https' \
  --header 'X-Forwarded-Uri: /docs' \
  http://reveille:8080/api/traefik/forward-auth
```

Responses:

| Status | Meaning |
| --- | --- |
| `204 No Content` | Target is already healthy, or host is unknown while `server.failClosedUnknownHosts` is false |
| `302 Found` | Target was started and browser should go to wait UI |
| `404 Not Found` | Host is unknown while `server.failClosedUnknownHosts` is true |
| `500 Internal Server Error` | Readiness check failed unexpectedly |
| `503 Service Unavailable` | Dockhand start failed |

When Reveille redirects, the `Location` header points to:

```text
/_reveille/wait?host=app.example.com&returnTo=/docs&token=<wait-token>
```

The wait redirect is relative, so the browser stays on the same public app
origin that made the original request.

The wait token is signed by Reveille, expires after 24 hours, and is bound to
the managed host and sanitized return path. Wait, status, lease, and stop routes
reject missing, expired, invalid, or host-mismatched tokens.

## `GET /_reveille/wait`

Renders the wait page.

Example:

```text
GET /_reveille/wait?host=app.example.com&returnTo=/docs&token=<wait-token>
```

Query parameters:

| Parameter | Required | Purpose |
| --- | --- | --- |
| `host` | yes, unless forwarded/request host identifies the target | managed host |
| `returnTo` | no | local path to return to after readiness |
| `token` | yes | signed wait token from the `forwardAuth` redirect |

`returnTo` is sanitized. Empty, absolute, protocol-relative, invalid, or
non-`/` paths become `/`.

Response:

| Status | Content |
| --- | --- |
| `200 OK` | HTML wait page |
| `403 Forbidden` | Wait token is missing, invalid, expired, or for a different host |
| `404 Not Found` | Host is not managed by Reveille |
| `500 Internal Server Error` | Wait-page config could not be rendered |

## `GET /_reveille/wait?format=status`

Returns wait/status JSON. This is the canonical browser status endpoint.

Example:

```text
GET /_reveille/wait?host=app.example.com&returnTo=/docs&format=status&token=<wait-token>
```

Query parameters:

| Parameter | Required | Purpose |
| --- | --- | --- |
| `host` | yes, unless forwarded/request host identifies the target | managed host |
| `returnTo` | no | ignored when token is present; token return path is returned |
| `format=status` | yes | dispatches the wait route to status JSON |
| `token` | yes | signed wait token from the `forwardAuth` redirect |

Example response:

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

Fields:

| Field | Meaning |
| --- | --- |
| `host` | managed host |
| `healthy` | whether readiness currently passes |
| `returnTo` | sanitized return path |
| `lease` | active lease label, currently `Never` for no-stop leases |
| `leaseActive` | whether a lease is active |
| `expiresAt` | finite lease expiration timestamp |
| `never` | whether automatic stop is disabled |
| `statusMessage` | user-facing readiness message |
| `readinessState` | `ready`, `waiting_for_health`, `health_unreachable`, or `health_unhealthy` |
| `healthError` | health URL request error |
| `lastCheck` | health URL check timestamp |
| `healthStatus` | HTTP status returned by `healthUrl` |

Statuses:

| Status | Meaning |
| --- | --- |
| `200 OK` | Status JSON returned |
| `403 Forbidden` | Wait token is missing, invalid, expired, or for a different host |
| `404 Not Found` | Host is not managed by Reveille |
| `502 Bad Gateway` | Dockhand readiness check failed |

## `POST /_reveille/wait`

Creates a lease or stops the target, depending on `action`.

### Create Lease

Example form request:

```http
POST /_reveille/wait?host=app.example.com
Content-Type: multipart/form-data

action=lease
lease=2h
token=<wait-token>
```

The `action=lease` value is optional on this route. Any `POST` to
`/_reveille/wait` that is not `action=stop` is handled as a lease request.

The `lease` value must match one of the configured lease option labels. Matching
is case-insensitive, and `never` is accepted for the `Never` option. If `lease`
is omitted, Reveille uses the target's default lease.

Finite response:

```json
{
  "host": "app.example.com",
  "never": false,
  "expiresAt": "2026-06-17T18:30:00Z"
}
```

Never response:

```json
{
  "host": "app.example.com",
  "never": true
}
```

### Stop Target

Example form request:

```http
POST /_reveille/wait?host=app.example.com
Content-Type: multipart/form-data

action=stop
token=<wait-token>
```

Response:

```json
{"status":"stopped"}
```

Statuses:

| Status | Meaning |
| --- | --- |
| `200 OK` | Lease created or target stopped |
| `400 Bad Request` | Invalid lease option |
| `403 Forbidden` | Wait token is missing, invalid, expired, or for a different host |
| `404 Not Found` | Host is not managed by Reveille |
| `502 Bad Gateway` | Stop failed |

## Compatibility API

The wait route is the canonical browser control channel. Compatibility API
routes remain available under the public path.

### `GET /_reveille/api/status`

Compatibility status endpoint.

Example:

```text
GET /_reveille/api/status?host=app.example.com&returnTo=/docs&token=<wait-token>
```

Response shape and status codes match:

```text
GET /_reveille/wait?host=app.example.com&returnTo=/docs&format=status&token=<wait-token>
```

### `POST /_reveille/api/lease`

Compatibility lease endpoint.

Form example:

```http
POST /_reveille/api/lease?host=app.example.com&token=<wait-token>
Content-Type: application/x-www-form-urlencoded

lease=2h
```

JSON example:

```http
POST /_reveille/api/lease?host=app.example.com
Content-Type: application/json
X-Reveille-Token: <wait-token>

{"lease":"2h"}
```

JSON lease request bodies are limited to 1 MiB. Empty, malformed, or oversized
JSON returns `400 Bad Request`.

Response shape and status codes match the lease branch of:

```text
POST /_reveille/wait?host=app.example.com
```

### `POST /_reveille/api/stop`

Compatibility stop endpoint.

Example:

```text
POST /_reveille/api/stop?host=app.example.com&token=<wait-token>
```

Response shape and status codes match the stop branch of:

```text
POST /_reveille/wait?host=app.example.com
```

## Static Assets

Reveille serves embedded wait-page assets under:

```text
/_reveille/static/
```

Current assets:

| Path | Purpose |
| --- | --- |
| `/_reveille/static/wait.css` | wait-page styles |
| `/_reveille/static/wait.js` | wait-page behavior |

The wait page appends an asset version query string, such as:

```text
/_reveille/static/wait.js?v=<version>
```

## Related Docs

- Wait UI: [wait-ui.md](wait-ui.md)
- Readiness: [readiness.md](readiness.md)
- Leases: [leases.md](leases.md)
- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Dockhand: [dockhand.md](dockhand.md)
