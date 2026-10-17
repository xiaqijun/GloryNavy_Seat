#!/bin/bash
set -euo pipefail
role=${1:?Usage: install-storage-retention.sh app|edge}
[[ "$role" == app || "$role" == edge ]] || exit 2
source=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
install -d -m 0755 /usr/local/libexec/glorynavy
install -m 0755 "$source/rotate-storage.py" /usr/local/libexec/glorynavy/rotate-storage.py
install -m 0644 "$source/glorynavy-storage-retention@.service" /etc/systemd/system/
install -m 0644 "$source/glorynavy-storage-retention@.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now "glorynavy-storage-retention@$role.timer"
