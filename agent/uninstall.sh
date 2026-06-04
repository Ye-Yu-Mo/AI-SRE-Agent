#!/bin/sh
set -eu

AGENT_USER="ai-server-agent"
BIN_PATH="/usr/local/bin/ai-server-agent"
DATA_DIR="/var/lib/ai-server-agent"
CONF_DIR="/etc/ai-server-agent"
LOG_DIR="/var/log/ai-server-agent"
SERVICE_FILE="/etc/systemd/system/ai-server-agent.service"

PURGE=false
case "${1:-}" in
    --purge) PURGE=true ;;
esac

echo "=== AI Server Agent Uninstall ==="

# 1. Stop and disable
echo "Stopping service..."
systemctl stop ai-server-agent 2>/dev/null || true
systemctl disable ai-server-agent 2>/dev/null || true
rm -f "${SERVICE_FILE}"
systemctl daemon-reload

# 2. Remove binary
echo "Removing binary..."
rm -f "${BIN_PATH}"

# 3. Remove config (always, contains secret)
echo "Removing config..."
rm -rf "${CONF_DIR}"

# 4. Remove logs
rm -rf "${LOG_DIR}"

# 5. Data — keep by default
if [ "${PURGE}" = true ]; then
    echo "Purging all data..."
    rm -rf "${DATA_DIR}"
else
    echo "Keeping data directory: ${DATA_DIR}"
    echo "  (use --purge to remove all data including audit log)"
fi

# 6. Remove user
if id -u "${AGENT_USER}" >/dev/null 2>&1; then
    userdel "${AGENT_USER}" 2>/dev/null || true
    echo "Removed user ${AGENT_USER}."
fi

echo ""
echo "=== Done ==="
