#!/bin/sh
set -eu

# AI Server Agent — install script
# curl -fsSL https://install.example.com/agent.sh | sh

AGENT_USER="ai-server-agent"
BIN_DIR="/usr/local/bin"
BIN_PATH="${BIN_DIR}/ai-server-agent"
DATA_DIR="/var/lib/ai-server-agent"
CONF_DIR="/etc/ai-server-agent"
LOG_DIR="/var/log/ai-server-agent"
SERVICE_FILE="/etc/systemd/system/ai-server-agent.service"
PORT="${AGENT_PORT:-9090}"

echo "=== AI Server Agent Install ==="
echo ""

# 1. Create user if not exists
if ! id -u "${AGENT_USER}" >/dev/null 2>&1; then
    echo "Creating user ${AGENT_USER}..."
    useradd --system --no-create-home --shell /usr/sbin/nologin "${AGENT_USER}"
else
    echo "User ${AGENT_USER} already exists."
fi

# 2. Create directories
echo "Creating directories..."
mkdir -p "${DATA_DIR}" "${CONF_DIR}" "${LOG_DIR}"
chown "${AGENT_USER}:${AGENT_USER}" "${DATA_DIR}" "${LOG_DIR}"
chmod 750 "${DATA_DIR}" "${CONF_DIR}" "${LOG_DIR}"

# 3. Install binary
echo "Installing binary..."
cp "${AGENT_BINARY:-./ai-server-agent}" "${BIN_PATH}"
chmod 755 "${BIN_PATH}"

# 4. Generate shared secret and write env file
if [ -z "${AGENT_SECRET:-}" ]; then
    AGENT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '=+/')
    echo "Generated secret: ${AGENT_SECRET}"
    echo "Store this securely, you will need it to connect MCP Server."
fi
cat > "${CONF_DIR}/env" <<ENV
AGENT_SECRET=${AGENT_SECRET}
AGENT_PORT=${PORT}
AGENT_DATA_DIR=${DATA_DIR}
ENV
chmod 600 "${CONF_DIR}/env"
chown "${AGENT_USER}:${AGENT_USER}" "${CONF_DIR}/env"

# 5. Write systemd unit
echo "Installing systemd service..."
cat > "${SERVICE_FILE}" <<UNIT
[Unit]
Description=AI Server Agent
Documentation=https://github.com/ai-sre/agent
After=network.target

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
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

# 6. Enable and start
systemctl daemon-reload
systemctl enable ai-server-agent
systemctl start ai-server-agent

# 7. Verify
sleep 2
if systemctl is-active --quiet ai-server-agent; then
    echo ""
    echo "=== Done ==="
    echo "Agent is running. Check status:"
    echo "  systemctl status ai-server-agent"
    echo ""
    echo "Data dir:  ${DATA_DIR}"
    echo "Config:    ${CONF_DIR}"
    echo "Logs:      journalctl -u ai-server-agent"
else
    echo ""
    echo "ERROR: Agent failed to start. Check:"
    echo "  systemctl status ai-server-agent"
    echo "  journalctl -u ai-server-agent"
    exit 1
fi
