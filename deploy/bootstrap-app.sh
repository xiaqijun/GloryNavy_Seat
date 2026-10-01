#!/bin/bash
# Provision directories only. Supply seat.env and db-password over SSH separately.
set -euo pipefail
test "$(id -u)" -eq 0
command -v docker >/dev/null
command -v nft >/dev/null
docker compose version >/dev/null
if ! id glorynavy >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/glorynavy --shell /usr/sbin/nologin glorynavy
fi
install -d -m 0755 /opt/glorynavy /opt/glorynavy/releases
install -d -m 0700 /etc/glorynavy /var/backups/glorynavy
install -d -m 0700 -o glorynavy -g glorynavy /var/lib/glorynavy /var/lib/glorynavy/sde
printf 'Application directories ready\n'
