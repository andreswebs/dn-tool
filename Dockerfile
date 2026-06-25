# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /build
COPY src/go.mod .
COPY src/go.sum .
RUN go mod download
COPY src/ ./

ARG TARGETOS TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -buildid= -X github.com/andreswebs/dn-tool/internal/version.Override=${VERSION}" \
    -o dn-tool ./cmd/dn-tool

FROM alpine:latest AS runtime

RUN apk add --no-cache ca-certificates

ARG APP_UID="2000"
ARG APP_GID="2000"
ARG APP_USER="app"
ARG APP_GROUP="app"

RUN \
    addgroup -g "${APP_GID}" "${APP_GROUP}" && \
    adduser \
      -u "${APP_UID}" \
      -g "" \
      -G "${APP_GROUP}" \
      -h "/home/${APP_USER}" \
      -D \
      -s "/bin/sh" \
      "${APP_USER}"

COPY --from=build /build/dn-tool /usr/local/bin/dn-tool

RUN mkdir -p /var/lib/defined/bin && \
    chown -R "${APP_USER}:${APP_GROUP}" /var/lib/defined

USER ${APP_USER}

ENTRYPOINT [ "/usr/local/bin/dn-tool" ]
