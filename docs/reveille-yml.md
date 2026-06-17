# Reveille.yml Reference

`reveille.yml` is Reveille's runtime configuration file. It controls the HTTP
server, Dockhand connection defaults, and lease behavior.

Use [targets/get-started.md](targets/get-started.md) for per-host target
files. Use this page for the main service-level config.

## Files

Use `reveille.example.yml` as the committed, documented template. Copy it to
`reveille.yml` for local development or deployment:

```sh
cp reveille.example.yml reveille.yml
```

`reveille.yml` is local runtime config and is intentionally ignored by Git.

## Canonical Runtime Filename

Use `reveille.yml` as the primary runtime config filename.

Example:

```yaml
server:
  listen: ":8080"
  publicPath: "/_reveille"
  failClosedUnknownHosts: false

log:
  level: "info"

dockhand:
  baseUrl: "http://dockhand:3000"
  environmentId: 1
  timeout: "30s"

defaults:
  lease: "2h"
  leaseOptions:
    - "30m"
    - "1h"
    - "2h"
    - "4h"
    - "never"
  startTimeout: "3m"
  stopGrace: "30s"
  pollInterval: "5s"
```

## Server

```yaml
server:
  listen: ":8080"
  publicPath: "/_reveille"
  failClosedUnknownHosts: false
```

- `server.listen`: address Reveille listens on
- `server.publicPath`: public path prefix used for the wait UI and Reveille API
- `server.failClosedUnknownHosts`: return `404 Not Found` for unknown
  `forwardAuth` hosts instead of allowing pass-through with `204 No Content`

## Log

```yaml
log:
  level: "info"
```

- `log.level`: global log threshold for Reveille runtime messages
- Supported values: `debug`, `info`, `warn`, `warning`, `error`
- Default: `info`
- `warning` is accepted and normalized to `warn`

Recommended use:

- `info`: normal day-to-day operations
- `debug`: active troubleshooting when you want extra runtime detail
- `warn`: quieter production logs that still keep suspicious conditions
- `error`: failures only

## Dockhand

```yaml
dockhand:
  baseUrl: "http://dockhand:3000"
  environmentId: 1
  timeout: "30s"
```

- `dockhand.baseUrl`: Dockhand API base URL
- `dockhand.environmentId`: default Dockhand environment used when a target
  entry does not override `environment`
- `dockhand.timeout`: HTTP timeout for Dockhand API calls
- `DOCKHAND_API_TOKEN`: optional bearer token provided through the environment

Use `target.<name>.environment` in the host file when a specific target needs a
different Dockhand environment than the default.

## Defaults

```yaml
defaults:
  lease: "2h"
  leaseOptions:
    - "30m"
    - "1h"
    - "2h"
    - "4h"
    - "never"
  startTimeout: "3m"
  stopGrace: "30s"
  pollInterval: "5s"
```

- `defaults.lease`: default lease applied on the wait page
- `defaults.leaseOptions`: selectable lease durations shown to the user
- `defaults.startTimeout`: maximum time allowed for a start operation
- `defaults.stopGrace`: timeout used when stopping a target
- `defaults.pollInterval`: how often the wait page checks target readiness

Durations use Go duration syntax such as `30m`, `1h`, `2h`, or `4h3m`.

## How It Relates To Host Files

- `reveille.yml` defines service-wide defaults
- `target.<name>.environment` overrides the default Dockhand environment for a
  specific host entry
- `lease:` and `routing:` blocks inside a host file override the global
  defaults for that host entry only
