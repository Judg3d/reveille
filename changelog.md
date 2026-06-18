# Changelog

## 2026-06-18

- Added HMAC-signed wait tokens and origin validation for wait-page control
  requests, so lease, stop, and status actions must come from a valid Reveille
  wait-page session.
- Added `failClosedUnknownHosts` support so unknown Traefik forward-auth hosts
  can fail closed with `404` instead of being allowed through by default.
- Hardened wait-flow redirects, health checks, and request handling by using
  relative wait URLs, validating `healthUrl` values at config load, and limiting
  JSON lease request bodies to 1 MiB.
- Removed unused lease state and added coverage for token handling, origin
  rejection, fail-closed host behavior, JSON body limits, health URL validation,
  relative redirects, and lease close cancellation.
- Removed the global Dockhand `environmentId` fallback; each target now requires
  its own `environment` so container starts are explicit per target.
- Updated Compose examples to use optional `.env` loading, expose Reveille only
  on the Docker network for Traefik, and remove direct host-port publishing
  guidance from the public examples.
- Swapped public Compose examples to a GHCR image placeholder and scrubbed
  private registry/domain examples from the repository history before public
  release.
- Updated public docs and Compose examples to reference
  `https://github.com/Judg3d/reveille` and `ghcr.io/judg3d/reveille`.
- Added GitHub Actions CI for Go tests and GHCR container image builds.
- Removed wait-page static asset cache busting now that the backend/template
  contract has stabilized, and removed the related documentation.

## 2026-06-17

- Split Traefik documentation into `docs/traefik/get-started.md` and
  `docs/traefik/reference.md`, leaving `docs/traefik-wiring.md` as a short
  compatibility landing page.
- Added a documented `reveille.example.yml` template and made `reveille.yml`
  local untracked runtime config.
- Added `docs/runtime-flow.md` to document the end-to-end browser, Traefik,
  Reveille, Dockhand, wait-page, redirect, and lease-expiry flow.
- Simplified `README.md` into a general project overview with a minimal Compose
  example and moved detailed setup links into `docs/README.md`.
- Added a runtime-flow note that Compose-managed targets should be created with
  `docker compose up -d --no-start` before Reveille starts them on demand.

## 2026-06-16

- Rebuilt the wait UI with a clearer timer-selection screen, responsive run
  window controls, readiness/counter state, improved status messages, and
  dark-mode styling.
- Changed the wait flow so the page stays on timer selection until the current
  browser session starts a timer, then switches to countdown mode while polling
  readiness and redirecting only after the target is healthy.
- Moved browser control calls onto `/_reveille/wait` so the wait page can render,
  create leases, stop apps, and poll status through the same Traefik-routed
  prefix instead of depending on separate `/_reveille/api/*` browser routing.
- Added static asset cache busting for the wait page and fixed embedded client
  config so it renders as a JSON object rather than an escaped JSON string.
- Added Traefik Docker labels for the Reveille UI route in `compose.yml` and
  made label-based routing the source of truth for `/_reveille/*`.
- Split the server package into smaller files for construction, routes,
  handlers, status responses, wait-page rendering, and HTTP helpers, leaving
  `server.go` as the glue layer.
- Added an `internal/readiness` package for readiness state/message policy, with
  focused tests outside the HTTP handler layer.
- Added a configurable `log.level` setting in `reveille.yml` with leveled
  runtime logging (`debug`, `info`, `warn`, `error`) across startup, host
  reloads, lease lifecycle, and wait-page/server flows.
- Documented `log.level` in `reveille.yml` and the config reference, with
  `info` as the default runtime logging level and `warning` accepted as `warn`.
- Reworked the wait-page status flow so the browser reconciles against
  `/_reveille/api/status` on load and after every lease POST attempt, instead of
  trusting the POST result alone.
- Expanded status responses with explicit `leaseActive` and `statusMessage`
  fields, added lease/status logging, and surfaced clearer UI messaging when
  redirect is still blocked on health checks.
- Added readiness diagnostics to `/_reveille/api/status`, including
  `readinessState`, `healthError`, `healthStatus`, and `lastCheck`, and updated
  the wait page to show whether the health endpoint is unreachable or simply
  returning a non-healthy response.
- Added config and status tests covering log-level parsing plus lease-backed
  `/api/status` responses for finite, `Never`, and healthy states.
- Fixed Traefik forward-auth redirects to use the public forwarded host/proto
  instead of leaking the internal `http://reveille:8080` service URL back to
  browsers.
- Added tests covering public wait-page redirect generation and relative-path
  fallback behavior.
- Narrowed host parsing to a single keyed `target:` map format, with each map
  key becoming the default target name when `name` is omitted.
- Removed older top-level `host` and `targets:` list layouts from the parser so
  configuration matches the documented canonical shape.
- Added host-loader tests for the keyed `target:` map shape, including explicit
  name override behavior and rejection of legacy layouts.

## 2026-05-18

- Defined Reveille as a Go service that integrates with Traefik via
  `forwardAuth` and Dockhand via REST API.
- Split detailed runtime behavior into `docs/behavior.md` so `README.md` stays
  focused on project goals and direction.
- Documented the stopped-container flow: start through Dockhand, show wait page,
  poll status every 5 seconds, redirect when healthy, and stop again when a
  finite lease expires.
- Added lease behavior, including configured durations and `Never`.
- Added Traefik wiring notes for static providers, reusable file-provider
  middleware, protected app routers, and the unprotected `/_reveille/*` route.
- Added Dockhand API assumptions for container and stack lifecycle calls.
- Added `.workflow/handoff.md` as temporary implementation context for the
  initial Go build.
