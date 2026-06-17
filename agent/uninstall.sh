#!/bin/sh
set -eu

AGENT_USER="ai-server-agent"
BIN_PATH="/usr/local/bin/ai-server-agent"
DATA_DIR="/var/lib/ai-server-agent"
CONF_DIR="/etc/ai-server-agent"
LOG_DIR="/var/log/ai-server-agent"
SERVICE_FILE="/etc/systemd/system/ai-server-agent.service"

case "${1:-}" in
  --purge) PURGE=true ;;
  --force) PURGE=true ;;
  *) PURGE=false ;;
esac

echo "=== AI Server Agent Uninstall ==="

systemctl stop ai-server-agent 2>/dev/null || true
systemctl disable ai-server-agent 2>/dev/null || true
rm -f "${SERVICE_FILE}"
systemctl daemon-reload

rm -f "${BIN_PATH}"
rm -rf "${CONF_DIR}"
rm -rf "${LOG_DIR}"

if [ "${PURGE}" = true ]; then
  echo "Purging all data..."
  rm -rf "${DATA_DIR}"
else
  echo "Keeping data: ${DATA_DIR}"
  echo "  (use --purge to remove audit log and state data)"
fi

id -u "${AGENT_USER}" >/dev/null 2>&1 && userdel "${AGENT_USER}" 2>/dev/null || true

echo "Done."
