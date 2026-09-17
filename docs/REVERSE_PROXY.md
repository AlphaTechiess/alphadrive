# Reverse Proxy & HTTPS Configuration

AlphaDrive is optimized to run behind modern reverse proxies such as Caddy, Nginx, Traefik, or Apache for automated SSL/TLS certificate management, HTTP/2 or HTTP/3 termination, and performance.

---

## 1. Caddy (Recommended)

Caddy provides automatic HTTPS with Let's Encrypt / ZeroSSL, automatic renewals, and HTTP/3 support with zero maintenance.

### `/etc/caddy/Caddyfile`
```caddyfile
drive.example.com {
    encode zstd gzip

    # Reverse proxy to AlphaDrive backend
    reverse_proxy 127.0.0.1:8080 {
        # Streaming settings for large file transfers
        flush_interval -1
    }

    # Security Headers
    header {
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "strict-origin-when-cross-origin"
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    }

    # Request size limit (100 GB or 0 for unlimited)
    request_body {
        max_size 100GB
    }
}
```

### Reload Caddy:
```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

---

## 2. Nginx + Certbot (Let's Encrypt)

### `/etc/nginx/sites-available/alphadrive.conf`
```nginx
server {
    listen 80;
    server_name drive.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name drive.example.com;

    # SSL Certificates (Managed by Certbot)
    ssl_certificate /etc/letsencrypt/live/drive.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/drive.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # Maximum upload size (0 = unlimited)
    client_max_body_size 0;
    client_body_buffer_size 512k;
    client_body_timeout 300s;

    # Security Headers
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        # Proxy Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable buffering for chunked uploads & video streaming
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

### Enable & Reload Nginx:
```bash
sudo ln -sf /etc/nginx/sites-available/alphadrive.conf /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## 3. Traefik (Docker / Dynamic File Provider)

### `/etc/traefik/dynamic/alphadrive.yml`
```yaml
http:
  routers:
    alphadrive:
      rule: "Host(`drive.example.com`)"
      entryPoints:
        - websecure
      service: alphadrive-service
      tls:
        certResolver: letsencrypt

  services:
    alphadrive-service:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8080"
```

---

## 4. Cloudflare DNS & Proxy Notice

If proxying through Cloudflare (Orange Cloud enabled):
1. Set **SSL/TLS Encryption mode** in Cloudflare to **Full (strict)**.
2. Note that Cloudflare Free tier has a 100MB client upload limit per request unless using chunked uploads or disabling the proxy (Grey Cloud) for direct high-speed transfers.