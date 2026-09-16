# Backup & Disaster Recovery Guide

AlphaDrive stores all data within a single directory tree specified by `ALPHADRIVE_DATA_DIR` (default: `/var/lib/alphadrive/data`):

- `alphadrive.db`, `alphadrive.db-wal`, `alphadrive.db-shm`: SQLite database containing accounts, metadata, sessions, and share permissions.
- `files/objects/`: Raw physical files stored by cryptographic storage keys.
- `files/temp/`: In-progress uploads and staging files.

---

## 1. Hot Backup (Zero Downtime)

Because AlphaDrive operates SQLite in **WAL (Write-Ahead Logging)** mode, you should use SQLite's online backup API or VACUUM INTO to create an atomic, non-corrupted database snapshot while the service is actively running.

### Automated Backup Script (`/usr/local/bin/backup-alphadrive.sh`)

```bash
#!/usr/bin/env bash
set -euo pipefail

DATA_DIR="/var/lib/alphadrive/data"
BACKUP_ROOT="/var/backups/alphadrive"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
TARGET_DIR="${BACKUP_ROOT}/${TIMESTAMP}"

mkdir -p "${TARGET_DIR}/files"

# 1. Hot snapshot of SQLite database using sqlite3 command line tool
# This safely flushes pending WAL transactions and generates an atomic database copy
sqlite3 "${DATA_DIR}/alphadrive.db" ".backup '${TARGET_DIR}/alphadrive.db'"

# 2. Copy the objects directory
rsync -a --delete "${DATA_DIR}/files/objects/" "${TARGET_DIR}/files/objects/"

# 3. Create a compressed archive of the snapshot
tar -czf "${BACKUP_ROOT}/alphadrive_backup_${TIMESTAMP}.tar.gz" -C "${BACKUP_ROOT}" "${TIMESTAMP}"
rm -rf "${TARGET_DIR}"

# 4. Retain last 14 days of backups
find "${BACKUP_ROOT}" -name "alphadrive_backup_*.tar.gz" -mtime +14 -delete

echo "AlphaDrive backup completed: ${BACKUP_ROOT}/alphadrive_backup_${TIMESTAMP}.tar.gz"
```

Make the script executable and configure a daily cron job:
```bash
chmod +x /usr/local/bin/backup-alphadrive.sh
(crontab -l 2>/dev/null; echo "0 3 * * * /usr/local/bin/backup-alphadrive.sh > /var/log/alphadrive-backup.log 2>&1") | crontab -
```

---

## 2. Cold Backup (Maintenance Mode)

If taking a full machine snapshot or stopping the service for maintenance:

```bash
# Stop AlphaDrive service
sudo systemctl stop alphadrive

# Archive data directory
sudo tar -czvf /var/backups/alphadrive_cold_$(date +%Y%m%d).tar.gz /var/lib/alphadrive/data

# Restart service
sudo systemctl start alphadrive
```

---

## 3. Disaster Recovery Restoration

To restore AlphaDrive on a fresh VPS or rollback to a previous state:

### Step 1: Stop Service
```bash
sudo systemctl stop alphadrive
```

### Step 2: Extract Backup Archive
```bash
# Clean or move current data directory
sudo mv /var/lib/alphadrive/data /var/lib/alphadrive/data.broken

# Extract the backup snapshot
sudo mkdir -p /var/lib/alphadrive/data/files/objects /var/lib/alphadrive/data/files/temp
sudo tar -xzf /path/to/alphadrive_backup_YYYYMMDD_HHMMSS.tar.gz -C /tmp/

# Move restored database and objects into place
sudo cp /tmp/YYYYMMDD_HHMMSS/alphadrive.db /var/lib/alphadrive/data/alphadrive.db
sudo cp -r /tmp/YYYYMMDD_HHMMSS/files/objects/* /var/lib/alphadrive/data/files/objects/

# Cleanup temp files
rm -rf /tmp/YYYYMMDD_HHMMSS
```

### Step 3: Verify Permissions
```bash
sudo chown -R alphadrive:alphadrive /var/lib/alphadrive
sudo chmod 700 /var/lib/alphadrive /var/lib/alphadrive/data
```

### Step 4: Run Doctor Diagnostics
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive doctor
```

Verify that all tests report `[OK]`, including database integrity and file consistency.

### Step 5: Start Service
```bash
sudo systemctl start alphadrive
sudo systemctl status alphadrive
```
