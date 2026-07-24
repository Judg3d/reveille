# Reveille Improvement Plan

Working plan for hardening Reveille and growing it into a complete on-demand
lifecycle manager. Based on a full review of the codebase on 2026-07-24.

Phases are ordered by risk: P0 fixes correctness/safety gaps, P1 hardens
security and supply chain, P2 adds the features that round out the product.
Items within a phase are roughly independent and can be shipped separately.

---

## P0 — Correctness and safety

### 1. Orphaned wakes: apply a lease when a target is started

**Problem.** `forwardAuth` (`internal/server/handlers.go`) starts the target on
any request to a managed host, then redirects to the wait page. A lease is only
created when a person picks a timer. A crawler, uptime bot, or a visitor who
closes the tab wakes the container and it runs forever.

**Plan.**
- When `forwardAuth` triggers a start, immediately arm a provisional lease
  using the host's default lease (or a dedicated `defaults.orphanGrace`,
  e.g. `10m`).
- When the user submits a timer on the wait page, the real lease replaces the
  provisional one (existing `Manager.Set` semantics already handle
  replacement).
- Add a per-host `startMode: auto | manual` option. In `manual` mode,
  `forwardAuth` does not start the target; the wait page shows a "Start app"
  button and the start happens on that authenticated POST. This keeps scanners
  from waking services at all.

**Acceptance.** A request from a client that never loads the wait page results
in the container being stopped after the grace/default lease expires. Manual
mode never starts a target from `forwardAuth` alone.

### 2. Lease persistence across restarts

**Problem.** `leases.Manager` (`internal/leases/leases.go`) keeps leases and
timers in memory. A Reveille restart or crash drops every timer, leaving
running containers unmanaged until someone visits again.

**Plan.**
- Persist active leases to a state file (JSON at this scale; path via
  `server.stateFile`, default `/var/lib/reveille/state.json`, volume-mounted).
- Write on every lease change; on boot, load the file and re-arm timers.
  Leases already expired at boot trigger an immediate stop.
- Flush state in `Manager.Close()` during graceful shutdown.

**Acceptance.** `docker restart reveille` mid-lease: countdown continues and
the target still stops at the original expiry.

### 3. Cache health checks and container resolution

**Problem.** Every proxied request runs `healthy()`. For container targets that
is `Dockhand.Healthy` → `findContainer` → a full container list
(`internal/dockhand/client.go`). One page load with 30 assets = 30 list calls.
Concurrent first-hits also all fire `Start`.

**Plan.**
- Add a per-host health cache with a short TTL (2–5 s, configurable as
  `defaults.healthCacheTTL`).
- Use `golang.org/x/sync/singleflight` so concurrent checks for the same host
  share one upstream call; same for `Start`.
- Cache resolved container name → ID in the Dockhand client; invalidate on
  lookup miss (container recreated).

**Acceptance.** A burst of N parallel requests to one host produces at most
one Dockhand health call per TTL window and at most one start call.

### 4. HTTP server timeouts

**Problem.** `cmd/reveille/main.go` builds `http.Server` with no timeouts on a
publicly reachable endpoint (slowloris, stuck connections).

**Plan.** Set `ReadHeaderTimeout` (5s), `ReadTimeout` (30s), `WriteTimeout`
(30s), `IdleTimeout` (120s); make them configurable only if a need appears.

### 5. Wire up the wait-token key

**Problem.** `server.Dependencies.TokenKey` exists but `main.go` never sets
it, so the HMAC key regenerates on every restart and invalidates in-flight
wait sessions.

**Plan.** Add `server.tokenSecret` to `reveille.yml` with env expansion (same
pattern as `dockhand.apiToken`), fall back to the current random key when
unset. Document the restart trade-off in `docs/security.md`.

---

## P1 — Security and supply chain

### 6. Move the wait session out of the query string

**Problem.** The wait token is accepted via query string
(`internal/server/tokens.go`), so it lands in Traefik access logs and browser
history; the JS `replaceState` cleanup is best-effort. Origin validation
(`internal/server/http_helpers.go`) only applies when the browser sends
`Origin`/`Referer`.

**Plan.**
- On the wait redirect, set the token in an `HttpOnly; Secure; SameSite=Lax`
  cookie scoped to the managed host; keep query-param acceptance as a
  fallback for one release, then drop it.
- With SameSite cookies in place, keep the origin check as defense in depth.

### 7. Security headers on the wait page

**Plan.** Add middleware for wait/static responses:
- `Content-Security-Policy: default-src 'none'; script-src 'self'; style-src
  'self'; img-src 'self'; connect-src 'self'` (assets are all self-hosted;
  keep the `#reveille-config` JSON script pattern CSP-compatible).
- `Referrer-Policy: no-referrer` (matters while tokens can appear in URLs).
- `X-Content-Type-Options: nosniff`, `frame-ancestors 'none'` via CSP.

### 8. Rate limiting and start deduplication

**Plan.** Per-host token bucket on `/_reveille/*` mutating routes and on
forward-auth-triggered starts. Log and return `429`. Singleflight from item 3
already dedupes concurrent starts.

### 9. Trust boundary for internal routes

**Problem.** Anything on the shared Docker network can call
`/api/traefik/forward-auth` and trigger starts.

**Plan.** Optional shared secret: Traefik's `forwardAuth` middleware adds a
header via `customRequestHeaders`; Reveille rejects forward-auth calls without
it when `server.forwardAuthSecret` is set. Alternative considered (second
listener bound to an internal-only network) is more moving parts for the same
result.

### 10. Don't show raw health errors to anonymous visitors

**Problem.** `healthError` (internal URLs, container DNS names) is rendered on
the public wait page via `wait.js` detail messages.

**Plan.** Default to generic messages ("health endpoint unreachable"); gate
full detail behind `server.exposeHealthDetail: true` for debugging.

### 11. CI and image hardening

- Scope `packages: write` to the image job only (top-level in
  `.github/workflows/ci.yml` today).
- Add `govulncheck`, `golangci-lint`, and a trivy image scan to CI.
- Pin base images by digest in the `Dockerfile`; add
  `HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/healthz`.
- Add a `LICENSE` file — the repo is public and currently has none.

---

## P2 — Product completeness

### 12. Idle-based auto-stop / sliding leases

`forwardAuth` already sees every request. Record last-activity per host and
add lease options like `idle:20m`: the lease extends while traffic flows and
the target stops after the idle window. This is the headline feature of
comparable tools (Sablier, ContainerNursery) and Reveille is one step away.

### 13. Admin dashboard

A separate authenticated route (or second listener): all managed hosts,
running/stopped state, lease countdowns, start/stop/extend buttons. Today the
only control surface is per-host wait pages.

### 14. Provider abstraction

`server.Dependencies` takes a concrete `*dockhand.Client`. Define a
`Provider` interface (`Start`, `Stop`, `Healthy`) and make Dockhand one
implementation; add a direct Docker-socket provider next. Removes the hard
Dockhand coupling and widens the audience.

### 15. Notifications

Notify on wake, auto-stop, and **failed stops** — a failed stop currently
only reaches the log (`internal/leases/leases.go`), meaning a container
silently stays up.

Design: an `internal/notify` package with a small `Notifier` interface
(`Notify(ctx, Event)`) and one implementation per channel, fanned out to all
configured channels. Events: `wake`, `lease_expired_stop`, `manual_stop`,
`stop_failed`, with host and lease context. Delivery is best-effort with a
short timeout; failures log but never block the stop/start path.

Channels (all plain HTTP, no SDKs):

- **Gotify**: `POST {serverUrl}/message` with `X-Gotify-Key: {appToken}`;
  JSON body `title`/`message`/`priority`. Config: `notify.gotify.url`,
  `notify.gotify.token` (env-expandable), optional `priority`.
- **Telegram**: `POST https://api.telegram.org/bot{token}/sendMessage` with
  `chat_id` + `text`. Config: `notify.telegram.token` (env-expandable),
  `notify.telegram.chatId`. Bot token via @BotFather.
- **ntfy**: `POST {serverUrl}/{topic}` with the message as body.
- **Generic webhook**: `POST {url}` with the event as JSON, for anything else.

Per-event filtering (`notify.events: [stop_failed, ...]`) so noisy events
like `wake` can be opted out while keeping failure alerts.

### 16. Quality of life

- `-version` flag and build info baked into the image (`-ldflags -X`).
- `reveille validate` subcommand for config + target files (usable in CI).
- fsnotify-based host reload instead of the 5 s poll hardcoded in `main.go`.
- Migrate `internal/logging` to `log/slog` with a JSON output option.
- Lease extension button on the wait page countdown view.

---

## Small cleanups (fold into nearby work)

- `waitOrigin` (`internal/server/http_helpers.go`) has a dead if/else — both
  branches set `https` — and disagrees with `expectedOrigin`, which defaults
  to `http`. Unify.
- Lease selection matches the display label case-insensitively
  (`internal/server/handlers.go`); match canonical duration values instead,
  and drop the redundant `"Never"` special case.
- Health checks use the global `http.DefaultClient` (`main.go`); use a
  dedicated client with redirects disabled and transport limits.
- The Dockhand env name→ID cache never invalidates; refresh on a negative
  lookup after a cache hit.
- Host reload does not reconcile leases: a host removed from `targets/`
  keeps its timer keyed to the old `hosts.Host` snapshot. Cancel or re-key
  leases on reload.

---

## Status

| # | Item | Phase | Status |
| --- | --- | --- | --- |
| 1 | Orphaned wakes / provisional lease + `startMode` | P0 | done |
| 2 | Lease persistence | P0 | done |
| 3 | Health/start caching + singleflight | P0 | done |
| 4 | HTTP server timeouts | P0 | done |
| 5 | `server.tokenSecret` wiring | P0 | done |
| 6 | Cookie-based wait session | P1 | done |
| 7 | Security headers | P1 | done |
| 8 | Rate limiting | P1 | done |
| 9 | Forward-auth shared secret | P1 | done |
| 10 | Generic health errors by default | P1 | done |
| 11 | CI/image hardening + LICENSE | P1 | done |
| 12 | Idle-based auto-stop | P2 | done |
| 13 | Admin dashboard | P2 | done |
| 14 | Provider abstraction | P2 | done |
| 15 | Notifications (Gotify, Telegram, ntfy, webhook) | P2 | done |
| 16 | Quality of life | P2 | done |
