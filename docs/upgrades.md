# Upgrade & Maintenance Guide

Upgrading AlphaDrive is straightforward thanks to its single-binary architecture and automated forward-only database migration system.

---

## Standard Upgrade Procedure

### Step 1: Create a Pre-Upgrade Backup
Always create a backup of your current database and storage before upgrading:
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive backup --output /var/backups/alphadrive-pre-upgrade-$(date +%Y%m%d_%H%M%S).tar.gz
```

### Step 2: Stop Service
```bash
sudo systemctl stop alphadrive
```

### Step 3: Replace Binary
Download or compile the new release binary and replace the existing binary in `/usr/local/bin`:
```bash
sudo install -m 755 alphadrive-linux-amd64 /usr/local/bin/alphadrive
```

### Step 4: Run Health & Migration Check
Run `alphadrive doctor` to ensure the new binary recognizes the database and storage directory:
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive doctor
```

### Step 5: Start Service
Start the service. Upon boot, any new schema migrations are applied atomically:
```bash
sudo systemctl start alphadrive
sudo systemctl status alphadrive
```

### Step 6: Verify Deployment
Check the health endpoint and view server logs:
```bash
curl -f http://127.0.0.1:8080/healthz
sudo journalctl -u alphadrive -n 50 --no-pager
```

---

## Rollback Procedure

If an upgrade encounters unexpected issues, rollback is simple:

1. Stop service:
   ```bash
   sudo systemctl stop alphadrive
   ```
2. Reinstall the previous binary version.
3. If necessary, restore the pre-upgrade backup using the disaster recovery instructions in [`docs/backup-restore.md`](./backup-restore.md).
4. Restart service:
   ```bash
   sudo systemctl start alphadrive
   ```
