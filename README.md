# AlphaDrive

[![GitHub Repository](https://img.shields.io/badge/GitHub-AlphaTechiess%2Falphadrive-blue?logo=github)](https://github.com/AlphaTechiess/alphadrive)
[![Contribute with Razorpay](https://img.shields.io/badge/Contribute-Razorpay-1688fe?logo=razorpay&logoColor=white)](https://rzp.io/rzp/alphadrive)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://github.com/AlphaTechiess/alphadrive/blob/main/LICENSE)

A modern, fast, lightweight, and self-hosted cloud drive built specifically for Linux VPS servers and standalone deployments.

---

## Overview

AlphaDrive provides an elegant web interface and public sharing platform for personal and team file management. It is designed to run efficiently on low-resource VPS hardware (e.g. 512 MB – 1 GB RAM) without requiring heavy container engines, Node.js runtimes, or external database servers.

### Key Highlights

- **Single Compiled Binary**: Entire backend and web assets (HTML/CSS/JS/SVGs) are embedded directly into a single static executable.
- **Minimal Footprint**: Written in pure Go with standard `net/http` and pure-Go SQLite (`modernc.org/sqlite`).
- **Unlimited Storage & Uploads**: Zero artificial file size limits or storage caps. Your drive capacity is bounded purely by the physical storage available on your VPS disk.
- **Instant Owner Onboarding**: Zero-CLI setup; first browser visit automatically presents the Owner Setup screen to create your administrator account.
- **In-App Account & User Management**: Easily update your username, change password, or provision new team members straight from the header settings dialog.
- **Physical Disk Telemetry**: Live VPS disk detection reporting real total, used, and free filesystem space.
- **Public Sharing Engine**: Secure custom or auto-generated link slugs (`[a-z0-9-]`), optional Argon2id password protection, configurable link expiry, instant revocation, and streaming multi-file ZIP downloads.
- **Multi-Format Previews**: In-browser preview for images, video streaming, audio player, PDF viewer, and code/text viewer.
- **Complete File Lifecycle**: Folder hierarchy, drag-and-drop uploads, instant download, trash, restore, and permanent deletion.
- **Native Operations & Tooling**: Built-in CLI commands for server operation, user management, password resets, system diagnostics (`doctor`), and hot online backups (`backup`).

---

## Architecture

AlphaDrive adheres to a minimal, high-efficiency topology:

```
Internet
   │
   ▼
Caddy or Nginx (Reverse Proxy + TLS)
   │
   ▼
AlphaDrive Daemon (127.0.0.1:8080)
   │
   ├── SQLite (WAL mode, embedded migrations)
   └── Local Filesystem (/var/lib/alphadrive/data/files/objects)
```

- **Backend**: Go 1.22+ (`net/http`)
- **Database**: Embedded SQLite with WAL mode, foreign keys, and transactional migrations
- **Frontend**: Vanilla JavaScript (ES modules) and native responsive CSS
- **Storage**: Local filesystem with content-addressed storage keys

---

## Quick Start (Linux VPS)

### One-Line Install (Recommended)

Run this single command on any fresh Ubuntu/Debian/Rocky/RHEL/Arch VPS (amd64 / arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash
```

The installer automatically:
1. Detects OS and CPU architecture (x86_64 / aarch64).
2. Downloads and cryptographically verifies (SHA256) the static release binary.
3. Provisions a least-privilege `alphadrive` system user and sandboxed directories.
4. Configures optional HTTPS with automatic SSL via Caddy, Nginx, or Traefik.
5. Installs and enables a hardened `systemd` service with health checks.
6. Installs maintenance helpers: `alphadrive-update` and `alphadrive-uninstall`.

#### Automated / Headless Install Options
```bash
# Non-interactive IP:Port setup
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --port 8080

# Non-interactive custom domain + automatic HTTPS with Caddy
curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh | sudo bash -s -- --non-interactive --domain drive.example.com --proxy caddy
```

---

### Manual Build & Run

If you prefer building from source:

```bash
# Clone repository
git clone https://github.com/AlphaTechiess/alphadrive.git
cd alphadrive

# Build binary
go build -ldflags "-s -w" -o alphadrive ./cmd/alphadrive

# Start AlphaDrive
./alphadrive serve
```

### Complete Setup in Browser

Open `http://<your-vps-ip>:8080` (or `https://your-domain.com`) in your browser.
AlphaDrive will automatically display the **Owner Setup** screen to create your admin account:
- Enter your Name, Username, and Password (minimum 12 characters).
- Click **Create Owner Account** — you are immediately logged in and ready to manage files!

---

## In-App Account Settings

Click the **Account Icon** in the top-right corner to manage your profile anytime:
- **Username Tab**: Update your active login username.
- **Password Tab**: Securely change your account password (verifies current password).
- **Add User Tab** *(Admin only)*: Easily invite or provision new users directly from the web interface without using the command line.

---

## CLI Reference

AlphaDrive also includes complete operations tooling in the single binary for headless or scriptable workflows:

```bash
# Start server
alphadrive serve [--config /path/to/config.json]

# Create a new user via CLI (optional alternative to web UI)
ALPHADRIVE_PASSWORD="YourPassword123" alphadrive create-user --username <name> [--admin=true]

# Reset an existing user's password and revoke active sessions
ALPHADRIVE_PASSWORD="NewPassword123" alphadrive reset-password --username <name>

# Create a hot, atomic backup of database and file storage (no sqlite3 CLI needed)
alphadrive backup --output /var/backups/alphadrive-$(date +%Y%m%d).tar.gz

# Run deep database and storage integrity diagnostics
alphadrive doctor

# Print release and build metadata
alphadrive version
```

---

## Documentation & Guides

Comprehensive guides and operational runbooks are maintained on the **[AlphaDrive GitHub Wiki](https://github.com/AlphaTechiess/alphadrive/wiki)**:

- [**Self-Hosting & VPS Deployment Guide**](https://github.com/AlphaTechiess/alphadrive/wiki/Self-hosting): Complete setup with Systemd, Caddy, Nginx + SSL, and zero-downtime configuration.
- [**Backup & Disaster Recovery Guide**](https://github.com/AlphaTechiess/alphadrive/wiki/Backup-&-Disaster-Recovery-Guide): Native atomic hot backups, cron retention schedules, and restoration instructions.
- [**Upgrade & Maintenance Guide**](https://github.com/AlphaTechiess/alphadrive/wiki/Upgrade-&-Maintenance-Guide): Safe single-binary upgrades, automated SQLite migrations, and health diagnostics.

---

## Support & Contribution

If you love using AlphaDrive and want to support ongoing development and maintenance, you can contribute here:

[![Contribute with Razorpay](https://img.shields.io/badge/Contribute-Razorpay-1688fe?style=for-the-badge&logo=razorpay&logoColor=white)](https://rzp.io/rzp/alphadrive)

---

## License

See [`LICENSE`](./LICENSE) for licensing details.
