# ── Stage 1: build the Svelte UI ─────────────────────────────────────────────
FROM node:22-alpine AS ui-builder

WORKDIR /src/ui

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package manifests first for layer caching.
COPY ui/package.json ui/pnpm-lock.yaml ./

RUN pnpm install --frozen-lockfile

# Copy the rest of the UI source and build.
COPY ui/ ./
RUN pnpm run build
# Output: /src/ui/dist/

# ── Stage 2: build the Go binary ─────────────────────────────────────────────
FROM golang:1.23-alpine AS go-builder

# ca-certificates needed for TLS calls to LLM providers at runtime.
RUN apk add --no-cache ca-certificates gcc musl-dev

WORKDIR /src

# Download Go modules first (better layer caching).
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree.
COPY . .

# Bring in the UI dist produced by stage 1.
COPY --from=ui-builder /src/ui/dist ./ui/dist/

# Build a statically-linked binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /agent-orchestrator \
    ./cmd

# ── Stage 3: minimal runtime image ────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates for outbound HTTPS (LLM API calls).
RUN apk add --no-cache ca-certificates tzdata

# Non-root user for security.
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app

# Copy only the binary from the build stage.
COPY --from=go-builder /agent-orchestrator /app/agent-orchestrator

# Default config location — mount a real config.yaml at runtime.
COPY config.example.yaml /app/config.yaml

# Data directory for SQLite — should be a named volume in production.
RUN mkdir -p /app/data && chown -R app:app /app

USER app

EXPOSE 8080

ENTRYPOINT ["/app/agent-orchestrator"]
CMD ["--config", "/app/config.yaml", "--db", "/app/data/agent.db"]
