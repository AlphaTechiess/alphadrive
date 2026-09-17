# AlphaDrive Installation Guide

This guide covers all methods for installing and deploying AlphaDrive, including automated one-line scripts, headless CI/CD installation, manual pre-compiled binary deployment, and building directly from source.

---

## 1. Automated One-Line VPS Installer (Recommended)

The easiest way to install AlphaDrive on any Linux VPS is using the official automated installer:

```bash
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash
```

### What the installer does:
- Detects your CPU architecture (`amd64` or `arm64`) and operating system.
- Downloads the latest stable binary and cryptographically verifies its SHA256 checksum.
- Provisions a dedicated unprivileged system user `alphadrive` and group `alphadrive`.
- Sets up data directories in `/var/lib/alphadrive/data` and configuration in `/etc/alphadrive/alphadrive.env`.
- Configures and enables a hardened `systemd` unit with automatic restarts.
- Configures reverse proxies (Caddy, Nginx, or Traefik) if custom domain is selected.
- Installs maintenance helper commands: `alphadrive-update` and `alphadrive-uninstall`.

---

## 2. Headless & Automated Installation Flags

For automated cloud provisioning (cloud-init, Ansible, Terraform, Bash scripts), `install.sh` accepts command-line flags to run non-interactively without user prompts:

```bash
# Standalone VPS IP mode on port 8080 (No domain required)
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --port 8080

# Production custom domain with Caddy automatic SSL
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --domain drive.example.com --proxy caddy

# Production custom domain with Nginx + Certbot
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --domain drive.example.com --proxy nginx

# Install a specific pinned release version
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --version 1.0.5
```

### CLI Flag Reference
| Flag | Values | Description |
| :--- | :--- | :--- |
| `--non-interactive`, `-y` | None | Run without interactive prompts |
| `--version` | `1.0.5`, `latest` | Specific version to install |
| `--port` | `1-65535` | Port for the backend service (Default: `8080`) |
| `--bind` | `0.0.0.0`, `127.0.0.1` | Network interface to bind |
| `--domain` | `drive.example.com` | Custom domain name |
| `--proxy` | `caddy`, `nginx`, `traefik`, `none` | Reverse proxy selection |
| `--skip-healthcheck` | None | Skip post-install HTTP verification |

---

## 3. Manual Pre-Compiled Binary Installation

If you prefer deploying the binary manually without running the installer script:

1. **Download Release**:
   Download the latest release binary for your architecture from [GitHub Releases](https://github.com/AlphaTechiess/alphadrive/releases):
   ```bash
   # For x86_64 / amd64
   curl -LO https://github.com/AlphaTechiess/alphadrive/releases/latest/download/alphadrive-linux-amd64
   chmod +x alphadrive-linux-amd64
   sudo mv alphadrive-linux-amd64 /usr/local/bin/alphadrive

   # For ARM64 / aarch64
   curl -LO https://github.com/AlphaTechiess/alphadrive/releases/latest/download/alphadrive-linux-arm64
   chmod +x alphadrive-linux-arm64
   sudo mv alphadrive-linux-arm64 /usr/local/bin/alphadrive
   ```

2. **Create Service User & Directories**:
   ```bash
   sudo useradd --system --user-group --shell /sbin/nologin --home-dir /var/lib/alphadrive alphadrive
   sudo mkdir -p /var/lib/alphadrive/data /etc/alphadrive
   sudo chown -R alphadrive:alphadrive /var/lib/alphadrive
   ```

3. **Configure Environment File** (`/etc/alphadrive/alphadrive.env`):
   ```ini
   ALPHADRIVE_PORT=8080
   ALPHADRIVE_BIND=0.0.0.0
   ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data
   ```

4. **Create Systemd Service** (`/etc/systemd/system/alphadrive.service`):
   ```ini
   [Unit]
   Description=AlphaDrive Cloud Storage Daemon
   After=network.target

   [Service]
   Type=simple
   User=alphadrive
   Group=alphadrive
   WorkingDirectory=/var/lib/alphadrive
   EnvironmentFile=/etc/alphadrive/alphadrive.env
   ExecStart=/usr/local/bin/alphadrive start
   Restart=always
   RestartSec=5s

   # Sandboxing & Security
   NoNewPrivileges=true
   ProtectSystem=strict
   ProtectHome=true
   ReadWritePaths=/var/lib/alphadrive

   [Install]
   WantedBy=multi-user.target
   ```

5. **Start and Enable Service**:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now alphadrive
   sudo systemctl status alphadrive
   ```

---

## 4. Building from Source

To compile AlphaDrive from source:

### Prerequisites:
- Go 1.22 or higher installed (`go version`)
- Git

### Build Steps:
```bash
git clone https://github.com/AlphaTechiess/alphadrive.git
cd alphadrive

# Compile static binary with embedded assets
go build -ldflags "-s -w" -o alphadrive ./cmd/alphadrive

# Verify binary
./alphadrive version
```

### Running Locally:
```bash
export ALPHADRIVE_PORT=8080
export ALPHADRIVE_BIND=127.0.0.1
export ALPHADRIVE_DATA_DIR=./data
./alphadrive start
```