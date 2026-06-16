# Changelog

## 2026-06-16

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
