# syntax=docker/dockerfile:1
# brainDB: multi-stage image. Copyright 2026 Gabriel Koziestański, Apache License 2.0 (see LICENSE, NOTICE).
#   docker build -t braindb .                      # runtime image (web UI embedded)
#   docker build --target test -t braindb:test .   # runs Go tests + web typecheck during build

ARG GO_VERSION=1.26
ARG NODE_VERSION=22

# ---------- web UI (React + Vite) ----------
FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# Vite writes to ../internal/server/ui/dist (embedded by the Go server)
RUN mkdir -p /src/internal/server/ui && npm run build

# ---------- Go build ----------
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/internal/server/ui/dist ./internal/server/ui/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/braindb ./cmd/braindb

# ---------- tests (optional target) ----------
FROM build AS test
COPY examples ./examples
RUN go vet ./... && go test ./...
COPY --from=web /src/web /src/web
RUN apk add --no-cache nodejs npm && cd /src/web && npm run typecheck

# ---------- runtime ----------
FROM alpine:3.20 AS runtime
LABEL org.opencontainers.image.title="brainDB" \
      org.opencontainers.image.authors="Gabriel Koziestański" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="Graph database that works like a brain: long-term memory for AI agents"
RUN apk add --no-cache ca-certificates tini wget poppler-utils \
 && adduser -D -u 10001 -h /data braindb
COPY --from=build /out/braindb /usr/local/bin/braindb
COPY examples /opt/braindb/examples
COPY LICENSE NOTICE /opt/braindb/
COPY docker/entrypoint.sh /usr/local/bin/braindb-entrypoint
RUN chmod 0755 /usr/local/bin/braindb-entrypoint && mkdir -p /data && chown braindb:braindb /data
USER braindb
WORKDIR /data
ENV BRAINDB_ADDR=0.0.0.0:7791 \
    BRAINDB_BRAIN=/data/brain.xml \
    BRAINDB_EXAMPLE=""
EXPOSE 7791
VOLUME ["/data"]
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:7791/api/health >/dev/null || exit 1
ENTRYPOINT ["/sbin/tini", "--", "braindb-entrypoint"]
CMD ["serve"]
