# Backup, Restore & Disaster Recovery

AlphaDrive includes native hot online backup and disaster recovery tools built directly into the core binary.

---

## 1. Creating a Hot Online Backup

You can execute a hot backup while the server is running without any downtime:

```bash
# Create a hot backup archive
sudo -u alphadrive alphadrive backup --data-dir /var/lib/alphadrive/data --out /var/backups/alphadrive-$(date +%F).tar.gz
```

### What the backup contains:
- **SQLite Database Snapshot**: Uses SQLite's online vacuum/backup API to create a clean, consistent snapshot without database locks.
- **Session Keys**: Preserves `session.key` to maintain active logins.
- **Binary Object Store**: Archives all uploaded file blobs (`files/objects/`).
- **Metadata**: Embedded backup manifest and schema version.

---

## 2. Automated Daily Backups (Cron)

To schedule automated daily backups at 03:00 AM:

```bash
sudo tee /etc/cron.daily/alphadrive-backup > /dev/null << 'EOF'
#!/bin/bash
BACKUP_DIR="/var/backups/alphadrive"
mkdir -p "$BACKUP_DIR"
BACKUP_FILE="$BACKUP_DIR/alphadrive-$(date +\%Y\%m\%d_\%H\%M\%S).tar.gz"

sudo -u alphadrive /usr/local/bin/alphadrive backup --data-dir /var/lib/alphadrive/data --out "$BACKUP_FILE"

# Keep last 7 days of backups
find "$BACKUP_DIR" -type f -name "*.tar.gz" -mtime +7 -delete
EOF

sudo chmod +x /etc/cron.daily/alphadrive-backup
```

---

## 3. Full Server Restoration

To restore AlphaDrive from a backup archive (e.g. onto a new VPS):

1. **Install AlphaDrive binary** on the target server.
2. **Stop the daemon**:
   ```bash
   sudo systemctl stop alphadrive
   ```
3. **Execute restore**:
   ```bash
   sudo -u alphadrive alphadrive restore --data-dir /var/lib/alphadrive/data --in /var/backups/alphadrive-2026-09-17.tar.gz
   ```
4. **Start the daemon**:
   ```bash
   sudo systemctl start alphadrive
   ```
5. All users, files, folders, shares, and settings are fully restored.