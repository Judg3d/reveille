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
- `server.tokenSecret`: key used to sign wait-page session tokens; supports
  environment expansion (`${REVEILLE_TOKEN_SECRET}`). When unset, a random key
  is generated at startup and in-flight wait sessions break on restart.
- `server.forwardAuthSecret`: optional shared secret for the forward-auth
  endpoint. When set, Traefik must send it as the `X-Reveille-Auth` request
  header (via `customRequestHeaders` on the middleware); calls without it are
  rejected with `403`.
- `server.exposeHealthDetail`: show raw health-check errors on the public wait
  page. Default `false` shows generic messages because raw errors can leak
  internal URLs and container names.
- `server.stateFile`: lease state file so timers survive restarts. Defaults to
  `/var/lib/reveille/state.json`; set to `""` to disable persistence. When the
  path is not writable, persistence is disabled with a warning.
- `server.hostsReloadInterval`: how often target files are re-read. Default
  `5s`.

Environment fallbacks: `REVEILLE_TOKEN_SECRET`, `REVEILLE_FORWARD_AUTH_SECRET`.

## Log

```yaml
log:
  level: "info"
  format: "text"
```

- `log.level`: global log threshold for Reveille runtime messages
- Supported values: `debug`, `info`, `warn`, `warning`, `error`
- Default: `info`
- `warning` is accepted and normalized to `warn`
- `log.format`: `text` (default) or `json`

Recommended use:

- `info`: normal day-to-day operations
- `debug`: active troubleshooting when you want extra runtime detail
- `warn`: quieter production logs that still keep suspicious conditions
- `error`: failures only

## Dockhand

```yaml
dockhand:
  baseUrl: "http://dockhand:3000"
  timeout: "30s"
```

- `dockhand.baseUrl`: Dockhand API base URL
- `dockhand.apiToken`: optional Dockhand bearer token; supports environment
  expansion such as `${DOCKHAND_API_TOKEN}`
- `dockhand.timeout`: HTTP timeout for Dockhand API calls
- `DOCKHAND_API_TOKEN`: optional bearer token provided through the environment

Set `target.<name>.environment` in each host file. Reveille does not use a
global Dockhand environment fallback.

## Provider

```yaml
provider: "dockhand"
docker:
  socket: "/var/run/docker.sock"
  timeout: "30s"
```

- `provider`: `dockhand` (default) or `docker`
- The `docker` provider talks to the Docker Engine API directly over the unix
  socket and supports **container targets only**; stack targets still require
  Dockhand. Mount the socket into the container when using it.

## Admin

```yaml
admin:
  listen: ":8081"
  token: "${REVEILLE_ADMIN_TOKEN}"
```

- `admin.listen`: enables the operator dashboard on a separate listener when
  set. Keep it off the public network.
- `admin.token`: optional bearer token required for the dashboard and its API
  (`Authorization: Bearer`, `X-Reveille-Admin-Token`, or `?token=`).
  Environment fallback: `REVEILLE_ADMIN_TOKEN`.

See [operations.md](operations.md) for dashboard usage.

## Notify

```yaml
notify:
  events: ["stop_failed", "lease_expired_stop"]
  gotify:
    url: "https://gotify.example.com"
    token: "${GOTIFY_TOKEN}"
    priority: 5
  telegram:
    token: "${TELEGRAM_BOT_TOKEN}"
    chatId: "123456789"
  ntfy:
    url: "https://ntfy.sh"
    topic: "reveille"
  webhook:
    url: "https://example.com/hooks/reveille"
```

- `notify.events`: optional filter; omit to receive every event. Valid values:
  `wake`, `lease_expired_stop`, `manual_stop`, `stop_failed`.
- Configure any subset of channels; delivery is best-effort with a 10s
  timeout and never blocks a start or stop.
- Telegram needs a bot token from @BotFather and the chat ID to send to.

## Defaults

```yaml
defaults:
  lease: "2h"
  leaseOptions:
    - "30m"
    - "1h"
    - "2h"
    - "4h"
    - "idle:20m"
    - "never"
  startTimeout: "3m"
  stopGrace: "30s"
  pollInterval: "5s"
  healthCacheTTL: "3s"
  orphanGrace: "10m"
```

- `defaults.lease`: default lease applied on the wait page
- `defaults.leaseOptions`: selectable lease durations shown to the user.
  Each value is a Go duration, `never`, or `idle:<duration>` for an idle
  timer that keeps sliding forward while the app still receives requests and
  stops the target once the idle window passes with no traffic.
- `defaults.startTimeout`: maximum time allowed for a start operation
- `defaults.stopGrace`: timeout used when stopping a target
- `defaults.pollInterval`: how often the wait page checks target readiness
- `defaults.healthCacheTTL`: short-lived cache for health answers so request
  bursts share one provider round-trip; `0s` disables caching
- `defaults.orphanGrace`: length of the provisional safety-net timer armed
  when a target starts without anyone picking a lease (see
  [leases.md](leases.md))

Durations use Go duration syntax such as `30m`, `1h`, `2h`, or `4h3m`.

## How It Relates To Host Files

- `reveille.yml` defines service-wide defaults
- `target.<name>.environment` defines the Dockhand environment for a specific
  host entry
- `lease:` and `routing:` blocks inside a host file override the global
  defaults for that host entry only
