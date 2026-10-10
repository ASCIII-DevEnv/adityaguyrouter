# Stage 1: Build the ultra-small Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod file
COPY go.mod ./

# Copy source code
COPY . .

# Compile static binary with optimizations (-ldflags="-s -w" to strip debug symbols)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o antigravity-gateway .

# Stage 2: Final ultra-lightweight Alpine container
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/antigravity-gateway /app/antigravity-gateway

# Copy web frontend assets
COPY public /app/public

# Default environment variables
ENV PORT=3456
ENV DATA_DIR=/app/data

# Volume for persistent accounts & API keys
VOLUME ["/app/data"]

EXPOSE 3456

CMD ["/app/antigravity-gateway"]
