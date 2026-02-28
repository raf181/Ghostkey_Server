# Containerfile for Podman - Lightweight and Secure Deployment
# Using distroless base image for minimal attack surface
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Install required system dependencies including CGO build tools
RUN apk add --no-cache ca-certificates git gcc musl-dev sqlite-dev

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application with CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s' \
    -a -installsuffix cgo \
    -o ghostkey-server .

# Use minimal Alpine image for runtime (distroless doesn't support CGO binaries)
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite tzdata && \
    adduser -D -s /bin/sh -u 65532 nonroot

# Copy CA certificates and binary
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/ghostkey-server /usr/local/bin/ghostkey-server
COPY --from=builder /app/config.json /app/config.json

# Set working directory
WORKDIR /app

# Create data directories and set proper permissions
RUN mkdir -p /app/data && \
    chown -R nonroot:nonroot /app && \
    chmod -R 755 /app

# Switch to non-root user
USER nonroot:nonroot

# Expose port
EXPOSE 5000

# Run the application
ENTRYPOINT ["/usr/local/bin/ghostkey-server"]
