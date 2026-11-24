#!/usr/bin/env bash
set -euo pipefail

# استفاده:
#   curl -fsSL https://raw.githubusercontent.com/VahidDashtbani/cloud-gaming/main/install.sh | sudo bash -s -- <STATIC_IP>
# یا با مشخص کردن مخزن:
#   sudo ./install.sh <STATIC_IP> [owner/repo] [branch_or_tag]

SERVER_IP="${1:-}"
REPO_SLUG="${2:-VahidDashtbani/cloud-gaming}"
REF="${3:-main}"

if [[ -z "$SERVER_IP" ]]; then
  echo "❌ لطفاً IP ثابت سرور را وارد کنید:"
  echo "   curl -fsSL https://raw.githubusercontent.com/${REPO_SLUG}/${REF}/install.sh | sudo bash -s -- <STATIC_IP>"
  exit 1
fi

if [[ "$EUID" -ne 0 ]]; then
  echo "❌ این اسکریپت را با sudo یا root اجرا کنید."
  exit 1
fi

INSTALL_DIR="/opt/cloud-gaming"
SRC_DIR="/tmp/cloud-gaming-src"
mkdir -p "$INSTALL_DIR"
rm -rf "$SRC_DIR"

echo "⬇️ دریافت سورس‌کد از GitHub (${REPO_SLUG} @ ${REF}) ..."
if command -v git >/dev/null 2>&1; then
  git clone --depth 1 --branch "$REF" "https://github.com/${REPO_SLUG}.git" "$SRC_DIR"
else
  mkdir -p "$SRC_DIR"
  curl -fsSL "https://github.com/${REPO_SLUG}/archive/refs/heads/${REF}.tar.gz" -o /tmp/cg-src.tar.gz
  tar -xzf /tmp/cg-src.tar.gz -C "$SRC_DIR" --strip-components=1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "⚙️ نصب Go ..."
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y && apt-get install -y golang-go
  elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm go
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y golang
  else
    echo "❌ Go نصب نیست و مدیر بسته شناخته‌شده‌ای پیدا نشد. لطفاً Go را نصب کنید."
    exit 1
  fi
fi

echo "🔨 بیلد linux-relay و windows-agent.exe ..."
cd "$SRC_DIR"
go mod tidy
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$INSTALL_DIR/linux-relay" ./cmd/linux-relay
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$INSTALL_DIR/windows-agent.exe" ./cmd/windows-agent
cp -r "$SRC_DIR/deploy/windows" "$INSTALL_DIR/windows-files"
chmod +x "$INSTALL_DIR/linux-relay"

cat << SERVICE_EOF > /etc/systemd/system/cloud-gaming-relay.service
[Unit]
Description=Cloud Gaming Linux Relay & Control Plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/linux-relay -addr :8080 -public-host ${SERVER_IP}:8080 -token-file ${INSTALL_DIR}/pairing_secret.key
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SERVICE_EOF

systemctl daemon-reload
systemctl enable --now cloud-gaming-relay.service

SECRET_FILE="${INSTALL_DIR}/pairing_secret.key"
for _ in {1..15}; do
  [[ -s "$SECRET_FILE" ]] && break
  sleep 0.5
done

SECRET_KEY=$(tr -d '[:space:]' < "$SECRET_FILE")
PAYLOAD_JSON=$(printf '{"relay_ws":"ws://%s:8080/ws/node","token":"%s"}' "$SERVER_IP" "$SECRET_KEY")
TOKEN_B64=$(printf "%s" "$PAYLOAD_JSON" | base64 -w0 | tr '+/' '-_' | tr -d '=')
CLIENT_TOKEN="cg1_${TOKEN_B64}"

printf "%s\n" "$CLIENT_TOKEN" > "${INSTALL_DIR}/client_token.txt"
rm -rf "$SRC_DIR" /tmp/cg-src.tar.gz

echo ""
echo "✅ نصب کامل از روی GitHub انجام شد!"
echo "🌐 پنل وب: http://${SERVER_IP}:8080"
echo "🪟 فایل‌های ویندوز در: ${INSTALL_DIR}/windows-files (شامل windows-agent.exe و install-windows.ps1)"
echo "🔑 توکن اتصال کلاینت ویندوز (در ${INSTALL_DIR}/client_token.txt ذخیره شد):"
echo "$CLIENT_TOKEN"
echo ""
