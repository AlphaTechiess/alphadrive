# AlphaDrive One-Line Installer & Deployment Guide

This guide covers the architecture, configuration, operation, and troubleshooting of the AlphaDrive automated one-line installer system.

---

## 1. Quick One-Line Installation

AlphaDrive can be installed on any fresh Linux VPS using a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash
```

The installer handles operating system validation, architecture detection, cryptographic checksum verification, unprivileged user sandboxing, systemd service configuration, networking, reverse proxies, and pre-flight health checks.

---

## 2. System Requirements & Compatibility

### Supported Operating Systems
- **Ubuntu**: 20.04, 22.04, 24.04 LTS
- **Debian**: 11 (Bullseye), 12 (Bookworm)
- **RHEL / Rocky Linux / AlmaLinux / CentOS Stream**: 8, 9
- **Fedora**: 38+
- **Arch Linux**
- **Alpine Linux** (with systemd/OpenRC compatibility)

### Supported Hardware Architectures
- `amd64` / `x86_64` (Standard 64-bit Intel/AMD)
- `arm64` / `aarch64` (64-bit ARM, Ampere, AWS Graviton, Raspberry Pi 4/5)

### System Minimums
- **RAM**: 512 MB (1 GB+ recommended)
- **Disk**: 500 MB base + storage for your uploaded files
- **Supervision**: `systemd` (required for service management)
- **Dependencies on Host**: None! (Zero Go, Node.js, npm, Docker, or external database required)

---

## 3. Installation Modes

During interactive installation, you can select between three access modes:

```
How would you like to access AlphaDrive?

  1) VPS IP + Port (Default, simplest standalone setup)
  2) Custom Domain + HTTPS (Automatic SSL via Reverse Proxy)
  3) Local / LAN (Private network deployment)
```

### Mode 1: VPS IP + Port (Default)
- **Description**: Exposes AlphaDrive directly on all interfaces (`0.0.0.0:<port>`).
- **Default Port**: `8080` (customizable to any free port 1-65535).
- **Domain Required**: No.
- **Reverse Proxy**: None.
- **Access URL**: `http://<your-vps-ip>:8080`
- **Notice**: A plain HTTP warning is displayed. HTTPS is recommended for public internet deployments.

### Mode 2: Custom Domain + HTTPS
- **Description**: Binds AlphaDrive locally (`127.0.0.1:<port>`) and provisions a reverse proxy with TLS encryption.
- **Domain Required**: Yes (e.g. `drive.example.com`).
- **DNS Verification**: The installer checks that your domain points to the server's public IPv4 address before proceeding.
- **Reverse Proxy Options**:
  1. **Caddy (Recommended)**: Automatic Let's Encrypt TLS certificate provisioning and renewal with zero manual configuration.
  2. **Nginx**: Creates site configuration in `/etc/nginx/sites-available/alphadrive.conf` and prompts Certbot for SSL.
  3. **Traefik**: Generates `/etc/traefik/dynamic/alphadrive.yml` file provider config.
  4. **None / Manual**: Outputs reverse proxy guidelines for custom edge proxies.
- **Access URL**: `https://drive.example.com`

### Mode 3: Local / LAN
- **Description**: Configured for private internal subnet access (e.g. `http://192.168.1.50:8080` or home lab networks).
- **DNS / Public TLS Required**: No.

---

## 4. Automated & Headless Deployment (CLI Options)

For automated server provisioning scripts, cloud-init, Ansible, or Terraform, the installer supports non-interactive execution:

```bash
# Automated IP:Port installation
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --port 8080

# Automated custom domain with Caddy HTTPS
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --domain drive.example.com --proxy caddy

# Install a specific version release
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --version 1.0.1
```

### Full CLI Flag Reference

| Flag | Argument | Description | Default |
| :--- | :--- | :--- | :--- |
| `--version` | `VERSION` | Release version to install (e.g. `1.0.1` or `v1.0.1`) | `latest` stable |
| `--port` | `PORT` | AlphaDrive backend port | `8080` |
| `--bind` | `IP` | Bind IP address | `0.0.0.0` (IP mode) / `127.0.0.1` (Domain mode) |
| `--domain` | `DOMAIN` | Domain for HTTPS (e.g. `drive.example.com`) | None |
| `--proxy` | `caddy\|nginx\|traefik\|none` | Reverse proxy for domain mode | `caddy` (Domain mode) / `none` (IP mode) |
| `--no-proxy` | — | Disable reverse proxy configuration | — |
| `--access-mode` | `1\|2\|3` | 1 = IP+Port, 2 = Domain+HTTPS, 3 = Local/LAN | `1` |
| `--non-interactive`, `-y` | — | Run without interactive prompts | `false` |
| `--debug` | — | Enable verbose bash debug execution (`set -x`) | `false` |
| `-h`, `--help` | — | Show help menu and exit | — |

---

## 5. Security & System Isolation

The installer configures a least-privilege, sandboxed deployment adhering to modern Linux security standards:

### Filesystem Layout
- **Binary**: `/opt/alphadrive/alphadrive` (`chmod 755 root:root`), symlinked to `/usr/local/bin/alphadrive`.
- **Configuration**: `/etc/alphadrive/alphadrive.env` (`chmod 640 root:alphadrive`).
- **Data & Database**: `/var/lib/alphadrive/data` (`chmod 700 alphadrive:alphadrive`).
- **Service Logs**: `/var/log/alphadrive` (`chmod 750 alphadrive:alphadrive`).
- **Backups**: `/var/backups` (`chmod 750`).

### Dedicated Unprivileged User
AlphaDrive runs under a dedicated system user:
- User: `alphadrive`
- Shell: `/usr/sbin/nologin`
- Home: `/var/lib/alphadrive`

### Hardened Systemd Service
The systemd unit (`/etc/systemd/system/alphadrive.service`) implements kernel-level isolation:
```ini
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
```

---

## 6. Mandatory Cryptographic Checksum Verification

Every binary download is validated against official SHA256 checksums before installation or modification of system files:

```
Download binary -> Download checksums.txt -> Compute SHA256 -> Match?
                                                                 ├── Yes -> Install & launch
                                                                 └── No  -> ABORT & delete artifact
```

If `checksums.txt` is missing, corrupted, or the hash differs, the installer immediately purges the staging files, aborts execution, and leaves the existing host system untouched.

---

## 7. Firewall Configuration

The installer automatically detects active firewalls (`ufw`):
- **IP Mode**: Prompts to allow `<port>/tcp` (e.g. `8080/tcp`).
- **Domain HTTPS Mode**: Ensures ports `80/tcp` and `443/tcp` are open for HTTP-01 ACME challenges and HTTPS traffic.
- Existing firewall policies are preserved; the installer never flushes or resets firewall rules.

---

## 8. Safe Upgrades & Rollback

### Upgrading AlphaDrive
To upgrade an existing installation to the latest release:

```bash
alphadrive-update
```
Or rerun the installer:
```bash
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash
```

### Upgrade Safety Guarantees
1. **Pre-Upgrade Hot Backup**: A consistent atomic snapshot of the SQLite database and storage objects is created at `/var/backups/alphadrive-preupgrade-*.tar.gz` before replacing the binary. If backup generation fails, the upgrade aborts immediately.
2. **Configuration & Data Preservation**: `/etc/alphadrive/alphadrive.env` and `/var/lib/alphadrive/data` are strictly preserved.
3. **Automated Rollback**: If the new version fails the `/healthz` check post-startup, the installer restores the previous binary and restarts the healthy service.

---

## 9. Native Backups & Disaster Recovery

AlphaDrive provides built-in online backup tooling in the single binary (no `sqlite3` CLI needed):

### Creating a Hot Backup
```bash
# Create an atomic backup archive
alphadrive backup --output /var/backups/alphadrive-$(date +%Y%m%d).tar.gz
```

### Restoring from Backup
```bash
# 1. Stop service
sudo systemctl stop alphadrive

# 2. Extract backup archive into data directory
sudo tar -xzf /var/backups/alphadrive-20260917.tar.gz -C /var/lib/alphadrive/data

# 3. Ensure permissions
sudo chown -R alphadrive:alphadrive /var/lib/alphadrive

# 4. Restart service
sudo systemctl start alphadrive
```

---

## 10. Uninstallation

AlphaDrive includes a safe uninstaller helper:

```bash
alphadrive-uninstall
```

The uninstaller:
1. Stops and disables the systemd service.
2. Removes `/etc/systemd/system/alphadrive.service` and reloads systemd.
3. Removes `/opt/alphadrive`, `/etc/alphadrive`, and `/usr/local/bin/alphadrive*`.
4. **Data Protection**: Prompts with an explicit confirmation dialog before deleting `/var/lib/alphadrive`. If declined, all database records and uploaded files remain safely stored on disk.

---

## 11. Troubleshooting & Common Issues

### 1. Port Collision (`Port is already in use`)
- **Cause**: Another daemon (e.g. Apache, existing proxy) is listening on the selected port.
- **Resolution**: Choose a different port (e.g. `8081` or `9000`) or stop the competing service.

### 2. DNS Resolution Mismatch
- **Cause**: Your domain's DNS `A` record does not match the public IP of your VPS.
- **Resolution**: Update your domain's DNS provider with your VPS IP, wait for TTL propagation, and re-run the installer.

### 3. Service Failed to Start
- **Diagnostics**:
  ```bash
  sudo systemctl status alphadrive
  sudo journalctl -u alphadrive -n 50 --no-pager
  alphadrive doctor
  ```

---

## 12. Manual VPS Verification Checklist

When deploying to a physical or cloud Linux VPS (DigitalOcean, Hetzner, AWS, Linode, Vultr):

- [ ] Execute `curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash`.
- [ ] Verify system check detects OS, architecture, RAM, and systemd.
- [ ] Choose **VPS IP + Port** mode (Port 8080).
- [ ] Open `http://<vps-ip>:8080/setup` in browser.
- [ ] Create Owner administrator account (password $\ge$ 12 chars).
- [ ] Verify automatic login to dashboard.
- [ ] Upload test images, documents, videos.
- [ ] Verify in-browser previews and instant file download.
- [ ] Create subfolder, move files, test trash and restore.
- [ ] Create public share link with custom slug and password protection.
- [ ] Open public share in incognito window and verify ZIP download.
- [ ] Reboot VPS (`sudo reboot`) and verify `alphadrive.service` starts on boot.
- [ ] Run `alphadrive-update` to verify safe upgrade and pre-upgrade backup creation.
