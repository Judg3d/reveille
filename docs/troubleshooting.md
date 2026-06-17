# Troubleshooting

This page is organized by symptom. Start with what you can see, run the checks,
then follow the related docs for deeper detail.

## Baseline Checks

Start with logs:

```sh
docker logs reveille --tail 120
docker logs traefik --tail 120
docker logs dockhand --tail 120
```

Then confirm Traefik can reach Reveille:

```sh
docker exec traefik wget -qO- http://reveille:8080/healthz
```

Expected response:

```text
ok
```

When debugging network paths, run the command from the container that owns that
path. Use the Traefik container for public routing checks. Use the Reveille
container for Dockhand and target health URL checks.

## `404 NOT_FOUND` On `/_reveille/wait`

Likely causes:

- Traefik is not routing `/_reveille/*` to Reveille
- the Reveille UI route has lower priority than the app router
- the Reveille UI route points to the wrong service or port
- the Reveille UI route uses the `reveille@file` forward-auth middleware

Check the wait route through Traefik:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com&format=status&token=<wait-token>'
```

Expected result: JSON from Reveille with fields such as `healthy`,
`leaseActive`, and `readinessState`.

Use the `token` from a real Reveille redirect or wait-page URL. Missing or
mismatched tokens return `403 Forbidden`.

If the response is still `404`, check the Traefik `PathPrefix('/_reveille')`
router, its priority, and its service target.

Related docs:

- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Operations: [operations.md](operations.md)

## Wait Page Loads But Timer Save Fails

Likely causes:

- stale cached wait-page JavaScript
- `POST /_reveille/wait` is not routed to Reveille
- the browser is using an old compatibility API path
- the submitted lease is not in `lease.options`

Test the timer POST through Traefik:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  --post-data 'action=lease&lease=15m&token=<wait-token>' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com'
```

Expected result: JSON with `expiresAt` for a finite lease, or `never: true` for
a no-stop lease.

Look in Reveille logs for:

- `lease accepted for <host>`
- `lease rejected for <host>: invalid lease`

If the response is `400 invalid lease`, the submitted lease value is not one of
the configured options for that host.

If the direct POST works but the browser fails, hard-refresh the page or use a
private window. Also confirm the page source references the current
`wait.js?v=...` asset version.

Related docs:

- Wait UI: [wait-ui.md](wait-ui.md)
- Leases: [leases.md](leases.md)

## Wait Page Stuck Or Never Redirects

Likely causes:

- `healthUrl` is unreachable from the Reveille container
- `healthUrl` returns a status not listed in `healthyStatus`
- Dockhand reports the container as stopped or unhealthy
- this browser session has not started a timer
- the active lease expired

Inspect status JSON:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com&returnTo=/&format=status&token=<wait-token>'
```

Important fields:

| Field | What To Check |
| --- | --- |
| `healthy` | Must be `true` before redirect |
| `leaseActive` | Must be `true` before redirect |
| `readinessState` | Shows `ready`, `waiting_for_health`, `health_unreachable`, or `health_unhealthy` |
| `healthStatus` | Non-healthy HTTP status from `healthUrl` |
| `healthError` | URL, DNS, connection, or timeout error |
| `expiresAt` | Finite lease deadline |
| `never` | Whether automatic stop is disabled |

The wait UI redirects only after the target is healthy and the current browser
session has started a timer.

Related docs:

- Readiness: [readiness.md](readiness.md)
- Wait UI: [wait-ui.md](wait-ui.md)
- Leases: [leases.md](leases.md)

## Redirect Goes To The Wrong Host Or Scheme

Likely causes:

- `forwardAuth.trustForwardHeader` is not configured as expected
- the app router hostname does not match the Reveille target `hostname`
- host-level `routing.returnToHeader` points at the wrong header

Reveille now returns a relative wait-page redirect. It still uses
`routing.returnToHeader`, or `X-Forwarded-Uri` by default, to preserve the
original path as `returnTo`.

Check the forward-auth call:

```sh
docker exec traefik wget -S -O- \
  --header 'X-Forwarded-Host: app.example.com' \
  --header 'X-Forwarded-Proto: https' \
  --header 'X-Forwarded-Uri: /' \
  http://reveille:8080/api/traefik/forward-auth
```

If redirects point at `http://reveille:8080/...`, the running container is using
an older Reveille build.

Related docs:

- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Target parser reference: [targets/parser-reference.md](targets/parser-reference.md)

## App Opens Without Reveille Starting It

Likely causes:

- the app router does not include `reveille@file`
- the public hostname is missing from Reveille host files
- the app router host does not match the target `hostname`
- Reveille has not loaded or reloaded the expected host file

Unknown hosts pass through with `204 No Content`. This keeps Reveille from
blocking unrelated Traefik routes, but it also means hostname mismatches look
like bypasses. If every app router using `reveille@file` should be managed by
Reveille, set `server.failClosedUnknownHosts: true`; unknown hosts will return
`404 Not Found` and log a warning.

Check the app router middleware list, then run:

```sh
docker exec traefik wget -S -O- \
  --header 'X-Forwarded-Host: app.example.com' \
  --header 'X-Forwarded-Proto: https' \
  --header 'X-Forwarded-Uri: /' \
  http://reveille:8080/api/traefik/forward-auth
```

Look for `reloaded ... host entries` in Reveille logs and verify the target file
contains the same hostname Traefik uses.

Related docs:

- Runtime flow: [runtime-flow.md](runtime-flow.md)
- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Target quick start: [targets/get-started.md](targets/get-started.md)

## Dockhand Start Fails

Likely causes:

- `dockhand.baseUrl` is wrong or unreachable
- `DOCKHAND_API_TOKEN` is missing or invalid
- the target uses the wrong Dockhand environment
- the container `id` or stack `name` is wrong
- the container or stack has not been created in Dockhand/Docker yet

Look for:

```text
start <host>: ...
```

Check Dockhand from the Reveille container:

```sh
docker exec reveille wget -S -O- http://dockhand:3000/api/environments
docker exec reveille wget -S -O- 'http://dockhand:3000/api/containers?all=true&env=1'
```

Replace `1` with the target environment ID.

Related docs:

- Dockhand: [dockhand.md](dockhand.md)
- Operations: [operations.md](operations.md)

## Health URL Unreachable

Likely causes:

- `healthUrl` works from the Docker host but not from the Reveille container
- the hostname in `healthUrl` is not resolvable on Reveille's Docker network
- the app is not listening yet
- the URL is invalid
- the request timed out

Check from the Reveille container:

```sh
docker exec reveille wget -S -O- http://app:8080/health
```

In status JSON, this usually appears as:

```json
{
  "readinessState": "health_unreachable",
  "healthError": "..."
}
```

Related docs:

- Readiness: [readiness.md](readiness.md)

## Health URL Unhealthy

Likely causes:

- the endpoint returns a status not listed in `healthyStatus`
- the endpoint redirects to login
- the app is still booting
- the endpoint returns an application error

Check from the Reveille container:

```sh
docker exec reveille wget -S -O- http://app:8080/health
```

In status JSON, this usually appears as:

```json
{
  "readinessState": "health_unhealthy",
  "healthStatus": 503
}
```

Add the expected status to `healthyStatus` only when that status really means
the app is ready to receive traffic.

Related docs:

- Readiness: [readiness.md](readiness.md)
- Target parser reference: [targets/parser-reference.md](targets/parser-reference.md)

## Dockhand Health Not Ready

This applies to container targets without `healthUrl`.

Likely causes:

- the container is stopped
- Dockhand does not report `state: running`
- Dockhand `status` does not begin with `Up`
- Docker health status is present but not `healthy`

Check Dockhand container state:

```sh
docker exec reveille wget -S -O- 'http://dockhand:3000/api/containers?all=true&env=1'
```

Look for the configured container `id` or name, then inspect `state`, `status`,
and `health`.

Related docs:

- Dockhand: [dockhand.md](dockhand.md)
- Readiness: [readiness.md](readiness.md)

## Timer Not Saved Or Not Remembered

Likely causes:

- the submitted lease is not in `lease.options`
- Reveille restarted and cleared in-memory leases
- another timer save replaced the active lease
- browser session storage was cleared or belongs to a different tab session

Check timer save directly:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  --post-data 'action=lease&lease=15m&token=<wait-token>' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com'
```

Then inspect status JSON for `leaseActive`, `expiresAt`, and `never`.

Remember that backend leases live in Reveille memory. Browser session storage is
only a UI guard that records whether this browser session started a timer.

Related docs:

- Leases: [leases.md](leases.md)
- Wait UI: [wait-ui.md](wait-ui.md)

## Target Does Not Stop After Timer

Likely causes:

- the active lease is `never`
- Reveille restarted and lost the in-memory timer
- Dockhand stop failed
- a replacement lease extended the timer

Check status JSON for `never` and `expiresAt`.

Look for Reveille logs:

- `lease expired for <host>; requesting stop`
- `lease stop failed for <host>`
- `lease stop succeeded for <host>`

Related docs:

- Leases: [leases.md](leases.md)
- Dockhand: [dockhand.md](dockhand.md)

## Quick Reference

| Symptom | First Check |
| --- | --- |
| `404 NOT_FOUND` on wait route | Traefik `/_reveille` route |
| Timer save fails | `POST /_reveille/wait` through Traefik |
| Wait page never redirects | Status JSON readiness fields |
| Redirect host or scheme is wrong | Forwarded host/proto headers |
| App bypasses Reveille | App router middleware and target hostname |
| Dockhand start fails | Dockhand URL, token, environment, target ID/name |
| Target does not stop | Lease logs, `never`, and Dockhand stop |

## Related Docs

- Operations: [operations.md](operations.md)
- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Wait UI: [wait-ui.md](wait-ui.md)
- Leases: [leases.md](leases.md)
- Readiness: [readiness.md](readiness.md)
- Dockhand: [dockhand.md](dockhand.md)
