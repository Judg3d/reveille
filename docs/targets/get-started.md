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

## Recommended Layout

Prefer a keyed `target:` map. It keeps one file compact, makes target names
obvious, and avoids repeating the same wrapper structure for each host.

```yaml
# hosts/apps.yml
target:
  jellyfin:
    id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com

  audiobookshelf:
    id: audiobookshelf
    environment: homelab
    hostname: audio.example.com

  paperless:
    type: stack
    environment: homelab
    hostname: paperless.example.com
    healthUrl: http://paperless-webserver:8000/
```

The map key becomes `target.name` when `name` is omitted.

If you want the exact parser rules and validation behavior, see
[parser-reference.md](parser-reference.md).

## Container Target

Use this for a Dockhand container inside the keyed `target:` map. Reveille
checks readiness through Dockhand by default.

```yaml
# hosts/apps.yml
target:
  jellyfin:
    id: jellyfin
    environment: homelab
    hostname: jellyfin.example.com
```

How to format a container entry:

- use one key under `target:`
- set `id` to the Dockhand container name or ID
- set `hostname` to the public hostname that should wake it
- add `environment` only when you need to override the default Dockhand
  environment

Readiness behavior:

- If the container is stopped, Reveille starts it through Dockhand.
- Reveille waits until Dockhand reports the container as running.
- If Docker health status exists, Reveille waits for `healthy`.

## Container With HTTP Readiness Override

Use `healthUrl` only when Docker/Dockhand says the container is running before
the actual web app is ready.

```yaml
# hosts/apps.yml
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

When `healthUrl` is set, Reveille uses that URL instead of Dockhand readiness.

## Stack Target

Use this for a Dockhand compose stack inside the keyed `target:` map. Stack
targets currently need an HTTP readiness URL because a stack can contain
multiple containers.

```yaml
# hosts/apps.yml
target:
  paperless:
    type: stack
    environment: homelab
    hostname: paperless.example.com
    healthUrl: http://paperless-webserver:8000/
    healthyStatus:
      - 200
      - 302
```

How to format a stack entry:

- use one key under `target:`
- set `type: stack`
- set `hostname` to the public hostname that should wake it
- set `healthUrl` to an internal URL Reveille can use to decide when the app is
  ready
- set `name` only when the Dockhand stack name is different from the YAML key
- add `environment` only when you need to override the default Dockhand
  environment

## Multiple Entries

Add as many named entries under `target:` as needed.

Container and stack entries can be mixed in the same file. Reveille evaluates
each named entry independently.

```yaml
# hosts/apps.yml
target:
  media:
    id: jellyfin
    hostname: media.example.com

  app1:
    type: stack
    hostname: app1.example.com
    healthUrl: http://app1-web:8080/health

  app2:
    type: stack
    hostname: app2.example.com
    healthUrl: http://app2-web:8080/health
```

If you need the Dockhand target name to differ from the YAML key, set `name`
explicitly:

```yaml
target:
  app1:
    type: stack
    name: app1-prod
    hostname: app1.example.com
    healthUrl: http://app1-web:8080/health
```

## Lease Overrides

If omitted, the defaults from `reveille.yml` are used. Add this section only when
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
