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

export DEBIAN_FRONTEND=noninteractive

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

# Remove cloud-init SSH override (Ubuntu 22.04/24.04 sets PasswordAuthentication there)
rm -f /etc/ssh/sshd_config.d/50-cloud-init.conf 2>/dev/null || true

# Some VPS images lack "Include /etc/ssh/sshd_config.d/*.conf" in sshd_config,
# so drop-in files get silently ignored. Apply settings to main config as well.
SSHD_CFG="/etc/ssh/sshd_config"

# Port
if grep -q "^Port " "$SSHD_CFG"; then
  sed -i "s/^Port .*/Port $SSH_PORT/" "$SSHD_CFG"
elif grep -q "^#Port " "$SSHD_CFG"; then
  sed -i "s/^#Port .*/Port $SSH_PORT/" "$SSHD_CFG"
else
  echo "Port $SSH_PORT" >> "$SSHD_CFG"
fi

# PermitRootLogin
if grep -q "^PermitRootLogin " "$SSHD_CFG"; then
  sed -i "s/^PermitRootLogin .*/PermitRootLogin no/" "$SSHD_CFG"
elif grep -q "^#PermitRootLogin " "$SSHD_CFG"; then
  sed -i "s/^#PermitRootLogin .*/PermitRootLogin no/" "$SSHD_CFG"
else
  echo "PermitRootLogin no" >> "$SSHD_CFG"
fi

# PasswordAuthentication
if grep -q "^PasswordAuthentication " "$SSHD_CFG"; then
  sed -i "s/^PasswordAuthentication .*/PasswordAuthentication no/" "$SSHD_CFG"
elif grep -q "^#PasswordAuthentication " "$SSHD_CFG"; then
  sed -i "s/^#PasswordAuthentication .*/PasswordAuthentication no/" "$SSHD_CFG"
else
  echo "PasswordAuthentication no" >> "$SSHD_CFG"
fi

# PubkeyAuthentication
if grep -q "^PubkeyAuthentication " "$SSHD_CFG"; then
  sed -i "s/^PubkeyAuthentication .*/PubkeyAuthentication yes/" "$SSHD_CFG"
elif grep -q "^#PubkeyAuthentication " "$SSHD_CFG"; then
  sed -i "s/^#PubkeyAuthentication .*/PubkeyAuthentication yes/" "$SSHD_CFG"
else
  echo "PubkeyAuthentication yes" >> "$SSHD_CFG"
fi

# Restart SSH — Ubuntu 24.04 uses socket-activated "ssh", older uses "sshd".
# systemctl list-units may not show socket-activated services, so just try directly.
if systemctl restart ssh 2>/dev/null; then
  echo "  Restarted ssh (Ubuntu 24.04+)"
elif systemctl restart sshd 2>/dev/null; then
  echo "  Restarted sshd"
else
  echo "  WARNING: Could not restart SSH service. Restart manually!"
fi

# Verify SSH is listening on the correct port
sleep 1
if ss -tlnp | grep -q ":$SSH_PORT"; then
  echo "  SSH confirmed on port $SSH_PORT"
else
  echo "  WARNING: SSH not detected on port $SSH_PORT. Check sshd_config manually."
fi

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
# Docker bypasses UFW (writes iptables directly), so ufw deny has no effect
# on Docker-published ports. Use DOCKER-USER chain instead.
echo ">>> Closing Dokploy UI port 3000 (use SSH tunnel)..."

# Wait for Docker to create DOCKER-USER chain (Dokploy starts containers)
for i in $(seq 1 30); do
  if iptables -L DOCKER-USER -n &>/dev/null; then
    break
  fi
  echo "  Waiting for Docker DOCKER-USER chain... ($i/30)"
  sleep 2
done

if iptables -L DOCKER-USER -n &>/dev/null; then
  # Allow localhost (SSH tunnel), drop everything else to port 3000
  iptables -I DOCKER-USER -s 127.0.0.1 -p tcp --dport 3000 -j ACCEPT
  iptables -I DOCKER-USER 2 -p tcp --dport 3000 -j DROP

  # Persist iptables rules across reboots
  echo iptables-persistent iptables-persistent/autosave_v4 boolean true | debconf-set-selections
  echo iptables-persistent iptables-persistent/autosave_v6 boolean true | debconf-set-selections
  apt-get install -y -qq iptables-persistent > /dev/null
  netfilter-persistent save > /dev/null 2>&1

  echo "  Port 3000 blocked via iptables DOCKER-USER chain"
else
  echo "  WARNING: DOCKER-USER chain not found. Block port 3000 manually:"
  echo "    iptables -I DOCKER-USER -s 127.0.0.1 -p tcp --dport 3000 -j ACCEPT"
  echo "    iptables -I DOCKER-USER 2 -p tcp --dport 3000 -j DROP"
fi
echo "  Access via: ssh -L 3000:localhost:3000 deploy@<vps-ip> -p $SSH_PORT"

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
