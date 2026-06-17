# Traefik Wiring

Traefik documentation now follows the same quick-start/reference shape as the
target configuration docs:

- [traefik/get-started.md](traefik/get-started.md): shortest path to a working
  Traefik setup
- [traefik/reference.md](traefik/reference.md): detailed middleware, router,
  wait-page control channel, forwarded-header, and troubleshooting behavior

Use the quick start first. Use the reference page when you need to reason about
which Traefik route is handling `/_reveille/*` or why a timer request did not
reach Reveille.
