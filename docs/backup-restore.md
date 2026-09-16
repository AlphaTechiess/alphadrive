# Backup & Disaster Recovery Guide

AlphaDrive stores all data within a single directory tree specified by `ALPHADRIVE_DATA_DIR` (default: `/var/lib/alphadrive/data`):

- `alphadrive.db`, `alphadrive.db-wal`, `alphadrive.db-shm`: SQLite database containing accounts, metadata, sessions, and share grants.
- `files/objects/`: Raw physical files stored by cryptographic storage keys.
- `files/temp/`: In-progress uploads and staging files.

---

## 1. Native Hot Backup (Zero Downtime)

AlphaDrive includes a built-in backup engine that creates an atomic, consistent snapshot of the SQLite database (using SQLite's online backup capability) along with all physical file objects in `files/objects/`. **No external `sqlite3` CLI tool or third-party backup software is required.**

### Running a Native Backup
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive backup --output /var/backups/alphadrive-$(date +%Y%m%d_%H%M%S).tar.gz
```

### Automated Backup Script (`/usr/local/bin/backup-alphadrive.sh`)

```bash
#!/usr/bin/env bash
set -euo pipefail

BACKUP_ROOT="/var/backups/alphadrive"
DATA_DIR="/var/lib/alphadrive/data"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
TARGET_FILE="${BACKUP_ROOT}/alphadrive_backup_${TIMESTAMP}.tar.gz"

mkdir -p "${BACKUP_ROOT}"

# Run native AlphaDrive atomic hot backup
sudo -u alphadrive ALPHADRIVE_DATA_DIR="${DATA_DIR}" /usr/local/bin/alphadrive backup --output "${TARGET_FILE}"

# Retain last 14 days of backups
find "${BACKUP_ROOT}" -name "alphadrive_backup_*.tar.gz" -mtime +14 -delete

echo "AlphaDrive backup completed: ${TARGET_FILE}"
```

Make the script executable and schedule it in crontab:
```bash
chmod +x /usr/local/bin/backup-alphadrive.sh
(crontab -l 2>/dev/null; echo "0 3 * * * /usr/local/bin/backup-alphadrive.sh > /var/log/alphadrive-backup.log 2>&1") | crontab -
```

---

## 2. Cold Backup (Maintenance Mode)

When taking a full machine snapshot or stopping the service for server maintenance:

```bash
# Stop AlphaDrive service
sudo systemctl stop alphadrive

# Archive data directory
sudo tar -czvf /var/backups/alphadrive_cold_$(date +%Y%m%d).tar.gz /var/lib/alphadrive/data

# Restart service
sudo systemctl start alphadrive
```

---

## 3. Disaster Recovery & Restoration Runbook

To restore AlphaDrive on a fresh VPS or recover from data corruption:

### Step 1: Stop Service
```bash
sudo systemctl stop alphadrive
```

### Step 2: Extract Backup Archive
```bash
# Backup existing broken data directory if needed
sudo mv /var/lib/alphadrive/data /var/lib/alphadrive/data.old

# Create clean destination directory
sudo mkdir -p /var/lib/alphadrive/data/files/objects /var/lib/alphadrive/data/files/temp

# Extract the backup snapshot
sudo tar -xzf /path/to/alphadrive_backup_YYYYMMDD_HHMMSS.tar.gz -C /var/lib/alphadrive/data/
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

Verify that all diagnostic tests report `[OK]`, including database integrity and file consistency.

### Step 5: Start Service
```bash
sudo systemctl start alphadrive
sudo systemctl status alphadrive
```
