# CLI Reference & Maintenance Tools

AlphaDrive includes built-in CLI commands for server operation, diagnostics, backups, and maintenance.

---

## 1. `alphadrive start`
Starts the AlphaDrive HTTP server daemon.

```bash
alphadrive start [--port 8080] [--bind 0.0.0.0] [--data-dir /var/lib/alphadrive/data]
```

### Options:
- `--port`: TCP port to listen on (Default: `8080` or `$ALPHADRIVE_PORT`).
- `--bind`: Interface IP address (Default: `0.0.0.0` or `$ALPHADRIVE_BIND`).
- `--data-dir`: Data storage directory (Default: `./data` or `$ALPHADRIVE_DATA_DIR`).

---

## 2. `alphadrive doctor`
Performs comprehensive pre-flight system diagnostics and outputs a health audit.

```bash
alphadrive doctor [--data-dir /var/lib/alphadrive/data]
```

### Diagnostics Performed:
- Host OS & Kernel version.
- Disk capacity, free space, and mount point permissions.
- SQLite database integrity, WAL journal mode, and schema migrations.
- Object store directory access and permissions.
- Session key presence and file permissions (`0600`).
- Network port availability and firewall status.

---

## 3. `alphadrive backup`
Creates an online hot backup archive of the database and file storage.

```bash
alphadrive backup --data-dir <path> --out <backup.tar.gz>
```

---

## 4. `alphadrive restore`
Restores AlphaDrive data from a backup archive.

```bash
alphadrive restore --data-dir <path> --in <backup.tar.gz>
```

---

## 5. `alphadrive version`
Outputs the compiled release version, commit hash, and build timestamp.

---

## 6. System Helper Commands

Installed by the official Linux installer:
- `alphadrive-update`: Automatically downloads the latest GitHub release, verifies SHA256 checksums, takes a pre-upgrade hot backup, updates the binary, and restarts the service.
- `alphadrive-uninstall`: Gracefully uninstalls the systemd service, helper scripts, and optionally archives data.