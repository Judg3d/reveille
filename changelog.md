# Changelog

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
- Added `docs/handoff.md` as the implementation starting point for the initial
  Go build.
