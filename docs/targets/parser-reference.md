# Host Parser Reference

This page describes how Reveille interprets a host file after the YAML is
loaded. Use [get-started.md](get-started.md) when you just want to create a
working file quickly.

## Canonical Shape

Reveille expects one top-level `target:` map:

```yaml
target:
  convertx:
    type: stack
    hostname: convert.example.com
    healthUrl: http://10.0.0.50:3003/healthcheck
```

Each key under `target:` is one managed entry.

## Parse Rules

- `target:` is required and must be a YAML mapping.
- Each child under `target:` must also be a mapping.
- The child key becomes the default `name` when `name:` is omitted.
- `hostname:` is what Reveille matches against the incoming Traefik host.
- `type:` defaults to `container` when omitted.
- `healthyStatus:` defaults to `[200]` when omitted.
- `routing.returnToHeader` defaults to `X-Forwarded-Uri` when omitted.

## Entry Fields

Common fields:

- `<key>.type`: `container` or `stack`
- `<key>.environment`: Dockhand environment ID or environment name
- `<key>.hostname`: public hostname that should trigger this target
- `<key>.healthUrl`: optional for containers, required for stacks
- `<key>.healthyStatus`: optional list of HTTP status codes treated as healthy

Container-only field:

- `<key>.id`: container ID or container name in Dockhand

Stack-only field:

- `<key>.name`: optional stack name override; defaults to the YAML key

## Validation Rules

- Container entries must provide `id`.
- Stack entries must provide `healthUrl`.
- Stack entries may omit `name`; the YAML key is used instead.
- Every entry must provide `hostname`.

## Example

```yaml
target:
  jellyfin:
    id: jellyfin
    hostname: jellyfin.example.com

  convertx:
    type: stack
    hostname: convert.example.com
    healthUrl: http://10.0.0.50:3003/healthcheck

  paperless:
    type: stack
    name: paperless-prod
    hostname: paperless.example.com
    healthUrl: http://paperless-webserver:8000/
    healthyStatus:
      - 200
      - 302
```
