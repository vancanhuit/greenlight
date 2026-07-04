# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS builder

RUN --mount=type=cache,target=/var/lib/apt \
    apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    git

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV MISE_DATA_DIR="/mise"
ENV MISE_CONFIG_DIR="/mise"
ENV MISE_CACHE_DIR="/mise/cache"
ENV MISE_INSTALL_PATH="/usr/local/bin/mise"
ENV PATH="/mise/shims:$PATH"
ENV DEBIAN_FRONTEND=noninteractive

RUN curl https://mise.run | sh

WORKDIR /app

COPY mise.toml mise.toml
COPY mise.lock mise.lock
RUN --mount=type=cache,target=/mise/cache \
    mise trust && mise install go && mise install sqlc

COPY go.mod .
COPY go.sum .
COPY vendor vendor

COPY sqlc.yaml sqlc.yaml
COPY migrations migrations
COPY cmd cmd
COPY internal internal

ARG VERSION
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build,id=go-build-$TARGETARCH \
    VERSION=$VERSION GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=vendor \
    mise run build

FROM gcr.io/distroless/base-debian13
COPY --from=builder /app/bin/api /bin/api
ENTRYPOINT ["/bin/api"]
