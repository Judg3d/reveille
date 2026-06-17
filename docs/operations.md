# Operations

This page lists common commands for running and checking a containerized
Reveille deployment.

Use this when Reveille is running with Docker or Compose and you want to verify
that Traefik, Reveille, Dockhand, and a managed target can all talk to each
other.

## Validate Compose

Validate the Compose file before starting or recreating containers:

```sh
docker compose -f compose.yml config
```

If you are using one of the repository examples directly:

```sh
docker compose -f compose.example.yml config
docker compose -f compose.dev.yml config
```

`compose.example.yml` runs an image. `compose.dev.yml` builds the image from the
local checkout.

## Start Or Restart Reveille

Start or recreate the Reveille service:

```sh
docker compose -f compose.yml up -d reveille
```

Recreate after changing `reveille.yml`, host files, Compose environment, or the
image tag:

```sh
docker compose -f compose.yml up -d --force-recreate reveille
```

If your deployment uses a prebuilt image, pull it before recreating:

```sh
docker compose -f compose.yml pull reveille
docker compose -f compose.yml up -d reveille
```

If your deployment builds locally:

```sh
docker compose -f compose.yml build reveille
docker compose -f compose.yml up -d reveille
```

## Inspect Logs

Start with Reveille logs:

```sh
docker logs reveille --tail 120
```

Follow logs while testing a browser request:

```sh
docker logs reveille -f
```

Traefik and Dockhand logs usually explain routing and Docker-control failures:

```sh
docker logs traefik --tail 120
docker logs dockhand --tail 120
```

Useful Reveille log messages include:

| Message | Meaning |
| --- | --- |
| `reveille listening on ...` | Reveille started and bound its HTTP listener |
| `reloaded ... host entries` | Host files loaded or reloaded successfully |
| `health <host>: ...` | Forward-auth readiness check failed |
| `start <host>: ...` | Dockhand start failed |
| `status <host>: health check failed` | Wait UI status readiness check failed |
| `lease accepted for <host>` | Timer save succeeded |
| `lease rejected for <host>` | Timer save used an invalid lease option |
| `lease expired for <host>; requesting stop` | Finite lease timer fired |
| `lease stop failed for <host>` | Automatic Dockhand stop failed |

Set `log.level: "debug"` in `reveille.yml` when you need status polling details.

## Check Reveille Health

From the Docker host, if Reveille publishes port `8080`:

```sh
wget -qO- http://localhost:8080/healthz
```

Expected response:

```text
ok
```

From the Traefik container, test the internal container DNS name Traefik should
use:

```sh
docker exec traefik wget -qO- http://reveille:8080/healthz
```

If this fails, Traefik probably cannot reach Reveille on the shared Docker
network.

## Check Dockhand Reachability

Run checks from the Reveille container because Reveille's network view is what
matters:

```sh
docker exec reveille wget -S -O- http://dockhand:3000/api/environments
```

Check the default Dockhand environment:

```sh
docker exec reveille wget -S -O- 'http://dockhand:3000/api/containers?all=true&env=1'
```

Replace `1` with `dockhand.environmentId` or the target's resolved environment
ID.

If Dockhand requires a token, these unauthenticated checks may return an auth
error. In that case, verify that `DOCKHAND_API_TOKEN` is present in the Reveille
container:

```sh
docker exec reveille printenv DOCKHAND_API_TOKEN
```

## Check Traefik ForwardAuth

Run the forward-auth request from the Traefik container:

```sh
docker exec traefik wget -S -O- \
  --header 'X-Forwarded-Host: app.example.com' \
  --header 'X-Forwarded-Proto: https' \
  --header 'X-Forwarded-Uri: /' \
  http://reveille:8080/api/traefik/forward-auth
```

Interpret the result:

| Response | Meaning |
| --- | --- |
| `204 No Content` | Target is ready, or the host is not managed by Reveille |
| `302 Found` | Reveille started the target and redirected to the wait UI |
| `500` | Readiness check failed unexpectedly |
| `503` | Dockhand start failed |

Use a hostname that exists in your Reveille host files. Unknown hosts pass
through with `204 No Content`.

## Check The Browser Wait Route

The wait UI route must be reachable through Traefik at the public app host:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com&format=status'
```

Expected behavior:

- the request reaches Reveille
- the response is JSON
- the JSON includes `healthy`, `leaseActive`, and `readinessState`

If this returns `404 NOT_FOUND`, check the Traefik `PathPrefix('/_reveille')`
route, route priority, and service target for the Reveille UI route.

## Test Timer Save

Save a lease through the same browser control channel the wait UI uses:

```sh
docker exec traefik wget -S -O- --no-check-certificate \
  --header 'Host: app.example.com' \
  --post-data 'action=lease&lease=15m' \
  'https://127.0.0.1/_reveille/wait?host=app.example.com'
```

Successful finite lease responses include `expiresAt`. A `never` lease response
includes `never: true`.

If the response is `400 invalid lease`, the submitted value is not in the host's
configured `lease.options`.

## Test Target Readiness

For a target with `healthUrl`, test the URL from the Reveille container:

```sh
docker exec reveille wget -S -O- http://app:8080/health
```

The response status must be listed in the target's `healthyStatus`. If
`healthyStatus` is omitted, Reveille treats `200` as healthy.

For a container target without `healthUrl`, readiness comes from Dockhand
container state:

```sh
docker exec reveille wget -S -O- 'http://dockhand:3000/api/containers?all=true&env=1'
```

Look for the configured container `id` or name, then inspect its `state`,
`status`, and `health` fields.

## Rebuild Or Upgrade

For image-based deployments:

```sh
docker compose -f compose.yml pull reveille
docker compose -f compose.yml up -d reveille
docker logs reveille --tail 120
```

For local builds:

```sh
docker compose -f compose.yml build reveille
docker compose -f compose.yml up -d reveille
docker logs reveille --tail 120
```

After an upgrade, retest:

1. `/healthz` from Traefik
2. `/_reveille/wait?...&format=status` through Traefik
3. timer save through `POST /_reveille/wait`
4. one managed app domain in a browser

## Related Docs

- Runtime flow: [runtime-flow.md](runtime-flow.md)
- Traefik reference: [traefik/reference.md](traefik/reference.md)
- Dockhand: [dockhand.md](dockhand.md)
- Readiness: [readiness.md](readiness.md)
- Leases: [leases.md](leases.md)
