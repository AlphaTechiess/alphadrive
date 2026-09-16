# AlphaDrive

A modern, fast, lightweight, and self-hosted cloud drive built specifically for Linux VPS servers and standalone deployments.

---

## Overview

AlphaDrive provides an elegant web interface and public sharing platform for personal and team file management. It is designed to run efficiently on low-resource VPS hardware (e.g. 512 MB – 1 GB RAM) without requiring heavy container engines, Node.js runtimes, or external database servers.

### Key Highlights

- **Single Compiled Binary**: Entire backend and web assets (HTML/CSS/JS/SVGs) are embedded directly into a single static executable.
- **Minimal Footprint**: Written in pure Go with standard `net/http` and pure-Go SQLite (`modernc.org/sqlite`).
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

### 1. Build from Source
```bash
go build -ldflags "-s -w -X main.Version=1.0.0 -X main.BuildDate=$(date -u +%Y-%m-%d) -X main.Commit=$(git rev-parse --short HEAD)" -o alphadrive ./cmd/alphadrive
```

### 2. Create Initial Admin User
```bash
ALPHADRIVE_PASSWORD="YourStrongPassword123" ./alphadrive create-user --username admin --admin=true
```

### 3. Run Server
```bash
./alphadrive serve
```

---

## CLI Reference

AlphaDrive includes complete operations tooling in the single binary:

```bash
# Start server
alphadrive serve [--config /path/to/config.json]

# Create a new user (minimum 12-character password)
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

## Documentation

Detailed operations and deployment guides are available in the [`docs/`](./docs) directory:

- [**Self-Hosting & VPS Deployment Guide**](./docs/self-hosting.md): Systemd setup, Caddy/Nginx reverse proxy, and environment configuration.
- [**Backup & Disaster Recovery Guide**](./docs/backup-restore.md): Native hot backups, retention scripts, and restoration runbook.
- [**Upgrades & Migration Guide**](./docs/upgrades.md): Safe binary upgrades and automated database migrations.

---

## License

See [`LICENSE`](./LICENSE) for licensing details.
