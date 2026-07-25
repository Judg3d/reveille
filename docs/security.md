# Security

This page covers security notes for running Reveille behind Traefik with
Dockhand.

Reveille can start and stop Docker targets through Dockhand, so treat its
configuration and network placement as operationally sensitive.

## Trust Boundaries

The normal request path is:

```text
Browser
  -> Traefik app router
  -> Reveille forwardAuth
  -> Dockhand API
  -> Docker target
```

The browser also reaches Reveille through the public wait route:

```text
https://app.example.com/_reveille/*
```

Keep these boundaries clear:

- browsers use the public app host and `/_reveille/*`
- Traefik calls Reveille's `forwardAuth` endpoint over the internal Docker
  network
- Reveille calls Dockhand over an internal network path
- Reveille trusts forwarded headers from Traefik for host matching and redirect
  construction

## Dockhand Token

The Dockhand token is the main secret Reveille uses.

Preferred setup:

```yaml
services:
  reveille:
    env_file:
      - path: .env
        required: false
```

Put the token in `.env`:

```dotenv
DOCKHAND_API_TOKEN=<token>
```

Reveille also supports `dockhand.apiToken` in `reveille.yml`, including
environment expansion:

```yaml
dockhand:
  apiToken: "${DOCKHAND_API_TOKEN}"
```

Prefer `DOCKHAND_API_TOKEN` from the environment or a deployment secret instead
of putting the token directly in `reveille.yml`.

When a token is configured, Reveille sends it to Dockhand as:

```http
Authorization: Bearer <token>
```

If no token is configured, Reveille does not send an `Authorization` header.

## Local Ignored Files

The repository ignores local deployment files that may contain secrets, private
hostnames, or environment-specific settings:

| Path | Why It Is Local |
| --- | --- |
| `.env` and `.env.*` | deployment environment variables and tokens |
| `reveille.yml` | local runtime config |
| `compose.yml` | local deployment wiring |
| `targets/` | target hostnames, container names, and internal URLs |

The tracked examples are safe templates:

- `.env.example`
- `reveille.example.yml`
- `compose.example.yml`
- docs under `docs/targets/`

Review diffs before committing docs or examples that include real hostnames,
internal URLs, container names, or tokens.

## Public Reveille Routes

The browser-facing Reveille route is controlled by `server.publicPath`, which
defaults to:

```text
/_reveille
```

Under that prefix, Reveille serves:

| Path | Purpose |
| --- | --- |
| `/_reveille/wait` | wait page, status polling, lease creation, manual stop |
| `/_reveille/static/*` | wait-page CSS and JavaScript |
| `/_reveille/api/status` | compatibility status API |
| `/_reveille/api/lease` | compatibility lease API |
| `/_reveille/api/stop` | compatibility stop API |

Traefik must route `/_reveille/*` to Reveille. That route is public because the
browser needs it during the wait flow.

## Do Not Protect `/_reveille` With `reveille@file`

App routers should use the `reveille@file` forward-auth middleware.

The Reveille UI router must not use `reveille@file`.

If the Reveille UI route is protected by Reveille's own forward-auth middleware,
the wait page and control channel can authenticate themselves in a loop. Common
symptoms include:

- wait page loops
- timer saves fail
- `/_reveille/wait` returns unexpected redirects or errors
- `404 NOT_FOUND` appears because the app router won the route instead

Use only non-Reveille middlewares on the UI route, such as an HTTPS forwarded
header middleware if your Traefik setup requires it.

## Forwarded Headers

Reveille uses forwarded headers from Traefik:

| Header | Use |
| --- | --- |
| `X-Forwarded-Host` | target lookup during `forwardAuth` |
| `X-Forwarded-Proto` | expected origin for mutating wait-control requests |
| `X-Forwarded-Uri` | default return path after readiness |
| `routing.returnToHeader` | optional host-level replacement for return path |

Reveille sanitizes `returnTo` so redirects stay on local paths. Empty, absolute,
protocol-relative, invalid, or non-`/` paths become `/`.

Wait redirects use the trusted forwarded proto and managed target hostname to
build a public `Location` URL. They do not use the internal Reveille service
name.

Reveille also signs each wait redirect with a 24-hour token. The token is
bound to the managed host and sanitized return path, and wait/status/lease/stop
routes reject missing, expired, invalid, or host-mismatched tokens. Mutating
wait-control requests also reject mismatched `Origin` or `Referer` headers when
browsers send them.

Traefik should be the trusted source of these headers. Do not point the
`forwardAuth` middleware at a public domain; use the internal container DNS
address:

```text
http://reveille:8080/api/traefik/forward-auth
```

## Host Matching And Pass-Through

During `forwardAuth`, Reveille looks up the target by `X-Forwarded-Host`.

If the host is unknown, Reveille returns `204 No Content` and lets the request
pass. This avoids breaking unrelated Traefik routes, but it also means hostname
typos bypass Reveille instead of failing closed.

Set `server.failClosedUnknownHosts: true` when every router using the Reveille
forward-auth middleware is expected to have a matching target. In that mode,
unknown forward-auth hosts return `404 Not Found` and Reveille logs a warning.

Check that every app router using `reveille@file` has a matching target
`hostname` in the Reveille host files.

## Network Placement

Recommended network shape:

- Traefik can reach `http://reveille:8080`
- Reveille can reach Dockhand at `dockhand.baseUrl`
- Reveille can reach any configured target `healthUrl`
- browsers reach Reveille only through the app host's `/_reveille/*` route

The example Compose file exposes Reveille only to other containers on the
shared Docker network:

```yaml
expose:
  - "8080"
```

Do not publish Reveille with a host-level `ports:` mapping for normal
deployments. Browser traffic should reach Reveille only through Traefik's
`/_reveille/*` route on the managed app host.

## Logging

Reveille logs operational details such as:

- managed hostnames
- health check errors
- Dockhand errors
- lease accepted/rejected messages
- lease expiry and stop results

The Reveille code does not log the Dockhand bearer token. Still, be careful when
sharing logs because hostnames, internal URLs, container names, and health
errors may reveal deployment details.

Use `log.level: "debug"` only while actively troubleshooting. Debug logs include
status details such as readiness state, lease state, health status, and health
errors.

## Wait Session Tokens And Cookies

Reveille signs each wait redirect with a 24-hour HMAC token bound to the
managed host and sanitized return path. The wait page also stores the token in
an `HttpOnly` cookie (`reveille_wait`, `SameSite=Lax`), so follow-up status,
lease, and stop calls work without carrying the token in URLs.

Set `server.tokenSecret` so tokens survive restarts and stay verifiable:

```yaml
server:
  tokenSecret: "${REVEILLE_TOKEN_SECRET}"
```

Without it a random key is generated at startup and in-flight wait sessions
break on restart.

## Security Headers

All Reveille responses carry a strict `Content-Security-Policy` (self-hosted
assets only), `Referrer-Policy: no-referrer`, and
`X-Content-Type-Options: nosniff`. The CSP also blocks framing
(`frame-ancestors 'none'`).

## Rate Limiting

Mutating wait-control requests (lease create, stop) are rate limited per host
(burst of 10, refilling 1/s) and answer `429 Too Many Requests` beyond that.
Start operations triggered through forward-auth are deduplicated: concurrent
requests share one provider call, and excess start attempts redirect to the
wait page without contacting the provider.

## Forward-Auth Shared Secret

Any container on the shared Docker network can reach
`http://reveille:8080/api/traefik/forward-auth`. To restrict it to Traefik,
set a shared secret and add it to the middleware:

```yaml
server:
  forwardAuthSecret: "${REVEILLE_FORWARD_AUTH_SECRET}"
```

```yaml
http:
  middlewares:
    reveille:
      forwardAuth:
        address: "http://reveille:8080/api/traefik/forward-auth"
        authRequestHeaders: []
```

Configure Traefik to add the header on the way in (for example with a
`headers.customRequestHeaders` middleware chained before `reveille@file`):

```yaml
http:
  middlewares:
    reveille-secret:
      headers:
        customRequestHeaders:
          X-Reveille-Auth: "<secret>"
```

Requests without the correct `X-Reveille-Auth` header are rejected with 403.

## Health Error Detail

By default the public wait page shows generic health messages. Raw
health-check errors can include internal URLs and container names; enable
them only while troubleshooting:

```yaml
server:
  exposeHealthDetail: true
```

## Manual Start Mode

`target.<name>.startMode: manual` prevents forward-auth from starting the
target. Visitors land on the wait page and the start happens only when a
timer is chosen there. Use it for targets that internet scanners keep waking.

## Admin Dashboard

When `admin.listen` is set, Reveille serves an operator dashboard on that
listener. Do not publish it through Traefik or a host port on an untrusted
interface. Set `admin.token` whenever other workloads share the network; the
dashboard can start and stop every managed target.

## Checklist

| Item | Why |
| --- | --- |
| `DOCKHAND_API_TOKEN` comes from env or a secret | avoids committed secrets |
| `reveille.yml` and `.env` stay uncommitted | protects deployment config |
| `targets/` stays local unless intentionally sanitized | avoids leaking private hostnames and internal URLs |
| Reveille UI route does not use `reveille@file` | prevents wait-route recursion or blocking |
| App routers use `reveille@file` only where intended | controls which apps Reveille manages |
| Target `hostname` values match public app hosts | avoids accidental pass-through |
| Target `healthUrl` values use `http://` or `https://` and no embedded credentials | avoids surprising outbound request behavior |
| Forwarded headers come from Traefik | keeps host matching and redirects correct |
| Dockhand and health URLs use internal network paths | avoids unnecessary public exposure |

## Related Docs

- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Dockhand: [dockhand.md](dockhand.md)
- Runtime config: [reveille-yml.md](reveille-yml.md)
- Troubleshooting: [troubleshooting.md](troubleshooting.md)
