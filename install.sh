#!/usr/bin/env bash
# ==============================================================================
# AlphaDrive - Production-Grade One-Line Installer
# Repository: https://github.com/AlphaTechiess/alphadrive
# License: MIT
# ==============================================================================

set -Eeuo pipefail

# Reopen stdin from /dev/tty if running in a pipe with an active terminal
if [ ! -t 0 ] && (exec < /dev/tty) 2>/dev/null; then
    exec < /dev/tty
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

info()    { printf '%bℹ%b %s\n' "$BLUE" "$NC" "$*"; }
success() { printf '%b✓%b %s\n' "$GREEN" "$NC" "$*"; }
warn()    { printf '%b⚠%b %s\n' "$YELLOW" "$NC" "$*"; }
error()   { printf '%b✗%b %s\n' "$RED" "$NC" "$*" >&2; }
debug()   { if [ "${DEBUG:-false}" = "true" ]; then printf '%b[DEBUG]%b %s\n' "$MAGENTA" "$NC" "$*" >&2; fi; }
fatal()   { error "$*"; exit 1; }

# Global constants & variables
REPO="AlphaTechiess/alphadrive"
DEFAULT_PORT=8080
TARGET_VERSION=""
PORT=""
BIND_IP=""
DOMAIN=""
PROXY=""
ACCESS_MODE=""
NON_INTERACTIVE=false
DEBUG=false
UPGRADE_MODE=false
REPAIR_MODE=false
TMP_DIR="$(mktemp -d /tmp/alphadrive-install.XXXXXX)"
TMP_BINARY=""
RESOLVED_TAG=""
PUBLIC_URL=""
INSECURE_COOKIES="true"
HEALTH_CHECK_IP="127.0.0.1"

# System detected properties
ARCH=""
OS_NAME=""
RAM_INFO=""
DISK_INFO=""

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
  --version VERSION       Target AlphaDrive release version (default: latest stable)
  --port PORT             Service port (default: 8080)
  --bind IP               Bind IP address (default: 0.0.0.0 for IP mode, 127.0.0.1 for domain mode)
  --domain DOMAIN         Custom domain for HTTPS (e.g. drive.example.com)
  --proxy PROXY           Reverse proxy: caddy | nginx | traefik | none (default: caddy for domain mode)
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

  # Install specific version
  curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --version 1.0.1
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --version)
                TARGET_VERSION="$2"; shift 2 ;;
            --version=*)
                TARGET_VERSION="${1#*=}"; shift ;;
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

validate_cli_args() {
    if [ -n "$PORT" ]; then
        if ! [[ "$PORT" =~ ^[0-9]+$ ]] || [ "$PORT" -lt 1 ] || [ "$PORT" -gt 65535 ]; then
            fatal "Invalid port '${PORT}'. Port must be a number between 1 and 65535."
        fi
    fi

    if [ -n "$ACCESS_MODE" ]; then
        if ! [[ "$ACCESS_MODE" =~ ^[1-3]$ ]]; then
            fatal "Invalid access mode '${ACCESS_MODE}'. Must be 1 (IP), 2 (Domain), or 3 (LAN)."
        fi
    fi

    if [ -n "$PROXY" ]; then
        case "$PROXY" in
            caddy|nginx|traefik|none) ;;
            *) fatal "Invalid proxy '${PROXY}'. Supported options: caddy | nginx | traefik | none" ;;
        esac
    fi

    if [ -n "$DOMAIN" ]; then
        DOMAIN="$(echo "$DOMAIN" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | tr '[:upper:]' '[:lower:]')"
        if [[ ! "$DOMAIN" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]; then
            fatal "Invalid domain format '${DOMAIN}'. Please provide a valid FQDN (e.g. drive.example.com)."
        fi
    fi

    if [ -n "$TARGET_VERSION" ]; then
        if [[ ! "$TARGET_VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
            fatal "Invalid version '${TARGET_VERSION}'. Expected semver format (e.g. 1.0.1 or v1.0.1)."
        fi
    fi

    if [ "${NON_INTERACTIVE}" = "true" ]; then
        if [ "$ACCESS_MODE" = "2" ] && [ -z "$DOMAIN" ]; then
            fatal "--domain is required when --access-mode=2 in non-interactive mode"
        fi
    fi
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
    printf '\n%b[1/7] Checking system environment...%b\n' "$BOLD" "$NC"

    # 1. OS check
    if [ ! -f /etc/os-release ]; then
        fatal "Unsupported operating system: /etc/os-release not found. AlphaDrive requires Linux (Debian, Ubuntu, RHEL, Rocky, AlmaLinux, Arch, or compatible)."
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
            fatal "Required utility '${util}' is missing. Please install it using your system package manager."
        fi
    done

    # 7. Checksum tool check
    if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
        fatal "A SHA256 verification utility ('sha256sum' or 'shasum') is required for secure installation."
    fi

    success "OS: ${OS_NAME}"
    success "Architecture: ${ARCH}"
    success "RAM: ${RAM_INFO}"
    success "Disk: ${DISK_INFO}"
    success "systemd: available"
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
    if [ -f "/opt/alphadrive/alphadrive" ] || systemctl list-unit-files alphadrive.service >/dev/null 2>&1 || [ -f "/etc/alphadrive/alphadrive.env" ]; then
        if [ "${NON_INTERACTIVE}" = "true" ]; then
            UPGRADE_MODE=true
            return 0
        fi

        echo ""
        warn "An existing AlphaDrive installation was detected."
        echo ""
        echo "  1) Upgrade to latest release (preserves data and config)"
        echo "  2) Repair / Reconfigure (modify settings or reinstall files)"
        echo "  3) Cancel"
        echo ""
        read -r -p "Select [1-3] (default 1): " choice
        choice="${choice:-1}"
        case "$choice" in
            1)
                UPGRADE_MODE=true ;;
            2)
                UPGRADE_MODE=false
                REPAIR_MODE=true ;;
            3|*)
                info "Installation cancelled by user."
                exit 0 ;;
        esac
    fi
}

resolve_version() {
    if [ -n "${TARGET_VERSION}" ]; then
        if [[ "${TARGET_VERSION}" != v* ]]; then
            RESOLVED_TAG="v${TARGET_VERSION}"
        else
            RESOLVED_TAG="${TARGET_VERSION}"
        fi
        info "Target release version specified: ${RESOLVED_TAG}"
        return 0
    fi

    info "Discovering latest stable AlphaDrive release..."
    # 1. Try GitHub API
    local api_tag=""
    api_tag="$(curl -fsSL --max-time 5 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)"

    # 2. Fallback to GitHub Release redirect location header (works without API rate limits)
    if [ -z "${api_tag}" ]; then
        api_tag="$(curl -fsSI --max-time 5 "https://github.com/${REPO}/releases/latest" 2>/dev/null | grep -i '^location:' | sed -E 's/.*tag\/(.*)/\1/' | tr -d '\r\n ' || true)"
    fi

    if [ -n "${api_tag}" ]; then
        RESOLVED_TAG="${api_tag}"
        success "Discovered latest release: ${RESOLVED_TAG}"
    else
        fatal "Unable to determine the latest stable AlphaDrive release.\nPlease specify a version explicitly with --version VERSION (e.g. --version 1.0.1) or try again later."
    fi
}

download_and_verify() {
    printf '\n%b[3/7] Downloading release & verifying checksums...%b\n' "$BOLD" "$NC"
    local binary_name="alphadrive-linux-${ARCH}"
    local download_url="https://github.com/${REPO}/releases/download/${RESOLVED_TAG}/${binary_name}"
    local checksums_url="https://github.com/${REPO}/releases/download/${RESOLVED_TAG}/checksums.txt"

    info "Downloading AlphaDrive release binary (${binary_name})..."
    TMP_BINARY="${TMP_DIR}/alphadrive"

    if ! curl -fsSL --progress-bar "$download_url" -o "$TMP_BINARY"; then
        rm -f "$TMP_BINARY"
        fatal "Failed to download AlphaDrive binary from:\n${download_url}\nPlease verify that release '${RESOLVED_TAG}' and artifact '${binary_name}' exist."
    fi
    success "Binary download complete"

    # Mandatory Checksum Verification
    info "Downloading release checksums (checksums.txt)..."
    local tmp_checksums="${TMP_DIR}/checksums.txt"
    if ! curl -fsSL --max-time 10 "$checksums_url" -o "$tmp_checksums"; then
        rm -f "$TMP_BINARY" "$tmp_checksums"
        fatal "MANDATORY Checksum verification failed: Unable to download checksums.txt from:\n${checksums_url}\nInstallation aborted for security."
    fi

    info "Verifying SHA256 checksum..."
    local expected_hash=""
    expected_hash="$(grep -E "(^|[[:space:]])${binary_name}($|[[:space:]])" "${tmp_checksums}" | awk '{print $1}' | tr '[:upper:]' '[:lower:]' | head -n 1 || true)"

    if [ -z "${expected_hash}" ] || [ "${#expected_hash}" -ne 64 ]; then
        rm -f "$TMP_BINARY" "$tmp_checksums"
        fatal "Checksum for '${binary_name}' was not found in checksums.txt.\nInstallation aborted for security."
    fi

    local actual_hash=""
    if command -v sha256sum >/dev/null 2>&1; then
        actual_hash="$(sha256sum "${TMP_BINARY}" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
    elif command -v shasum >/dev/null 2>&1; then
        actual_hash="$(shasum -a 256 "${TMP_BINARY}" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
    fi

    if [ "${expected_hash}" != "${actual_hash}" ]; then
        rm -f "$TMP_BINARY" "$tmp_checksums"
        fatal "SECURITY ALERT: Checksum verification failed!\nExpected: ${expected_hash}\nActual:   ${actual_hash}\nThe downloaded artifact may be corrupt or tampered with.\nInstallation aborted."
    fi

    success "SHA256 checksum verified: ${actual_hash:0:16}..."
    chmod +x "${TMP_BINARY}"
}

run_wizard() {
    printf '\n%b[2/7] Selecting access mode & networking...%b\n' "$BOLD" "$NC"

    # Non-interactive argument validation
    if [ "${NON_INTERACTIVE}" = "true" ]; then
        ACCESS_MODE="${ACCESS_MODE:-1}"
        PORT="${PORT:-$DEFAULT_PORT}"
        if [ "$ACCESS_MODE" = "2" ] && [ -z "$DOMAIN" ]; then
            fatal "--domain is required when --access-mode=2 in non-interactive mode"
        fi
        PROXY="${PROXY:-caddy}"
        if [ "$ACCESS_MODE" = "2" ]; then
            BIND_IP="${BIND_IP:-127.0.0.1}"
        else
            BIND_IP="${BIND_IP:-0.0.0.0}"
            PROXY="none"
        fi
        return 0
    fi

    if [ "${REPAIR_MODE}" = "true" ] && [ -f "/etc/alphadrive/alphadrive.env" ]; then
        # Load existing config for default suggestions
        # shellcheck disable=SC1091
        EXISTING_LISTEN="$(grep '^ALPHADRIVE_LISTEN_ADDRESS=' /etc/alphadrive/alphadrive.env | cut -d'=' -f2 || true)"
        EXISTING_URL="$(grep '^ALPHADRIVE_PUBLIC_BASE_URL=' /etc/alphadrive/alphadrive.env | cut -d'=' -f2 || true)"
        info "Current configuration detected: Listen=${EXISTING_LISTEN:-unknown}, URL=${EXISTING_URL:-unknown}"
        read -r -p "Do you want to reconfigure network and access settings? [y/N]: " reconf_choice
        if [[ ! "$reconf_choice" =~ ^[yY](es)?$ ]]; then
            info "Preserving current configuration."
            return 0
        fi
    fi

    if [ -z "$ACCESS_MODE" ]; then
        cat << "EOF"
How would you like to access AlphaDrive?

  1) VPS IP + Port (Default, simplest standalone setup)
  2) Custom Domain + HTTPS (Automatic SSL via Reverse Proxy)
  3) Local / LAN (Private network deployment)

EOF
        read -r -p "Select [1-3] (default 1): " access_choice
        access_choice="${access_choice:-1}"
        case "$access_choice" in
            1) ACCESS_MODE=1 ;;
            2) ACCESS_MODE=2 ;;
            3) ACCESS_MODE=3 ;;
            *) ACCESS_MODE=1 ;;
        esac
    fi

    # Mode 1 & Mode 3: Direct IP / LAN
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
        BIND_IP="${BIND_IP:-0.0.0.0}"
        PROXY="none"

        if [ "$ACCESS_MODE" = "1" ]; then
            echo ""
            warn "WARNING:"
            echo "AlphaDrive will be served directly over plain HTTP on port ${PORT}."
            echo "For public internet deployments, HTTPS is recommended to protect"
            echo "credentials and file transfers in transit."
            echo ""
        fi

    # Mode 2: Custom Domain + HTTPS
    elif [ "$ACCESS_MODE" = "2" ]; then
        if [ -z "$DOMAIN" ]; then
            while true; do
                read -r -p "Enter your domain (e.g. drive.example.com): " input_domain
                input_domain="$(echo "$input_domain" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | tr '[:upper:]' '[:lower:]')"
                if [[ ! "$input_domain" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]; then
                    error "Invalid domain format. Please enter a valid FQDN (e.g. drive.example.com)."
                    continue
                fi
                DOMAIN="$input_domain"
                break
            done
        fi

        # DNS Check
        local server_ip
        server_ip="$(get_public_ip)"
        if [ -n "$server_ip" ]; then
            local domain_ip=""
            domain_ip="$(getent hosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | head -n 1 || true)"
            if [ -n "$domain_ip" ] && [ "$domain_ip" != "$server_ip" ]; then
                echo ""
                warn "WARNING:"
                echo "'${DOMAIN}' currently resolves to ${domain_ip}, but this server's public IP is ${server_ip}."
                echo "Automatic HTTPS certificate provisioning will fail until your DNS A record points to ${server_ip}."
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
            echo "  3) Traefik (Generate configuration file)"
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
        BIND_IP="${BIND_IP:-127.0.0.1}"
    fi
}

setup_system() {
    printf '\n%b[4/7] Installing AlphaDrive binary & data paths...%b\n' "$BOLD" "$NC"

    # Pre-upgrade backup if upgrading
    if [ "${UPGRADE_MODE}" = "true" ] && [ -f /opt/alphadrive/alphadrive ] && [ -d /var/lib/alphadrive/data ]; then
        info "Creating pre-upgrade atomic backup..."
        mkdir -p /var/backups
        local backup_file
        backup_file="/var/backups/alphadrive-preupgrade-$(date +%Y%m%d_%H%M%S).tar.gz"
        local backup_out
        if backup_out="$(ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /opt/alphadrive/alphadrive backup --output "${backup_file}" 2>&1)" && [ -s "${backup_file}" ]; then
            chmod 600 "${backup_file}"
            success "Pre-upgrade backup created at: ${backup_file}"
        else
            warn "Pre-upgrade backup command output:"
            printf '%s\n' "$backup_out" >&2
            fatal "Pre-upgrade backup failed! Upgrade cancelled.\nExisting AlphaDrive installation has not been modified."
        fi

        # Backup existing binary for rollback
        cp -p /opt/alphadrive/alphadrive /opt/alphadrive/alphadrive.bak.upgrade
    fi

    # Create dedicated unprivileged system user
    if ! id -u alphadrive >/dev/null 2>&1; then
        if command -v useradd >/dev/null 2>&1; then
            useradd -r -s /usr/sbin/nologin -d /var/lib/alphadrive -M alphadrive 2>/dev/null || useradd -r -s /bin/false -d /var/lib/alphadrive -M alphadrive
        elif command -v adduser >/dev/null 2>&1; then
            adduser -S -D -H -s /sbin/nologin alphadrive 2>/dev/null || true
        fi
        success "Created unprivileged system user 'alphadrive'"
    fi

    # Create filesystem directory hierarchy
    mkdir -p /opt/alphadrive
    mkdir -p /etc/alphadrive
    mkdir -p /var/lib/alphadrive/data
    mkdir -p /var/log/alphadrive
    mkdir -p /var/backups

    chown -R alphadrive:alphadrive /var/lib/alphadrive /var/log/alphadrive
    chmod 700 /var/lib/alphadrive /var/lib/alphadrive/data
    chmod 750 /var/log/alphadrive
    chmod 750 /var/backups
    chmod 755 /opt/alphadrive

    # Stop service if running
    if systemctl is-active --quiet alphadrive 2>/dev/null; then
        info "Stopping running alphadrive service..."
        systemctl stop alphadrive
    fi

    # Install binary
    install -m 755 "${TMP_BINARY}" /opt/alphadrive/alphadrive
    ln -sf /opt/alphadrive/alphadrive /usr/local/bin/alphadrive
    success "Installed binary to /opt/alphadrive/alphadrive (symlinked to /usr/local/bin/alphadrive)"

    # Determine Base URL
    if [ "$ACCESS_MODE" = "2" ] && [ -n "$DOMAIN" ]; then
        PUBLIC_URL="https://${DOMAIN}"
        INSECURE_COOKIES="false"
        HEALTH_CHECK_IP="127.0.0.1"
    elif [ "$ACCESS_MODE" = "3" ]; then
        local lan_ip
        lan_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")"
        PUBLIC_URL="http://${lan_ip}:${PORT}"
        INSECURE_COOKIES="true"
        HEALTH_CHECK_IP="127.0.0.1"
    else
        local pub_ip
        pub_ip="$(get_public_ip)"
        if [ -n "$pub_ip" ]; then
            PUBLIC_URL="http://${pub_ip}:${PORT}"
        else
            PUBLIC_URL="http://YOUR-VPS-IP:${PORT}"
        fi
        INSECURE_COOKIES="true"
        HEALTH_CHECK_IP="127.0.0.1"
    fi

    info "Configuring environment & systemd service..."

    # Write /etc/alphadrive/alphadrive.env
    if [ ! -f /etc/alphadrive/alphadrive.env ] || [ "${UPGRADE_MODE}" = "false" ]; then
        cat > /etc/alphadrive/alphadrive.env << EOF
# AlphaDrive Runtime Environment Configuration
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
    else
        info "Preserved existing /etc/alphadrive/alphadrive.env"
    fi

    # Write hardened systemd service unit
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

# Sandboxing and security hardening
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
    success "Configured systemd service unit '/etc/systemd/system/alphadrive.service'"
}

configure_reverse_proxy() {
    printf '\n%b[5/7] Configuring networking & reverse proxy...%b\n' "$BOLD" "$NC"

    case "$PROXY" in
        caddy)
            info "Configuring Caddy reverse proxy for ${DOMAIN}..."
            if ! command -v caddy >/dev/null 2>&1; then
                info "Installing Caddy web server..."
                if command -v apt-get >/dev/null 2>&1; then
                    apt-get update -qq
                    apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
                    apt-get update -qq
                    apt-get install -y -qq caddy
                elif command -v dnf >/dev/null 2>&1; then
                    dnf install -y 'dnf-command(copr)'
                    dnf copr enable -y @caddy/caddy
                    dnf install -y caddy
                elif command -v pacman >/dev/null 2>&1; then
                    pacman -Sy --noconfirm caddy
                else
                    fatal "Automatic Caddy installation is not supported on this distribution. Please install Caddy manually or choose a different proxy."
                fi
            fi

            mkdir -p /etc/caddy
            local caddyfile="/etc/caddy/Caddyfile"
            if [ -f "$caddyfile" ]; then
                cp "$caddyfile" "${caddyfile}.bak.$(date +%s)"
            fi

            cat > "$caddyfile" << EOF
# AlphaDrive Caddy Reverse Proxy Configuration
${DOMAIN} {
    # Security Headers (AlphaDrive also sets these internally)
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        Referrer-Policy "strict-origin-when-cross-origin"
    }

    # Enable compression for static assets and HTML/text streams
    encode zstd gzip

    # Reverse proxy all requests to AlphaDrive daemon
    # Preserves upstream Content-Type and response headers transparently
    reverse_proxy 127.0.0.1:${PORT} {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}

        transport http {
            read_buffer 16384
            response_header_timeout 300s
        }
    }
}
EOF

            local val_out
            if val_out="$(caddy validate --config "$caddyfile" 2>&1)"; then
                systemctl enable caddy >/dev/null 2>&1 || true
                systemctl reload caddy 2>/dev/null || systemctl restart caddy 2>/dev/null || true
                success "Caddy reverse proxy active for ${DOMAIN} with automatic HTTPS"
            else
                warn "Caddy configuration validation failed:"
                printf '%s\n' "$val_out" >&2
                warn "Please check ${caddyfile}"
            fi
            ;;

        nginx)
            info "Configuring Nginx reverse proxy for ${DOMAIN}..."
            if ! command -v nginx >/dev/null 2>&1; then
                if command -v apt-get >/dev/null 2>&1; then
                    apt-get update -qq && apt-get install -y -qq nginx
                elif command -v dnf >/dev/null 2>&1; then
                    dnf install -y nginx
                elif command -v pacman >/dev/null 2>&1; then
                    pacman -Sy --noconfirm nginx
                else
                    fatal "Automatic Nginx installation not supported on this OS. Please install Nginx manually."
                fi
            fi

            mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
            local nginx_conf="/etc/nginx/sites-available/alphadrive.conf"
            if [ -f "$nginx_conf" ]; then
                cp "$nginx_conf" "${nginx_conf}.bak.$(date +%s)"
            fi

            cat > "$nginx_conf" << EOF
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
            ln -sf "$nginx_conf" /etc/nginx/sites-enabled/alphadrive.conf
            if nginx -t >/dev/null 2>&1; then
                systemctl enable nginx >/dev/null 2>&1 || true
                systemctl reload nginx 2>/dev/null || systemctl restart nginx 2>/dev/null || true
                success "Nginx reverse proxy configured and active"

                # Check for certbot
                if command -v certbot >/dev/null 2>&1; then
                    info "Obtaining Let's Encrypt TLS certificate via Certbot..."
                    if certbot --nginx -d "${DOMAIN}" --non-interactive --agree-tos --register-unsafely-without-email; then
                        success "HTTPS certificate provisioned successfully for ${DOMAIN}"
                    else
                        warn "Certbot automatic TLS certificate issuance failed. Please run 'certbot --nginx -d ${DOMAIN}' manually."
                    fi
                else
                    warn "Nginx reverse proxy is configured, but HTTPS certificate setup requires manual completion (e.g. 'apt install certbot python3-certbot-nginx && certbot --nginx -d ${DOMAIN}')."
                fi
            else
                warn "Nginx syntax test failed. Please verify /etc/nginx/sites-available/alphadrive.conf"
            fi
            ;;

        traefik)
            mkdir -p /etc/traefik/dynamic
            local traefik_conf="/etc/traefik/dynamic/alphadrive.yml"
            cat > "$traefik_conf" << EOF
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
            success "Traefik dynamic configuration generated at: ${traefik_conf}"
            info "Ensure your Traefik instance includes the /etc/traefik/dynamic file provider."
            ;;

        none)
            debug "No reverse proxy configured"
            ;;
    esac
}

verify_health() {
    printf '\n%b[6/7] Starting AlphaDrive daemon & running health checks...%b\n' "$BOLD" "$NC"
    systemctl restart alphadrive

    printf '%b[7/7] Verifying healthcheck endpoint...%b\n' "$BOLD" "$NC"
    local health_url="http://${HEALTH_CHECK_IP}:${PORT}/healthz"
    local attempts=0
    local max_attempts=15

    while [ "$attempts" -lt "$max_attempts" ]; do
        if curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
            success "AlphaDrive daemon is healthy and responding (HTTP 200 on /healthz)"
            # Clean up upgrade backup binary on success
            rm -f /opt/alphadrive/alphadrive.bak.upgrade
            return 0
        fi
        sleep 1
        attempts=$((attempts + 1))
    done

    error "Health check failed on ${health_url} after ${max_attempts} seconds!"
    echo ""
    error "Recent service journal logs:"
    journalctl -u alphadrive -n 30 --no-pager || true
    echo ""

    # Attempt rollback if upgrading
    if [ "${UPGRADE_MODE}" = "true" ] && [ -f /opt/alphadrive/alphadrive.bak.upgrade ]; then
        warn "Attempting automatic rollback to previous version..."
        cp -p /opt/alphadrive/alphadrive.bak.upgrade /opt/alphadrive/alphadrive
        systemctl restart alphadrive
        sleep 2
        if curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
            warn "Rollback successful. Previous version restored and healthy."
        else
            error "Rollback failed. Service remains unhealthy."
        fi
    fi

    fatal "AlphaDrive failed to start properly. Please check logs above."
}

install_helpers() {
    # 1. Update utility
    cat > /usr/local/bin/alphadrive-update << 'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ "$(id -u)" -ne 0 ]; then
    exec sudo -E bash "$0" "$@"
fi
echo "Updating AlphaDrive to latest release..."
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
echo "WARNING: User data and database are located in /var/lib/alphadrive."
read -r -p "Do you want to PERMANENTLY DELETE ALL USER DATA in /var/lib/alphadrive? [y/N]: " delete_data
if [[ "$delete_data" =~ ^[yY](es)?$ ]]; then
    rm -rf /var/lib/alphadrive /var/log/alphadrive
    echo "User data and database permanently deleted."
else
    echo "User data preserved in /var/lib/alphadrive."
fi

echo ""
echo "AlphaDrive has been successfully uninstalled."
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
    local db_file="/var/lib/alphadrive/data/alphadrive.db"
    local is_new_install=true
    if [ -f "$db_file" ] && [ -s "$db_file" ]; then
        is_new_install=false
    fi

    echo ""
    echo "============================================================"
    printf '            %b%bAlphaDrive Installation Complete%b\n' "$GREEN" "$BOLD" "$NC"
    echo "============================================================"
    echo ""
    printf 'Version:      %b%s%b\n' "$BOLD" "${RESOLVED_TAG}" "$NC"
    printf 'Architecture: %b%s%b\n' "$BOLD" "${ARCH}" "$NC"
    printf 'Port:         %b%s%b\n' "$BOLD" "${PORT}" "$NC"
    if [ "$ACCESS_MODE" = "2" ]; then
        printf 'Access Mode:  %bCustom Domain + HTTPS (%s)%b\n' "$BOLD" "${PROXY}" "$NC"
    elif [ "$ACCESS_MODE" = "3" ]; then
        printf 'Access Mode:  %bLocal / LAN%b\n' "$BOLD" "$NC"
    else
        printf 'Access Mode:  %bVPS IP + Port%b\n' "$BOLD" "$NC"
    fi
    echo ""
    echo "AlphaDrive URL:"
    printf '  %b%b%s%b\n' "$CYAN" "$BOLD" "${PUBLIC_URL}" "$NC"
    echo ""

    if [ "$is_new_install" = "true" ]; then
        echo "First-time Owner Setup:"
        printf '  %b%b%s/setup%b\n' "$GREEN" "$BOLD" "${PUBLIC_URL}" "$NC"
        echo ""
    fi

    echo "Service Management:"
    echo "  systemctl status alphadrive   # View service status"
    echo "  systemctl restart alphadrive  # Restart service"
    echo "  journalctl -u alphadrive -f   # View live logs"
    echo ""
    echo "Operational Commands:"
    echo "  alphadrive doctor             # Run diagnostics"
    echo "  alphadrive backup             # Create atomic backup"
    echo "  alphadrive-update             # Upgrade to latest release"
    echo "  alphadrive-uninstall          # Uninstall AlphaDrive"
    echo ""
    echo "Documentation & Guides:"
    echo "  https://github.com/AlphaTechiess/alphadrive/wiki"
    echo "============================================================"
    echo ""
}

main() {
    parse_args "$@"
    validate_cli_args
    check_privileges "$@"
    check_system
    check_existing_install
    run_wizard
    resolve_version
    download_and_verify
    setup_system
    configure_reverse_proxy
    verify_health
    install_helpers
    check_firewall
    print_completion
}

main "$@"