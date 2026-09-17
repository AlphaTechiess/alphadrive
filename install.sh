#!/usr/bin/env bash
# ==============================================================================
# AlphaDrive - Production-Grade One-Line Installer
# Repository: https://github.com/AlphaTechiess/alphadrive
# License: MIT
# ==============================================================================

set -Eeuo pipefail

# Reopen stdin from /dev/tty if running in a pipe (e.g. curl ... | sudo bash)
if [ ! -t 0 ] && [ -e /dev/tty ]; then
    exec < /dev/tty || true
fi

# Styling & Colors
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    MAGENTA='\033[0;35m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    MAGENTA=''
    CYAN=''
    BOLD=''
    NC=''
fi

info()    { printf "${BLUE}ℹ${NC} %s\n" "$*"; }
success() { printf "${GREEN}✓${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}⚠${NC} %s\n" "$*"; }
error()   { printf "${RED}✗${NC} %s\n" "$*" >&2; }
debug()   { if [ "${DEBUG:-false}" = "true" ]; then printf "${MAGENTA}[DEBUG]${NC} %s\n" "$*" >&2; fi; }
fatal()   { error "$*"; exit 1; }

# Global constants & variables
REPO="AlphaTechiess/alphadrive"
DEFAULT_PORT=8080
VERSION=""
PORT=""
BIND_IP=""
DOMAIN=""
PROXY=""
ACCESS_MODE=""
NON_INTERACTIVE=false
DEBUG=false
UPGRADE_MODE=false
TMP_DIR="$(mktemp -d /tmp/alphadrive-install.XXXXXX)"

cleanup() {
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

show_help() {
    cat << EOF
AlphaDrive Installer

Usage:
  install.sh [options]

Options:
  --version VERSION       Target AlphaDrive release version (default: latest)
  --port PORT             Service port (default: 8080)
  --bind IP               Bind IP address (default: 0.0.0.0 or 127.0.0.1)
  --domain DOMAIN         Custom domain for HTTPS (e.g. drive.example.com)
  --proxy PROXY           Reverse proxy: caddy | nginx | traefik | none (default: caddy)
  --no-proxy              Disable reverse proxy configuration
  --access-mode MODE      1=VPS IP+Port, 2=Custom Domain+HTTPS, 3=Local/LAN
  --non-interactive, -y   Run without interactive prompts (for automation)
  --debug                 Enable verbose debug output
  -h, --help              Show this help menu

Examples:
  # Standard interactive installation
  curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash

  # Automated IP + port installation
  curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --port 8080

  # Automated domain installation with Caddy
  curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --domain drive.example.com --proxy caddy
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --version)
                VERSION="$2"; shift 2 ;;
            --version=*)
                VERSION="${1#*=}"; shift ;;
            --port)
                PORT="$2"; shift 2 ;;
            --port=*)
                PORT="${1#*=}"; shift ;;
            --bind)
                BIND_IP="$2"; shift 2 ;;
            --bind=*)
                BIND_IP="${1#*=}"; shift ;;
            --domain)
                DOMAIN="$2"; shift 2 ;;
            --domain=*)
                DOMAIN="${1#*=}"; shift ;;
            --proxy)
                PROXY="$2"; shift 2 ;;
            --proxy=*)
                PROXY="${1#*=}"; shift ;;
            --no-proxy)
                PROXY="none"; shift ;;
            --access-mode)
                ACCESS_MODE="$2"; shift 2 ;;
            --access-mode=*)
                ACCESS_MODE="${1#*=}"; shift ;;
            --non-interactive|-y|--yes)
                NON_INTERACTIVE=true; shift ;;
            --debug)
                DEBUG=true; set -x; shift ;;
            -h|--help)
                show_help; exit 0 ;;
            *)
                fatal "Unknown option: $1 (run with --help for available options)" ;;
        esac
    done
}

check_privileges() {
    if [ "$(id -u)" -ne 0 ]; then
        if command -v sudo >/dev/null 2>&1; then
            info "Elevation required: re-running with sudo..."
            exec sudo -E bash "$0" "$@"
        else
            fatal "This installer requires root privileges. Please run as root or install sudo."
        fi
    fi
}

check_system() {
    # 1. OS check
    if [ ! -f /etc/os-release ]; then
        fatal "Unsupported operating system. /etc/os-release not found. AlphaDrive supports Linux (Debian, Ubuntu, and compatible)."
    fi
    # shellcheck disable=SC1091
    . /etc/os-release
    OS_NAME="${PRETTY_NAME:-$NAME}"

    # 2. Architecture check
    UNAME_M="$(uname -m)"
    case "${UNAME_M}" in
        x86_64|amd64)
            ARCH="amd64" ;;
        aarch64|arm64)
            ARCH="arm64" ;;
        *)
            fatal "Unsupported architecture: ${UNAME_M}\n\nAlphaDrive currently supports:\n- amd64 (x86_64)\n- arm64 (aarch64)\n\nInstallation cancelled." ;;
    esac

    # 3. Systemd check
    if ! command -v systemctl >/dev/null 2>&1 || [ ! -d /run/systemd/system ]; then
        fatal "systemd was not detected. AlphaDrive relies on systemd for process supervision.\nInstallation cannot proceed on non-systemd environments."
    fi

    # 4. RAM check
    if [ -f /proc/meminfo ]; then
        TOTAL_RAM_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
        RAM_MB=$((TOTAL_RAM_KB / 1024))
        if [ "$RAM_MB" -ge 1024 ]; then
            RAM_INFO="$((RAM_MB / 1024)) GB"
        else
            RAM_INFO="${RAM_MB} MB"
        fi
    else
        RAM_INFO="Available"
    fi

    # 5. Disk check
    DISK_INFO="$(df -h / 2>/dev/null | awk 'NR==2 {print $4 " free"}' || echo 'Available')"

    # 6. Basic utilities check
    for util in curl tar grep sed awk; do
        if ! command -v "$util" >/dev/null 2>&1; then
            fatal "Required tool '${util}' is missing. Please install it using your system package manager."
        fi
    done
}

get_public_ip() {
    local ip=""
    for service in "https://api.ipify.org" "https://ifconfig.me" "https://icanhazip.com" "https://checkip.amazonaws.com"; do
        ip="$(curl -fsS --max-time 2 "$service" 2>/dev/null || true)"
        if [[ "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
            echo "$ip"
            return 0
        fi
    done
    echo ""
}

is_port_in_use() {
    local port="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -tuln | grep -qE "(:|\])${port}\s"
    elif command -v netstat >/dev/null 2>&1; then
        netstat -tuln | grep -qE "(:|\])${port}\s"
    elif command -v lsof >/dev/null 2>&1; then
        lsof -i ":${port}" >/dev/null 2>&1
    else
        return 1
    fi
}

get_process_on_port() {
    local port="$1"
    if command -v ss >/dev/null 2>&1; then
        ss -tulpn 2>/dev/null | grep -E "(:|\])${port}\s" | awk '{print $NF}' | head -n 1
    elif command -v lsof >/dev/null 2>&1; then
        lsof -i ":${port}" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1 " (PID: " $2 ")"}'
    else
        echo "Unknown process"
    fi
}

check_existing_install() {
    if [ -f "/opt/alphadrive/alphadrive" ] || systemctl list-unit-files alphadrive.service >/dev/null 2>&1; then
        if [ "${NON_INTERACTIVE}" = "true" ]; then
            UPGRADE_MODE=true
            return 0
        fi

        echo ""
        warn "An existing AlphaDrive installation was detected."
        echo ""
        echo "  1) Upgrade to latest release"
        echo "  2) Repair / Reconfigure"
        echo "  3) Cancel"
        echo ""
        read -r -p "Select [1-3] (default 1): " choice
        choice="${choice:-1}"
        case "$choice" in
            1)
                UPGRADE_MODE=true ;;
            2)
                UPGRADE_MODE=false ;;
            3|*)
                info "Installation cancelled by user."
                exit 0 ;;
        esac
    fi
}

resolve_version_and_download() {
    info "Resolving AlphaDrive release version..."
    if [ -z "${VERSION}" ]; then
        TAG="$(curl -fsSL --max-time 5 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)"
        if [ -z "${TAG}" ]; then
            TAG="v1.0.0"
        fi
    else
        if [[ "${VERSION}" != v* ]]; then
            TAG="v${VERSION}"
        else
            TAG="${VERSION}"
        fi
    fi

    BINARY_NAME="alphadrive-linux-${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY_NAME}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

    info "Downloading AlphaDrive ${TAG} for ${ARCH}..."
    TMP_BINARY="${TMP_DIR}/alphadrive"

    if ! curl -fsSL --progress-bar "$DOWNLOAD_URL" -o "$TMP_BINARY"; then
        fatal "Failed to download AlphaDrive binary from:\n${DOWNLOAD_URL}\nPlease verify that the release exists on GitHub."
    fi
    success "Download complete"

    # Checksum verification
    TMP_CHECKSUMS="${TMP_DIR}/checksums.txt"
    if curl -fsSL --max-time 5 "$CHECKSUMS_URL" -o "$TMP_CHECKSUMS" 2>/dev/null; then
        info "Verifying SHA256 checksum..."
        EXPECTED_HASH="$(grep "${BINARY_NAME}" "${TMP_CHECKSUMS}" | awk '{print $1}' || true)"
        if [ -n "${EXPECTED_HASH}" ]; then
            ACTUAL_HASH="$(sha256sum "${TMP_BINARY}" | awk '{print $1}')"
            if [ "${EXPECTED_HASH}" != "${ACTUAL_HASH}" ]; then
                rm -f "${TMP_BINARY}"
                fatal "Checksum verification failed!\nExpected: ${EXPECTED_HASH}\nActual:   ${ACTUAL_HASH}\nInstallation aborted for security."
            fi
            success "SHA256 checksum verified (${ACTUAL_HASH:0:12}...)"
        fi
    fi

    chmod +x "${TMP_BINARY}"
}

run_wizard() {
    if [ "${NON_INTERACTIVE}" = "true" ]; then
        ACCESS_MODE="${ACCESS_MODE:-1}"
        PORT="${PORT:-$DEFAULT_PORT}"
        if [ "$ACCESS_MODE" = "2" ] && [ -z "$DOMAIN" ]; then
            fatal "--domain is required when --access-mode=2 in non-interactive mode"
        fi
        PROXY="${PROXY:-caddy}"
        BIND_IP="${BIND_IP:-0.0.0.0}"
        if [ "$ACCESS_MODE" = "2" ]; then
            BIND_IP="${BIND_IP:-127.0.0.1}"
        fi
        return 0
    fi

    cat << "EOF"
------------------------------------------------------------
                    AlphaDrive Installer
              Lightweight. Self-hosted. Yours.
------------------------------------------------------------
EOF
    success "OS: ${OS_NAME}"
    success "Architecture: ${ARCH}"
    success "RAM: ${RAM_INFO}"
    success "Disk: ${DISK_INFO}"
    success "systemd: available"
    echo "------------------------------------------------------------"
    echo ""

    if [ -z "$ACCESS_MODE" ]; then
        echo "How would you like to access AlphaDrive?"
        echo ""
        echo "  1) VPS IP + Port (Default, simplest setup)"
        echo "  2) Custom Domain + HTTPS (Automatic SSL via Reverse Proxy)"
        echo "  3) Local / LAN (Local network deployment)"
        echo ""
        read -r -p "Select [1-3] (default 1): " access_choice
        access_choice="${access_choice:-1}"
        case "$access_choice" in
            1) ACCESS_MODE=1 ;;
            2) ACCESS_MODE=2 ;;
            3) ACCESS_MODE=3 ;;
            *) ACCESS_MODE=1 ;;
        esac
    fi

    # Access Mode 1 & 3: IP + Port / LAN
    if [ "$ACCESS_MODE" = "1" ] || [ "$ACCESS_MODE" = "3" ]; then
        if [ -z "$PORT" ]; then
            while true; do
                read -r -p "Enter AlphaDrive port [${DEFAULT_PORT}]: " input_port
                input_port="${input_port:-$DEFAULT_PORT}"
                if ! [[ "$input_port" =~ ^[0-9]+$ ]] || [ "$input_port" -lt 1 ] || [ "$input_port" -gt 65535 ]; then
                    error "Invalid port number. Please enter a port between 1 and 65535."
                    continue
                fi
                if is_port_in_use "$input_port"; then
                    proc_info="$(get_process_on_port "$input_port")"
                    warn "Port ${input_port} is already in use (${proc_info}). Please select another port."
                    continue
                fi
                PORT="$input_port"
                break
            done
        fi
        BIND_IP="0.0.0.0"
        PROXY="none"

        echo ""
        warn "WARNING:"
        echo "AlphaDrive is currently configured to be served over plain HTTP."
        echo "For public internet deployments, HTTPS is recommended so sessions"
        echo "and file transfers are protected in transit."
        echo ""

    # Access Mode 2: Custom Domain + HTTPS
    elif [ "$ACCESS_MODE" = "2" ]; then
        if [ -z "$DOMAIN" ]; then
            while true; do
                read -r -p "Enter your domain (e.g. drive.example.com): " input_domain
                input_domain="$(echo "$input_domain" | tr -d ' ' | tr '[:upper:]' '[:lower:]')"
                if [[ ! "$input_domain" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]; then
                    error "Invalid domain format. Please enter a valid FQDN (e.g. drive.example.com)."
                    continue
                fi
                DOMAIN="$input_domain"
                break
            done
        fi

        # Domain DNS check
        SERVER_IP="$(get_public_ip)"
        if [ -n "$SERVER_IP" ]; then
            DOMAIN_IP="$(getent hosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | head -n 1 || true)"
            if [ -n "$DOMAIN_IP" ] && [ "$DOMAIN_IP" != "$SERVER_IP" ]; then
                echo ""
                warn "WARNING:"
                echo "${DOMAIN} currently resolves to ${DOMAIN_IP}, but this server's public IP is ${SERVER_IP}."
                echo "Automatic HTTPS certificate provisioning will fail until your DNS A record points to ${SERVER_IP}."
                echo ""
                read -r -p "Continue anyway? [y/N]: " dns_confirm
                if [[ ! "$dns_confirm" =~ ^[yY](es)?$ ]]; then
                    info "Please update your DNS records and re-run the installer."
                    exit 0
                fi
            fi
        fi

        if [ -z "$PROXY" ]; then
            echo ""
            echo "Choose a reverse proxy for HTTPS:"
            echo ""
            echo "  1) Caddy (Recommended - Automatic TLS & Zero-Config)"
            echo "  2) Nginx (Standard reverse proxy with Certbot SSL)"
            echo "  3) Traefik (Manual configuration guidance)"
            echo "  0) None / Manual configuration"
            echo ""
            read -r -p "Select [0-3] (default 1): " proxy_choice
            proxy_choice="${proxy_choice:-1}"
            case "$proxy_choice" in
                1) PROXY="caddy" ;;
                2) PROXY="nginx" ;;
                3) PROXY="traefik" ;;
                0) PROXY="none" ;;
                *) PROXY="caddy" ;;
            esac
        fi

        PORT="${PORT:-$DEFAULT_PORT}"
        BIND_IP="127.0.0.1"
    fi
}

setup_system() {
    info "Configuring system user and directories..."

    # Pre-upgrade backup if upgrading
    if [ "${UPGRADE_MODE}" = "true" ] && [ -f /opt/alphadrive/alphadrive ] && [ -d /var/lib/alphadrive/data ]; then
        info "Creating atomic pre-upgrade backup..."
        BACKUP_OUT="/var/backups/alphadrive-preupgrade-$(date +%Y%m%d_%H%M%S).tar.gz"
        if sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /opt/alphadrive/alphadrive backup --output "${BACKUP_OUT}" >/dev/null 2>&1; then
            success "Pre-upgrade backup saved to ${BACKUP_OUT}"
        else
            warn "Unable to create native pre-upgrade backup. Proceeding with file copy..."
        fi
    fi

    # Create dedicated unprivileged system user
    if ! id -u alphadrive >/dev/null 2>&1; then
        useradd -r -s /usr/sbin/nologin -d /var/lib/alphadrive -M alphadrive
        success "Created unprivileged system user 'alphadrive'"
    fi

    # Create directories
    mkdir -p /opt/alphadrive
    mkdir -p /etc/alphadrive
    mkdir -p /var/lib/alphadrive/data
    mkdir -p /var/log/alphadrive
    mkdir -p /var/backups

    chown -R alphadrive:alphadrive /var/lib/alphadrive /var/log/alphadrive
    chmod 700 /var/lib/alphadrive /var/lib/alphadrive/data
    chmod 750 /var/log/alphadrive
    chmod 750 /var/backups

    # Stop service if running
    systemctl stop alphadrive 2>/dev/null || true

    # Install binary
    install -m 755 "${TMP_BINARY}" /opt/alphadrive/alphadrive
    ln -sf /opt/alphadrive/alphadrive /usr/local/bin/alphadrive
    success "Installed AlphaDrive binary to /opt/alphadrive/alphadrive"

    # Determine Base URL
    if [ "$ACCESS_MODE" = "2" ] && [ -n "$DOMAIN" ]; then
        PUBLIC_URL="https://${DOMAIN}"
        INSECURE_COOKIES="false"
    else
        SERVER_IP="$(get_public_ip)"
        if [ -z "$SERVER_IP" ]; then
            SERVER_IP="127.0.0.1"
        fi
        PUBLIC_URL="http://${SERVER_IP}:${PORT}"
        INSECURE_COOKIES="true"
    fi

    # Write /etc/alphadrive/alphadrive.env
    if [ ! -f /etc/alphadrive/alphadrive.env ] || [ "${UPGRADE_MODE}" = "false" ]; then
        cat > /etc/alphadrive/alphadrive.env << EOF
# AlphaDrive Environment Configuration
ALPHADRIVE_LISTEN_ADDRESS=${BIND_IP}:${PORT}
ALPHADRIVE_PUBLIC_BASE_URL=${PUBLIC_URL}
ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data
ALPHADRIVE_MAX_UPLOAD_BYTES=0
ALPHADRIVE_STORAGE_QUOTA_BYTES=0
ALPHADRIVE_INSECURE_COOKIES=${INSECURE_COOKIES}
EOF
        chown root:alphadrive /etc/alphadrive/alphadrive.env
        chmod 640 /etc/alphadrive/alphadrive.env
        success "Configuration written to /etc/alphadrive/alphadrive.env"
    fi

    # Write systemd service unit
    cat > /etc/systemd/system/alphadrive.service << 'EOF'
[Unit]
Description=AlphaDrive Cloud Storage Daemon
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=alphadrive
Group=alphadrive
WorkingDirectory=/var/lib/alphadrive
ExecStart=/opt/alphadrive/alphadrive serve
Restart=always
RestartSec=5s

LimitNOFILE=65536
LimitNPROC=4096

EnvironmentFile=/etc/alphadrive/alphadrive.env

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictRealtime=true
LockPersonality=true
MemoryDenyWriteExecute=true

ReadWritePaths=/var/lib/alphadrive /var/log/alphadrive

StandardOutput=journal
StandardError=journal
SyslogIdentifier=alphadrive

[Install]
WantedBy=multi-user.target
EOF

    chmod 644 /etc/systemd/system/alphadrive.service
    systemctl daemon-reload
    systemctl enable alphadrive >/dev/null 2>&1
    systemctl restart alphadrive
    success "Configured and started systemd service 'alphadrive'"
}

configure_reverse_proxy() {
    case "$PROXY" in
        caddy)
            info "Configuring Caddy reverse proxy for ${DOMAIN}..."
            if ! command -v caddy >/dev/null 2>&1; then
                info "Installing Caddy web server..."
                apt-get update -qq && apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl >/dev/null 2>&1 || true
                curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null || true
                curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null 2>&1 || true
                apt-get update -qq && apt-get install -y -qq caddy >/dev/null 2>&1 || true
            fi

            mkdir -p /etc/caddy
            CADDYFILE="/etc/caddy/Caddyfile"
            if [ -f "$CADDYFILE" ]; then
                cp "$CADDYFILE" "${CADDYFILE}.bak.$(date +%s)"
                if ! grep -q "${DOMAIN}" "$CADDYFILE"; then
                    cat >> "$CADDYFILE" << EOF

${DOMAIN} {
    reverse_proxy 127.0.0.1:${PORT}
}
EOF
                fi
            else
                cat > "$CADDYFILE" << EOF
${DOMAIN} {
    reverse_proxy 127.0.0.1:${PORT}
}
EOF
            fi

            if caddy validate --config "$CADDYFILE" >/dev/null 2>&1; then
                systemctl enable caddy >/dev/null 2>&1 || true
                systemctl reload caddy >/dev/null 2>&1 || systemctl restart caddy >/dev/null 2>&1 || true
                success "Caddy configured and reloaded for ${DOMAIN}"
            else
                warn "Caddy syntax check failed. Please verify /etc/caddy/Caddyfile"
            fi
            ;;

        nginx)
            info "Configuring Nginx reverse proxy for ${DOMAIN}..."
            if ! command -v nginx >/dev/null 2>&1; then
                apt-get update -qq && apt-get install -y -qq nginx >/dev/null 2>&1 || true
            fi

            mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
            NGINX_CONF="/etc/nginx/sites-available/alphadrive.conf"
            cat > "$NGINX_CONF" << EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    client_max_body_size 0;
    client_body_timeout 300s;
    proxy_read_timeout 300s;
    proxy_send_timeout 300s;
    proxy_request_buffering off;
    proxy_buffering off;

    location / {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
    }
}
EOF
            ln -sf "$NGINX_CONF" /etc/nginx/sites-enabled/alphadrive.conf
            if nginx -t >/dev/null 2>&1; then
                systemctl enable nginx >/dev/null 2>&1 || true
                systemctl reload nginx >/dev/null 2>&1 || true
                success "Nginx reverse proxy configured and active"
                if command -v certbot >/dev/null 2>&1; then
                    info "Obtaining Let's Encrypt TLS certificate via Certbot..."
                    certbot --nginx -d "${DOMAIN}" --non-interactive --agree-tos --register-unsafely-without-email || true
                fi
            else
                warn "Nginx syntax test failed. Please check /etc/nginx/sites-available/alphadrive.conf"
            fi
            ;;

        traefik)
            echo ""
            info "Traefik Dynamic Configuration Sample:"
            cat << EOF
http:
  routers:
    alphadrive:
      rule: "Host(\`${DOMAIN}\`)"
      entryPoints: ["websecure"]
      service: "alphadrive"
      tls:
        certResolver: "letsencrypt"
  services:
    alphadrive:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:${PORT}"
EOF
            echo ""
            info "AlphaDrive daemon is listening on 127.0.0.1:${PORT}."
            ;;

        none)
            debug "No reverse proxy configured"
            ;;
    esac
}

verify_health() {
    info "Verifying AlphaDrive health check..."
    local HEALTH_URL="http://127.0.0.1:${PORT}/healthz"
    local ATTEMPTS=0
    local MAX_ATTEMPTS=15

    while [ "$ATTEMPTS" -lt "$MAX_ATTEMPTS" ]; do
        if curl -fsS --max-time 2 "$HEALTH_URL" >/dev/null 2>&1; then
            success "AlphaDrive daemon is healthy and responding (HTTP 200)"
            return 0
        fi
        sleep 1
        ATTEMPTS=$((ATTEMPTS + 1))
    done

    error "Health check failed after ${MAX_ATTEMPTS} seconds!"
    echo ""
    error "Recent service journal output:"
    journalctl -u alphadrive -n 25 --no-pager || true
    echo ""
    fatal "AlphaDrive daemon failed to start properly. Check logs above."
}

install_helpers() {
    # 1. Update utility
    cat > /usr/local/bin/alphadrive-update << 'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ "$(id -u)" -ne 0 ]; then
    exec sudo -E bash "$0" "$@"
fi
echo "Updating AlphaDrive to latest version..."
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | bash -s -- --non-interactive "$@"
EOF
    chmod 755 /usr/local/bin/alphadrive-update

    # 2. Uninstaller utility
    cat > /usr/local/bin/alphadrive-uninstall << 'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ "$(id -u)" -ne 0 ]; then
    exec sudo -E bash "$0" "$@"
fi

echo "============================================================"
echo "                AlphaDrive Uninstaller"
echo "============================================================"
echo ""
read -r -p "Are you sure you want to uninstall AlphaDrive? [y/N]: " confirm
if [[ ! "$confirm" =~ ^[yY](es)?$ ]]; then
    echo "Uninstall cancelled."
    exit 0
fi

echo "Stopping and disabling systemd service..."
systemctl stop alphadrive 2>/dev/null || true
systemctl disable alphadrive 2>/dev/null || true
rm -f /etc/systemd/system/alphadrive.service
systemctl daemon-reload

echo "Removing binary and configuration files..."
rm -rf /opt/alphadrive /etc/alphadrive /usr/local/bin/alphadrive /usr/local/bin/alphadrive-update /usr/local/bin/alphadrive-uninstall

echo ""
echo "Do you want to permanently delete user data and database in /var/lib/alphadrive?"
read -r -p "DELETE ALL USER DATA? [y/N]: " delete_data
if [[ "$delete_data" =~ ^[yY](es)?$ ]]; then
    rm -rf /var/lib/alphadrive /var/log/alphadrive
    echo "User data deleted."
else
    echo "User data preserved in /var/lib/alphadrive."
fi

echo ""
echo "AlphaDrive has been uninstalled."
EOF
    chmod 755 /usr/local/bin/alphadrive-uninstall
}

check_firewall() {
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
        if [ "$ACCESS_MODE" = "1" ] || [ "$ACCESS_MODE" = "3" ]; then
            if ! ufw status | grep -qE "${PORT}/tcp"; then
                warn "UFW firewall is active. Port ${PORT}/tcp may be blocked."
                if [ "${NON_INTERACTIVE}" = "true" ]; then
                    ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
                    success "Allowed port ${PORT}/tcp in UFW"
                else
                    read -r -p "Allow port ${PORT}/tcp in UFW firewall? [Y/n]: " ufw_choice
                    if [[ ! "$ufw_choice" =~ ^[nN] ]]; then
                        ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
                        success "Allowed port ${PORT}/tcp in UFW"
                    fi
                fi
            fi
        elif [ "$ACCESS_MODE" = "2" ]; then
            for p in 80 443; do
                if ! ufw status | grep -qE "${p}/tcp"; then
                    ufw allow "${p}/tcp" >/dev/null 2>&1 || true
                fi
            done
            success "Ensured ports 80/443 are open in UFW for HTTPS"
        fi
    fi
}

print_completion() {
    local DB_FILE="/var/lib/alphadrive/data/alphadrive.db"
    local IS_NEW_INSTALL=true
    if [ -f "$DB_FILE" ] && [ -s "$DB_FILE" ]; then
        IS_NEW_INSTALL=false
    fi

    echo ""
    echo "============================================================"
    printf "                  ${GREEN}${BOLD}AlphaDrive is ready!${NC}\n"
    echo "============================================================"
    echo ""
    echo "Access AlphaDrive at:"
    printf "  ${CYAN}${BOLD}%s${NC}\n" "${PUBLIC_URL}"
    echo ""

    if [ "$IS_NEW_INSTALL" = "true" ]; then
        echo "First-time Owner Setup:"
        printf "  ${GREEN}${BOLD}%s/setup${NC}\n" "${PUBLIC_URL}"
        echo ""
    fi

    echo "Useful Management Commands:"
    echo "  systemctl status alphadrive   # View service status"
    echo "  systemctl restart alphadrive  # Restart service"
    echo "  journalctl -u alphadrive -f   # View live logs"
    echo "  alphadrive doctor             # Run deep diagnostics"
    echo "  alphadrive backup             # Create atomic backup"
    echo "  alphadrive-update             # Upgrade to latest release"
    echo "  alphadrive-uninstall          # Uninstall AlphaDrive"
    echo ""
    echo "Wiki & Documentation:"
    echo "  https://github.com/AlphaTechiess/alphadrive/wiki"
    echo "============================================================"
    echo ""
}

main() {
    parse_args "$@"
    check_privileges "$@"
    check_system
    check_existing_install
    resolve_version_and_download
    run_wizard
    setup_system
    configure_reverse_proxy
    verify_health
    install_helpers
    check_firewall
    print_completion
}

main "$@"