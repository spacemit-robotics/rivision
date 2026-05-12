#!/bin/bash
# RiVision Worker Installation Script
# Usage: ./install.sh [--prefix /opt/rivision/rivision_worker]

set -e

# Default values
PREFIX="/opt/rivision/rivision_worker"
USER="root"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --prefix)
            PREFIX="$2"
            shift 2
            ;;
        --user)
            USER="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--prefix <path>] [--user <user>]"
            echo "  --prefix  Installation directory (default: /opt/rivision/rivision_worker)"
            echo "  --user    Service user (default: root)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "=== RiVision Worker Installation ==="
echo "Prefix: $PREFIX"
echo "User: $USER"
echo ""

# Create directories
echo "[1/6] Creating directories..."
mkdir -p "$PREFIX"/{bin,lib,config,data,logs}
mkdir -p "$PREFIX/data"/{thumbnails,recordings,vectors}

# Copy binaries
echo "[2/6] Installing binaries..."
if [ -f "rivision_worker" ]; then
    cp rivision_worker "$PREFIX/bin/"
fi
if [ -f "rivision_vlm" ]; then
    cp rivision_vlm "$PREFIX/bin/"
fi
chmod +x "$PREFIX/bin"/*

# Copy libraries (if any)
echo "[3/6] Installing libraries..."
if [ -d "lib" ]; then
    cp -r lib/* "$PREFIX/lib/" 2>/dev/null || true
fi

# Copy config
echo "[4/6] Installing configuration..."
if [ -d "config" ]; then
    cp -n config/*.yaml "$PREFIX/config/" 2>/dev/null || true
fi

# Install systemd service
echo "[5/6] Installing systemd service..."
if [ -f "scripts/rivision-worker.service" ]; then
    # Update paths in service file
    sed -e "s|/opt/rivision/rivision_worker|$PREFIX|g" \
        -e "s|User=root|User=$USER|g" \
        scripts/rivision-worker.service > /etc/systemd/system/rivision-worker.service
    
    systemctl daemon-reload
    echo "Service installed. Use: systemctl enable --now rivision-worker"
fi

# Set permissions
echo "[6/6] Setting permissions..."
chown -R "$USER:$USER" "$PREFIX"

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Configuration: $PREFIX/config/worker.yaml"
echo "Data: $PREFIX/data/"
echo "Logs: $PREFIX/logs/"
echo ""
echo "Commands:"
echo "  systemctl enable rivision-worker   # Enable on boot"
echo "  systemctl start rivision-worker    # Start service"
echo "  systemctl status rivision-worker   # Check status"
echo "  journalctl -u rivision-worker -f   # View logs"
echo ""
