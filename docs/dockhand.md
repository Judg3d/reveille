# Dockhand

Reveille uses Dockhand as its Docker control plane for target start, stop, and
container readiness.

Reveille does not talk to Docker directly. It calls Dockhand over HTTP.

The important readiness distinction:

**Dockhand is always used for start and stop. Dockhand readiness is used only
when a target does not define `healthUrl`.**

If a target defines `healthUrl`, Reveille uses that HTTP endpoint for the
ready/not-ready decision, but still uses Dockhand to start and stop the target.

## Configuration

Dockhand connection defaults live in `reveille.yml`:

```yaml
dockhand:
  baseUrl: "http://dockhand:3000"
  environmentId: 1
  timeout: "30s"
```

- `dockhand.baseUrl`: internal URL Reveille uses to call Dockhand
- `dockhand.environmentId`: default Dockhand environment ID
- `dockhand.timeout`: HTTP client timeout for Dockhand API calls
- `DOCKHAND_API_TOKEN`: optional bearer token from the environment

`baseUrl` should normally use the container DNS name that Reveille can reach,
such as `http://dockhand:3000`.

## Authentication

When `DOCKHAND_API_TOKEN` is set, Reveille sends:

```http
Authorization: Bearer <token>
```

Reveille also sends:

```http
Accept: application/json
```

If no token is configured, Reveille does not send an `Authorization` header.

Prefer environment variables or deployment secrets for the token. Avoid putting
tokens directly in committed config. The example Compose file passes the token
through:

```yaml
environment:
  DOCKHAND_API_TOKEN: ${DOCKHAND_API_TOKEN:-}
```

## Environments

Dockhand API calls include an environment query parameter:

```text
?env=<id>
```

By default, Reveille uses `dockhand.environmentId`.

A target can override the default environment:

```yaml
target:
  jellyfin:
    id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com
```

The target `environment` value can be:

- a numeric environment ID, such as `"7"`
- an environment name, such as `homelab`

For numeric values, Reveille uses the ID directly. For names, Reveille calls
Dockhand `/api/environments`, finds the matching name case-insensitively, and
caches the resolved ID.

## Container Targets

Container targets are the default target type.

```yaml
target:
  jellyfin:
    id: jellyfin
    hostname: jellyfin.example.com
```

Container targets require `id`. The configured `id` can be a Docker container
ID, an ID prefix, a container name, or a name listed by Dockhand.

Before starting, stopping, or checking a container, Reveille lists containers
from Dockhand and resolves the configured value to the container ID Dockhand
returned.

If the container target does not define `healthUrl`, readiness comes from
Dockhand container state:

- `state: running` is running
- `status` beginning with `Up` is running
- no Docker health status, or `health: none`, counts as healthy when running
- `health: healthy` counts as healthy
- any other health value is not healthy

If the container target defines `healthUrl`, that URL replaces Dockhand
readiness for the ready/not-ready decision. Dockhand still handles start and
stop.

## Stack Targets

Stack targets use Dockhand stack start and stop calls.

```yaml
target:
  paperless:
    type: stack
    name: paperless
    hostname: paperless.example.com
    healthUrl: http://paperless-webserver:8000/
```

Stack targets require:

- `type: stack`
- `name`
- `healthUrl`

Stacks must define `healthUrl` because a stack can contain multiple containers.
Reveille does not use Dockhand stack readiness. It uses `healthUrl` for
readiness and Dockhand for stack start/stop.

## Dockhand Calls

Reveille currently uses these Dockhand routes:

| Reveille action | Method | Dockhand path | Notes |
| --- | --- | --- | --- |
| List containers | `GET` | `/api/containers` | Uses `all=true` and `env=<id>` |
| Start container | `POST` | `/api/containers/<id>/start` | Uses resolved container ID and `env=<id>` |
| Stop container | `POST` | `/api/containers/<id>/stop` | Uses resolved container ID and `env=<id>` |
| Start stack | `POST` | `/api/stacks/<name>/start` | Uses target stack name and `env=<id>` |
| Stop stack | `POST` | `/api/stacks/<name>/stop` | Uses target stack name and `env=<id>` |
| List environments | `GET` | `/api/environments` | Used when target environment is a name |

Dockhand must return a 2xx status for these calls. Non-2xx responses become
Reveille errors that include the method, path, and Dockhand status.

## Timeouts And Errors

Dockhand calls use `dockhand.timeout` as the HTTP client timeout.

Some operations also have higher-level Reveille timeouts:

- `defaults.startTimeout`: wraps the start call during `forwardAuth`
- `defaults.stopGrace`: wraps manual stop calls
- lease expiry stop: uses the lease manager's internal stop timeout

Common failure surfaces:

- `forwardAuth` returns an error if readiness or start fails
- wait UI status polling returns an error if readiness fails unexpectedly
- manual stop returns an error if Dockhand stop fails
- lease expiry logs stop failures and removes the active lease from memory

## Troubleshooting

| Symptom | Likely Cause | Check |
| --- | --- | --- |
| Start fails | Bad `baseUrl`, token, environment, or target ID/name | Reveille logs and Dockhand reachability |
| Container not found | Wrong `target.<name>.id` or environment | Host file and Dockhand container list |
| Stack start fails | Wrong `target.<name>.name` or missing stack | Host file and Dockhand stack list |
| Environment not found | Target environment name does not match Dockhand | `/api/environments` and target config |
| Dockhand says container is running but UI waits | `healthUrl` overrides Dockhand readiness | Target config and readiness status JSON |
| Token works locally but not in Compose | Environment variable is not passed into the container | `.env`, Compose config, and Reveille container env |

## Related Docs

- Readiness: [readiness.md](readiness.md)
- Runtime config: [reveille-yml.md](reveille-yml.md)
- Target parser reference: [targets/parser-reference.md](targets/parser-reference.md)
- Runtime flow: [runtime-flow.md](runtime-flow.md)
