FROM golang:1.26.4-alpine3.23 AS build

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/reveille ./cmd/reveille

FROM alpine:3.23

RUN adduser -D -H -u 10001 reveille \
	&& apk add --no-cache ca-certificates \
	&& mkdir -p /var/lib/reveille \
	&& chown reveille:reveille /var/lib/reveille

USER reveille
EXPOSE 8080

# Lease state lives here so timers survive container restarts; mount a volume
# to keep it across container recreates.
VOLUME ["/var/lib/reveille"]

COPY --from=build /out/reveille /usr/local/bin/reveille

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/reveille"]
CMD ["-config", "/etc/reveille/reveille.yml", "-hosts", "/etc/reveille/hosts"]
