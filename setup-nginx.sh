#!/bin/bash

# Setup script for deploying Counter API with Nginx
# Run with sudo

set -e

echo "🚀 Setting up Counter API with Nginx..."

# Configuration
DOMAIN=${DOMAIN:-"api.example.com"}
NGINX_SITES_AVAILABLE="/etc/nginx/sites-available"
NGINX_SITES_ENABLED="/etc/nginx/sites-enabled"

echo "📋 Configuration:"
echo "   Domain: $DOMAIN"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "❌ Please run as root (sudo $0)"
    exit 1
fi

# Update domain in nginx config
echo "📝 Configuring nginx for $DOMAIN..."
sed -i "s/api.example.com/$DOMAIN/g" /home/bets/pro/real/counter/nginx.conf

# Copy nginx config
echo "📦 Installing nginx configuration..."
cp /home/bets/pro/real/counter/nginx.conf "$NGINX_SITES_AVAILABLE/counter-api"

# Enable site
echo "🔗 Enabling site..."
ln -sf "$NGINX_SITES_AVAILABLE/counter-api" "$NGINX_SITES_ENABLED/counter-api"

# Test nginx configuration
echo "🧪 Testing nginx configuration..."
nginx -t

# Install certbot for Let's Encrypt if not present
if ! command -v certbot &> /dev/null; then
    echo "📜 Installing certbot..."
    apt update
    apt install -y certbot python3-certbot-nginx
fi

# Ask about SSL
echo ""
read -p "🔐 Do you want to set up SSL with Let's Encrypt? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🔐 Obtaining SSL certificate for $DOMAIN..."
    certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --email admin@$DOMAIN --redirect

    echo "✅ SSL certificate installed!"
    echo "📅 Certificate will auto-renew"
else
    echo "⚠️  Skipping SSL setup (HTTP only)"
    echo "   You can run later: certbot --nginx -d $DOMAIN"
fi

# Create systemd service for the app
echo ""
echo "🔧 Creating systemd service..."

cat > /etc/systemd/system/counter-api.service <<EOF
[Unit]
Description=Counter API
After=network.target nginx.service

[Service]
Type=simple
User=$SUDO_USER
WorkingDirectory=/home/bets/pro/real/counter
ExecStart=/home/bets/pro/real/counter/counter
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=counter-api

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/home/bets/pro/real/counter

# Environment
Environment="DATABASE_URL=$DATABASE_URL"
EnvironmentFile=/home/bets/pro/real/counter/.env

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
systemctl daemon-reload

echo ""
echo "✅ Setup complete!"
echo ""
echo "📝 Next steps:"
echo "   1. Update .env with your production settings"
echo "   2. Run migrations: ./counter --db-migrate=up"
echo "   3. Start service: sudo systemctl start counter-api"
echo "   4. Enable on boot: sudo systemctl enable counter-api"
echo "   5. Reload nginx: sudo systemctl reload nginx"
echo ""
echo "🔍 Useful commands:"
echo "   View logs: sudo journalctl -u counter-api -f"
echo "   Restart: sudo systemctl restart counter-api"
echo "   Status: sudo systemctl status counter-api"
echo ""
echo "📊 Monitor:"
echo "   Rate limits: Check X-RateLimit headers in responses"
echo "   Nginx logs: tail -f /var/log/nginx/counter-api-error.log"
echo "   App logs: sudo journalctl -u counter-api -f"
