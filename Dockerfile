# NullGate 3.0 — api service (Go panel API + Xray engine)
# Railway: set Root Directory = / (repo root) for this service.

# ───────────────────────── stage 1: build Go binary ─────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ───────────────────── stage 2: runtime + pre-installed Xray ─────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata unzip wget

# Pre-install the pinned Xray release at build time so the container does NOT
# need to reach GitHub at boot (the Go supervisor will find the binary and
# skip its own downloader).
ARG XRAY_VERSION=v26.3.27
ARG TARGETARCH
RUN set -eux; \
    case "$TARGETARCH" in \
      arm64) A="Xray-linux-arm64-v8a.zip" ;; \
      386)   A="Xray-linux-32.zip" ;; \
      *)     A="Xray-linux-64.zip" ;; \
    esac; \
    wget -qO /tmp/xray.zip \
      "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${A}"; \
    mkdir -p /app/xray; \
    unzip -o /tmp/xray.zip -d /app/xray; \
    chmod 0755 /app/xray/xray; \
    rm -f /tmp/xray.zip

WORKDIR /app
COPY --from=build /out/api ./api

# Run as non-root. Xray binds 9000 (>1024) and writes runtime files to
# /tmp/nullgate, both fine without privileges.
RUN addgroup -S ng && adduser -S -G ng ng && chown -R ng:ng /app
USER ng

ENV PORT=8080 \
    XRAY_BIN=/app/xray/xray \
    NG_WORKDIR=/tmp/nullgate

EXPOSE 8080 9000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1 || exit 1

CMD ["./api"]
