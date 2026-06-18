#!/bin/sh
set -eu

# AI Server Agent — One-line install
# curl -fsSL https://raw.githubusercontent.com/Ye-Yu-Mo/AI-SRE-Agent/main/agent/install.sh | sh

BIN_URL="${AGENT_DOWNLOAD_URL:-https://github.com/Ye-Yu-Mo/AI-SRE-Agent/releases/latest/download/ai-server-agent}"
AGENT_USER="ai-server-agent"
BIN_DIR="/usr/local/bin"
BIN_PATH="${BIN_DIR}/ai-server-agent"
DATA_DIR="/var/lib/ai-server-agent"
CONF_DIR="/etc/ai-server-agent"
LOG_DIR="/var/log/ai-server-agent"
SERVICE_FILE="/etc/systemd/system/ai-server-agent.service"
PORT="${AGENT_PORT:-9090}"

echo "=== AI Server Agent Install ==="

# 1. Create system user
id -u "${AGENT_USER}" >/dev/null 2>&1 || \
  useradd --system --no-create-home --shell /usr/sbin/nologin "${AGENT_USER}"

# Add to docker group for container access
if getent group docker >/dev/null; then
  usermod -aG docker "${AGENT_USER}" 2>/dev/null || true
fi

# 2. Create directories
mkdir -p "${DATA_DIR}" "${CONF_DIR}" "${LOG_DIR}"
chown "${AGENT_USER}:${AGENT_USER}" "${DATA_DIR}" "${LOG_DIR}"
chmod 750 "${DATA_DIR}" "${CONF_DIR}" "${LOG_DIR}"

# 3. Download or copy binary
if [ -f "${AGENT_BINARY:-}" ]; then
  echo "Using local binary: ${AGENT_BINARY}"
  cp "${AGENT_BINARY}" "${BIN_PATH}"
elif command -v curl >/dev/null 2>&1; then
  echo "Downloading binary..."
  curl -fsSL "${BIN_URL}" -o "${BIN_PATH}" || {
    echo "ERROR: Download failed. Set AGENT_BINARY=/path/to/binary for local install."
    exit 1
  }
else
  echo "ERROR: curl not found and no local binary provided."
  echo "Set AGENT_BINARY=/path/to/ai-server-agent for local install."
  exit 1
fi
chmod 755 "${BIN_PATH}"

# Docker Compose v2 wrapper
if command -v docker >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then
  cat > /usr/local/bin/docker-compose <<'SHIM'
#!/bin/sh
exec docker compose "$@"
SHIM
  chmod 755 /usr/local/bin/docker-compose
  echo "Created docker-compose wrapper for Docker Compose v2"
fi

# 4. Generate secret
if [ -z "${AGENT_SECRET:-}" ]; then
  AGENT_SECRET=$(head -c 32 /dev/urandom 2>/dev/null | base64 | tr -d '=+/' || \
    dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '=+/')
  echo "Generated AGENT_SECRET: ${AGENT_SECRET}"
fi

cat > "${CONF_DIR}/env" <<EOF
AGENT_SECRET=${AGENT_SECRET}
AGENT_PORT=${PORT}
AGENT_DATA_DIR=${DATA_DIR}
EOF
chmod 600 "${CONF_DIR}/env"
chown "${AGENT_USER}:${AGENT_USER}" "${CONF_DIR}/env"

# 5. Systemd service
cat > "${SERVICE_FILE}" <<UNIT
[Unit]
Description=AI Server Agent
After=network.target docker.service
[Service]
Type=simple
User=${AGENT_USER}
Group=${AGENT_USER}
EnvironmentFile=${CONF_DIR}/env
ExecStart=${BIN_PATH} serve
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}
[Install]
WantedBy=multi-user.target
UNIT

# 6. Start
systemctl daemon-reload
systemctl enable --now ai-server-agent
sleep 2

if systemctl is-active --quiet ai-server-agent; then
  echo ""
  echo "=== Done ==="
  echo "Agent running on port ${PORT}"
  echo "AGENT_SECRET: ${AGENT_SECRET}"
  echo ""
  echo "Config this secret in your .mcp.json env:"
  echo "  AGENT_SECRET=${AGENT_SECRET}"
  echo ""
  echo "Check: systemctl status ai-server-agent"
  echo "Logs:  journalctl -u ai-server-agent -f"
else
  echo "ERROR: Agent failed to start. Check: systemctl status ai-server-agent"
  exit 1
fi
