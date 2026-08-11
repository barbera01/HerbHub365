#!/usr/bin/env bash
# Install the HerbHub watering binary as a systemd service.
# Run ON the Raspberry Pi, as root, with the 'watering' binary next to this script:
#
#   scp watering install.sh pi@herbhub.local:/tmp/
#   ssh pi@herbhub.local 'sudo bash /tmp/install.sh'
#
# Build the binary on your dev machine first:
#   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o watering .
set -euo pipefail

[[ $EUID -eq 0 ]] || { echo "run as root: sudo bash $0" >&2; exit 1; }

BIN="$(dirname "$(readlink -f "$0")")/watering"
ENV_EXAMPLE="$(dirname "$(readlink -f "$0")")/watering.env.example"
INSTALL_DIR=/opt/herbhub
SVC=watering

[[ -f "$BIN" ]] || { echo "binary not found at $BIN — copy it next to this script" >&2; exit 1; }

# Sanity check: must be a 64-bit ARM Linux binary
if ! file "$BIN" | grep -q 'ELF 64-bit.*ARM aarch64'; then
  echo "WARNING: $BIN doesn't look like an arm64 Linux binary:" >&2
  file "$BIN" >&2
fi

# Dedicated unprivileged user; gpio group grants access to /dev/gpiochip0
id herbhub >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin herbhub
usermod -aG gpio herbhub

mkdir -p "$INSTALL_DIR"
# Stop first so we don't overwrite a running binary in place
systemctl stop "$SVC" 2>/dev/null || true
install -m 0755 "$BIN" "$INSTALL_DIR/watering"

if [[ -f "$ENV_EXAMPLE" ]] && [[ ! -f /etc/default/$SVC ]]; then
  install -m 0640 "$ENV_EXAMPLE" "/etc/default/$SVC"
fi

cat > /etc/systemd/system/$SVC.service <<'EOF'
[Unit]
Description=HerbHub365 watering relay API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/herbhub/watering
EnvironmentFile=-/etc/default/watering
Restart=always
RestartSec=3

# Run unprivileged; gpio group grants access to /dev/gpiochip0
User=herbhub
Group=herbhub
SupplementaryGroups=gpio

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
StateDirectory=herbhub-watering
DeviceAllow=/dev/gpiochip0 rw
DevicePolicy=closed

# Give the process time to switch relays off on stop (SIGTERM handler)
TimeoutStopSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$SVC"

sleep 2
systemctl --no-pager --full status "$SVC" | head -n 12
echo
curl -fsS http://localhost:8181/healthz && echo
echo "installed OK — API on :8181"
