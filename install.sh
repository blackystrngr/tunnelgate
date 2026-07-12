#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------
REPO_URL="https://github.com/blackystrngr/tunnelgate.git"
INSTALL_DIR="/opt/tunnelgate"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/tunnelgate"
DATA_DIR="/var/lib/tunnelgate"
CERT_DIR="/etc/tunnelgate/certs"
NGINX_SITE="tunnelgate.conf"
SERVICE_PREFIX="tunnelgate"
SYSTEMD_DIR="/etc/systemd/system"

# ---------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[+]${NC} $*"; }
log_error() { echo -e "${RED}[X]${NC} $*" >&2; }

# ---------------------------------------------------------------------
# Root check
# ---------------------------------------------------------------------
if [[ $EUID -ne 0 ]]; then
    log_error "Must run as root: sudo $0"
    exit 1
fi

# ---------------------------------------------------------------------
# Cleanup (preserves certs)
# ---------------------------------------------------------------------
if [[ $# -gt 0 && "$1" == "--clean" ]]; then
    log_info "Cleaning up (preserving $CERT_DIR)..."
    for svc in proxy api renew; do
        systemctl stop "${SERVICE_PREFIX}-${svc}.service" 2>/dev/null || true
        systemctl disable "${SERVICE_PREFIX}-${svc}.service" 2>/dev/null || true
    done
    systemctl stop "${SERVICE_PREFIX}-renew.timer" 2>/dev/null || true
    systemctl disable "${SERVICE_PREFIX}-renew.timer" 2>/dev/null || true
    rm -f "${SYSTEMD_DIR}/${SERVICE_PREFIX}-"*.service
    rm -f "${SYSTEMD_DIR}/${SERVICE_PREFIX}-"*.timer
    systemctl daemon-reload
    rm -f "${BIN_DIR}/tunnelgate"
    rm -rf "$CONFIG_DIR" "$DATA_DIR"
    rm -rf "$INSTALL_DIR"
    rm -f "/etc/nginx/sites-available/$NGINX_SITE"
    rm -f "/etc/nginx/sites-enabled/$NGINX_SITE"
    systemctl reload nginx 2>/dev/null || true
    log_info "Cleanup done. Certificates remain in $CERT_DIR"
    exit 0
fi

# ---------------------------------------------------------------------
# OS detection
# ---------------------------------------------------------------------
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS=$ID
else
    log_error "Unsupported OS"
    exit 1
fi
case $OS in
    debian|ubuntu) ;;
    *) log_error "Unsupported OS: $OS"; exit 1 ;;
esac

# ---------------------------------------------------------------------
# Install packages and Go
# ---------------------------------------------------------------------
log_info "Installing system packages..."
apt-get update -y
apt-get install -y curl wget git make nginx-extras certbot dropbear iptables-persistent openssl sqlite3

log_info "Installing Go 1.23..."
GO_VERSION="1.23.0"
GO_ARCH="linux-amd64"
[[ "$(uname -m)" == "aarch64" ]] && GO_ARCH="linux-arm64"
cd /tmp
wget -q "https://go.dev/dl/go${GO_VERSION}.${GO_ARCH}.tar.gz"
tar -C /usr/local -xzf "go${GO_VERSION}.${GO_ARCH}.tar.gz"
export PATH="/usr/local/go/bin:$PATH"
echo 'export PATH=/usr/local/go/bin:$PATH' >> /etc/profile

# ---------------------------------------------------------------------
# Clone repo
# ---------------------------------------------------------------------
log_info "Cloning source..."
if [[ -d "$INSTALL_DIR/.git" ]]; then
    cd "$INSTALL_DIR"
    git fetch origin && git reset --hard origin/main
else
    rm -rf "$INSTALL_DIR"
    git clone "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# ---------------------------------------------------------------------
# Build (go mod tidy and build)
# ---------------------------------------------------------------------
log_info "Building..."
go mod download
go mod tidy
make clean
make build

cp bin/tunnelgate /usr/local/bin/
chmod +x /usr/local/bin/tunnelgate

# ---------------------------------------------------------------------
# Interactive config
# ---------------------------------------------------------------------
log_info "Configuration"
read -p "Domain: " DOMAIN
read -p "Email: " EMAIL
echo "Cert method: 1) HTTP-01  2) DNS-01 Cloudflare  3) Cloudflare Origin"
read -p "Choice: " CERT_CHOICE
case $CERT_CHOICE in
    1) CERT_METHOD="le_http01" ;;
    2) CERT_METHOD="le_dns_cf"; read -p "CF API Token: " CF_TOKEN ;;
    3) CERT_METHOD="cf_origin"; read -p "CF Email: " CF_EMAIL; read -p "CF Global Key: " CF_GLOBAL_KEY ;;
    *) log_error "Invalid"; exit 1 ;;
esac
read -p "HTTP ports (comma, e.g., 80,8080): " HTTP_PORTS
read -p "TLS ports (comma, e.g., 443,8443): " TLS_PORTS

HTTP_YAML=$(echo "$HTTP_PORTS" | sed 's/,/ /g' | xargs | sed 's/ /, /g')
TLS_YAML=$(echo "$TLS_PORTS" | sed 's/,/ /g' | xargs | sed 's/ /, /g')

mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$CERT_DIR"
cat > "$CONFIG_DIR/config.json" <<EOF
{
  "domain": "$DOMAIN",
  "email": "$EMAIL",
  "backend_host": "127.0.0.1",
  "backend_port": 109,
  "proxy": {"listen_host": "127.0.0.1", "listen_port": 8888, "idle_timeout_seconds": 180, "shared_pass": ""},
  "api": {"listen_host": "127.0.0.1", "listen_port": 8080, "token": "$(openssl rand -hex 24)"},
  "nginx": {"http_ports": [$HTTP_YAML], "tls_ports": [$TLS_YAML]},
  "cert_method": "$CERT_METHOD",
  "cf_api_token": "${CF_TOKEN:-""}",
  "cf_email": "${CF_EMAIL:-""}",
  "cf_global_api_key": "${CF_GLOBAL_KEY:-""}",
  "database": "$DATA_DIR/users.db",
  "log_level": "info",
  "log_format": "text"
}
EOF
chmod 600 "$CONFIG_DIR/config.json"

# ---------------------------------------------------------------------
# Systemd
# ---------------------------------------------------------------------
log_info "Installing systemd units..."
cat > "$SYSTEMD_DIR/${SERVICE_PREFIX}-proxy.service" <<EOF
[Unit]
Description=TunnelGate Proxy
After=network.target
[Service]
ExecStart=/usr/local/bin/tunnelgate
Restart=always
User=nobody
Group=nogroup
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable ${SERVICE_PREFIX}-proxy.service

# ---------------------------------------------------------------------
# Nginx and cert
# ---------------------------------------------------------------------
log_info "Configuring Nginx..."
tunnelgate nginx configure 2>/dev/null || log_info "Nginx config done manually later"
tunnelgate cert renew 2>/dev/null || log_info "Cert will be obtained later"

# ---------------------------------------------------------------------
# Firewall
# ---------------------------------------------------------------------
log_info "Firewall..."
iptables -F
iptables -P INPUT DROP
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -j ACCEPT
for p in $(echo "$HTTP_PORTS,$TLS_PORTS" | tr ',' ' '); do
    iptables -A INPUT -p tcp --dport "$p" -j ACCEPT
done
netfilter-persistent save

# ---------------------------------------------------------------------
# Start
# ---------------------------------------------------------------------
systemctl start ${SERVICE_PREFIX}-proxy.service

# ---------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------
echo ""
log_info "TunnelGate installed!"
echo "Domain: $DOMAIN"
echo "HTTP ports: $HTTP_PORTS"
echo "TLS ports: $TLS_PORTS"
echo "Add user: tunnelgate user add <name> --days 30"
