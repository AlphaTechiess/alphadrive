# AlphaDrive Configuration Reference

AlphaDrive is configured using environment variables, command-line flags, or an environment file (`/etc/alphadrive/alphadrive.env`).

---

## âš™ï¸ Environment Variables Reference

| Variable | Description | Default Value | Example |
| :--- | :--- | :--- | :--- |
| `ALPHADRIVE_PORT` | HTTP port to listen on | `8080` | `8080` or `3000` |
| `ALPHADRIVE_BIND` | Interface address to bind | `0.0.0.0` | `127.0.0.1` (behind proxy) |
| `ALPHADRIVE_DATA_DIR` | Base path for database, sessions, and files | `./data` | `/var/lib/alphadrive/data` |
| `ALPHADRIVE_DATABASE_PATH` | Custom SQLite database file location | `<data_dir>/alphadrive.db` | `/var/lib/alphadrive/data/alphadrive.db` |
| `ALPHADRIVE_FILES_DIR` | Custom path for uploaded binary object files | `<data_dir>/files/objects` | `/mnt/storage/objects` |
| `ALPHADRIVE_SESSION_KEY` | Persistent 32-byte secret for cookie hashing | Auto-generated in data dir | `hex_encoded_32_bytes` |

---

## ðŸ“ Directory Structure Layout

When AlphaDrive runs, it maintains the following sandboxed directory hierarchy:

```
/var/lib/alphadrive/data/
â”œâ”€â”€ alphadrive.db          # Primary SQLite database (WAL mode)
â”œâ”€â”€ alphadrive.db-wal      # SQLite Write-Ahead Log
â”œâ”€â”€ alphadrive.db-shm      # SQLite Shared Memory index
â”œâ”€â”€ session.key            # Cryptographic session key (0600 permissions)
â””â”€â”€ files/
    â””â”€â”€ objects/           # Content-addressed opaque binary objects
        â”œâ”€â”€ 3a/
        â”‚   â””â”€â”€ 3a7f8e...  # Encapsulated file blobs (no plain filenames)
        â””â”€â”€ b4/
            â””â”€â”€ b49921...
```

---

## ðŸ”’ Security Best Practices

1. **Unprivileged Execution**: Never run AlphaDrive as `root`. Use the `alphadrive` system user.
2. **Reverse Proxy Binding**: When running behind Caddy, Nginx, or Traefik, always set `ALPHADRIVE_BIND=127.0.0.1` so the port is not exposed directly to the public internet.
3. **HTTPS / TLS**: Always terminate TLS at your reverse proxy in production to ensure all credentials and file transfers are encrypted in transit.
4. **File Permissions**: Ensure `/var/lib/alphadrive` has ownership `alphadrive:alphadrive` and mode `0700` or `0750`.