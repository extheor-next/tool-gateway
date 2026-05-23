FROM node:24 AS webui-builder

WORKDIR /app/webui
COPY webui/package.json webui/package-lock.json ./
RUN npm ci
COPY config.example.json /app/config.example.json
COPY webui ./
RUN npm run build

FROM golang:1.26 AS go-builder
WORKDIR /app
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_VERSION
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN set -eux; \
    GOOS="${TARGETOS:-$(go env GOOS)}"; \
    GOARCH="${TARGETARCH:-$(go env GOARCH)}"; \
    BUILD_VERSION_RESOLVED="${BUILD_VERSION:-}"; \
    if [ -z "${BUILD_VERSION_RESOLVED}" ] && [ -f VERSION ]; then BUILD_VERSION_RESOLVED="$(cat VERSION | tr -d "[:space:]")"; fi; \
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build -buildvcs=false -ldflags="-s -w -X tool-gateway/internal/version.BuildVersion=${BUILD_VERSION_RESOLVED}" -o /out/tool-gateway ./cmd/tool-gateway

FROM busybox:1.36.1-musl AS busybox-tools

FROM debian:bookworm-slim AS runtime-base
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && groupadd -r tool-gateway && useradd -r -g tool-gateway -d /app -s /sbin/nologin tool-gateway \
    && mkdir -p /app/data /data && chown -R tool-gateway:tool-gateway /app /data \
    && rm -rf /var/lib/apt/lists/*
COPY --from=busybox-tools /bin/busybox /usr/local/bin/busybox
EXPOSE 5001
CMD ["/usr/local/bin/tool-gateway"]

FROM runtime-base AS runtime-from-source
COPY --from=go-builder /out/tool-gateway /usr/local/bin/tool-gateway

COPY --from=go-builder --chown=tool-gateway:tool-gateway /app/config.example.json /app/config.example.json
COPY --from=webui-builder --chown=tool-gateway:tool-gateway /app/static/admin /app/static/admin
USER tool-gateway

FROM busybox-tools AS dist-extract
ARG TARGETARCH
COPY dist/docker-input/linux_amd64.tar.gz /tmp/tool-gateway_linux_amd64.tar.gz
COPY dist/docker-input/linux_arm64.tar.gz /tmp/tool-gateway_linux_arm64.tar.gz
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) ARCHIVE="/tmp/tool-gateway_linux_amd64.tar.gz" ;; \
      arm64) ARCHIVE="/tmp/tool-gateway_linux_arm64.tar.gz" ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    tar -xzf "${ARCHIVE}" -C /tmp; \
    PKG_DIR="$(find /tmp -maxdepth 1 -type d -name "tool-gateway_*_linux_${TARGETARCH}" | head -n1)"; \
    test -n "${PKG_DIR}"; \
    mkdir -p /out/static; \
    cp "${PKG_DIR}/tool-gateway" /out/tool-gateway; \
    cp "${PKG_DIR}/config.example.json" /out/config.example.json; \
    cp -R "${PKG_DIR}/static/admin" /out/static/admin

FROM runtime-base AS runtime-from-dist
COPY --from=dist-extract /out/tool-gateway /usr/local/bin/tool-gateway

COPY --from=dist-extract --chown=tool-gateway:tool-gateway /out/config.example.json /app/config.example.json
COPY --from=dist-extract --chown=tool-gateway:tool-gateway /out/static/admin /app/static/admin
USER tool-gateway

FROM runtime-from-source AS final
