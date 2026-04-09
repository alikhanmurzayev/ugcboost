#!/usr/bin/env bash
#
# UGCBoost VPS Bootstrap Script
#
# Usage (run as root on a fresh Ubuntu 22.04/24.04 VPS):
#   curl -sSL <raw-url>/bootstrap.sh | bash -s -- --env staging --domain ugcboost.kz
#
# Or locally:
#   scp bootstrap.sh root@<vps-ip>:/tmp/
#   ssh root@<vps-ip> 'bash /tmp/bootstrap.sh --env staging --domain ugcboost.kz'
#
# What it does:
#   1. Creates 'deploy' user with SSH key
#   2. SSH hardening (custom port, disable passwords)
#   3. UFW firewall (Cloudflare IPs only for HTTP/HTTPS)
#   4. fail2ban
#   5. Docker
#   6. Dokploy
#   7. Locks down Dokploy UI (port 3000 closed, SSH tunnel only)

set -euo pipefail

# --- Parse arguments ---
ENV=""
DOMAIN=""
SSH_PORT=2222

while [[ $# -gt 0 ]]; do
  case $1 in
    --env)     ENV="$2"; shift 2 ;;
    --domain)  DOMAIN="$2"; shift 2 ;;
    --ssh-port) SSH_PORT="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$ENV" || -z "$DOMAIN" ]]; then
  echo "Usage: $0 --env <staging|production> --domain <domain>"
  exit 1
fi

echo "=== UGCBoost Bootstrap: env=$ENV domain=$DOMAIN ssh_port=$SSH_PORT ==="

# --- 1. Create deploy user ---
echo ">>> Creating deploy user..."
if ! id deploy &>/dev/null; then
  adduser --disabled-password --gecos "" deploy
  usermod -aG sudo deploy
  echo "deploy ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/deploy
fi

mkdir -p /home/deploy/.ssh
chmod 700 /home/deploy/.ssh

# Copy root's authorized_keys if they exist
if [[ -f /root/.ssh/authorized_keys ]]; then
  cp /root/.ssh/authorized_keys /home/deploy/.ssh/authorized_keys
  chmod 600 /home/deploy/.ssh/authorized_keys
  chown -R deploy:deploy /home/deploy/.ssh
  echo "  Copied SSH keys from root to deploy"
else
  echo "  WARNING: No /root/.ssh/authorized_keys found."
  echo "  Add your SSH public key to /home/deploy/.ssh/authorized_keys manually!"
fi

# --- 2. SSH hardening ---
echo ">>> SSH hardening (port $SSH_PORT)..."
SSHD_CONFIG="/etc/ssh/sshd_config"
cp "$SSHD_CONFIG" "${SSHD_CONFIG}.bak"

sed -i "s/^#\?Port .*/Port $SSH_PORT/" "$SSHD_CONFIG"
sed -i "s/^#\?PermitRootLogin .*/PermitRootLogin no/" "$SSHD_CONFIG"
sed -i "s/^#\?PasswordAuthentication .*/PasswordAuthentication no/" "$SSHD_CONFIG"
sed -i "s/^#\?PubkeyAuthentication .*/PubkeyAuthentication yes/" "$SSHD_CONFIG"

systemctl restart sshd
echo "  SSH reconfigured on port $SSH_PORT"

# --- 3. UFW firewall ---
echo ">>> Configuring UFW..."
apt-get update -qq && apt-get install -y -qq ufw > /dev/null

ufw --force reset > /dev/null
ufw default deny incoming
ufw default allow outgoing

# SSH
ufw allow "$SSH_PORT/tcp" comment "SSH"

# Cloudflare IP ranges (HTTP/HTTPS only from Cloudflare)
CF_IPV4=(
  173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22
  141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20
  197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13
  104.24.0.0/14 172.64.0.0/13 131.0.72.0/22
)

for ip in "${CF_IPV4[@]}"; do
  ufw allow from "$ip" to any port 80,443 proto tcp comment "Cloudflare" > /dev/null
done

ufw --force enable
echo "  UFW enabled: SSH($SSH_PORT) + HTTP/HTTPS from Cloudflare only"

# --- 4. fail2ban ---
echo ">>> Installing fail2ban..."
apt-get install -y -qq fail2ban > /dev/null

cat > /etc/fail2ban/jail.local <<JAIL
[sshd]
enabled = true
port = $SSH_PORT
maxretry = 5
bantime = 3600
findtime = 600
JAIL

systemctl enable fail2ban
systemctl restart fail2ban
echo "  fail2ban configured"

# --- 5. Docker ---
echo ">>> Installing Docker..."
if ! command -v docker &>/dev/null; then
  curl -fsSL https://get.docker.com | sh
  usermod -aG docker deploy
  echo "  Docker installed"
else
  echo "  Docker already installed"
fi

# --- 6. Dokploy ---
echo ">>> Installing Dokploy..."
if ! docker ps --format '{{.Names}}' | grep -q dokploy; then
  curl -sSL https://dokploy.com/install.sh | sh
  echo "  Dokploy installed"
else
  echo "  Dokploy already running"
fi

# --- 7. Close Dokploy UI port (SSH tunnel only) ---
echo ">>> Closing Dokploy UI port 3000 (use SSH tunnel)..."
ufw deny 3000/tcp comment "Dokploy UI - SSH tunnel only" > /dev/null
echo "  Port 3000 blocked. Access via: ssh -L 3000:localhost:3000 deploy@<vps-ip> -p $SSH_PORT"

# --- Done ---
echo ""
echo "=== Bootstrap complete ==="
echo ""
echo "Next steps:"
echo "  1. Test SSH: ssh deploy@<vps-ip> -p $SSH_PORT"
echo "  2. Access Dokploy: ssh -L 3000:localhost:3000 deploy@<vps-ip> -p $SSH_PORT"
echo "     Then open http://localhost:3000 in browser"
echo "  3. Configure Cloudflare DNS for $DOMAIN"
echo "  4. Create Dokploy project and add services"
