# Nginx + Counter API - Quick Start Guide

## Overview

This guide shows how to deploy Counter API behind nginx in production.

## Architecture

```
Internet
    |
    v
Nginx (port 80/443)
    |
    v (X-Real-IP header)
Counter API (localhost:8080)
```

**Why Nginx?**
- SSL termination
- Passes real client IP (fixes rate limiting)
- Additional security layer
- Load balancing (when you have multiple instances)
- Request buffering

---

## Installation Steps

### 1. Install Nginx

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install nginx

# RHEL/CentOS
sudo yum install nginx
```

### 2. Configure Nginx

```bash
# Copy the provided config
sudo cp nginx.conf /etc/nginx/sites-available/counter

# Edit domain name
sudo nano /etc/nginx/sites-available/counter
# Change api.example.com to your domain

# Enable site
sudo ln -s /etc/nginx/sites-available/counter /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload nginx
sudo systemctl reload nginx
```

### 3. Setup SSL (Let's Encrypt)

```bash
# Install certbot
sudo apt install certbot python3-certbot-nginx

# Get certificate (automatically configures nginx)
sudo certbot --nginx -d api.yourdomain.com

# Certificates auto-renew
sudo certbot renew --dry-run
```

### 4. Run Your API

```bash
# Run migrations
./counter --db-migrate=up

# Start in background
nohup ./counter > counter.log 2>&1 &

# Or use systemd (recommended)
sudo systemctl start counter
```

---

## Testing

### Test 1: Verify Nginx Passes Client IP

```bash
# From your local machine
curl -v https://api.yourdomain.com/tenants/test-tenant 2>&1 | grep -i "x-real-ip"

# Should see your IP address
```

### Test 2: Verify Rate Limiting Works

```bash
# Make 11 POST requests
for i in {1..11}; do
  curl -X POST https://api.yourdomain.com/tenants/test/counters/test/inc
done

# 11th request should return 429
```

### Test 3: Check SSL

```bash
# Test SSL configuration
curl https://www.ssllabs.com/ssltest/analyze.html?d=api.yourdomain.com

# Should get A or A+ rating
```

---

## Production Checklist

- Nginx installed and configured
- SSL certificate installed (Let's Encrypt)
- DNS configured (A record to your server)
- Firewall allows ports 80 and 443
- DATABASE_URL set in .env
- API_KEY set in .env (random, strong)
- Migrations run: `./counter --db-migrate=up`
- Service starts: `systemctl start counter`
- Service enabled on boot: `systemctl enable counter`
- Nginx reloaded: `systemctl reload nginx`
- Rate limiting tested
- SSL tested (ssltest.com)

---

## Common Issues

### Issue: "Rate limiting not working"

**Symptoms:** All requests appear to come from same IP

**Fix:** Ensure nginx is passing client IP:
```nginx
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

### Issue: "502 Bad Gateway"

**Symptoms:** Nginx can't reach your app

**Fixes:**
1. Check app is running: `systemctl status counter`
2. Check app is listening on 8080: `netstat -tlnp | grep 8080`
3. Check nginx can reach localhost: `curl http://localhost:8080`

### Issue: "SSL Certificate Error"

**Fix:**
```bash
# Revoke and get new certificate
sudo certbot revoke --cert-path /etc/letsencrypt/live/api.example.com/cert.pem
sudo certbot --nginx -d api.example.com
```

### Issue: "Rate Limit Too Aggressive"

**Fix:** Adjust in .env:
```bash
RATE_LIMIT_REQUESTS=100          # Increase for more lenient
RATE_LIMIT_GET_MULTIPLIER=5     # More GET requests
RATE_LIMIT_WINDOW=60            # Per minute
```

---

## Monitoring

### Check Rate Limit Status in Responses

```bash
# Look for these headers:
curl -I https://api.yourdomain.com/endpoint

X-RateLimit-Limit: 30          # Your limit
Retry-After: 45                # If rate limited
```

### View Logs

```bash
# Nginx access logs
sudo tail -f /var/log/nginx/api.example.com-access.log

# Nginx error logs
sudo tail -f /var/log/nginx/api.example.com-error.log

# Application logs
sudo journalctl -u counter -f
```

---

## Performance Tuning

### Nginx Worker Processes

Edit `/etc/nginx/nginx.conf`:
```nginx
worker_processes auto;
worker_connections 1024;
keepalive_timeout 65;
```

### Enable Gzip Compression

Add to nginx server block:
```nginx
gzip on;
gzip_vary on;
gzip_proxied any;
gzip_comp_level 6;
gzip_types text/plain text/css application/json application/javascript;
```

---

## Scaling Up

### Multiple API Instances

```nginx
upstream counter_api {
    server 127.0.0.1:8080;
    server 127.0.0.1:8081;
    server 127.0.0.1:8082;
    keepalive 32;
}
```

Then start multiple instances:
```bash
./counter --port 8080 &
./counter --port 8081 &
./counter --port 8082 &
```

---

## Security Hardening

### 1. Firewall Rules

```bash
# Only allow necessary ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 22/tcp  # SSH
sudo ufw enable
```

### 2. Hide Nginx Version

```nginx
# In http block
server_tokens off;
```

---

## Quick Reference

| Task | Command |
|------|---------|
| Reload nginx | `sudo systemctl reload nginx` |
| Restart nginx | `sudo systemctl restart nginx` |
| Start API | `sudo systemctl start counter` |
| Stop API | `sudo systemctl stop counter` |
| Restart API | `sudo systemctl restart counter` |
| View logs | `sudo journalctl -u counter -f` |
| Test nginx config | `sudo nginx -t` |
| Renew SSL | `sudo certbot renew` |

---

## Troubleshooting Commands

```bash
# Check if API is running
curl http://localhost:8080/tenants/test-tenant

# Check nginx is proxying correctly
curl -H "Host: api.example.com" http://localhost/

# Check SSL certificate
sudo certbot certificates

# Check nginx is listening
sudo netstat -tlnp | grep nginx

# Check API is listening
sudo netstat -tlnp | grep 8080

# Test from external client
curl https://api.yourdomain.com/health

# View recent 429 responses
sudo tail -f /var/log/nginx/api.example.com-access.log | grep " 429 "
```
