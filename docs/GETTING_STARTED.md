# Getting Started with AlphaDrive

AlphaDrive is an ultra-fast, self-hosted personal cloud storage and public sharing platform built in Go. It operates as a single static binary with embedded SQLite, zero runtime dependencies, and instant onboarding.

---

## ðŸ’» System Requirements

### Hardware Requirements
| Resource | Minimum | Recommended |
| :--- | :--- | :--- |
| **CPU** | 1 Core (x86_64 / amd64 or aarch64 / arm64) | 1-2 Cores |
| **RAM** | 512 MB | 1 GB or more |
| **Storage** | 500 MB base disk space + your user file quota | SSD / NVMe storage |
| **Network** | Static or dynamic IPv4 / IPv6 with open inbound port | 100 Mbps+ uplink |

### Operating System Compatibility
- **Ubuntu**: 20.04, 22.04, 24.04 LTS
- **Debian**: 11 (Bullseye), 12 (Bookworm)
- **RHEL / Rocky Linux / AlmaLinux / CentOS Stream**: 8, 9
- **Fedora**: 38+
- **Arch Linux**
- **Alpine Linux** (with systemd or OpenRC)
- **macOS / Windows**: Supported for local development and testing

---

## âš¡ Fast Track Deployment

Run the automated one-line installer on your Linux VPS:

```bash
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/AlphaTechiess/alphadrive/main/install.sh)"
```

The installer will guide you through:
1. Choosing an access mode (**VPS IP:Port** or **Custom Domain + HTTPS**).
2. Provisioning reverse proxy and TLS certificates automatically (if domain mode is chosen).
3. Starting the sandboxed `systemd` daemon.

---

## ðŸ§™ First-Time Onboarding (Owner Setup)

When AlphaDrive runs on a clean database, it starts in **Setup Mode**.

1. Open your web browser and navigate to your AlphaDrive URL (e.g. `http://<your-vps-ip>:8080` or `https://drive.example.com`).
2. You will be greeted by the **Owner Setup** screen.
3. Complete the setup form:
   - **Full Name**: (e.g. `John Doe`)
   - **Username**: Desired administrator username (3â€“32 alphanumeric characters, e.g. `admin` or `jdoe`).
   - **Password**: Secure master password (**minimum 12 characters**).
4. Click **Submit**.
5. You are immediately logged in as the instance Owner and redirected to your **My Drive** dashboard.

> **Security Note**: Once the owner account is created, setup mode is permanently closed. Subsequent account creations must be performed by an administrator via the in-app user management modal.

---

## ðŸ§­ Navigating the Dashboard

- **My Drive**: Your root storage folder. Upload files, create folders, navigate breadcrumbs, select items, and manage links.
- **Trash**: Deleted items are staged in the Trash. Items can be restored to their original location or permanently deleted.
- **Account Settings** (Top-right avatar / Mobile Account tab): Change username, update password, or add new users (Admin only).
- **Storage Telemetry** (Sidebar / Storage modal): Real-time VPS disk space detector displaying total, used, and free filesystem capacity.