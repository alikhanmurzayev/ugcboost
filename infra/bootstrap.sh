#!/usr/bin/env bash
#
# UGCBoost VPS Bootstrap Script (idempotent — safe to re-run)
#
# Usage:
#   scp bootstrap.sh root@<vps-ip>:/tmp/
#   ssh root@<vps-ip> 'bash /tmp/bootstrap.sh'
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

# --- Must run as root ---
if [[ $EUID -ne 0 ]]; then
  echo "ERROR: This script must be run as root"
  exit 1
fi

# --- Config ---
SSH_PORT=2222

echo "=== UGCBoost Bootstrap: ssh_port=$SSH_PORT ==="

export DEBIAN_FRONTEND=noninteractive

# --- Helper: set sshd_config directive (idempotent) ---
sshd_set() {
  local key="$1" value="$2" cfg="/etc/ssh/sshd_config"
  if grep -q "^${key} " "$cfg"; then
    sed -i "s/^${key} .*/${key} ${value}/" "$cfg"
  elif grep -q "^#${key} " "$cfg"; then
    sed -i "s/^#${key} .*/${key} ${value}/" "$cfg"
  else
    echo "${key} ${value}" >> "$cfg"
  fi
}

# --- 0. Update and upgrade packages ---
echo ">>> Updating and upgrading packages..."
apt-get update -qq
apt-get upgrade -y -qq > /dev/null
apt-get install -y -qq openssh-server > /dev/null

# --- 1. Create deploy user ---
echo ">>> Creating deploy user..."
if ! id deploy &>/dev/null; then
  adduser --disabled-password --gecos "" deploy
  echo "  Created user 'deploy'"
else
  echo "  User 'deploy' already exists"
fi

usermod -aG sudo deploy
echo "deploy ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/deploy
chmod 440 /etc/sudoers.d/deploy
visudo -cf /etc/sudoers.d/deploy

mkdir -p /home/deploy/.ssh
chmod 700 /home/deploy/.ssh

if [[ ! -s /root/.ssh/authorized_keys ]]; then
  echo "  FATAL: /root/.ssh/authorized_keys missing or empty."
  echo "  Cannot proceed — SSH hardening would lock out all access."
  exit 1
fi

if [[ ! -s /home/deploy/.ssh/authorized_keys ]]; then
  cp /root/.ssh/authorized_keys /home/deploy/.ssh/authorized_keys
  chmod 600 /home/deploy/.ssh/authorized_keys
  chown -R deploy:deploy /home/deploy/.ssh
  echo "  SSH keys copied from root to deploy"
else
  echo "  deploy already has authorized_keys, skipping"
fi

# --- 2. SSH hardening ---
echo ">>> SSH hardening (port $SSH_PORT)..."

rm -f /etc/ssh/sshd_config.d/50-cloud-init.conf 2>/dev/null || true

sshd_set Port "$SSH_PORT"
sshd_set PermitRootLogin no
sshd_set PasswordAuthentication no
sshd_set PubkeyAuthentication yes

# Validate config before restarting — broken config = lockout
mkdir -p /run/sshd
if ! sshd -t; then
  echo "  FATAL: sshd config validation failed!"
  exit 1
fi

# Ubuntu 22.10+ uses socket activation with sshd-socket-generator,
# which auto-reads Port from sshd_config and creates dual-stack listeners.
# We only need daemon-reload + restart. No manual socket override needed.
if systemctl is-active ssh.socket &>/dev/null; then
  echo "  Socket activation detected — reloading"
  # Remove any stale manual overrides (they break the generator's dual-stack config)
  rm -rf /etc/systemd/system/ssh.socket.d
  systemctl daemon-reload
  systemctl restart ssh.socket
else
  # Traditional SSH (Ubuntu < 22.10, other distros)
  if systemctl restart ssh 2>/dev/null; then
    true
  elif systemctl restart sshd 2>/dev/null; then
    true
  else
    echo "  WARNING: Could not restart SSH service. Restart manually!"
  fi
fi

sleep 1
if ss -tlnp | grep -q ":${SSH_PORT}\b"; then
  echo "  SSH confirmed on port $SSH_PORT"
else
  echo "  ERROR: SSH not listening on port $SSH_PORT!"
  echo "  Debug: $(ss -tlnp | grep ssh)"
  exit 1
fi

# --- 3. UFW firewall ---
echo ">>> Configuring UFW..."
apt-get install -y -qq ufw > /dev/null

ufw --force reset > /dev/null
ufw default deny incoming
ufw default allow outgoing

ufw allow "$SSH_PORT/tcp" comment "SSH"

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
echo ">>> Configuring fail2ban..."
apt-get install -y -qq fail2ban > /dev/null

cat > /etc/fail2ban/jail.local <<JAIL
[sshd]
enabled = true
port = $SSH_PORT
maxretry = 5
bantime = 3600
findtime = 600
backend = systemd
JAIL

systemctl enable fail2ban
systemctl restart fail2ban
echo "  fail2ban configured"

# --- 5. Docker ---
echo ">>> Installing Docker..."
if ! command -v docker &>/dev/null; then
  curl -fsSL --retry 3 --max-time 120 https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
  rm -f /tmp/get-docker.sh
  echo "  Docker installed"
else
  echo "  Docker already installed"
fi

# Verify Docker is running
if ! systemctl is-active docker &>/dev/null; then
  echo "  FATAL: Docker installed but not running!"
  systemctl status docker --no-pager || true
  exit 1
fi

usermod -aG docker deploy

# --- 6. Dokploy ---
echo ">>> Installing Dokploy..."
if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q dokploy; then
  curl -sSL --retry 3 --max-time 120 https://dokploy.com/install.sh -o /tmp/install-dokploy.sh
  sh /tmp/install-dokploy.sh
  rm -f /tmp/install-dokploy.sh
  echo "  Dokploy installed"
else
  echo "  Dokploy already running"
fi

# --- 7. Close Dokploy UI port (SSH tunnel only) ---
# Docker bypasses UFW (writes iptables directly), so ufw deny has no effect
# on Docker-published ports. Use DOCKER-USER chain instead.
echo ">>> Closing Dokploy UI port 3000 (use SSH tunnel)..."

for i in $(seq 1 30); do
  if iptables -L DOCKER-USER -n &>/dev/null; then
    break
  fi
  echo "  Waiting for Docker DOCKER-USER chain... ($i/30)"
  sleep 2
done

if iptables -L DOCKER-USER -n &>/dev/null; then
  # Idempotent: check before adding
  if ! iptables -C DOCKER-USER -s 127.0.0.1 -p tcp --dport 3000 -j ACCEPT 2>/dev/null; then
    iptables -I DOCKER-USER -s 127.0.0.1 -p tcp --dport 3000 -j ACCEPT
  fi
  if ! iptables -C DOCKER-USER -p tcp --dport 3000 -j DROP 2>/dev/null; then
    iptables -A DOCKER-USER -p tcp --dport 3000 -j DROP
  fi

  echo iptables-persistent iptables-persistent/autosave_v4 boolean true | debconf-set-selections
  echo iptables-persistent iptables-persistent/autosave_v6 boolean true | debconf-set-selections
  apt-get install -y -qq iptables-persistent > /dev/null
  netfilter-persistent save > /dev/null 2>&1

  echo "  Port 3000 blocked via iptables DOCKER-USER chain"
else
  echo "  WARNING: DOCKER-USER chain not found. Block port 3000 manually:"
  echo "    iptables -I DOCKER-USER -s 127.0.0.1 -p tcp --dport 3000 -j ACCEPT"
  echo "    iptables -A DOCKER-USER -p tcp --dport 3000 -j DROP"
fi

# --- 8. Final verification ---
echo ""
echo ">>> Verifying..."

ERRORS=0

if ss -tlnp | grep -q ":${SSH_PORT}\b"; then
  echo "  [OK] SSH on port $SSH_PORT"
else
  echo "  [FAIL] SSH not on port $SSH_PORT"
  ERRORS=$((ERRORS + 1))
fi

if command -v docker &>/dev/null; then
  echo "  [OK] Docker installed"
else
  echo "  [FAIL] Docker not found"
  ERRORS=$((ERRORS + 1))
fi

if docker ps --format '{{.Names}}' 2>/dev/null | grep -q dokploy; then
  echo "  [OK] Dokploy running"
else
  echo "  [FAIL] Dokploy not running"
  ERRORS=$((ERRORS + 1))
fi

if curl -sf --max-time 3 http://localhost:3000 > /dev/null 2>&1; then
  echo "  [OK] Dokploy UI responding on localhost:3000"
else
  echo "  [WARN] Dokploy UI not responding on localhost:3000 yet (may need a minute)"
fi

echo ""
if [[ $ERRORS -gt 0 ]]; then
  echo "=== Bootstrap completed with $ERRORS error(s) ==="
else
  echo "=== Bootstrap complete ==="
fi
echo ""
echo "Next steps:"
echo "  1. Test SSH: ssh deploy@<vps-ip> -p $SSH_PORT"
echo "  2. Access Dokploy: ssh -L 3000:localhost:3000 deploy@<vps-ip> -p $SSH_PORT"
echo "     Then open http://localhost:3000 in browser"
