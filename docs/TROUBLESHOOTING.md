# Troubleshooting & Frequently Asked Questions

Common issues and solutions when operating AlphaDrive.

---

## ðŸ” Common Issues & Solutions

### 1. Port Conflict (`bind: address already in use`)
- **Cause**: Another service is running on port 8080 (e.g. Apache, Docker, or Tomcat).
- **Solution**: Change the port in `/etc/alphadrive/alphadrive.env`:
  ```ini
  ALPHADRIVE_PORT=8090
  ```
  Restart the service: `sudo systemctl restart alphadrive`.

### 2. 502 Bad Gateway with Reverse Proxy
- **Cause**: AlphaDrive daemon is stopped or reverse proxy configuration points to the wrong port.
- **Solution**:
  1. Check service status: `sudo systemctl status alphadrive`.
  2. Verify backend binding in proxy config (`127.0.0.1:8080`).
  3. Inspect logs: `sudo journalctl -u alphadrive -n 50 -f`.

### 3. File Uploads Failing on Large Files
- **Cause**: Reverse proxy request size limit (e.g. Nginx `client_max_body_size`) is capping upload size.
- **Solution**:
  - In Nginx: set `client_max_body_size 0;` in your site configuration.
  - In Caddy: set `request_body { max_size 100GB }` in your Caddyfile.
  - In Cloudflare: If using free tier, bypass Cloudflare proxy (Grey Cloud) for direct high-speed transfers exceeding 100 MB.

### 4. Permission Denied Errors in Logs
- **Cause**: The `alphadrive` system user does not own `/var/lib/alphadrive`.
- **Solution**:
  ```bash
  sudo chown -R alphadrive:alphadrive /var/lib/alphadrive
  sudo chmod -R 750 /var/lib/alphadrive
  sudo systemctl restart alphadrive
  ```

---

## ðŸ› ï¸ Diagnostics with `alphadrive doctor`

To run an automated health check:

```bash
sudo -u alphadrive alphadrive doctor --data-dir /var/lib/alphadrive/data
```
This checks OS, permissions, SQLite WAL mode, database schemas, object storage, and port availability.