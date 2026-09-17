# AlphaDrive Documentation

Welcome to the comprehensive AlphaDrive documentation. This documentation covers everything from initial server provisioning to advanced reverse proxy setups, REST API integration, security architecture, and disaster recovery.

---

## ðŸ“š Documentation Index

### 1. Getting Started & Installation
- **[Getting Started](GETTING_STARTED.md)** â€” Hardware requirements, operating system support, and first-time setup wizard.
- **[Installation Guide](INSTALLATION.md)** â€” One-line automated VPS installer, headless script options, manual binary installation, and building from source.
- **[Installer Deep-Dive](INSTALLER.md)** â€” Detailed specification and operational mechanics of the `install.sh` deployment script.

### 2. Configuration & Networking
- **[Configuration Reference](CONFIGURATION.md)** â€” Environment variables, ports, directory paths, and runtime settings.
- **[Reverse Proxy & HTTPS Setup](REVERSE_PROXY.md)** â€” Production-ready configurations for Caddy, Nginx, Traefik, Apache, and Cloudflare.

### 3. Usage & Features
- **[User & Account Management](USER_MANAGEMENT.md)** â€” Initial owner setup, administrator privileges, creating users, changing usernames, and updating passwords.
- **[File & Storage Management](FILE_MANAGEMENT.md)** â€” File and folder uploads, nested navigation, media previews (image, video, audio, PDF, code), trash management, and VPS disk quota telemetry.
- **[Public Sharing Engine](PUBLIC_SHARING.md)** â€” Public links, custom slugs, Argon2id password protection, expiration timers, instant revocation, and streaming ZIP downloads.

### 4. Operations & Maintenance
- **[CLI Reference & Diagnostics](CLI_REFERENCE.md)** â€” Server commands, `alphadrive doctor` diagnostics, `alphadrive-update`, and uninstallation.
- **[Backup & Disaster Recovery](BACKUP_AND_RESTORE.md)** â€” Hot online SQLite backups, opaque file archive exports, and complete multi-server restores.
- **[Troubleshooting & FAQs](TROUBLESHOOTING.md)** â€” Solutions to common networking, permissions, proxy, and service issues.

### 5. Developer & Architecture
- **[Architecture & Security](ARCHITECTURE.md)** â€” Single-binary philosophy, embedded SQLite WAL mode, Opaque Storage Engine, and defense-in-depth security model.
- **[REST API Reference](API_REFERENCE.md)** â€” Complete HTTP REST API specification with request/response payloads, authentication, and error codes.

---

## ðŸš€ Quick Links
- **GitHub Repository**: [AlphaTechiess/alphadrive](https://github.com/AlphaTechiess/alphadrive)
- **Report an Issue**: [GitHub Issues](https://github.com/AlphaTechiess/alphadrive/issues)
- **Support & Contributions**: [Contribute with Razorpay](https://rzp.io/rzp/alphadrive)