# Traefik Quick Start

This page gets Reveille connected to Traefik with the common setup:

- a reusable file-provider `forwardAuth` middleware
- Docker labels on the Reveille container for the browser-facing wait UI route
- `reveille@file` attached to each app router you want Reveille to manage

For detailed route behavior and alternatives, see
[reference.md](reference.md).

## Prerequisites

- Traefik and Reveille share a Docker network.
- Traefik can resolve `http://reveille:8080`.
- Reveille can reach Dockhand.
- Each managed app already has a Traefik router.
- Each Reveille target entry uses the same public hostname as the app router.

## 1. Add The Middleware

Add a reusable middleware to a Traefik file-provider config:

```yaml
http:
  middlewares:
    reveille:
      forwardAuth:
        address: http://reveille:8080/api/traefik/forward-auth
        trustForwardHeader: true
        authRequestHeaders:
          - X-Forwarded-Method
          - X-Forwarded-Proto
          - X-Forwarded-Host
          - X-Forwarded-Uri
          - X-Forwarded-For
```

This address is internal. It is called by Traefik, not by the browser.

## 2. Add The Reveille UI Route

If Reveille runs in Docker with Traefik, add labels to the Reveille service:

```yaml
services:
  reveille:
    image: ghcr.io/judg3d/reveille:latest
    expose:
      - "8080"
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=<traefik-shared-network>"
      - "traefik.http.routers.reveille-ui.rule=PathPrefix(`/_reveille`)"
      - "traefik.http.routers.reveille-ui.entrypoints=<https-entrypoint>"
      - "traefik.http.routers.reveille-ui.tls=true"
      - "traefik.http.routers.reveille-ui.tls.certresolver=<cert-resolver>"
      - "traefik.http.routers.reveille-ui.priority=100000"
      - "traefik.http.routers.reveille-ui.middlewares=<https-header-middleware>@file"
      - "traefik.http.routers.reveille-ui.service=reveille-ui"
      - "traefik.http.services.reveille-ui.loadbalancer.server.port=8080"
    networks:
      - <traefik-shared-network>

networks:
  <traefik-shared-network>:
    external: true
```

Replace the placeholders:

- `<traefik-shared-network>`: Docker network shared by Traefik and Reveille
- `<https-entrypoint>`: HTTPS entrypoint, commonly `websecure`
- `<cert-resolver>`: ACME resolver, for example `cloudflare` or `letsencrypt`
- `<https-header-middleware>`: optional forwarded-header middleware; remove the
  middleware label if you do not use one

Do not add a host-level `ports:` mapping for the Traefik path. Reveille only
needs to be reachable by Traefik on the shared Docker network.

Do not attach `reveille@file` to the `reveille-ui` router.

## 3. Attach Middleware To An App Router

Add `reveille@file` to every app router Reveille should manage:

```yaml
http:
  routers:
    pdf:
      rule: "Host(`pdf.example.com`)"
      service: pdf
      entryPoints:
        - websecure
      tls:
        certResolver: cloudflare
      middlewares:
        - sslheader@file
        - reveille@file

  services:
    pdf:
      loadBalancer:
        servers:
          - url: "http://10.0.0.50:8002"
        passHostHeader: true
```

## 4. Add A Matching Target

The Reveille target hostname must match the Traefik router hostname:

```yaml
target:
  pdf:
    type: stack
    environment: docker
    hostname: pdf.example.com
    healthUrl: http://10.0.0.50:8002/api/v1/info/status
```

## 5. Test The Flow

Open the managed app domain while the target is stopped.

Expected behavior:

1. Traefik calls Reveille through `reveille@file`.
2. Reveille starts the target through Dockhand.
3. The browser redirects to `https://pdf.example.com/_reveille/wait?...`.
4. The user selects a timer.
5. The wait page counts down while polling readiness.
6. Reveille redirects back to the original app URL once healthy.
