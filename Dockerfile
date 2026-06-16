FROM golang:1.26.4-alpine3.23 AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/reveille ./cmd/reveille

FROM alpine:3.23

RUN adduser -D -H -u 10001 reveille \
	&& apk add --no-cache ca-certificates

USER reveille
EXPOSE 8080

COPY --from=build /out/reveille /usr/local/bin/reveille

ENTRYPOINT ["/usr/local/bin/reveille"]
CMD ["-config", "/etc/reveille/reveille.yml", "-hosts", "/etc/reveille/hosts"]
