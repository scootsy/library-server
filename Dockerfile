# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Install SQLite dev headers required by mattn/go-sqlite3 (CGo).
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Cache dependencies before copying full source.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically-linked binary.
COPY . .
# fts5 tag enables the SQLite FTS5 full-text search extension (required for works_fts).
RUN CGO_ENABLED=1 GOOS=linux \
    go build -tags fts5 -ldflags="-s -w -extldflags '-static'" \
    -o /build/codex ./cmd/codex

# ── Runtime stage ──────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

# Create a non-root user for the process.
RUN groupadd -r codex && useradd -r -g codex -d /config codex

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Standard mount points (bind-mount your actual media here at runtime).
RUN mkdir -p /config /media /cache && chown -R codex:codex /config /media /cache

COPY --from=builder /build/codex /usr/local/bin/codex

USER codex

# Config directory exposed as a volume so the database persists across restarts.
VOLUME ["/config"]

# Media and cache directories. Mount your library here.
VOLUME ["/media", "/cache"]

EXPOSE 8080

# Health check hits the /health endpoint every 30 s.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/codex"]
CMD ["--config", "/config/config.yaml"]
