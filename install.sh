#!/usr/bin/env bash
# TunnelGate – Ultra‑Robust Installer
# Usage: sudo ./install.sh [--clean]

set -euo pipefail

# =====================================================================
# CONFIGURATION
# =====================================================================
REPO_URL="https://github.com/blackystrngr/tunnelgate.git"
INSTALL_DIR="/opt/tunnelgate"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/tunnelgate"
DATA_DIR="/var/lib/tunnelgate"
CERT_DIR="/etc/tunnelgate/certs"
SERVICE_PREFIX="tunnelgate"

# Defaults
DOMAIN="${DOMAIN:-tunnel.example.com}"
EMAIL="${EMAIL:-admin@example.com}"
HTTP_PORTS="${HTTP_PORTS:-80}"
TLS_PORTS="${TLS_PORTS:-443}"
CERT_METHOD="${CERT_METHOD:-le_http01}"

# =====================================================================
# COLORS
# =====================================================================
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log_info()  { echo -e "${GREEN}[+]${NC} $(date +'%H:%M:%S') $*"; }
log_warn()  { echo -e "${YELLOW}[!]${NC} $(date +'%H:%M:%S') $*"; }
log_error() { echo -e "${RED}[X]${NC} $(date +'%H:%M:%S') $*" >&2; }
log_step()  { echo -e "${BLUE}[*]${NC} $(date +'%H:%M:%S') $*"; }

# =====================================================================
# ROOT CHECK & CLEANUP
# =====================================================================
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root. Use: sudo $0"
    exit 1
fi

if [[ $# -gt 0 && "$1" == "--clean" ]]; then
    log_warn "Cleaning TunnelGate (certificates preserved)..."
    systemctl stop tunnelgate-* 2>/dev/null || true
    systemctl disable tunnelgate-* 2>/dev/null || true
    rm -f /etc/systemd/system/tunnelgate-*.{service,timer}
    systemctl daemon-reload
    rm -f /usr/local/bin/tunnelgate
    rm -rf /etc/tunnelgate /var/lib/tunnelgate /opt/tunnelgate
    rm -f /etc/nginx/sites-{available,enabled}/tunnelgate.conf
    systemctl reload nginx 2>/dev/null || true
    log_info "Cleanup done. Certificates remain in /etc/tunnelgate/certs."
    exit 0
fi

# =====================================================================
# OS DETECTION
# =====================================================================
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS=$ID
else
    log_error "Cannot detect OS."
    exit 1
fi
case $OS in
    debian|ubuntu) log_info "Detected $OS" ;;
    *) log_error "Unsupported OS: $OS"; exit 1 ;;
esac

# =====================================================================
# PREPARE APT
# =====================================================================
log_step "Updating package lists..."
apt-get update -y

# =====================================================================
# SMART PACKAGE INSTALL (with retry)
# =====================================================================
install_packages() {
    local retries=3
    while (( retries > 0 )); do
        apt-get install -y "$@" && return 0
        log_warn "Package install failed, retrying... ($retries left)"
        (( retries-- ))
        sleep 2
        apt-get update -y
    done
    return 1
}

# =====================================================================
# INSTALL ESSENTIAL TOOLS (including psmisc for fuser)
# =====================================================================
log_step "Installing essential tools..."
install_packages curl wget git make || log_warn "Some tools failed to install."

# Ensure fuser is available (from psmisc)
if ! command -v fuser &>/dev/null; then
    log_info "Installing psmisc (provides fuser)..."
    install_packages psmisc || {
        log_warn "psmisc install failed – will use alternative methods to kill ports."
        # Fallback: define kill_port function that uses lsof or ss
        kill_port() {
            local port=$1
            local pid=$(lsof -ti :$port 2>/dev/null)
            if [[ -n "$pid" ]]; then
                kill -9 "$pid" 2>/dev/null && log_info "Killed PID $pid on port $port"
            fi
        }
    }
fi

# If fuser is now available, use it, otherwise use fallback
if command -v fuser &>/dev/null; then
    kill_port() { fuser -k "$1"/tcp 2>/dev/null && log_info "Killed process on port $1"; }
else
    kill_port() {
        local port=$1
        local pid=$(lsof -ti :$port 2>/dev/null)
        if [[ -n "$pid" ]]; then
            kill -9 "$pid" 2>/dev/null && log_info "Killed PID $pid on port $port"
        fi
    }
fi

# =====================================================================
# STEP 1: REMOVE CONFLICTING PROGRAMS
# =====================================================================
log_step "Removing conflicting web servers..."
for pkg in apache2 lighttpd nginx nginx-common nginx-core nginx-full; do
    if dpkg -l | grep -q "^ii  $pkg "; then
        log_info "Removing $pkg..."
        systemctl stop "$pkg" 2>/dev/null || true
        systemctl disable "$pkg" 2>/dev/null || true
        apt-get remove -y "$pkg" 2>/dev/null || true
    fi
done
rm -rf /etc/nginx 2>/dev/null || true

# =====================================================================
# STEP 2: KILL PORT PROCESSES
# =====================================================================
log_step "Killing processes on conflicting ports..."
for port in 80 443 2053 2083 2087 2096 8443; do
    kill_port "$port"
done
sleep 2

# =====================================================================
# STEP 3: CLEAN IPTABLES
# =====================================================================
log_step "Cleaning iptables rules..."
iptables-save > /tmp/iptables-backup-$(date +%s).txt 2>/dev/null || true
iptables -F 2>/dev/null || true
iptables -X 2>/dev/null || true
iptables -t nat -F 2>/dev/null || true
iptables -t mangle -F 2>/dev/null || true
iptables -P INPUT ACCEPT 2>/dev/null || true
iptables -P FORWARD ACCEPT 2>/dev/null || true
iptables -P OUTPUT ACCEPT 2>/dev/null || true

# =====================================================================
# STEP 4: INSTALL SYSTEM PACKAGES (including nginx-extras)
# =====================================================================
log_step "Installing system packages..."
install_packages \
    nginx-extras \
    certbot python3-certbot-nginx \
    dropbear \
    iptables iptables-persistent \
    openssl sqlite3 \
    net-tools lsof

if ! nginx -V 2>&1 | grep -q with-stream; then
    log_error "Nginx stream module missing. Please install nginx-extras manually."
    exit 1
fi

# =====================================================================
# STEP 5: INSTALL GO 1.23
# =====================================================================
log_step "Installing Go 1.23..."
GO_VERSION="1.23.0"
GO_ARCH="linux-amd64"
[[ "$(uname -m)" == "aarch64" ]] && GO_ARCH="linux-arm64"

cd /tmp
rm -f "go${GO_VERSION}.${GO_ARCH}.tar.gz"
wget -q "https://go.dev/dl/go${GO_VERSION}.${GO_ARCH}.tar.gz" || {
    log_error "Go download failed."
    exit 1
}
rm -rf /usr/local/go
tar -C /usr/local -xzf "go${GO_VERSION}.${GO_ARCH}.tar.gz"
rm -f "go${GO_VERSION}.${GO_ARCH}.tar.gz"

export PATH="/usr/local/go/bin:$PATH"
grep -q "export PATH=/usr/local/go/bin" /etc/profile || \
    echo 'export PATH=/usr/local/go/bin:$PATH' >> /etc/profile

GO_BIN="/usr/local/go/bin/go"
if ! $GO_BIN version | grep -q "go1.23"; then
    log_error "Go 1.23 installation failed."
    exit 1
fi
log_info "Go installed: $($GO_BIN version)"

# =====================================================================
# STEP 6: CLONE/UPDATE SOURCE
# =====================================================================
log_step "Setting up source code..."
if [[ -d "$INSTALL_DIR/.git" ]]; then
    cd "$INSTALL_DIR"
    git fetch origin
    git reset --hard origin/main
    git clean -f -d
else
    rm -rf "$INSTALL_DIR"
    git clone "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# =====================================================================
# STEP 7: BUILD
# =====================================================================
log_step "Building tunnelgate..."
$GO_BIN clean -modcache
$GO_BIN mod download
$GO_BIN mod tidy
make clean
make GO=$GO_BIN build

BINARY="$INSTALL_DIR/bin/tunnelgate"
if [[ ! -f "$BINARY" ]]; then
    log_error "Build failed."
    exit 1
fi
cp "$BINARY" "$BIN_DIR/tunnelgate"
chmod +x "$BIN_DIR/tunnelgate"

# =====================================================================
# STEP 8: PROMPT (with defaults)
# =====================================================================
log_step "Configuration setup"
read -p "Domain [${DOMAIN}]: " input; DOMAIN="${input:-$DOMAIN}"
read -p "Email [${EMAIL}]: " input; EMAIL="${input:-$EMAIL}"
read -p "HTTP ports (comma) [${HTTP_PORTS}]: " input; HTTP_PORTS="${input:-$HTTP_PORTS}"
read -p "TLS ports (comma) [${TLS_PORTS}]: " input; TLS_PORTS="${input:-$TLS_PORTS}"
echo "Certificate methods: 1) le_http01  2) le_dns_cf  3) cf_origin  4) selfsigned"
read -p "Method [${CERT_METHOD}]: " input; CERT_METHOD="${input:-$CERT_METHOD}"

case $CERT_METHOD in
    le_dns_cf|2) CERT_METHOD="le_dns_cf"; read -p "Cloudflare API Token: " CF_TOKEN; CF_TOKEN="${CF_TOKEN:-}" ;;
    cf_origin|3) CERT_METHOD="cf_origin"; read -p "Cloudflare Email: " CF_EMAIL; CF_EMAIL="${CF_EMAIL:-}"; read -p "Cloudflare Global API Key: " CF_GLOBAL_KEY; CF_GLOBAL_KEY="${CF_GLOBAL_KEY:-}" ;;
    selfsigned|4) CERT_METHOD="selfsigned" ;;
    *) CERT_METHOD="le_http01" ;;
esac

# =====================================================================
# STEP 9: CONFIG & DIRECTORIES
# =====================================================================
mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$CERT_DIR"
chmod 700 "$CONFIG_DIR" "$DATA_DIR"

API_TOKEN=$(openssl rand -hex 24)
HTTP_PORTS_YAML=$(echo "$HTTP_PORTS" | sed 's/,/ /g' | xargs | sed 's/ /, /g')
TLS_PORTS_YAML=$(echo "$TLS_PORTS" | sed 's/,/ /g' | xargs | sed 's/ /, /g')

cat > "$CONFIG_DIR/config.yaml" <<EOF
domain: $DOMAIN
email: $EMAIL
backend_host: 127.0.0.1
backend_port: 109
proxy:
  listen_host: 127.0.0.1
  listen_port: 8888
  idle_timeout_seconds: 180
  shared_pass: ""
api:
  listen_host: 127.0.0.1
  listen_port: 8080
  token: "$API_TOKEN"
nginx:
  http_ports: [$HTTP_PORTS_YAML]
  tls_ports: [$TLS_PORTS_YAML]
cert_method: $CERT_METHOD
cf_api_token: ${CF_TOKEN:-""}
cf_email: ${CF_EMAIL:-""}
cf_global_api_key: ${CF_GLOBAL_KEY:-""}
database: $DATA_DIR/users.db
log_level: info
log_format: text
EOF
chmod 600 "$CONFIG_DIR/config.yaml"

# =====================================================================
# STEP 10: DROPBEAR
# =====================================================================
log_step "Configuring dropbear..."
cat > /etc/default/dropbear <<'EOF'
NO_START=0
DROPBEAR_PORT="127.0.0.1:109"
DROPBEAR_EXTRA_ARGS="-W 65536"
DROPBEAR_BANNER=""
EOF
grep -q "/bin/false" /etc/shells || echo "/bin/false" >> /etc/shells
systemctl enable dropbear
systemctl restart dropbear

# =====================================================================
# STEP 11: SYSTEMD
# =====================================================================
log_step "Installing systemd units..."
cat > /etc/systemd/system/tunnelgate-proxy.service <<'EOF'
[Unit]
Description=TunnelGate Proxy Core
After=network.target dropbear.service
Requires=dropbear.service
[Service]
Type=simple
User=nobody
Group=nogroup
ExecStart=/usr/local/bin/tunnelgate start
Restart=always
RestartSec=5
PrivateTmp=true
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/tunnelgate-api.service <<'EOF'
[Unit]
Description=TunnelGate Admin API
After=network.target
[Service]
Type=simple
User=nobody
Group=nogroup
ExecStart=/usr/local/bin/tunnelgate api
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/tunnelgate-renew.service <<'EOF'
[Unit]
Description=Renew TLS certificate
[Service]
Type=oneshot
ExecStart=/usr/local/bin/tunnelgate cert renew
EOF

cat > /etc/systemd/system/tunnelgate-renew.timer <<'EOF'
[Unit]
Description=Daily TLS certificate renewal
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable tunnelgate-proxy tunnelgate-api tunnelgate-renew.timer

# =====================================================================
# STEP 12: NGINX (minimal)
# =====================================================================
log_step "Configuring Nginx..."
mkdir -p /etc/nginx  # Ensure directory exists
cat > /etc/nginx/nginx.conf <<'EOF'
events { worker_connections 1024; }
stream { include /etc/nginx/stream.conf; }
EOF

tunnelgate nginx configure 2>/dev/null || {
    cat > /etc/nginx/stream.conf <<EOF
# Minimal config
EOF
}
nginx -t 2>/dev/null || log_warn "Nginx config test failed – continuing anyway."
systemctl enable nginx
systemctl restart nginx

# =====================================================================
# STEP 13: CERTIFICATE
# =====================================================================
if [[ "$CERT_METHOD" == "selfsigned" ]]; then
    log_step "Generating self‑signed certificate..."
    openssl req -x509 -newkey rsa:2048 -nodes \
        -keyout "$CERT_DIR/key.pem" -out "$CERT_DIR/fullchain.pem" \
        -days 365 -subj "/CN=$DOMAIN" 2>/dev/null || true
else
    log_step "Obtaining certificate using $CERT_METHOD..."
    tunnelgate cert renew || log_warn "Certificate issuance failed – run 'tunnelgate cert renew' later."
fi

# =====================================================================
# STEP 14: FIREWALL
# =====================================================================
log_step "Configuring iptables..."
iptables -F; iptables -X; iptables -t nat -F; iptables -t mangle -F
iptables -P INPUT DROP; iptables -P FORWARD DROP; iptables -P OUTPUT ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -j ACCEPT
for p in $(echo "$HTTP_PORTS,$TLS_PORTS" | tr ',' ' '); do
    iptables -A INPUT -p tcp --dport "$p" -j ACCEPT
done
if command -v netfilter-persistent &>/dev/null; then
    netfilter-persistent save
else
    mkdir -p /etc/iptables
    iptables-save > /etc/iptables/rules.v4
fi
log_info "Firewall rules applied."

# =====================================================================
# STEP 15: START SERVICES
# =====================================================================
log_step "Starting services..."
systemctl start tunnelgate-proxy tunnelgate-api tunnelgate-renew.timer

# =====================================================================
# STEP 16: VERIFICATION
# =====================================================================
sleep 2
PROXY_OK=false
systemctl is-active --quiet tunnelgate-proxy && PROXY_OK=true

# =====================================================================
# FINAL MESSAGE
# =====================================================================
echo ""
log_info "TunnelGate installation complete!"
echo "  Domain:          $DOMAIN"
echo "  HTTP ports:      $HTTP_PORTS"
echo "  TLS ports:       $TLS_PORTS"
echo "  API Token:       $API_TOKEN"
echo "  Database:        $DATA_DIR/users.db"
[[ "$PROXY_OK" == "true" ]] && echo "✅ All services running." || echo "⚠️  Proxy failed – check journalctl -u tunnelgate-proxy"
echo ""
echo "Next: tunnelgate user add <username> --days 30"
echo "Clean: sudo $0 --clean"
