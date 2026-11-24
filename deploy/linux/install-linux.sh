#!/usr/bin/env bash
set -euo pipefail

SERVER_IP="${1:-127.0.0.1}"

mkdir -p /opt/cloud-gaming
cp ./bin/linux-relay /opt/cloud-gaming/linux-relay
chmod +x /opt/cloud-gaming/linux-relay

sed "s|-public-host 127.0.0.1:8080|-public-host ${SERVER_IP}:8080|g" ./deploy/linux/cloud-gaming-relay.service > /etc/systemd/system/cloud-gaming-relay.service

systemctl daemon-reload
systemctl enable --now cloud-gaming-relay.service
sleep 1

SECRET_KEY=$(cat /opt/cloud-gaming/pairing_secret.key)
TOKEN_B64=$(printf '{"relay_ws":"ws://%s:8080/ws/node","token":"%s"}' "$SERVER_IP" "$SECRET_KEY" | base64 -w0 | tr '+/' '-_' | tr -d '=')
CLIENT_TOKEN="cg1_${TOKEN_B64}"
printf "%s\n" "$CLIENT_TOKEN" > /opt/cloud-gaming/client_token.txt

echo "✅ سرور لینوکس روی ${SERVER_IP}:8080 نصب و فعال شد."
echo "🌐 پنل وب: http://${SERVER_IP}:8080"
echo "🔑 توکن اتصال کلاینت ویندوز (ذخیره شده در /opt/cloud-gaming/client_token.txt):"
echo "$CLIENT_TOKEN"
