# Self-Hosting Guide: AlphaDrive on Linux VPS

AlphaDrive is designed for frictionless self-hosting on any standard Linux VPS (Ubuntu, Debian, AlmaLinux, Rocky Linux, Alpine, Arch, etc.) as a single static Go binary with an embedded SQLite database and embedded frontend assets.

---

## 1. System Requirements

- **OS**: Linux (amd64 / arm64) or Windows / macOS
- **RAM**: 256 MB minimum (512 MB+ recommended for high concurrency)
- **Disk**: Adequate NVMe / SSD / HDD storage for user uploads
- **Dependencies**: None. (No Docker, Node.js, Python, or external database required)

---

## 2. Quick Installation Steps

### Step 1: Clone Repository & Build Binary

```bash
# Clone official repository
git clone https://github.com/AlphaTechiess/alphadrive.git
cd alphadrive

# Build optimized binary
go build -ldflags "-s -w" -o alphadrive ./cmd/alphadrive

# Install binary to system path
sudo install -m 755 alphadrive /usr/local/bin/alphadrive

# Create dedicated unprivileged system user and group
sudo useradd -r -s /usr/sbin/nologin -d /var/lib/alphadrive alphadrive

# Create data and config directories
sudo mkdir -p /var/lib/alphadrive/data
sudo mkdir -p /etc/alphadrive
sudo chown -R alphadrive:alphadrive /var/lib/alphadrive
sudo chmod 700 /var/lib/alphadrive /var/lib/alphadrive/data
```

### Step 2: Configure Environment

Create `/etc/alphadrive/alphadrive.env`:

```env
# Network configuration
ALPHADRIVE_LISTEN_ADDRESS=127.0.0.1:8080
ALPHADRIVE_PUBLIC_BASE_URL=https://drive.yourdomain.com

# Storage path
ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data

# Upload and storage capacity (0 = Unlimited / bounded only by host VPS disk capacity)
ALPHADRIVE_MAX_UPLOAD_BYTES=0
ALPHADRIVE_STORAGE_QUOTA_BYTES=0

# Set to true only during development over plain HTTP without HTTPS reverse proxy
ALPHADRIVE_INSECURE_COOKIES=false
```

Set secure permissions on the env file:
```bash
sudo chown root:alphadrive /etc/alphadrive/alphadrive.env
sudo chmod 640 /etc/alphadrive/alphadrive.env
```

### Step 3: Install and Enable Systemd Service
```bash
sudo cp packaging/systemd/alphadrive.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now alphadrive
```

Verify service status:
```bash
sudo systemctl status alphadrive
```

### Step 4: Complete Owner Setup in Browser
Navigate to your instance domain or IP (e.g. `https://drive.yourdomain.com`).
Because no users exist yet, AlphaDrive will automatically display the **Owner Setup** screen:
1. Provide your Name, Administrator Username, and Password (minimum 12 characters).
2. Click **Create Owner Account**.
3. You will be logged in immediately.

*Note: You can also provision users via CLI using `alphadrive create-user` if preferred.*

---

## 3. In-App User & Account Management

Administrators and users can manage credentials directly in the web UI by clicking the **Account Icon** in the top-right navigation bar:
- **Change Username**: Update login handle with live availability checks.
- **Update Password**: Update login password with old password verification and $\ge 12$-character enforcement.
- **Add Users (Admin)**: Create new users (with optional Admin privileges) straight from the UI. No server CLI access needed!

---

## 4. Reverse Proxy Configuration

### Option A: Caddy (Recommended)

Caddy automatically handles HTTPS certificates and transparent proxying.

1. Install Caddy: `sudo apt install caddy`
2. Place the configuration in `/etc/caddy/Caddyfile`:
```caddy
drive.yourdomain.com {
    request_body {
        max_size 10GB
    }
    reverse_proxy 127.0.0.1:8080
}
```
3. Reload Caddy: `sudo systemctl reload caddy`

### Option B: Nginx + Certbot

1. Install Nginx and Certbot: `sudo apt install nginx certbot python3-certbot-nginx`
2. Copy `packaging/nginx/alphadrive.conf` to `/etc/nginx/sites-available/alphadrive.conf`
3. Edit the `server_name` to match your domain.
4. Enable the site and obtain certificates:
```bash
sudo ln -s /etc/nginx/sites-available/alphadrive.conf /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
sudo certbot --nginx -d drive.yourdomain.com
```

---

## 5. Operational CLI Commands

### Health & Integrity Diagnostics (`doctor`)
Run non-destructive SQLite integrity checks, verify physical object consistency, and report filesystem capacity:
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive doctor
```

### Hot Atomic Backup (`backup`)
Generate a complete, consistent backup of SQLite database and physical storage:
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data /usr/local/bin/alphadrive backup --output /var/backups/alphadrive-$(date +%Y%m%d).tar.gz
```

### Password Reset
Reset password for any user and invalidate active sessions:
```bash
sudo -u alphadrive ALPHADRIVE_DATA_DIR=/var/lib/alphadrive/data ALPHADRIVE_PASSWORD="NewPassword12345" /usr/local/bin/alphadrive reset-password --username admin
```

### Version Information
```bash
alphadrive version
```
