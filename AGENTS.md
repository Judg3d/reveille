# AGENTS.md

This file is the operating guide for agents working in this repository.

## Project Layout

- `cmd/reveille/`: application entrypoint.
- `internal/config/`: runtime config parsing and validation.
- `internal/hosts/`: target file loading, parsing, validation, and reloads.
- `internal/dockhand/`: Dockhand API client.
- `internal/health/`: HTTP health checks.
- `internal/leases/`: lease state, timers, and stop callbacks.
- `internal/readiness/`: readiness state and status message policy.
- `internal/server/`: HTTP routes, handlers, wait-page rendering, and helpers.
- `internal/server/static/`: wait-page JavaScript and CSS.
- `internal/server/templates/`: embedded wait-page HTML templates.
- `docs/`: user-facing documentation.
- `targets/`: local target examples/config used by Compose.

## Development Procedure

1. Read the relevant package before editing.
2. Keep changes scoped to the failing behavior or requested feature.
3. Prefer existing package boundaries over adding new abstractions.
4. Keep `server.go` as glue only. Put handlers, view rendering, HTTP helpers,
   and readiness policy in their existing focused files/packages.
5. Run `gofmt` on changed Go files.
6. Run tests before reporting completion.
7. Update docs and `changelog.md` for user-visible behavior, config, routing,
   or operational changes.

## Go Commands

Use:

```sh
gofmt -w ./cmd ./internal
go test ./...
```

If the environment cannot write to the default Go caches, use writable caches:

```sh
GOCACHE=/tmp/go-build GOPATH=/tmp/go go test ./...
```

Some tests use `httptest` listeners. If sandboxing blocks local sockets, rerun
the same `go test ./...` command with the required permissions.

## Docker Commands

Validate Compose:

```sh
docker compose -f compose.yml config
```

Inspect logs:

```sh
docker logs reveille --tail 120
docker logs traefik --tail 120
```

## Traefik Checks

The Reveille middleware address should use the internal container DNS name:

```text
http://reveille:8080/api/traefik/forward-auth
```

The browser-facing route must send `/_reveille/*` to Reveille and must not use
the `reveille@file` middleware.

Useful checks from the Traefik container:

```sh
docker exec traefik wget -qO- http://reveille:8080/healthz
docker exec traefik wget -S -O- --no-check-certificate --header 'Host: app.example.com' 'https://127.0.0.1/_reveille/wait?host=app.example.com&format=status'
```

For timer POST checks:

```sh
docker exec traefik wget -S -O- --no-check-certificate --header 'Host: app.example.com' --post-data 'action=lease&lease=15m' 'https://127.0.0.1/_reveille/wait?host=app.example.com'
```

## Wait UI Procedure

- The wait page must render timer selection first.
- After the browser starts a timer, it switches to countdown/readiness polling.
- The countdown must count down from the backend lease expiration.
- The browser control channel is `/_reveille/wait`.
- Keep the compatibility API routes working unless intentionally removed.
- When changing the wait-page HTML/JS/CSS contract, bump `waitAssetVersion` in
  `internal/server/wait_view.go`.
- The client config in `#reveille-config` must render as a JSON object, not an
  escaped JSON string.

## Documentation Procedure

Update docs when behavior, configuration, routing, or operations change.
All user-facing documentation lives under `docs/`.
Base documentation on evidence from the codebase so it accurately informs
users.

Update `changelog.md` for user-visible changes.

## Git Hygiene

- Do not revert unrelated user changes.
- Check `git status --short` before and after work.
- Leave ignored `.workflow/` notes alone unless the task specifically asks for
  workflow updates.

## Commit Style

- Prefer verbose commit history over oversized catch-all commits.
- Split commits by coherent change when that makes review or rollback easier.
- Group tightly related edits in one commit when separating them would obscure
  the behavior being changed.
- Keep commit subjects and notes short, direct, and specific.
- Mention the user-visible behavior or subsystem touched, not every file edited.
