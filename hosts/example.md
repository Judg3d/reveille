# Dynamic Host Config

Reveille loads managed hosts from this directory. Add one or more `.yml` files
under `hosts/`.

The file maps a Traefik hostname to a Dockhand target. Traefik decides whether a
request calls Reveille through `forwardAuth`; Reveille decides which Dockhand
container or stack to start and when it is ready.

Host files are reloaded automatically, so adding, editing, or removing a file
does not require restarting Reveille.

## Container vs Stack

Most targets should be containers.

Use a container target when one Traefik hostname maps to one Dockhand container.
Reveille can ask Dockhand whether that container is running and healthy, so no
extra health URL is required.

Use a stack target when the thing to start and stop is a Dockhand compose stack.
Stacks can contain multiple containers, so Reveille cannot infer which one means
"the app is ready." Stack targets need `healthUrl`.

`type` defaults to `container`, so you only need to set it for stacks.

## Multiple Targets In One File

Use `targets:` when several apps share the same lease/routing defaults.

```yaml
# hosts/apps.yml
targets:
  - id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com

  - id: audiobookshelf
    environment: homelab
    hostname: audio.example.com

  - type: stack
    name: paperless
    environment: homelab
    hostname: paperless.example.com
    healthUrl: http://paperless-webserver:8000/
```

## Container Target

Use this for a single Dockhand container. Reveille checks readiness through
Dockhand by default.

```yaml
# hosts/jellyfin.yml
target:
  id: jellyfin
  environment: homelab
  hostname: jellyfin.example.com
```

Fields:

- `target.type`: optional; defaults to `container`
- `target.id`: container ID or container name in Dockhand
- `target.environment`: Dockhand environment ID or environment name
- `target.hostname`: hostname from the Traefik router

Readiness behavior:

- If the container is stopped, Reveille starts it through Dockhand.
- Reveille waits until Dockhand reports the container as running.
- If Docker health status exists, Reveille waits for `healthy`.

## Container With HTTP Readiness Override

Use `healthUrl` only when Docker/Dockhand says the container is running before
the actual web app is ready.

```yaml
# hosts/jellyfin.yml
target:
  id: jellyfin
  environment: homelab
  hostname: jellyfin.example.com
  healthUrl: http://jellyfin:8096/health
  healthyStatus:
    - 200
    - 302
```

When `healthUrl` is set, Reveille uses that URL instead of Dockhand readiness.

## Stack Target

Use this for a Dockhand compose stack. Stack targets currently need an HTTP
readiness URL because a stack can contain multiple containers.

```yaml
# hosts/paperless.yml
target:
  type: stack
  name: paperless
  environment: homelab
  hostname: paperless.example.com
  healthUrl: http://paperless-webserver:8000/
  healthyStatus:
    - 200
    - 302
```

Fields:

- `target.type`: `stack`
- `target.name`: Dockhand stack name
- `target.environment`: Dockhand environment ID or environment name
- `target.hostname`: hostname from the Traefik router
- `target.healthUrl`: internal URL Reveille can reach from its container

## Lease Overrides

If omitted, the defaults from `config.yml` are used. Add this section only when
a host needs different lease choices.

```yaml
lease:
  default: 2h
  options:
    - 15m
    - 1h
    - 2h
    - 4h3m
    - never
```

Durations use Go duration syntax, such as `2m`, `30m`, `5h`, or `4h3m`.
`never` means Reveille will not automatically stop the target.

## Routing Override

Most setups should not need this. By default Reveille redirects users back to
the original Traefik path from `X-Forwarded-Uri`.

```yaml
routing:
  returnToHeader: X-Forwarded-Uri
```
