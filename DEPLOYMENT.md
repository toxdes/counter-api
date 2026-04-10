# Deployment Guide - IP Spoofing Protection

## Overview

The rate limiter now only trusts `X-Real-IP` and `X-Forwarded-For` headers from **trusted proxies** (localhost/private networks) to prevent IP spoofing attacks.

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

### ⚠️ Behind CDN (Cloudflare, Fastly, AWS CloudFront)

**Problem:** CDN IPs are **public**, not trusted by default.

**Solution: Whitelist CDN IP Ranges**

Add to your deployment:

```go
// In production, configure trusted proxy CIDR ranges
var trustedProxyCIDRs = []string{
    "127.0.0.1/32",           // Localhost
    "10.0.0.0/8",             // Private network
    "172.16.0.0/12",          // Private network
    "192.168.0.0/16",         // Private network
    // Add your CDN/proxy ranges:
    "173.245.48.0/20",        // Cloudflare (example)
    "167.82.0.0/17",          // Fastly (example)
}

func isTrustedProxy(ip net.IP) bool {
    for _, cidr := range trustedProxyCIDRs {
        _, network, _ := net.ParseCIDR(cidr)
        if network.Contains(ip) {
            return true
        }
    }
    return false
}
```

**Cloudflare specific:**
```bash
# Cloudflare IP ranges (from https://www.cloudflare.com/ips/)
173.245.48.0/20
103.21.244.0/22
103.22.200.0/22
103.31.4.0/22
141.101.64.0/18
108.162.192.0/18
190.93.240.0/20
188.114.96.0/20
197.234.240.0/22
198.41.128.0/17
162.158.0.0/15
192.0.0.0/24
```

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
