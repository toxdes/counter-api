# Deployment Guide - IP Spoofing Protection

## Overview

The rate limiter trusts forwarded client-IP headers only from the local nginx
proxy. The application binds to `127.0.0.1` by default, so the public ingress
path should be Cloudflare (if used) → nginx → Counter API.

## Deployment Configurations

### ✅ Direct Deployment (Simplest)

Run your app directly on the server:
```bash
./counter
```

- Rate limiting uses direct client IP
- No proxy configuration needed

---

### ✅ Behind Nginx/Traefik (Recommended)

**Nginx Configuration:**
```nginx
server {
    listen 80;
    server_name api.example.com;

    location / {
        proxy_pass http://localhost:8080;

        # Pass real client IP
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;

        # Standard proxy headers
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
    }
}
```

**Your .env:**
```bash
SERVER_HOST=127.0.0.1
SERVER_PORT=8080
```

**Result:** Rate limiting works correctly using real client IPs

---

### ✅ Behind Docker/Kubernetes

**docker-compose.yml:**
```yaml
version: '3'
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    depends_on:
      - api

  api:
    build: .
    environment:
      - SERVER_HOST=0.0.0.0
      - SERVER_PORT=8080
    expose:
      - "8080"
```

**nginx.conf:**
```nginx
upstream api {
    server api:8080;
}

server {
    listen 80;
    location / {
        proxy_pass http://api;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
    }
}
```

---

### ⚠️ Behind Cloudflare

Use the repository's `nginx.conf`, which trusts Cloudflare CIDRs and consumes
`CF-Connecting-IP` before forwarding the normalized address to Go. Do not add
Cloudflare CIDRs to the Go application: Go should trust only its loopback nginx
peer. Keep the VPS firewall restricted to Cloudflare's current IPv4 and IPv6
ranges, and update the nginx allowlist when Cloudflare changes them.

---

## Testing Your Deployment

### Test 1: Verify Rate Limiting Works

```bash
# Make 11 requests rapidly
for i in {1..11}; do
  curl -w "Status: %{http_code}\n" -X POST \
    http://localhost:8080/tenants/test/counters/test/inc
done
```

**Expected:** 11th request returns 429

---

### Test 2: Verify IP Detection

```bash
# Check what IP the server sees
curl -v http://your-api.com/endpoint 2>&1 | grep -i "x-ratelimit"
```

---

### Test 3: Load Test from Multiple IPs

```bash
# Simulate requests from different IPs
curl -H "X-Forwarded-For: 1.2.3.4" http://your-api.com/endpoint
curl -H "X-Forwarded-For: 5.6.7.8" http://your-api.com/endpoint
```

**Expected:** Both use your server's IP (spoofing attempt blocked)

---

## Common Issues

### Issue: "All users rate-limited as one user"

**Cause:** Your proxy/CDN not recognized as trusted

**Fix:** Add proxy IP ranges to `isTrustedProxy()` function

---

### Issue: "Rate limiting not working"

**Cause:** Requests coming through trusted proxy but headers not set

**Fix:** Ensure proxy sets `X-Real-IP` or `X-Forwarded-For` header

---

## Production Checklist

- [ ] Deploy behind nginx/traefik (recommended)
- [ ] Configure proxy to pass client IP headers
- [ ] Test rate limiting from multiple sources
- [ ] If using CDN, whitelist CDN IP ranges
- [ ] Monitor for "all users rate-limited as one" issue
- [ ] Set up alerts for 429 response rates

---

## Security Best Practices

1. **Never expose app directly to internet** without proxy in production
2. **Use nginx/traefik** as entry point for rate limiting, SSL, etc.
3. **Monitor IP distribution** in logs to detect spoofing attempts
4. **Keep trusted proxy list minimal** - only add ranges you control
5. **Regularly update** CDN/proxy IP ranges

---

## Quick Reference

| Deployment | Works? | Changes Needed |
|------------|--------|----------------|
| Direct (no proxy) | ✅ Yes | None |
| Nginx (localhost) | ✅ Yes | Add X-Real-IP header |
| Traefik | ✅ Yes | Add X-Real-IP header |
| Docker/K8s | ✅ Yes | Sidecar proxy configuration |
| AWS ALB | ✅ Yes | Usually works (private IP) |
| Cloudflare | ⚠️ Maybe | Whitelist Cloudflare IPs |
| Fastly | ⚠️ Maybe | Whitelist Fastly IPs |
| HAProxy (public) | ❌ No | Use private IP or whitelist |
