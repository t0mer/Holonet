# syntax=docker/dockerfile:1

# Stage 1: frontend — built once on the native build platform (never under QEMU).
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
# Keep the tracked embed placeholder so the build has a valid outDir target.
RUN mkdir -p /app/internal/webui/dist
COPY internal/webui/dist/.gitkeep /app/internal/webui/dist/.gitkeep
RUN npm run build   # Vite outDir -> ../internal/webui/dist

# Stage 2: Go binary — runs on the native build platform, cross-compiles to target.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/webui/dist ./internal/webui/dist
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build \
    -trimpath -ldflags="-s -w -X github.com/t0mer/holonet/internal/version.Version=${VERSION}" \
    -o /out/holonet ./cmd/holonet
# Pre-create a data dir owned by the non-root uid so the mounted volume is writable.
RUN mkdir -p /data && chown 10001:10001 /data

# Stage 3: runtime — scratch (design §0). Certs are copied for outbound HTTPS to
# notification services; the app runs as a non-root numeric uid.
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/holonet /holonet
COPY --from=builder --chown=10001:10001 /data /data
USER 10001:10001
EXPOSE 8080
EXPOSE 1162/udp
VOLUME ["/data"]
ENV HOLONET_DB_PATH=/data/holonet.db
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/holonet", "--version"]
ENTRYPOINT ["/holonet"]
