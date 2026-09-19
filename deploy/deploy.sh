#!/usr/bin/env bash
# Збирає c3hub під ARM64 і заливає на VPS. Запускати з кореня репозиторію.
set -euo pipefail

HOST="${1:-oracle}"
GO="${GO:-go}"

echo "==> build"
( cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$GO" build -o ../c3hub ./cmd/c3hub )

echo "==> upload"
scp c3hub "$HOST:/tmp/c3hub"
scp deploy/c3hub.service "$HOST:/tmp/c3hub.service"

echo "==> install"
ssh "$HOST" 'sudo install -m 755 /tmp/c3hub /opt/c3hub/c3hub \
  && sudo install -m 644 /tmp/c3hub.service /etc/systemd/system/c3hub.service \
  && sudo systemctl daemon-reload \
  && sudo systemctl restart c3hub \
  && sleep 2 && systemctl is-active c3hub'

rm -f c3hub
echo "==> done"
