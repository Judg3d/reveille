# Traefik Wiring

This page shows how to connect Reveille to Traefik for a first-time setup using
the file provider.

Reveille is used as a `forwardAuth` middleware. Traefik continues to own the
public routers and backend services. Reveille only decides whether a request
should pass through immediately or wake a stopped target first.

## Before You Start

- Traefik must be able to reach `http://reveille:8080`
- Reveille must be able to reach Dockhand
- you must already have a working Traefik router for the app you want to manage
- your Reveille host file must use the same public hostname as that Traefik
  router

## What You Are Adding

To use Reveille with Traefik, you normally add two pieces:

1. a Traefik middleware named `reveille`
2. `reveille@file` in the middleware list for the app router you want to manage

In many setups, that is enough.

## Step 1: Add The Reveille Middleware

Add this to one of your Traefik file-provider YAML files:

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

This tells Traefik:

- Traefik calls Reveille before forwarding the request to the target service
- Reveille uses the forwarded host and URI to decide which target to manage
- when the target is healthy, Reveille returns `204` and Traefik continues as
  normal
- when the target is stopped, Reveille starts it and returns a redirect to the
  wait UI

## Step 2: Attach The Middleware To An App Router

Pick a router you want Reveille to manage and add `reveille@file` to its
middleware list:

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

In this example:

- `pdf.example.com` is the public host
- the `pdf` router calls Reveille first
- if Reveille returns `204`, Traefik forwards to `http://10.0.0.50:8002`

## Step 3: Add A Matching Reveille Host Entry

The Reveille target file must use the same public hostname:

```yaml
target:
  pdf:
    type: stack
    environment: docker
    hostname: pdf.example.com
    healthUrl: http://10.0.0.50:8002/api/v1/info/status
```

If the hostname in Traefik and the hostname in Reveille do not match, Reveille
will not manage that router correctly.

## Step 4: Decide Whether You Need A Dedicated Wait-UI Route

In this setup, Reveille redirects to `/_reveille/...` on the same public host.

Confirmed working behavior in this environment:

- with a dedicated Reveille router/service for `/_reveille/*`
- without a dedicated Reveille router/service

Because of that, the simplest first-time setup is:

1. add the `reveille` middleware
2. attach `reveille@file` to the app router
3. test the redirect flow

If that works in your Traefik layout, you may not need a separate Reveille UI
router at all.

If your Traefik layout needs an explicit route for the wait UI, add one like
this:

```yaml
http:
  routers:
    reveille-ui:
      rule: PathPrefix(`/_reveille`)
      entryPoints:
        - websecure
      service: reveille
      tls: true
      priority: 10000

  services:
    reveille:
      loadBalancer:
        servers:
          - url: http://reveille:8080
```

Do not attach `reveille@file` to the `reveille-ui` router.

## Step 5: Make Sure Networking Works

Traefik and Reveille must share a Docker network so `reveille` resolves from
Traefik.

Example expectation:

- Traefik container can resolve `reveille`
- Traefik can reach `http://reveille:8080/api/traefik/forward-auth`
- Reveille can reach Dockhand and any configured `healthUrl`

## Troubleshooting

If the request never reaches Reveille:

- verify Traefik and Reveille share a network
- verify the middleware file is loaded by Traefik
- verify the router actually includes `reveille@file`

If Reveille shows the wait page but never bypasses:

- verify the host file `hostname` matches the public host exactly
- verify the target uses the correct `environment`
- verify `healthUrl` returns a status Reveille treats as healthy

If the browser gets redirected to `http://reveille:8080/...`:

- Reveille is building an internal redirect instead of a public one
- the app should redirect to the public host path `/_reveille/...`
