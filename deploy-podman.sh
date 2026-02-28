#!/bin/bash

# Podman deployment script for Ghostkey Server
# This script provides a secure and lightweight deployment using Podman

set -euo pipefail

# Configuration
IMAGE_NAME="ghostkey-server"
CONTAINER_NAME="ghostkey-server"
DATA_DIR="./data"
CONFIG_FILE="./config.json"
SECRETS_FILE="./.secrets"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Podman is installed
check_podman() {
    if ! command -v podman &> /dev/null; then
        error "Podman is not installed. Please install Podman first."
        exit 1
    fi
    log "Podman is installed: $(podman --version)"
}

# Create necessary directories
create_directories() {
    log "Creating necessary directories..."
    mkdir -p "$DATA_DIR"
    
    # Set proper permissions for rootless containers
    # UID 65532 is the nonroot user in distroless images
    chmod 755 "$DATA_DIR"
    
    # Make config file readable by all
    chmod 644 "$CONFIG_FILE"
}

# Check if configuration file exists
check_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        error "Configuration file $CONFIG_FILE not found!"
        exit 1
    fi
    log "Configuration file found: $CONFIG_FILE"
}

# Generate secret key if not provided
generate_secret() {
    if [[ -z "${SECRET_KEY:-}" ]]; then
        # Check if secrets file exists and load from it
        if [[ -f "$SECRETS_FILE" ]]; then
            log "Loading SECRET_KEY from existing secrets file..."
            export SECRET_KEY=$(grep "^SECRET_KEY=" "$SECRETS_FILE" | cut -d'=' -f2)
            if [[ -n "$SECRET_KEY" ]]; then
                log "SECRET_KEY loaded from $SECRETS_FILE"
                return
            fi
        fi
        
        warn "SECRET_KEY not set. Generating a random key..."
        export SECRET_KEY=$(openssl rand -hex 32)
        
        # Save the key to secrets file
        log "Saving SECRET_KEY to $SECRETS_FILE"
        echo "# Ghostkey Server Secrets" > "$SECRETS_FILE"
        echo "# Generated on $(date)" >> "$SECRETS_FILE"
        echo "SECRET_KEY=$SECRET_KEY" >> "$SECRETS_FILE"
        
        # Set proper permissions (read/write for owner only)
        chmod 600 "$SECRETS_FILE"
        
        log "Generated and saved SECRET_KEY to $SECRETS_FILE"
        log "Please keep this file secure and do not commit it to version control!"
    fi
}

# Build the container image
build_image() {
    log "Building Podman image..."
    podman build \
        --tag "$IMAGE_NAME" \
        --file Containerfile \
        --force-rm \
        --no-cache \
        .
    log "Image built successfully: $IMAGE_NAME"
}

# Stop and remove existing container
cleanup_existing() {
    if podman container exists "$CONTAINER_NAME" 2>/dev/null; then
        log "Stopping existing container..."
        podman stop "$CONTAINER_NAME" 2>/dev/null || true
        log "Removing existing container..."
        podman rm "$CONTAINER_NAME" 2>/dev/null || true
    fi
}

# Run the container with security configurations
run_container() {
    log "Starting Ghostkey Server container with security configurations..."
    
    podman run -d \
        --name "$CONTAINER_NAME" \
        --publish 5000:5000 \
        --env SECRET_KEY="$SECRET_KEY" \
        --env GIN_MODE=release \
        --volume "$PWD/$DATA_DIR:/app/data:rw" \
        --volume "$PWD/$CARGO_DIR:/app/cargo_files:rw" \
        --tmpfs /tmp:rw,noexec,nosuid,size=100m \
        --security-opt no-new-privileges:true \
        --cap-drop ALL \
        --memory 256m \
        --cpus 0.5 \
        --restart unless-stopped \
        "$IMAGE_NAME"
    
    log "Container started successfully!"
}

# Show container status
show_status() {
    log "Container status:"
    podman ps --filter name="$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    
    log "Container logs (last 10 lines):"
    podman logs --tail 10 "$CONTAINER_NAME" 2>/dev/null || warn "Container not running or no logs available"
}

# Main deployment function
deploy() {
    log "Starting Podman deployment for Ghostkey Server..."
    
    check_podman
    create_directories
    check_config
    generate_secret
    cleanup_existing
    build_image
    run_container
    
    # Wait a moment for container to start
    sleep 5
    show_status
    
    log "Deployment completed successfully!"
    log "Ghostkey Server is now running on http://localhost:5000"
    log "Use 'podman logs $CONTAINER_NAME' to view logs"
    log "Use 'podman stop $CONTAINER_NAME' to stop the server"
}

# Script usage
usage() {
    echo "Usage: $0 [COMMAND]"
    echo "Commands:"
    echo "  deploy    - Deploy the Ghostkey Server (default)"
    echo "  stop      - Stop the running container"
    echo "  restart   - Restart the container"
    echo "  logs      - Show container logs"
    echo "  status    - Show container status"
    echo "  cleanup   - Stop and remove container and image"
    echo "  help      - Show this help message"
}

# Handle script arguments
case "${1:-deploy}" in
    deploy)
        deploy
        ;;
    stop)
        log "Stopping container..."
        podman stop "$CONTAINER_NAME"
        log "Container stopped."
        ;;
    restart)
        log "Restarting container..."
        podman restart "$CONTAINER_NAME"
        log "Container restarted."
        ;;
    logs)
        podman logs -f "$CONTAINER_NAME"
        ;;
    status)
        show_status
        ;;
    cleanup)
        log "Cleaning up..."
        podman stop "$CONTAINER_NAME" 2>/dev/null || true
        podman rm "$CONTAINER_NAME" 2>/dev/null || true
        podman rmi "$IMAGE_NAME" 2>/dev/null || true
        log "Cleanup completed."
        ;;
    help)
        usage
        ;;
    *)
        error "Unknown command: $1"
        usage
        exit 1
        ;;
esac
