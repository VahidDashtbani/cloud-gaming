#!/usr/bin/env bash
set -euo pipefail

SERVER_IP="${1:-}"
if [[ -z "$SERVER_IP" ]]; then
  echo "استفاده: sudo ./install-linux.sh <YOUR_STATIC_SERVER_IP>"
  exit 1
fi

mkdir -p /opt/cloud-gaming
cp ./bin/linux-relay /opt/cloud-gaming/linux-relay
chmod +x /opt/cloud-gaming/linux-relay
cp ./deploy/linux/cloud-gaming-relay.service /etc/systemd/system/

systemctl daemon-reload
systemctl enable --now cloud-gaming-relay.service

echo "✅ سرور لینوکس روی IP ثابت ${SERVER_IP}:8080 نصب و فعال شد."
echo "🌐 آدرس پنل وب: http://${SERVER_IP}:8080"
