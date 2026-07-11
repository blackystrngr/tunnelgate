#!/usr/bin/env bash
# TunnelGate – All‑in‑one installer (iptables edition)
# Usage: curl -sSL https://raw.githubusercontent.com/blackystrngr/tunnelgate/main/install.sh | sudo bash

set -euo pipefail

# ---------------------------------------------------------------------
# Colors and helpers
# ---------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

info()  { echo -e "${GREEN}[+]${NC} $1"; }
warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
error() { echo -e "${RED}[X]${NC} $1"; exit 1; }

# ---------------------------------------------------------------------
# Root check
# ---------------------------------------------------------------------
if [[ $EUID -ne 0 ]]; then
    error "This script must be run as root. Use: sudo bash install.sh"
fi

# ---------------------------------------------------------------------
# Detect OS
# ---------------------------------------------------------------------
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS=$ID
    VERSION=$VERSION_ID
else
    error "Unsupported OS – only Debian/Ubuntu are supported."
fi

case $OS in
    debian|ubuntu)
        info "Detected $OS $VERSION"
        ;;
    *)
        error "Unsupported OS: $OS – only Debian/Ubuntu are supported."
        ;;
esac

# ---------------------------------------------------------------------
# Install base packages (including iptables)
# ---------------------------------------------------------------------
info "Updating package lists..."
apt-get update -y

info "Installing required packages..."
apt-get install -y \
    curl \
    wget \
    git \
    make \
    golang-go \
    nginx-extras \
    certbot \
    python3-certbot-nginx \
    dropbear \
    iptables \
    iptables-persistent \
    openssl \
    sqlite3

# Verify Nginx stream module
if ! nginx -V 2>&1 | grep -q with-stream; then
    error "Nginx installed without stream module. Please install nginx-extras manually."
fi

# ---------------------------------------------------------------------
# Determine working directory
# ---------------------------------------------------------------------
if [[ -d "$(pwd)/.git" ]] && grep -q "tunnelgate" "$(pwd)/README.md" 2>/dev/null; then
    REPO_DIR="$(pwd)"
    info "Using existing repository at $REPO_DIR"
else
    REPO_DIR="/opt/tunnelgate"
    info "Cloning TunnelGate into $REPO_DIR ..."
    rm -rf "$REPO_DIR"
    git clone https://github.com/blackystrngr/tunnelgate.git "$REPO_DIR"
    cd "$REPO_DIR"
fi

# ---------------------------------------------------------------------
# Build the binary
# ---------------------------------------------------------------------
info "Building tunnelgate binary..."
make clean
make build

BINARY="$REPO_DIR/bin/tunnelgate"
if [[ ! -f "$BINARY" ]]; then
    error "Build failed – binary not found."
fi

cp "$BINARY" /usr/local/bin/tunnelgate
chmod +x /usr/local/bin/tunnelgate

# ---------------------------------------------------------------------
# Prepare directories and default config
# ---------------------------------------------------------------------
mkdir -p /etc/tunnelgate /var/lib/tunnelgate /etc/tunnelgate/certs
chmod 700 /etc/tunnelgate /var/lib/tunnelgate

if [[ ! -f /etc/tunnelgate/config.yaml ]]; then
    cp "$REPO_DIR/config.yaml.example" /etc/tunnelgate/config.yaml
    chmod 600 /etc/tunnelgate/config.yaml
fi

# ---------------------------------------------------------------------
# Install systemd services
# ---------------------------------------------------------------------
info "Installing systemd service units..."
cp "$REPO_DIR/systemd"/*.service /etc/systemd/system/ 2>/dev/null || warn "No systemd units found – skipping."
cp "$REPO_DIR/systemd"/*.timer /etc/systemd/system/ 2>/dev/null || true

systemctl daemon-reload

systemctl enable tunnelgate-proxy.service 2>/dev/null || true
systemctl enable tunnelgate-api.service 2>/dev/null || true
systemctl enable tunnelgate-renew.timer 2>/dev/null || true

# ---------------------------------------------------------------------
# Firewall (iptables) – safe setup
# ---------------------------------------------------------------------
info "Configuring firewall (iptables)..."

# Flush all existing rules
iptables -F
iptables -X
iptables -t nat -F
iptables -t mangle -F

# Set default policies: drop incoming/forward, allow outgoing
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT ACCEPT

# Allow loopback
iptables -A INPUT -i lo -j ACCEPT

# Allow established/related connections
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow SSH (critical – prevents lockout)
iptables -A INPUT -p tcp --dport 22 -j ACCEPT

# Allow TunnelGate ports (plain HTTP and HTTPS)
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Save rules permanently
netfilter-persistent save
info "Firewall rules applied and saved."

# ---------------------------------------------------------------------
# Run interactive setup
# ---------------------------------------------------------------------
info "Starting interactive configuration..."
tunnelgate init

# ---------------------------------------------------------------------
# Start services
# ---------------------------------------------------------------------
info "Starting services..."
systemctl start tunnelgate-proxy.service
systemctl start tunnelgate-api.service
systemctl start tunnelgate-renew.timer 2>/dev/null || true

# ---------------------------------------------------------------------
# Final message
# ---------------------------------------------------------------------
echo ""
info "TunnelGate has been successfully installed and started."
echo ""
echo "  - Domain:          $(grep ^domain: /etc/tunnelgate/config.yaml | awk '{print $2}')"
echo "  - HTTP port:       80 (ws://)"
echo "  - TLS port:        443 (wss://)"
echo "  - Admin API:       http://127.0.0.1:8080 (token in config)"
echo "  - Database:        /var/lib/tunnelgate/users.db"
echo ""
echo "Firewall rules (iptables) applied:"
echo "  - SSH (22), HTTP (80), HTTPS (443) are open."
echo "  - All other incoming ports are DROPPED."
echo ""
echo "Next steps:"
echo "  1. Add a user:    tunnelgate user add <username> --days 30"
echo "  2. Check status:  tunnelgate status"
echo "  3. View logs:     journalctl -u tunnelgate-proxy -f"
echo ""
echo "Configure HTTP Injector with:"
echo "  - SSH Host:       $(grep ^domain: /etc/tunnelgate/config.yaml | awk '{print $2}')"
echo "  - SSH Port:       80 or 443"
echo "  - Username:       <your user>"
echo "  - Password:       <the one you set>"
echo "  - Payload:        any HTTP GET with or without Upgrade header."
echo ""
